package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/store"
)

// --- organizations ---

func (s *Server) handleListOrgs(w http.ResponseWriter, r *http.Request) {
	orgs, err := s.Store.OrgsForUser(currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"organizations": orgs})
}

func (s *Server) handleCreateOrg(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	org := &store.Organization{Name: req.Name, Slug: uniqueSlug(s, slugify(req.Name))}
	if err := s.Store.CreateOrg(org); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.CreateMembership(&store.Membership{
		UserID: currentUser(r).ID, OrganizationID: org.ID, Role: store.RoleOwner,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, org)
}

// --- projects ---

func (s *Server) handleListProjects(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgAccess(w, r, orgID) == nil {
		return
	}
	ps, err := s.Store.ProjectsForOrg(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": ps})
}

func (s *Server) handleCreateProject(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgAccess(w, r, orgID) == nil {
		return
	}
	var req struct {
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "name required")
		return
	}
	// The orcinus project name is globally unique and DNS-safe.
	orcinusName := uniqueOrcinusProject(s, slugify(req.Name))
	p := &store.Project{
		OrganizationID: orgID,
		Name:           req.Name,
		Description:    req.Description,
		OrcinusProject: orcinusName,
	}
	if err := s.Store.CreateProject(p); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (s *Server) handleGetProject(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	writeJSON(w, http.StatusOK, p)
}

func (s *Server) handleDeleteProject(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	// Best-effort: remove every unit's isolated cluster resources.
	for _, u := range s.projectUnits(p.ID) {
		_ = s.Orcinus.DeleteProject(r.Context(), u.OrcinusProject)
	}
	_ = s.Orcinus.DeleteProject(r.Context(), p.OrcinusProject) // legacy grouping
	if err := s.Store.DeleteProject(p.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// loadProjectWithAccess loads the {projectID} route param and verifies the
// caller can access its organization. Returns nil (after writing an error) on
// failure.
func (s *Server) loadProjectWithAccess(w http.ResponseWriter, r *http.Request) *store.Project {
	p, err := s.Store.ProjectByID(chi.URLParam(r, "projectID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return nil
	}
	if s.requireOrgAccess(w, r, p.OrganizationID) == nil {
		return nil
	}
	return p
}

// unitProject returns the isolated orcinus project name for a deployable unit
// (application/database/compose) within a kapibara project. Each unit gets its
// own orcinus project so deploys don't prune sibling units.
func unitProject(project *store.Project, name string) string {
	return project.OrcinusProject + "-" + sanitizeDNS(name)
}

func uniqueOrcinusProject(s *Server, base string) string {
	name := base
	for i := 2; ; i++ {
		var n int64
		s.Store.DB.Model(&store.Project{}).Where("orcinus_project = ?", name).Count(&n)
		if n == 0 {
			return name
		}
		name = base + "-" + strconv.Itoa(i)
	}
}
