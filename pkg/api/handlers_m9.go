package api

import (
	"net/http"
	"net/url"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/store"
)

// handleListNodes lists cluster nodes (multi-node view).
func (s *Server) handleListNodes(w http.ResponseWriter, r *http.Request) {
	if s.Kube == nil {
		writeError(w, http.StatusServiceUnavailable, "cluster access unavailable (no kubeconfig)")
		return
	}
	nodes, err := s.Kube.Nodes(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": nodes})
}

// handleAuditLog returns recent audit entries (platform admins only).
func (s *Server) handleAuditLog(w http.ResponseWriter, r *http.Request) {
	if !currentUser(r).IsAdmin {
		writeError(w, http.StatusForbidden, "admin only")
		return
	}
	logs, err := s.Store.RecentAuditLogs(200)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"auditLogs": logs})
}

// handlePreviewDeploy deploys an application to an ephemeral preview project
// (per-branch/PR), isolated from the main project.
func (s *Server) handlePreviewDeploy(w http.ResponseWriter, r *http.Request) {
	app, project := s.loadAppWithAccess(w, r)
	if app == nil {
		return
	}
	var req struct {
		Branch string `json:"branch"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Branch == "" {
		writeError(w, http.StatusBadRequest, "branch required")
		return
	}

	previewName := project.OrcinusProject + "-preview-" + sanitizeDNS(req.Branch)

	// Deploy the app's chosen branch into an isolated ephemeral orcinus project.
	appCopy := *app
	appCopy.Branch = req.Branch
	appCopy.OrcinusProject = previewName
	syntheticProject := &store.Project{
		Base:           store.Base{ID: project.ID},
		OrganizationID: project.OrganizationID,
		Name:           project.Name + " (preview " + req.Branch + ")",
		OrcinusProject: previewName,
	}
	dep, err := s.Deployer.DeployApplication(r.Context(), &appCopy, syntheticProject, true)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"deploymentId":   dep.ID,
		"previewProject": previewName,
		"branch":         req.Branch,
		"status":         "deploying",
	})
}

// handlePreviewTeardown removes an ephemeral preview project from the cluster.
func (s *Server) handlePreviewTeardown(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	name := p.OrcinusProject + "-preview-" + sanitizeDNS(chi.URLParam(r, "branch"))
	if err := s.Orcinus.DeleteProject(r.Context(), name); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "torn-down", "previewProject": name})
}

func sanitizeDNS(s string) string {
	s = url.PathEscape(s)
	out := make([]rune, 0, len(s))
	for _, c := range s {
		switch {
		case c >= 'a' && c <= 'z', c >= '0' && c <= '9', c == '-':
			out = append(out, c)
		case c >= 'A' && c <= 'Z':
			out = append(out, c+32)
		default:
			out = append(out, '-')
		}
	}
	r := string(out)
	if r == "" {
		r = "branch"
	}
	return r
}
