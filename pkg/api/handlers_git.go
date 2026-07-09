package api

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/orcinustools/kapibara/pkg/gitprovider"
	"github.com/orcinustools/kapibara/pkg/store"
)

// --- OAuth state store (in-memory, single-instance) ---

// oauthState links an unauthenticated OAuth callback back to the org + provider
// details the authenticated user chose when starting the flow.
type oauthState struct {
	orgID   string
	kind    gitprovider.Kind
	name    string
	baseURL string
	userID  string
	expires time.Time
}

type oauthStateStore struct {
	mu sync.Mutex
	m  map[string]oauthState
}

func newOAuthStateStore() *oauthStateStore { return &oauthStateStore{m: map[string]oauthState{}} }

func (s *oauthStateStore) put(st oauthState) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token := hex.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	// Opportunistic GC of expired states.
	for k, v := range s.m {
		if time.Now().After(v.expires) {
			delete(s.m, k)
		}
	}
	s.m[token] = st
	return token
}

func (s *oauthStateStore) take(token string) (oauthState, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	st, ok := s.m[token]
	delete(s.m, token)
	if ok && time.Now().After(st.expires) {
		return oauthState{}, false
	}
	return st, ok
}

// --- helpers ---

func normalizeKind(t string) (gitprovider.Kind, bool) {
	switch gitprovider.Kind(t) {
	case gitprovider.GitHub:
		return gitprovider.GitHub, true
	case gitprovider.GitLab:
		return gitprovider.GitLab, true
	}
	return "", false
}

func (s *Server) oauthConfig(kind gitprovider.Kind, baseURL string) gitprovider.OAuthConfig {
	switch kind {
	case gitprovider.GitLab:
		return gitprovider.OAuthConfig{ClientID: s.Cfg.GitLabClientID, ClientSecret: s.Cfg.GitLabClientSecret, BaseURL: baseURL}
	default:
		return gitprovider.OAuthConfig{ClientID: s.Cfg.GitHubClientID, ClientSecret: s.Cfg.GitHubClientSecret, BaseURL: baseURL}
	}
}

