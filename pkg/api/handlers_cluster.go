package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"
)

// --- secrets (proxied to orcinus) ---

func (s *Server) handleListSecrets(w http.ResponseWriter, r *http.Request) {
	secrets, err := s.Orcinus.Secrets(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"secrets": secrets})
}

func (s *Server) handlePutSecret(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string            `json:"name"`
		Data map[string]string `json:"data"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" || len(req.Data) == 0 {
		writeError(w, http.StatusBadRequest, "name and data required")
		return
	}
	if err := s.Orcinus.PutSecret(r.Context(), req.Name, req.Data); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleDeleteSecret(w http.ResponseWriter, r *http.Request) {
	if err := s.Orcinus.DeleteSecret(r.Context(), chi.URLParam(r, "name")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// --- plugins (proxied to orcinus) ---

func (s *Server) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	plugins, err := s.Orcinus.Plugins(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"plugins": plugins})
}

func (s *Server) handleInstallPlugin(w http.ResponseWriter, r *http.Request) {
	var opts map[string]any
	// Options are optional; ignore decode errors on an empty body.
	_ = decodeJSONOptional(r, &opts)
	if err := s.Orcinus.InstallPlugin(r.Context(), chi.URLParam(r, "name"), opts); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "installing"})
}

func (s *Server) handleRemovePlugin(w http.ResponseWriter, r *http.Request) {
	if err := s.Orcinus.RemovePlugin(r.Context(), chi.URLParam(r, "name")); err != nil {
		writeError(w, http.StatusBadGateway, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}
