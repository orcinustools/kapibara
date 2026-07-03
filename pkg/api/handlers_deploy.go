package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/orcinus"
	"github.com/orcinustools/kapibara/pkg/store"
)

// --- compose apps ---

func (s *Server) handleListComposeApps(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	apps, err := s.Store.ComposeAppsForProject(p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"composeApps": apps})
}

func (s *Server) handleCreateComposeApp(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	var req struct {
		Name   string `json:"name"`
		Source string `json:"source"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if strings.TrimSpace(req.Source) == "" {
		writeError(w, http.StatusBadRequest, "source required")
		return
	}
	if req.Name == "" {
		req.Name = "compose"
	}
	app := &store.ComposeApp{ProjectID: p.ID, Name: req.Name, Source: req.Source, OrcinusProject: unitProject(p, req.Name)}
	if err := s.Store.CreateComposeApp(app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, app)
}

func (s *Server) handleUpdateComposeApp(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	app, err := s.Store.ComposeAppByID(chi.URLParam(r, "appID"))
	if err != nil || app.ProjectID != p.ID {
		writeError(w, http.StatusNotFound, "compose app not found")
		return
	}
	var req struct {
		Name   *string `json:"name"`
		Source *string `json:"source"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name != nil {
		app.Name = *req.Name
	}
	if req.Source != nil {
		app.Source = *req.Source
	}
	if err := s.Store.UpdateComposeApp(app); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, app)
}

// --- convert (preview) & deploy ---

type deployReq struct {
	// Either reference a stored compose app…
	ComposeAppID string `json:"composeAppId"`
	// …or pass a raw source inline.
	Source string `json:"source"`
	Wait   bool   `json:"wait"`
	Prune  *bool  `json:"prune"`
}

// resolveSource returns the compose source for a deploy/convert request,
// preferring an inline source, then a referenced compose app.
func (s *Server) resolveSource(w http.ResponseWriter, p *store.Project, req deployReq) (string, string, bool) {
	if strings.TrimSpace(req.Source) != "" {
		return req.Source, req.ComposeAppID, true
	}
	if req.ComposeAppID != "" {
		app, err := s.Store.ComposeAppByID(req.ComposeAppID)
		if err != nil || app.ProjectID != p.ID {
			writeError(w, http.StatusNotFound, "compose app not found")
			return "", "", false
		}
		return app.Source, app.ID, true
	}
	writeError(w, http.StatusBadRequest, "source or composeAppId required")
	return "", "", false
}

// composeTarget returns the isolated orcinus project for a compose deploy: the
// referenced compose app's project, or a stable default for inline deploys.
func (s *Server) composeTarget(p *store.Project, composeAppID string) string {
	if composeAppID != "" {
		if app, err := s.Store.ComposeAppByID(composeAppID); err == nil && app.OrcinusProject != "" {
			return app.OrcinusProject
		}
	}
	return unitProject(p, "compose")
}

func (s *Server) handleConvert(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	var req deployReq
	if !decodeJSON(w, r, &req) {
		return
	}
	source, composeAppID, ok := s.resolveSource(w, p, req)
	if !ok {
		return
	}
	res, err := s.Orcinus.Convert(r.Context(), orcinus.DeployRequest{
		Source:  source,
		Project: s.composeTarget(p, composeAppID),
	})
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (s *Server) handleDeploy(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	var req deployReq
	if !decodeJSON(w, r, &req) {
		return
	}
	source, composeAppID, ok := s.resolveSource(w, p, req)
	if !ok {
		return
	}

	// If the deploy came with an inline source and a compose app id, persist it.
	if composeAppID != "" && strings.TrimSpace(req.Source) != "" {
		if app, err := s.Store.ComposeAppByID(composeAppID); err == nil && app.ProjectID == p.ID {
			app.Source = req.Source
			_ = s.Store.UpdateComposeApp(app)
		}
	}
	// Inline deploy without a stored app: record an implicit "compose" unit so
	// project aggregation (pods/logs) can find its isolated orcinus project.
	if composeAppID == "" {
		apps, _ := s.Store.ComposeAppsForProject(p.ID)
		var found *store.ComposeApp
		for i := range apps {
			if apps[i].Name == "compose" {
				found = &apps[i]
				break
			}
		}
		if found == nil {
			ca := &store.ComposeApp{ProjectID: p.ID, Name: "compose", Source: source, OrcinusProject: unitProject(p, "compose")}
			if err := s.Store.CreateComposeApp(ca); err == nil {
				composeAppID = ca.ID
			}
		} else {
			found.Source = source
			_ = s.Store.UpdateComposeApp(found)
			composeAppID = found.ID
		}
	}

	dep := &store.Deployment{
		ProjectID:    p.ID,
		ComposeAppID: composeAppID,
		Kind:         "compose",
		Status:       store.DeployRunning,
		Source:       source,
	}
	if err := s.Store.CreateDeployment(dep); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	res, err := s.Orcinus.Deploy(r.Context(), orcinus.DeployRequest{
		Source:  source,
		Project: s.composeTarget(p, composeAppID),
		Wait:    req.Wait,
		Prune:   req.Prune,
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
	if b, e := json.Marshal(res.Installed); e == nil {
		dep.Installed = string(b)
	}
	_ = s.Store.UpdateDeployment(dep)

	writeJSON(w, http.StatusOK, map[string]any{
		"deployment": dep,
		"applied":    res.Applied,
		"installed":  res.Installed,
		"project":    res.Project,
	})
}

// --- runtime views (proxied from orcinus) ---

// unitRef identifies one deployable unit's isolated orcinus project.
type unitRef struct {
	Name           string
	Kind           string
	OrcinusProject string
	Service        string // primary service name (apps/dbs)
}

// projectUnits returns all deployable units of a kapibara project, each with its
// isolated orcinus project.
func (s *Server) projectUnits(projectID string) []unitRef {
	var refs []unitRef
	apps, _ := s.Store.ApplicationsForProject(projectID)
	for _, a := range apps {
		if a.OrcinusProject != "" {
			refs = append(refs, unitRef{a.Name, "application", a.OrcinusProject, sanitizeDNS(a.Name)})
		}
	}
	dbs, _ := s.Store.DatabasesForProject(projectID)
	for _, d := range dbs {
		if d.OrcinusProject != "" {
			refs = append(refs, unitRef{d.Name, "database", d.OrcinusProject, sanitizeDNS(d.Name)})
		}
	}
	cas, _ := s.Store.ComposeAppsForProject(projectID)
	for _, c := range cas {
		if c.OrcinusProject != "" {
			refs = append(refs, unitRef{c.Name, "compose", c.OrcinusProject, ""})
		}
	}
	return refs
}

func (s *Server) handleProjectPods(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	// Aggregate pods across every unit's isolated orcinus project.
	var all []orcinus.Pod
	for _, u := range s.projectUnits(p.ID) {
		pods, err := s.Orcinus.Pods(r.Context(), u.OrcinusProject)
		if err == nil {
			all = append(all, pods...)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"pods": all})
}

func (s *Server) handleListDeployments(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	ds, err := s.Store.DeploymentsForProject(p.ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deployments": ds})
}