// publicBase returns the externally reachable base URL for building redirect
// URIs, preferring the configured PublicURL and falling back to the request.
func (s *Server) publicBase(r *http.Request) string {
	if s.Cfg.PublicURL != "" {
		return s.Cfg.PublicURL
	}
	scheme := "http"
	if r.TLS != nil || r.Header.Get("X-Forwarded-Proto") == "https" {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func (s *Server) oauthRedirectURI(r *http.Request, kind gitprovider.Kind) string {
	return s.publicBase(r) + "/api/v1/git-providers/oauth/" + string(kind) + "/callback"
}

// --- CRUD ---

func (s *Server) handleListGitProviders(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgAccess(w, r, orgID) == nil {
		return
	}
	gs, err := s.Store.GitProvidersForOrg(orgID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"providers": gs})
}

// handleCreateGitProvider connects a provider via a personal access token. The
// token is validated by fetching the authenticated account before storing.
func (s *Server) handleCreateGitProvider(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	if s.requireOrgRole(w, r, orgID, store.RoleOwner, store.RoleAdmin) == nil {
		return
	}
	var req struct {
		Type    string `json:"type"`
		Name    string `json:"name"`
		Token   string `json:"token"`
		BaseURL string `json:"baseUrl"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	kind, ok := normalizeKind(req.Type)
	if !ok {
		writeError(w, http.StatusBadRequest, "type must be github or gitlab")
		return
	}
	if req.Token == "" {
		writeError(w, http.StatusBadRequest, "token required")
		return
	}
	login, err := gitprovider.New(kind, req.Token, req.BaseURL).Login(r.Context())
	if err != nil {
		writeError(w, http.StatusBadRequest, "could not validate token: "+err.Error())
		return
	}
	gp := &store.GitProvider{
		OrganizationID: orgID, Type: string(kind), Name: req.Name,
		AccountLogin: login, BaseURL: req.BaseURL, AuthKind: "pat", Token: req.Token,
	}
	if err := s.Store.CreateGitProvider(gp); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, gp)
}

func (s *Server) handleDeleteGitProvider(w http.ResponseWriter, r *http.Request) {
	gp := s.loadGitProviderWithAccess(w, r)
	if gp == nil {
		return
	}
	if s.requireOrgRole(w, r, gp.OrganizationID, store.RoleOwner, store.RoleAdmin) == nil {
		return
	}
	if err := s.Store.DeleteGitProvider(gp.ID); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

// handleListGitProviderRepos lists repositories the connected provider's token
// can access, for the repo picker.
func (s *Server) handleListGitProviderRepos(w http.ResponseWriter, r *http.Request) {
	gp := s.loadGitProviderWithAccess(w, r)
	if gp == nil {
		return
	}
	repos, err := gitprovider.New(gitprovider.Kind(gp.Type), gp.Token, gp.BaseURL).ListRepos(r.Context())
	if err != nil {
		writeError(w, http.StatusBadGateway, "could not list repositories: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"repositories": repos})
}

// loadGitProviderWithAccess loads {providerID} and verifies org membership.
func (s *Server) loadGitProviderWithAccess(w http.ResponseWriter, r *http.Request) *store.GitProvider {
	gp, err := s.Store.GitProviderByID(chi.URLParam(r, "providerID"))
	if err != nil {
		writeError(w, http.StatusNotFound, "git provider not found")
		return nil
	}
	if s.requireOrgAccess(w, r, gp.OrganizationID) == nil {
		return nil
	}
	return gp
}

// --- OAuth flow ---

// handleGitOAuthStart returns the provider authorize URL for the org, or 501 if
// OAuth credentials are not configured (use the PAT connect flow instead).
func (s *Server) handleGitOAuthStart(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	m := s.requireOrgRole(w, r, orgID, store.RoleOwner, store.RoleAdmin)
	if m == nil {
		return
	}
	kind, ok := normalizeKind(chi.URLParam(r, "type"))
	if !ok {
		writeError(w, http.StatusBadRequest, "type must be github or gitlab")
		return
	}
	baseURL := r.URL.Query().Get("baseUrl")
	oc := s.oauthConfig(kind, baseURL)
	if !oc.Configured() {
		writeError(w, http.StatusNotImplemented, "OAuth is not configured for this provider; connect with a personal access token instead")
		return
	}
	state := s.oauthStates.put(oauthState{
		orgID: orgID, kind: kind, name: r.URL.Query().Get("name"),
		baseURL: baseURL, userID: m.UserID, expires: time.Now().Add(10 * time.Minute),
	})
	scope := "repo"
	if kind == gitprovider.GitLab {
		scope = "read_api read_repository"
	}
	url := oc.AuthorizeURL(kind, s.oauthRedirectURI(r, kind), state, scope)
	writeJSON(w, http.StatusOK, map[string]string{"authorizeUrl": url})
}

// handleGitOAuthCallback is the public redirect target GitHub/GitLab send the
// user back to. It exchanges the code for a token and stores the provider.
func (s *Server) handleGitOAuthCallback(w http.ResponseWriter, r *http.Request) {
	kind, ok := normalizeKind(chi.URLParam(r, "type"))
	if !ok {
		http.Error(w, "unknown provider", http.StatusBadRequest)
		return
	}
	code := r.URL.Query().Get("code")
	st, ok := s.oauthStates.take(r.URL.Query().Get("state"))
	if code == "" || !ok || st.kind != kind {
		http.Error(w, "invalid or expired OAuth state", http.StatusBadRequest)
		return
	}
	oc := s.oauthConfig(kind, st.baseURL)
	token, err := oc.ExchangeCode(r.Context(), kind, code, s.oauthRedirectURI(r, kind))
	if err != nil {
		http.Error(w, "OAuth exchange failed: "+err.Error(), http.StatusBadGateway)
		return
	}
	login, err := gitprovider.New(kind, token, st.baseURL).Login(r.Context())
	if err != nil {
		http.Error(w, "could not read account: "+err.Error(), http.StatusBadGateway)
		return
	}
	name := st.name
	if name == "" {
		name = login
	}
	gp := &store.GitProvider{
		OrganizationID: st.orgID, Type: string(kind), Name: name,
		AccountLogin: login, BaseURL: st.baseURL, AuthKind: "oauth", Token: token,
	}
	if err := s.Store.CreateGitProvider(gp); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	// Redirect the browser back into the SPA settings page.
	http.Redirect(w, r, s.publicBase(r)+"/settings?git=connected", http.StatusFound)
}
