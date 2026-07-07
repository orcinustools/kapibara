package orcinus

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestDeployAndProjects(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/deploy":
			var req DeployRequest
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Project != "shop" {
				t.Errorf("project = %q, want shop", req.Project)
			}
			writeJSON(w, DeployResult{Applied: 3, Project: req.Project, Installed: []string{}})
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/projects":
			writeJSON(w, map[string]any{"projects": []Project{{Name: "shop", Workloads: 2, Ready: 2}}})
		default:
			http.Error(w, "not found", http.StatusNotFound)
		}
	}))
	defer srv.Close()

	c := New(srv.URL, "tok")
	res, err := c.Deploy(context.Background(), DeployRequest{Source: "services: {}", Project: "shop"})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if res.Applied != 3 {
		t.Errorf("applied = %d, want 3", res.Applied)
	}
	if gotAuth != "Bearer tok" {
		t.Errorf("auth header = %q, want Bearer tok", gotAuth)
	}

	projs, err := c.Projects(context.Background())
	if err != nil {
		t.Fatalf("projects: %v", err)
	}
	if len(projs) != 1 || projs[0].Name != "shop" {
		t.Errorf("projects = %+v", projs)
	}
}

func TestAPIError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":"bad input"}`))
	}))
	defer srv.Close()

	c := New(srv.URL, "")
	_, err := c.Projects(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	apiErr, ok := err.(*APIError)
	if !ok {
		t.Fatalf("error type = %T, want *APIError", err)
	}
	if apiErr.Status != 400 || apiErr.Message != "bad input" {
		t.Errorf("apiErr = %+v", apiErr)
	}
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(v)
}
