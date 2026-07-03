// Package orcinus is a client for the orcinus cluster-engine HTTP API.
//
// It mirrors the endpoints documented in orcinus/docs/API.md. kapibara never
// writes Kubernetes manifests directly: it composes docker-compose sources
// (optionally with x-orcinus-* hints) and hands them to orcinus, which owns
// conversion, apply/prune, plugin auto-install and ownership labels.
package orcinus

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client talks to a single orcinus API server.
type Client struct {
	BaseURL string
	Token   string
	HTTP    *http.Client
}

// New returns a client for the given orcinus API base URL and bearer token.
func New(baseURL, token string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		Token:   token,
		HTTP:    &http.Client{Timeout: 120 * time.Second},
	}
}

// APIError is a non-2xx response from orcinus.
type APIError struct {
	Status  int
	Message string
}

func (e *APIError) Error() string {
	return fmt.Sprintf("orcinus api: %d: %s", e.Status, e.Message)
}

// --- request/response types (mirror orcinus/pkg/api) ---

// DeployRequest is the JSON body for POST /api/v1/deploy and /convert.
type DeployRequest struct {
	Source    string `json:"source"`
	Project   string `json:"project,omitempty"`
	Namespace string `json:"namespace,omitempty"`
	Mode      string `json:"mode,omitempty"` // "" | compose | manifest
	Replicas  int    `json:"replicas,omitempty"`
	PVCSize   string `json:"pvcSize,omitempty"`
	Prune     *bool  `json:"prune,omitempty"`
	Wait      bool   `json:"wait,omitempty"`
	ACMEEmail string `json:"acmeEmail,omitempty"`
}

// DeployResult is the response from /api/v1/deploy.
type DeployResult struct {
	Applied   int      `json:"applied"`
	Project   string   `json:"project"`
	Installed []string `json:"installed"`
}

// ConvertResult is the response from /api/v1/convert.
type ConvertResult struct {
	Objects   int    `json:"objects"`
	Manifests string `json:"manifests"`
}

// Project summarizes a deployed project.
type Project struct {
	Name       string   `json:"Name"`
	Namespaces []string `json:"Namespaces"`
	Workloads  int      `json:"Workloads"`
	Ready      int      `json:"Ready"`
}

// Pod is one pod belonging to a project.
type Pod struct {
	Service  string `json:"service"`
	Name     string `json:"name"`
	Ready    string `json:"ready"`
	Status   string `json:"status"`
	Restarts int    `json:"restarts"`
	Node     string `json:"node"`
}

// ClusterStatus is the response from /api/v1/cluster.
type ClusterStatus struct {
	Name       string `json:"name"`
	Kubeconfig string `json:"kubeconfig"`
	Nodes      string `json:"nodes"`
}

// Secret is a control-plane view of a cluster secret.
type Secret struct {
	Name string `json:"name"`
	Keys int    `json:"keys"`
}

// Plugin is a catalog entry with install state.
type Plugin struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Installed   bool   `json:"installed"`
	Ready       bool   `json:"ready"`
}

// --- endpoint methods ---

