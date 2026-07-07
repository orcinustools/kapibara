package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// harness wraps an httptest server around a fresh in-memory kapibara.
type harness struct {
	t   *testing.T
	srv *httptest.Server
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	// A dummy engine so /healthz etc. don't hang; not used by these tests.
	engine := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(engine.Close)
	s := newTestServer(t, engine.URL)
	ts := httptest.NewServer(s.Handler())
	t.Cleanup(ts.Close)
	return &harness{t: t, srv: ts}
}

func (h *harness) do(method, path, token string, body any) (*http.Response, map[string]any) {
	h.t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req, _ := http.NewRequest(method, h.srv.URL+path, &buf)
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		h.t.Fatalf("%s %s: %v", method, path, err)
	}
	var out map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	return resp, out
}

func TestAuthOrgProjectFlow(t *testing.T) {
	h := newHarness(t)

	// Register (first user → admin + default org).
	resp, body := h.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": "ibnu@biznetgio.com", "password": "supersecret", "name": "Ibnu",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("register status = %d, body=%v", resp.StatusCode, body)
	}
	token, _ := body["token"].(string)
	if token == "" {
		t.Fatal("register returned no token")
	}
	if u, _ := body["user"].(map[string]any); u["isAdmin"] != true {
		t.Errorf("first user isAdmin = %v, want true", u["isAdmin"])
	}

	// /me with the session token.
	resp, me := h.do(http.MethodGet, "/api/v1/auth/me", token, nil)
	if resp.StatusCode != http.StatusOK || me["email"] != "ibnu@biznetgio.com" {
		t.Fatalf("me = %d %v", resp.StatusCode, me)
	}

	// /me without a token → 401.
	resp, _ = h.do(http.MethodGet, "/api/v1/auth/me", "", nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("me no-token status = %d, want 401", resp.StatusCode)
	}

	// List orgs — the default org should be present.
	resp, orgsBody := h.do(http.MethodGet, "/api/v1/orgs", token, nil)
	orgs, _ := orgsBody["organizations"].([]any)
	if resp.StatusCode != http.StatusOK || len(orgs) != 1 {
		t.Fatalf("orgs = %d %v", resp.StatusCode, orgsBody)
	}
	orgID := orgs[0].(map[string]any)["id"].(string)

	// Create a project in the org.
	resp, proj := h.do(http.MethodPost, "/api/v1/orgs/"+orgID+"/projects", token, map[string]string{
		"name": "My Shop", "description": "test",
	})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create project = %d %v", resp.StatusCode, proj)
	}
	if proj["orcinusProject"] != "my-shop" {
		t.Errorf("orcinusProject = %v, want my-shop", proj["orcinusProject"])
	}
	projID := proj["id"].(string)

	// Get the project back.
	resp, _ = h.do(http.MethodGet, "/api/v1/projects/"+projID, token, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get project = %d", resp.StatusCode)
	}

	// Login as the same user.
	resp, login := h.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email": "ibnu@biznetgio.com", "password": "supersecret",
	})
	if resp.StatusCode != http.StatusOK || login["token"] == "" {
		t.Fatalf("login = %d %v", resp.StatusCode, login)
	}

	// Wrong password → 401.
	resp, _ = h.do(http.MethodPost, "/api/v1/auth/login", "", map[string]string{
		"email": "ibnu@biznetgio.com", "password": "wrong",
	})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("bad login = %d, want 401", resp.StatusCode)
	}

	// Create an API token, then use it to authenticate.
	resp, tokBody := h.do(http.MethodPost, "/api/v1/tokens", token, map[string]string{"name": "ci"})
	apiTok, _ := tokBody["token"].(string)
	if resp.StatusCode != http.StatusCreated || apiTok == "" {
		t.Fatalf("create token = %d %v", resp.StatusCode, tokBody)
	}
	resp, meViaToken := h.do(http.MethodGet, "/api/v1/auth/me", apiTok, nil)
	if resp.StatusCode != http.StatusOK || meViaToken["email"] != "ibnu@biznetgio.com" {
		t.Fatalf("me via api token = %d %v", resp.StatusCode, meViaToken)
	}
}

func TestSecondUserNotAdminAndIsolation(t *testing.T) {
	h := newHarness(t)

	// First user.
	_, b1 := h.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": "a@x.com", "password": "password1",
	})
	t1 := b1["token"].(string)

	// Second user — not admin, own separate org.
	_, b2 := h.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": "b@x.com", "password": "password2",
	})
	if u := b2["user"].(map[string]any); u["isAdmin"] == true {
		t.Error("second user should not be admin")
	}
	t2 := b2["token"].(string)

	// User A's org id.
	_, orgsA := h.do(http.MethodGet, "/api/v1/orgs", t1, nil)
	orgA := orgsA["organizations"].([]any)[0].(map[string]any)["id"].(string)

	// User B must not be able to list/create projects in user A's org.
	resp, _ := h.do(http.MethodGet, "/api/v1/orgs/"+orgA+"/projects", t2, nil)
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("cross-org access = %d, want 403", resp.StatusCode)
	}
}
