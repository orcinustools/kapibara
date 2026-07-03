package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/orcinus"
	"github.com/orcinustools/kapibara/pkg/store"
	"github.com/orcinustools/kapibara/pkg/templates"
)

// handleListTemplates returns the one-click template catalog (without the raw
// compose bodies).
func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"templates": templates.Catalog})
}

// handleDeployTemplate instantiates a template with params into a project: it
// renders the compose, stores it as a ComposeApp, and deploys it.
func (s *Server) handleDeployTemplate(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	tmpl, ok := templates.Find(chi.URLParam(r, "name"))
	if !ok {
		writeError(w, http.StatusNotFound, "template not found")
		return
	}
	var req struct {
		Values map[string]string `json:"values"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	source, err := tmpl.Render(req.Values)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	app := &store.ComposeApp{ProjectID: p.ID, Name: tmpl.Name, Source: source, OrcinusProject: unitProject(p, tmpl.Name)}
	if err := s.Store.CreateComposeApp(app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dep := &store.Deployment{
		ProjectID: p.ID, ComposeAppID: app.ID, Kind: "template",
		Status: store.DeployRunning, Source: source,
	}
	_ = s.Store.CreateDeployment(dep)

	res, err := s.Orcinus.Deploy(r.Context(), orcinus.DeployRequest{
		Source: source, Project: app.OrcinusProject, Wait: true,
		ACMEEmail: r.URL.Query().Get("acmeEmail"),
	})
	if err != nil {
		dep.Status = store.DeployFailed
		dep.Error = err.Error()
		_ = s.Store.UpdateDeployment(dep)
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	dep.Status = store.DeploySuccess
	dep.Applied = res.Applied
	_ = s.Store.UpdateDeployment(dep)

	writeJSON(w, http.StatusOK, map[string]any{
		"deployment": dep, "composeApp": app, "applied": res.Applied,
	})
}

// handleWebhook is a PUBLIC endpoint git providers call on push. The secret in
// the path authorizes the deploy; no session/token is required.
func (s *Server) handleWebhook(w http.ResponseWriter, r *http.Request) {
	secret := chi.URLParam(r, "secret")
	app, err := s.Store.ApplicationByWebhookSecret(secret)
	if err != nil {
		writeError(w, http.StatusNotFound, "unknown webhook")
		return
	}
	if !app.AutoDeploy {
		writeJSON(w, http.StatusOK, map[string]string{"status": "auto-deploy disabled"})
		return
	}
	project, err := s.Store.ProjectByID(app.ProjectID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "project not found")
		return
	}
	dep, err := s.Deployer.DeployApplication(r.Context(), app, project, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"status": "deploying", "deploymentId": dep.ID, "app": app.Name,
	})
}
