package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// fakeEngine records deploy calls and serves canned responses.
type fakeEngine struct {
	lastProject string
	lastSource  string
	srv         *httptest.Server
}

func newFakeEngine(t *testing.T) *fakeEngine {
	f := &fakeEngine{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/healthz":
			w.WriteHeader(http.StatusOK)
		case r.URL.Path == "/api/v1/deploy":
			var req map[string]any
			_ = json.NewDecoder(r.Body).Decode(&req)
			f.lastProject, _ = req["project"].(string)
			f.lastSource, _ = req["source"].(string)
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"applied":3,"project":"` + f.lastProject + `","installed":[]}`))
		case r.URL.Path == "/api/v1/convert":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"objects":2,"manifests":"kind: Deployment"}`))
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	t.Cleanup(f.srv.Close)
	return f
}

func TestComposeDeployFlow(t *testing.T) {
	engine := newFakeEngine(t)
	s := newTestServer(t, engine.srv.URL)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	h := &harness{t: t, srv: ts}

	// Register + get org + project.
	_, reg := h.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": "d@x.com", "password": "password1",
	})
	token := reg["token"].(string)
	_, orgs := h.do(http.MethodGet, "/api/v1/orgs", token, nil)
	orgID := orgs["organizations"].([]any)[0].(map[string]any)["id"].(string)
	_, proj := h.do(http.MethodPost, "/api/v1/orgs/"+orgID+"/projects", token, map[string]string{"name": "shop"})
	projID := proj["id"].(string)
	orcinusProject := proj["orcinusProject"].(string)

	compose := "services:\n  web:\n    image: nginx:alpine\n    ports: [\"80\"]\n"

	// Convert (preview) — should proxy to the engine with the mapped project.
	resp, conv := h.do(http.MethodPost, "/api/v1/projects/"+projID+"/convert", token, map[string]any{
		"source": compose,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("convert = %d %v", resp.StatusCode, conv)
	}
	if conv["objects"].(float64) != 2 {
		t.Errorf("convert objects = %v", conv["objects"])
	}

	// Deploy inline source.
	resp, dep := h.do(http.MethodPost, "/api/v1/projects/"+projID+"/deploy", token, map[string]any{
		"source": compose, "wait": false,
	})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("deploy = %d %v", resp.StatusCode, dep)
	}
	if dep["applied"].(float64) != 3 {
		t.Errorf("applied = %v, want 3", dep["applied"])
	}
	// The engine must have received the compose unit's ISOLATED orcinus project
	// (project slug + "-compose"), not the bare project slug.
	wantProject := orcinusProject + "-compose"
	if engine.lastProject != wantProject {
		t.Errorf("engine project = %q, want %q", engine.lastProject, wantProject)
	}
	if engine.lastSource != compose {
		t.Errorf("engine source mismatch: %q", engine.lastSource)
	}
	// Deployment status recorded as success.
	d := dep["deployment"].(map[string]any)
	if d["status"] != "success" {
		t.Errorf("deployment status = %v, want success", d["status"])
	}

	// Deployment history has one entry.
	_, hist := h.do(http.MethodGet, "/api/v1/projects/"+projID+"/deployments", token, nil)
	if len(hist["deployments"].([]any)) != 1 {
		t.Errorf("deployments = %v", hist["deployments"])
	}
}

func TestDeployRequiresSource(t *testing.T) {
	engine := newFakeEngine(t)
	s := newTestServer(t, engine.srv.URL)
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	h := &harness{t: t, srv: ts}

	_, reg := h.do(http.MethodPost, "/api/v1/auth/register", "", map[string]string{
		"email": "e@x.com", "password": "password1",
	})
	token := reg["token"].(string)
	_, orgs := h.do(http.MethodGet, "/api/v1/orgs", token, nil)
	orgID := orgs["organizations"].([]any)[0].(map[string]any)["id"].(string)
	_, proj := h.do(http.MethodPost, "/api/v1/orgs/"+orgID+"/projects", token, map[string]string{"name": "empty"})
	projID := proj["id"].(string)

	resp, _ := h.do(http.MethodPost, "/api/v1/projects/"+projID+"/deploy", token, map[string]any{})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("deploy without source = %d, want 400", resp.StatusCode)
	}
}
