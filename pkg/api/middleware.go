package api

import (
	"context"
	"net/http"
	"strings"

	"github.com/orcinustools/kapibara/pkg/auth"
	"github.com/orcinustools/kapibara/pkg/store"
)

type ctxKey string

const userCtxKey ctxKey = "user"

// requireAuth authenticates a request via either a session JWT or an API token
// in the Authorization: Bearer header, and injects the user into the context.
func (s *Server) requireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tok := bearerToken(r)
		if tok == "" {
			writeError(w, http.StatusUnauthorized, "missing bearer token")
			return
		}

		var user *store.User
		if strings.HasPrefix(tok, "kap_") {
			// API token.
			u, err := s.Store.UserByAPITokenHash(auth.HashToken(tok))
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid api token")
				return
			}
			user = u
		} else {
			// Session JWT.
			claims, err := s.Auth.Verify(tok)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "invalid session token")
				return
			}
			u, err := s.Store.UserByID(claims.UserID)
			if err != nil {
				writeError(w, http.StatusUnauthorized, "user not found")
				return
			}
			user = u
		}

		ctx := context.WithValue(r.Context(), userCtxKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// auditRecorder records mutating actions (POST/PUT/DELETE) by authenticated
// users. It must run after requireAuth.
func (s *Server) auditRecorder(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead {
			next.ServeHTTP(w, r)
			return
		}
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		if u := currentUser(r); u != nil {
			_ = s.Store.CreateAuditLog(&store.AuditLog{
				UserID: u.ID, Email: u.Email,
				Action: r.Method + " " + r.URL.Path, Status: rec.status,
			})
		}
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

// Flush passes through so log streaming keeps working under the recorder.
func (r *statusRecorder) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// currentUser returns the authenticated user from the request context.
func currentUser(r *http.Request) *store.User {
	u, _ := r.Context().Value(userCtxKey).(*store.User)
	return u
}

// requireOrgAccess checks the current user is a member of orgID and returns the
// membership, writing a 403/404 and returning nil on failure.
func (s *Server) requireOrgAccess(w http.ResponseWriter, r *http.Request, orgID string) *store.Membership {
	u := currentUser(r)
	m, err := s.Store.Membership(u.ID, orgID)
	if err != nil {
		writeError(w, http.StatusForbidden, "not a member of this organization")
		return nil
	}
	return m
}

// requireOrgRole is like requireOrgAccess but additionally requires the caller's
// role to be one of the allowed roles (e.g. owner/admin for member management).
func (s *Server) requireOrgRole(w http.ResponseWriter, r *http.Request, orgID string, allowed ...store.Role) *store.Membership {
	m := s.requireOrgAccess(w, r, orgID)
	if m == nil {
		return nil
	}
	for _, role := range allowed {
		if m.Role == role {
			return m
		}
	}
	writeError(w, http.StatusForbidden, "insufficient role for this action")
	return nil
}

func bearerToken(r *http.Request) string {
	h := r.Header.Get("Authorization")
	if strings.HasPrefix(h, "Bearer ") {
		return strings.TrimSpace(h[len("Bearer "):])
	}
	return ""
}