// Deploy converts + applies the source to the cluster.
func (c *Client) Deploy(ctx context.Context, req DeployRequest) (*DeployResult, error) {
	var out DeployResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/deploy", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Convert renders the source to manifests without touching the cluster.
func (c *Client) Convert(ctx context.Context, req DeployRequest) (*ConvertResult, error) {
	var out ConvertResult
	if err := c.do(ctx, http.MethodPost, "/api/v1/convert", req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Projects lists deployed projects.
func (c *Client) Projects(ctx context.Context) ([]Project, error) {
	var out struct {
		Projects []Project `json:"projects"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/projects", nil, &out); err != nil {
		return nil, err
	}
	return out.Projects, nil
}

// Pods lists the pods of a project.
func (c *Client) Pods(ctx context.Context, project string) ([]Pod, error) {
	var out struct {
		Pods []Pod `json:"pods"`
	}
	path := "/api/v1/projects/" + url.PathEscape(project) + "/pods"
	if err := c.do(ctx, http.MethodGet, path, nil, &out); err != nil {
		return nil, err
	}
	return out.Pods, nil
}

// DeleteProject removes all resources of a project.
func (c *Client) DeleteProject(ctx context.Context, project string) error {
	path := "/api/v1/projects/" + url.PathEscape(project)
	return c.do(ctx, http.MethodDelete, path, nil, nil)
}

// Scale sets the replica count of a service.
func (c *Client) Scale(ctx context.Context, project, service string, replicas int) error {
	path := fmt.Sprintf("/api/v1/projects/%s/services/%s/scale",
		url.PathEscape(project), url.PathEscape(service))
	return c.do(ctx, http.MethodPost, path, map[string]int{"replicas": replicas}, nil)
}

// Rollback rolls a service back to its previous revision.
func (c *Client) Rollback(ctx context.Context, project, service string) error {
	path := fmt.Sprintf("/api/v1/projects/%s/services/%s/rollback",
		url.PathEscape(project), url.PathEscape(service))
	return c.do(ctx, http.MethodPost, path, nil, nil)
}

// Secrets lists cluster secrets.
func (c *Client) Secrets(ctx context.Context) ([]Secret, error) {
	var out struct {
		Secrets []Secret `json:"secrets"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/secrets", nil, &out); err != nil {
		return nil, err
	}
	return out.Secrets, nil
}

// PutSecret creates or updates an opaque secret.
func (c *Client) PutSecret(ctx context.Context, name string, data map[string]string) error {
	body := map[string]any{"name": name, "data": data}
	return c.do(ctx, http.MethodPost, "/api/v1/secrets", body, nil)
}

// DeleteSecret removes a secret.
func (c *Client) DeleteSecret(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/secrets/"+url.PathEscape(name), nil, nil)
}

// Plugins lists the plugin catalog with install state.
func (c *Client) Plugins(ctx context.Context) ([]Plugin, error) {
	var out struct {
		Plugins []Plugin `json:"plugins"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v1/plugins", nil, &out); err != nil {
		return nil, err
	}
	return out.Plugins, nil
}

// InstallPlugin installs a plugin with the given options.
func (c *Client) InstallPlugin(ctx context.Context, name string, opts map[string]any) error {
	return c.do(ctx, http.MethodPost, "/api/v1/plugins/"+url.PathEscape(name), opts, nil)
}

// RemovePlugin removes a plugin.
func (c *Client) RemovePlugin(ctx context.Context, name string) error {
	return c.do(ctx, http.MethodDelete, "/api/v1/plugins/"+url.PathEscape(name), nil, nil)
}

// Cluster returns cluster status.
func (c *Client) Cluster(ctx context.Context) (*ClusterStatus, error) {
	var out ClusterStatus
	if err := c.do(ctx, http.MethodGet, "/api/v1/cluster", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// Version returns the orcinus build/component versions.
func (c *Client) Version(ctx context.Context) (map[string]string, error) {
	var out map[string]string
	if err := c.do(ctx, http.MethodGet, "/version", nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// Healthy reports whether the orcinus API is reachable and healthy.
func (c *Client) Healthy(ctx context.Context) bool {
	err := c.do(ctx, http.MethodGet, "/healthz", nil, nil)
	return err == nil
}

// do performs a request, JSON-encoding body (if any) and decoding into out (if any).
func (c *Client) do(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("marshal request: %w", err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return fmt.Errorf("orcinus unreachable: %w", err)
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := strings.TrimSpace(string(data))
		var e struct {
			Error string `json:"error"`
		}
		if json.Unmarshal(data, &e) == nil && e.Error != "" {
			msg = e.Error
		}
		return &APIError{Status: resp.StatusCode, Message: msg}
	}
	if out != nil && len(data) > 0 {
		if err := json.Unmarshal(data, out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
	}
	return nil
}
