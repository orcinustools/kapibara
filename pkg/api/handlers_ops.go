package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// handleScaleService scales a service within a project to a replica count.
func (s *Server) handleScaleService(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	service := chi.URLParam(r, "service")
	var req struct {
		Replicas int `json:"replicas"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Replicas < 0 {
		writeError(w, http.StatusBadRequest, "replicas must be >= 0")
		return
	}
	target, ok := s.resolveServiceProject(p.ID, service)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found in project")
		return
	}
	if err := s.Orcinus.Scale(r.Context(), target, service, req.Replicas); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "scaled", "service": service, "replicas": req.Replicas})
}

// handleRollbackService rolls a service back to its previous revision.
func (s *Server) handleRollbackService(w http.ResponseWriter, r *http.Request) {
	p := s.loadProjectWithAccess(w, r)
	if p == nil {
		return
	}
	service := chi.URLParam(r, "service")
	target, ok := s.resolveServiceProject(p.ID, service)
	if !ok {
		writeError(w, http.StatusNotFound, "service not found in project")
		return
	}
	if err := s.Orcinus.Rollback(r.Context(), target, service); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"status": "rolled-back", "service": service})
}

// resolveServiceProject finds the isolated orcinus project owning a service name
// within a kapibara project. Applications/databases match by service name;
// otherwise a lone compose unit is used as a best-effort fallback.
func (s *Server) resolveServiceProject(projectID, service string) (string, bool) {
	units := s.projectUnits(projectID)
	for _, u := range units {
		if u.Service != "" && u.Service == service {
			return u.OrcinusProject, true
		}
	}
	// Fallback: compose units (services live inside the compose file).
	var compose []unitRef
	for _, u := range units {
		if u.Kind == "compose" {
			compose = append(compose, u)
		}
	}
	if len(compose) == 1 {
		return compose[0].OrcinusProject, true
	}
	if len(units) == 1 {
		return units[0].OrcinusProject, true
	}
	return "", false
}
