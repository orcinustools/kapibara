package api

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/notify"
	"github.com/orcinustools/kapibara/pkg/store"
)

type notificationReq struct {
	Name      string            `json:"name"`
	Type      string            `json:"type"`
	Config    map[string]string `json:"config"`
	OnSuccess bool              `json:"onSuccess"`
	OnFailure bool              `json:"onFailure"`
}

func (s *Server) handleCreateNotification(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgAccess(w, r, orgID) == nil {
		return
	}
	var req notificationReq
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Type == "" {
		writeError(w, http.StatusBadRequest, "type required")
		return
	}
	cfg, _ := json.Marshal(req.Config)
	n := &store.Notification{
		OrganizationID: orgID, Name: req.Name, Type: req.Type,
		Config: string(cfg), OnSuccess: req.OnSuccess, OnFailure: req.OnFailure, Enabled: true,
	}
	if err := s.Store.CreateNotification(n); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (s *Server) handleListNotifications(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgAccess(w, r, orgID) == nil {
		return
	}
	ns, err := s.Store.NotificationsForOrg(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"notifications": ns})
}

func (s *Server) handleDeleteNotification(w http.ResponseWriter, r *http.Request) {
	// Look up to verify org access.
	var n store.Notification
	if err := s.Store.DB.First(&n, "id = ?", chi.URLParam(r, "notifID")).Error; err != nil {
		writeError(w, http.StatusNotFound, "notification not found")
		return
	}
	if s.requireOrgAccess(w, r, n.OrganizationID) == nil {
		return
	}
	if err := s.Store.DeleteNotification(n.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// dispatchNotifications sends an event to all matching channels of an org. It is
// wired into the deployer.
func (s *Server) dispatchNotifications(ctx context.Context, orgID string, success bool, title, message string) {
	ns, err := s.Store.NotificationsForOrg(orgID)
	if err != nil {
		return
	}
	ev := notify.Event{Title: title, Message: message, Level: notify.Success}
	if !success {
		ev.Level = notify.Error
	}
	for _, n := range ns {
		if !n.Enabled {
			continue
		}
		if (success && !n.OnSuccess) || (!success && !n.OnFailure) {
			continue
		}
		var cfg map[string]string
		_ = json.Unmarshal([]byte(n.Config), &cfg)
		// Best-effort; ignore individual channel errors.
		_ = notify.Send(ctx, notify.Channel{Type: n.Type, Config: cfg}, ev)
	}
}
