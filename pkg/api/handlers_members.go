package api

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/store"
)

// memberView is the org-member shape returned to the UI: it joins the
// membership role with the underlying user's identity.
type memberView struct {
	UserID string     `json:"userId"`
	Email  string     `json:"email"`
	Name   string     `json:"name"`
	Role   store.Role `json:"role"`
}

// handleListMembers lists an organization's members. Any org member may view.
func (s *Server) handleListMembers(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgAccess(w, r, orgID) == nil {
		return
	}
	ms, err := s.Store.MembershipsForOrg(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	views := make([]memberView, 0, len(ms))
	for _, m := range ms {
		views = append(views, memberView{
			UserID: m.UserID, Email: m.User.Email, Name: m.User.Name, Role: m.Role,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"members": views})
}

// handleAddMember adds an existing user (by email) to the org with a role.
// Only owners/admins may add members. There is no invite flow: the user must
// already have a kapibara account.
func (s *Server) handleAddMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgRole(w, r, orgID, store.RoleOwner, store.RoleAdmin) == nil {
		return
	}
	var req struct {
		Email string     `json:"email"`
		Role  store.Role `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Email == "" {
		writeError(w, http.StatusBadRequest, "email required")
		return
	}
	if !validRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, or member")
		return
	}
	u, err := s.Store.UserByEmail(req.Email)
	if err != nil {
		writeError(w, http.StatusNotFound, "no user with that email (they must register first)")
		return
	}
	if _, err := s.Store.Membership(u.ID, orgID); err == nil {
		writeError(w, http.StatusConflict, "user is already a member")
		return
	}
	m := &store.Membership{UserID: u.ID, OrganizationID: orgID, Role: req.Role}
	if err := s.Store.CreateMembership(m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, memberView{
		UserID: u.ID, Email: u.Email, Name: u.Name, Role: req.Role,
	})
}

// handleUpdateMember changes a member's role. Only owners/admins may change
// roles, and the last owner cannot be demoted (would orphan the org).
func (s *Server) handleUpdateMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgRole(w, r, orgID, store.RoleOwner, store.RoleAdmin) == nil {
		return
	}
	targetUserID := chi.URLParam(r, "userID")
	var req struct {
		Role store.Role `json:"role"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !validRole(req.Role) {
		writeError(w, http.StatusBadRequest, "role must be owner, admin, or member")
		return
	}
	m, err := s.Store.Membership(targetUserID, orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	// Guard the last owner: demoting them would leave the org unmanageable.
	if m.Role == store.RoleOwner && req.Role != store.RoleOwner {
		if n, _ := s.Store.CountOrgOwners(orgID); n <= 1 {
			writeError(w, http.StatusConflict, "cannot demote the last owner")
			return
		}
	}
	m.Role = req.Role
	if err := s.Store.UpdateMembership(m); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "updated"})
}

// handleRemoveMember removes a member from the org. Only owners/admins may
// remove members, and the last owner cannot be removed.
func (s *Server) handleRemoveMember(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgRole(w, r, orgID, store.RoleOwner, store.RoleAdmin) == nil {
		return
	}
	targetUserID := chi.URLParam(r, "userID")
	m, err := s.Store.Membership(targetUserID, orgID)
	if err != nil {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}
	if m.Role == store.RoleOwner {
		if n, _ := s.Store.CountOrgOwners(orgID); n <= 1 {
			writeError(w, http.StatusConflict, "cannot remove the last owner")
			return
		}
	}
	if err := s.Store.DeleteMembership(m.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "removed"})
}

func validRole(r store.Role) bool {
	switch r {
	case store.RoleOwner, store.RoleAdmin, store.RoleMember:
		return true
	}
	return false
}
