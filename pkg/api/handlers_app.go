package api

import (
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/store"
)

type appReq struct {
	Name           string            `json:"name"`
	BuildType      string            `json:"buildType"` // dockerfile | nixpacks | image
	RepoURL        string            `json:"repoUrl"`
	Branch         string            `json:"branch"`
	ContextDir     string            `json:"contextDir"`
	DockerfilePath string            `json:"dockerfilePath"`
	Image          string            `json:"image"`
	Port           int               `json:"port"`
	Replicas       int               `json:"replicas"`
	Domain         string            `json:"domain"`
	TLS            bool              `json:"tls"`
	Env            map[string]string `json:"env"`
	SecretKeys     []string          `json:"secretKeys"`

	AutoscaleMin    int    `json:"autoscaleMin"`
	AutoscaleMax    int    `json:"autoscaleMax"`
	AutoscaleCPU    int    `json:"autoscaleCpu"`
	AutoscaleMemory int    `json:"autoscaleMemory"`
	Rollout         string `json:"rollout"`
}

func (s *Server) handleCreateApp(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	var req appReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	if req.BuildType == "" {
		req.BuildType = "dockerfile"
	}
	switch req.BuildType {
	case "dockerfile", "nixpacks":
		if req.RepoURL == "" {
			writeError(w, http.StatusBadRequest, "repoUrl required for git builds")
			return
		}
	case "image":
		if req.Image == "" {
			writeError(w, http.StatusBadRequest, "image required for build type image")
			return
		}
	default:
		writeError(w, http.StatusBadRequest, "invalid buildType")
		return
	}

	app := &store.Application{
		ProjectID:      p.ID,
		Name:           req.Name,
		OrcinusProject: unitProject(p, req.Name),
		BuildType:      req.BuildType,
		RepoURL:        req.RepoURL,
		Branch:         req.Branch,
		ContextDir:     req.ContextDir,
		DockerfilePath: req.DockerfilePath,
		Image:          req.Image,
		Port:           req.Port,
		Replicas:       req.Replicas,
		Domain:         req.Domain,
		TLS:            req.TLS,
		Env:            marshalEnv(req.Env),
		SecretKeys:     marshalList(req.SecretKeys),

		AutoscaleMin:    req.AutoscaleMin,
		AutoscaleMax:    req.AutoscaleMax,
		AutoscaleCPU:    req.AutoscaleCPU,
		AutoscaleMemory: req.AutoscaleMemory,
		Rollout:         req.Rollout,
		WebhookSecret:   randomSecret(),
		AutoDeploy:      true,
	}
	if err := s.Store.CreateApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, appView(s, app))
}

func (s *Server) handleListApps(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	apps, err := s.Store.ApplicationsForProject(p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"applications": apps})
}

func (s *Server) handleGetApp(w http.ResponseWriter, r *http.Request) {
	app, _ := s.loadAppWithAccess(w, r)
	if app == nil {
		return
	}
	writeJSON(w, http.StatusOK, appView(s, app))
}

func (s *Server) handleUpdateApp(w http.ResponseWriter, r *http.Request) {
	app, _ := s.loadAppWithAccess(w, r)
	if app == nil {
		return
	}
	var req appReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != "" {
		app.Name = req.Name
	}
	if req.BuildType != "" {
		app.BuildType = req.BuildType
	}
	app.RepoURL = orDefault(req.RepoURL, app.RepoURL)
	app.Branch = orDefault(req.Branch, app.Branch)
	app.ContextDir = orDefault(req.ContextDir, app.ContextDir)
	app.DockerfilePath = orDefault(req.DockerfilePath, app.DockerfilePath)
	app.Image = orDefault(req.Image, app.Image)
	if req.Port != 0 {
		app.Port = req.Port
	}
	if req.Replicas != 0 {
		app.Replicas = req.Replicas
	}
	app.Domain = orDefault(req.Domain, app.Domain)
	app.TLS = req.TLS
	if req.Env != nil {
		app.Env = marshalEnv(req.Env)
	}
	if req.SecretKeys != nil {
		app.SecretKeys = marshalList(req.SecretKeys)
	}
	if err := s.Store.UpdateApplication(app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

func (s *Server) handleDeleteApp(w http.ResponseWriter, r *http.Request) {
	app, _ := s.loadAppWithAccess(w, r)
	if app == nil {
		return
	}
	if err := s.Store.DB.Delete(&store.Application{}, "id = ?", app.ID).Error; err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (s *Server) handleDeployApp(w http.ResponseWriter, r *http.Request) {
	app, project := s.loadAppWithAccess(w, r)
	if app == nil {
		return
	}
	// async=true: build/deploy runs in the background; client polls the
	// returned deployment id for status + logs.
	dep, err := s.Deployer.DeployApplication(r.Context(), app, project, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, dep)
}

func (s *Server) handleGetDeployment(w http.ResponseWriter, r *http.Request) {
	dep, err := s.Store.DeploymentByID(chi.URLParam(r, "deploymentID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	p, err := s.Store.ProjectByID(dep.ProjectID)
	if err != nil || s.requireOrgAccess(w, r, p.OrganizationID) == nil {
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
		}
		return
	}
	writeJSON(w, http.StatusOK, dep)
}

// loadAppWithAccess loads {appID} and verifies org access, returning the app
// and its project (or nil after writing an error).
func (s *Server) loadAppWithAccess(w http.ResponseWriter, r *http.Request) (*store.Application, *store.Project) {
	app, err := s.Store.ApplicationByID(chi.URLParam(r, "appID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "application not found")
		return nil, nil
	}
	p, err := s.Store.ProjectByID(app.ProjectID)
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return nil, nil
	}
	if s.requireOrgAccess(w, r, p.OrganizationID) == nil {
		return nil, nil
	}
	return app, p
}

// appView augments an application with its webhook path (the secret itself is
// only revealed to authorized org members via this endpoint).
func appView(s *Server, app *store.Application) map[string]any {
	return map[string]any{
		"application": app,
		"webhookPath": "/api/v1/webhooks/" + app.WebhookSecret,
		"autoDeploy":  app.AutoDeploy,
	}
}

func marshalEnv(m map[string]string) string {
	if len(m) == 0 {
		return ""
	}
	b, _ := json.Marshal(m)
	return string(b)
}

func marshalList(l []string) string {
	if len(l) == 0 {
		return ""
	}
	b, _ := json.Marshal(l)
	return string(b)
}

func orDefault(v, def string) string {
	if v != "" {
		return v
	}
	return def
}
