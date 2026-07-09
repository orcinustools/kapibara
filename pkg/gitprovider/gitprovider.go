// Package gitprovider talks to source-control providers (GitHub, GitLab) to
// validate access tokens, list a user's repositories, and exchange OAuth codes
// for tokens. It is deliberately dependency-free (net/http + encoding/json).
package gitprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Kind is a supported provider type.
type Kind string

const (
	GitHub Kind = "github"
	GitLab Kind = "gitlab"
)

// Repo is a normalized repository across providers.
type Repo struct {
	FullName      string `json:"fullName"`
	CloneURL      string `json:"cloneUrl"`
	Private       bool   `json:"private"`
	DefaultBranch string `json:"defaultBranch"`
}

// Client calls a provider's REST API with a bearer/PAT token.
type Client struct {
	Kind    Kind
	Token   string
	BaseURL string // enterprise/self-hosted host; empty → public SaaS
	http    *http.Client
}

// New builds a provider client.
func New(kind Kind, token, baseURL string) *Client {
	return &Client{Kind: kind, Token: token, BaseURL: strings.TrimRight(baseURL, "/"),
		http: &http.Client{Timeout: 15 * time.Second}}
}

// apiBase returns the REST API root for the provider.
func (c *Client) apiBase() string {
	switch c.Kind {
	case GitLab:
		host := c.BaseURL
		if host == "" {
			host = "https://gitlab.com"
		}
		return host + "/api/v4"
	default: // github
		if c.BaseURL != "" {
			return c.BaseURL + "/api/v3" // GitHub Enterprise
		}
		return "https://api.github.com"
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.apiBase()+path, nil)
	if err != nil {
		return err
	}
	switch c.Kind {
	case GitLab:
		req.Header.Set("Authorization", "Bearer "+c.Token)
	default:
		req.Header.Set("Authorization", "Bearer "+c.Token)
		req.Header.Set("Accept", "application/vnd.github+json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden {
		return fmt.Errorf("provider rejected token (HTTP %d)", res.StatusCode)
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("provider error (HTTP %d)", res.StatusCode)
	}
	if out != nil {
		return json.Unmarshal(body, out)
	}
	return nil
}

// Login validates the token and returns the authenticated account's username.
func (c *Client) Login(ctx context.Context) (string, error) {
	switch c.Kind {
	case GitLab:
		var u struct {
			Username string `json:"username"`
		}
		if err := c.get(ctx, "/user", &u); err != nil {
			return "", err
		}
		return u.Username, nil
	default:
		var u struct {
			Login string `json:"login"`
		}
		if err := c.get(ctx, "/user", &u); err != nil {
			return "", err
		}
		return u.Login, nil
	}
}

// ListRepos returns repositories the token can access, most-recent first.
func (c *Client) ListRepos(ctx context.Context) ([]Repo, error) {
	switch c.Kind {
	case GitLab:
		var raw []struct {
			PathWithNamespace string `json:"path_with_namespace"`
			HTTPURLToRepo     string `json:"http_url_to_repo"`
			Visibility        string `json:"visibility"`
			DefaultBranch     string `json:"default_branch"`
		}
		if err := c.get(ctx, "/projects?membership=true&per_page=100&order_by=last_activity_at", &raw); err != nil {
			return nil, err
		}
		repos := make([]Repo, 0, len(raw))
		for _, r := range raw {
			repos = append(repos, Repo{
				FullName: r.PathWithNamespace, CloneURL: r.HTTPURLToRepo,
				Private: r.Visibility != "public", DefaultBranch: r.DefaultBranch,
			})
		}
		return repos, nil
	default:
		var raw []struct {
			FullName      string `json:"full_name"`
			CloneURL      string `json:"clone_url"`
			Private       bool   `json:"private"`
			DefaultBranch string `json:"default_branch"`
		}
		if err := c.get(ctx, "/user/repos?per_page=100&sort=updated", &raw); err != nil {
			return nil, err
		}
		repos := make([]Repo, 0, len(raw))
		for _, r := range raw {
			repos = append(repos, Repo{
				FullName: r.FullName, CloneURL: r.CloneURL,
				Private: r.Private, DefaultBranch: r.DefaultBranch,
			})
		}
		return repos, nil
	}
}

// --- OAuth (GitHub web application flow) ---

// OAuthConfig holds the registered OAuth app credentials for a provider.
type OAuthConfig struct {
	ClientID     string
	ClientSecret string
	BaseURL      string // enterprise host; empty → public
}

// Configured reports whether OAuth credentials are present.
func (o OAuthConfig) Configured() bool { return o.ClientID != "" && o.ClientSecret != "" }

// AuthorizeURL builds the provider authorize URL to redirect the user to.
func (o OAuthConfig) AuthorizeURL(kind Kind, redirectURI, state, scope string) string {
	q := url.Values{}
	q.Set("client_id", o.ClientID)
	q.Set("redirect_uri", redirectURI)
	q.Set("state", state)
	q.Set("scope", scope)
	switch kind {
	case GitLab:
		host := o.BaseURL
		if host == "" {
			host = "https://gitlab.com"
		}
		q.Set("response_type", "code")
		return host + "/oauth/authorize?" + q.Encode()
	default:
		host := "https://github.com"
		if o.BaseURL != "" {
			host = o.BaseURL
		}
		return host + "/login/oauth/authorize?" + q.Encode()
	}
}

// ExchangeCode swaps an authorization code for an access token.
func (o OAuthConfig) ExchangeCode(ctx context.Context, kind Kind, code, redirectURI string) (string, error) {
	form := url.Values{}
	form.Set("client_id", o.ClientID)
	form.Set("client_secret", o.ClientSecret)
	form.Set("code", code)
	form.Set("redirect_uri", redirectURI)

	var tokenURL string
	switch kind {
	case GitLab:
		host := o.BaseURL
		if host == "" {
			host = "https://gitlab.com"
		}
		form.Set("grant_type", "authorization_code")
		tokenURL = host + "/oauth/token"
	default:
		host := "https://github.com"
		if o.BaseURL != "" {
			host = o.BaseURL
		}
		tokenURL = host + "/login/oauth/access_token"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	client := &http.Client{Timeout: 15 * time.Second}
	res, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return "", fmt.Errorf("token exchange failed (HTTP %d)", res.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
		Error       string `json:"error"`
	}
	if err := json.Unmarshal(body, &tok); err != nil {
		return "", err
	}
	if tok.AccessToken == "" {
		if tok.Error != "" {
			return "", fmt.Errorf("token exchange error: %s", tok.Error)
		}
		return "", fmt.Errorf("token exchange returned no access token")
	}
	return tok.AccessToken, nil
}
