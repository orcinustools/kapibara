package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/orcinustools/kapibara/pkg/config"
	"github.com/orcinustools/kapibara/pkg/store"
)

// newTestServer builds a Server backed by an in-memory SQLite store and a fake
// orcinus API.
func newTestServer(t *testing.T, engineURL string) *Server {
	t.Helper()
	st, err := store.Open("file::memory:?cache=shared")
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	cfg := config.Config{OrcinusURL: engineURL}
	return New(cfg, st)
}

func TestHealthzAndVersion(t *testing.T) {
	// Fake orcinus that answers /healthz.
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer engine.Close()

	s := newTestServer(t, engine.URL)

	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("healthz code = %d", rec.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(rec.Body.Bytes(), &body)
	if body["status"] != "ok" {
		t.Errorf("status = %v", body["status"])
	}
	if body["engineHealthy"] != true {
		t.Errorf("engineHealthy = %v, want true", body["engineHealthy"])
	}

	rec = httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/version", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("version code = %d", rec.Code)
	}
}

func TestClusterProxy(t *testing.T) {
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/cluster" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"orcinus","kubeconfig":"/x","nodes":"node1"}`))
			return
		}
		http.Error(w, "no", http.StatusNotFound)
	}))
	defer engine.Close()

	s := newTestServer(t, engine.URL)

	// /api/v1/cluster requires auth: create a user + issue a session token.
	u := &store.User{Email: "t@x.com", PasswordHash: "x"}
	if err := s.Store.CreateUser(u); err != nil {
		t.Fatalf("create user: %v", err)
	}
	tok, err := s.Auth.Issue(u.ID, u.Email)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/api/v1/cluster", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	s.Handler().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("cluster code = %d, body=%s", rec.Code, rec.Body.String())
	}
	var cs map[string]string
	_ = json.Unmarshal(rec.Body.Bytes(), &cs)
	if cs["name"] != "orcinus" {
		t.Errorf("cluster name = %q", cs["name"])
	}
}
