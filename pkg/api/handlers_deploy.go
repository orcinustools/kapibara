package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gopkg.in/yaml.v3"

	"github.com/orcinustools/kapibara/pkg/deployer"
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
		Status:       store.DeployPending,
		Source:       source,
	}
	if err := s.Store.CreateDeployment(dep); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Expand Kapibara-registry image references (registry/<proj>/<img>) to the
	// full org-scoped gateway path so the cluster can pull them — the same
	// rewrite the application deploy path applies, but across every compose
	// service image.
	source = s.rewriteComposeImages(source, p)

	// Run the deploy asynchronously so it isn't bound by the HTTP request
	// timeout: apply the compose, then stream pod-readiness progress into the
	// deployment log. The client polls GET /deployments/{id} to follow along.
	go s.runComposeDeploy(s.composeTarget(p, composeAppID), source, req.Wait, req.Prune, dep)

	writeJSON(w, http.StatusAccepted, map[string]any{"deployment": dep})
}

// rewriteComposeImages rewrites each service's `image:` in a compose source via
// the registry rewrite (host + org scope), leaving external images untouched.
// Returns the original source unchanged if there is nothing to rewrite or on a
// parse error (orcinus then reports any real problem).
func (s *Server) rewriteComposeImages(source string, p *store.Project) string {
	if s.Cfg.RegistryPublic == "" {
		return source
	}
	scope := ""
	if org, err := s.Store.OrgByID(p.OrganizationID); err == nil {
		scope = org.Slug
	}
	var doc map[string]any
	if err := yaml.Unmarshal([]byte(source), &doc); err != nil {
		return source
	}
	svcs, ok := doc["services"].(map[string]any)
	if !ok {
		return source
	}
	changed := false
	for _, v := range svcs {
		svc, ok := v.(map[string]any)
		if !ok {
			continue
		}
		img, ok := svc["image"].(string)
		if !ok {
			continue
		}
		if nw := deployer.RewriteRegistryImage(img, s.Cfg.RegistryPublic, scope); nw != img {
			svc["image"] = nw
			changed = true
		}
	}
	if !changed {
		return source
	}
	out, err := yaml.Marshal(doc)
	if err != nil {
		return source
	}
	return string(out)
}

// runComposeDeploy applies a compose source via orcinus and, when wait is set,
// polls pod readiness — streaming progress into the deployment's log — until the
// pods are ready or a generous timeout elapses. It runs on a background context
// so it survives past the originating HTTP request.
func (s *Server) runComposeDeploy(target, source string, wait bool, prune *bool, dep *store.Deployment) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()

	var log strings.Builder
	flush := func() { dep.Log = log.String(); _ = s.Store.UpdateDeployment(dep) }
	logln := func(format string, a ...any) { fmt.Fprintf(&log, format+"\n", a...); flush() }
	fail := func(msg string) { dep.Status = store.DeployFailed; dep.Error = msg; logln("✗ %s", msg) }

	dep.Status = store.DeployRunning
	flush()

	logln("deploying to orcinus project %q…", target)
	res, err := s.Orcinus.Deploy(ctx, orcinus.DeployRequest{
		Source:    source,
		Project:   target,
		Wait:      false, // apply now; we poll readiness ourselves so we can stream it
		Prune:     prune,
		ACMEEmail: os.Getenv("KAPIBARA_ACME_EMAIL"),
	})
	if err != nil {
		fail("orcinus deploy: " + err.Error())
		return
	}
	dep.Applied = res.Applied
	if b, e := json.Marshal(res.Installed); e == nil {
		dep.Installed = string(b)
	}
	logln("applied %d object(s)", res.Applied)
	if len(res.Installed) > 0 {
		logln("plugins installed: %s", strings.Join(res.Installed, ", "))
	}

	if !wait {
		dep.Status = store.DeploySuccess
		logln("✓ done")
		return
	}

	logln("waiting for pods to become ready…")
	deadline := time.Now().Add(12 * time.Minute)
	last := ""
	for {
		if time.Now().After(deadline) {
			fail("timed out waiting for pods to become ready; last status: " + last)
			return
		}
		if pods, e := s.Orcinus.Pods(ctx, target); e == nil && len(pods) > 0 {
			ready := 0
			parts := make([]string, 0, len(pods))
			for _, pd := range pods {
				parts = append(parts, fmt.Sprintf("%s=%s(%s)", pd.Name, pd.Status, pd.Ready))
				if pd.Status == "Running" && podReady(pd.Ready) {
					ready++
				}
			}
			if cur := strings.Join(parts, "  "); cur != last {
				logln("pods: %s", cur)
				last = cur
			}
			if ready == len(pods) {
				dep.Status = store.DeploySuccess
				logln("✓ all %d pod(s) ready", ready)
				return
			}
		}
		select {
		case <-ctx.Done():
			fail(ctx.Err().Error())
			return
		case <-time.After(5 * time.Second):
		}
	}
}

// podReady reports whether a "ready" string like "1/1" means all containers ready.
func podReady(ready string) bool {
	a, b, ok := strings.Cut(ready, "/")
	return ok && a == b && a != "0" && a != ""
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

// handleRedeployDeployment re-applies a past deployment's captured compose
// snapshot to the cluster — a rollback to any historical deployment. For an
// application deployment the snapshot references the exact prior image, so no
// rebuild happens (true image rollback).
func (s *Server) handleRedeployDeployment(w http.ResponseWriter, r *http.Request) {
	old, err := s.Store.DeploymentByID(chi.URLParam(r, "deploymentID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "deployment not found")
		return
	}
	p, err := s.Store.ProjectByID(old.ProjectID)
	if err != nil || s.requireOrgAccess(w, r, p.OrganizationID) == nil {
		if err != nil {
			writeError(w, http.StatusNotFound, "project not found")
		}
		return
	}
	if strings.TrimSpace(old.Source) == "" {
		writeError(w, http.StatusBadRequest, "deployment has no source snapshot to redeploy")
		return
	}

	// Resolve the isolated orcinus project this deployment targeted.
	var target string
	switch old.Kind {
	case "application":
		app, err := s.Store.ApplicationByID(old.ApplicationID)
		if err != nil {
			writeError(w, http.StatusNotFound, "application not found")
			return
		}
		target = app.OrcinusProject
	default: // compose | template
		target = s.composeTarget(p, old.ComposeAppID)
	}

	dep := &store.Deployment{
		ProjectID:     p.ID,
		ComposeAppID:  old.ComposeAppID,
		ApplicationID: old.ApplicationID,
		Kind:          old.Kind,
		Status:        store.DeployRunning,
		Source:        old.Source,
		ImageRef:      old.ImageRef,
		CommitSHA:     old.CommitSHA,
	}
	if err := s.Store.CreateDeployment(dep); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Apply without blocking on readiness (avoids the client timeout on slow
	// rollouts); the redeploy returns once objects are applied.
	res, err := s.Orcinus.Deploy(r.Context(), orcinus.DeployRequest{
		Source:    old.Source,
		Project:   target,
		Wait:      false,
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
	if b, e := json.Marshal(res.Installed); e == nil {
		dep.Installed = string(b)
	}
	_ = s.Store.UpdateDeployment(dep)

	writeJSON(w, http.StatusOK, map[string]any{
		"deployment": dep, "applied": res.Applied, "rolledBackFrom": old.ID,
	})
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
