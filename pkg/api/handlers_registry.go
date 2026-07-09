package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	"github.com/orcinustools/kapibara/pkg/auth"
	"github.com/orcinustools/kapibara/pkg/store"
)

// The registry gateway turns Kapibara's public HTTPS endpoint into a Docker
// Registry v2 front for the in-cluster registry, using the standard Docker
// token-auth flow: anonymous pull, authenticated push.
//
//	1. A client hits /v2/* with no token → 401 + `WWW-Authenticate: Bearer
//	   realm=".../v2/token",service="kapibara"`.
//	2. The client fetches a token from /v2/token. A push scope requires Kapibara
//	   Basic credentials (email + password or a kap_ API token); a pull-only
//	   scope is granted anonymously.
//	3. The client retries with `Authorization: Bearer <token>`. The gateway
//	   verifies the (HMAC-signed) token, enforces push-vs-pull, and proxies to
//	   the upstream registry (which is itself open in-cluster).
//
// Enabled only when KAPIBARA_REGISTRY_UPSTREAM is set.

type regClaims struct {
	Push bool `json:"push"`
	// Scope is the org slug used to namespace pushes: a request path under
	// /v2/registry/<x> is rewritten to /v2/registry/<scope>/<x> (idempotent).
	Scope string `json:"scope,omitempty"`
	Exp   int64  `json:"exp"`
}

func (s *Server) registryHandler() http.Handler {
	upstream := strings.TrimSpace(s.Cfg.RegistryUpstream)
	if upstream == "" {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusNotImplemented, "registry gateway not configured (set KAPIBARA_REGISTRY_UPSTREAM)")
		})
	}
	target, err := url.Parse(upstream)
	if err != nil {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			writeError(w, http.StatusInternalServerError, "invalid registry upstream: "+err.Error())
		})
	}
	proxy := httputil.NewSingleHostReverseProxy(target)
	proxy.FlushInterval = 200 * time.Millisecond
	base := proxy.Director
	proxy.Director = func(req *http.Request) {
		base(req)
		req.Host = target.Host
		// The upstream is open; never forward the client's gateway credentials.
		req.Header.Del("Authorization")
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v2/token" {
			s.registryToken(w, r)
			return
		}
		claims, ok := s.verifyRegistryToken(bearerToken(r))
		if !ok {
			s.registryChallenge(w, r)
			return
		}
		if registryIsWrite(r.Method) && !claims.Push {
			s.registryChallenge(w, r)
			return
		}
		// Namespace the caller's repositories by their org: a path under
		// /v2/registry/<x> becomes /v2/registry/<scope>/<x> (idempotent). Pulls
		// carry the already-scoped path in the manifest, so only tokens that
		// carry a scope (pushers) trigger the rewrite.
		if claims.Scope != "" {
			r.URL.Path = scopeRegistryPath(r.URL.Path, claims.Scope)
		}
		proxy.ServeHTTP(w, r)
	})
}

// scopeRegistryPath inserts the org scope after "registry/" in a /v2 repository
// path, unless it is already present. e.g.
//
//	/v2/registry/worker/api/manifests/1        → /v2/registry/<scope>/worker/api/manifests/1
//	/v2/registry/<scope>/worker/api/blobs/...   → unchanged
func scopeRegistryPath(path, scope string) string {
	const pfx = "/v2/registry/"
	if scope == "" || !strings.HasPrefix(path, pfx) {
		return path
	}
	rest := strings.TrimPrefix(path, pfx)
	if strings.HasPrefix(rest, scope+"/") {
		return path // already scoped
	}
	return pfx + scope + "/" + rest
}

func registryIsWrite(method string) bool {
	switch method {
	case http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete:
		return true
	}
	return false
}

// registryChallenge sends the Bearer auth challenge pointing at the token endpoint.
func (s *Server) registryChallenge(w http.ResponseWriter, r *http.Request) {
	realm := s.publicBase(r) + "/v2/token"
	w.Header().Set("WWW-Authenticate", `Bearer realm="`+realm+`",service="kapibara"`)
	writeError(w, http.StatusUnauthorized, "authentication required")
}

// registryToken issues a signed access token. Push scopes require valid
// Kapibara credentials; pull-only scopes are granted anonymously.
func (s *Server) registryToken(w http.ResponseWriter, r *http.Request) {
	actions := ""
	if scope := r.URL.Query().Get("scope"); scope != "" {
		if parts := strings.Split(scope, ":"); len(parts) >= 3 {
			actions = parts[len(parts)-1]
		}
	}
	wantPush := strings.Contains(actions, "push") || strings.Contains(actions, "*")

	authed := false
	scope := ""
	if email, secret, ok := r.BasicAuth(); ok && email != "" && secret != "" {
		u := s.registryUser(email, secret)
		if u == nil {
			w.Header().Set("WWW-Authenticate", `Basic realm="kapibara"`)
			writeError(w, http.StatusUnauthorized, "invalid credentials")
			return
		}
		authed = true
		scope = s.registryScope(u) // org slug → namespaces this user's pushes
	}
	if wantPush && !authed {
		s.registryChallenge(w, r)
		return
	}
	claims := regClaims{Push: authed && wantPush, Exp: time.Now().Add(5 * time.Minute).Unix()}
	if claims.Push {
		claims.Scope = scope
	}
	tok := s.signRegistryToken(claims)
	writeJSON(w, http.StatusOK, map[string]any{
		"token":        tok,
		"access_token": tok,
		"expires_in":   300,
	})
}

// registryUser validates Docker Basic credentials against a Kapibara account
// (password or a kap_ API token) and returns the user, or nil if invalid.
func (s *Server) registryUser(email, secret string) *store.User {
	if strings.HasPrefix(secret, "kap_") {
		u, err := s.Store.UserByAPITokenHash(auth.HashToken(secret))
		if err == nil && u != nil && strings.EqualFold(u.Email, email) {
			return u
		}
		return nil
	}
	u, err := s.Store.UserByEmail(email)
	if err != nil || !auth.CheckPassword(u.PasswordHash, secret) {
		return nil
	}
	return u
}

// registryScope returns the org slug used to namespace a user's registry
// pushes — their primary (first) organization.
func (s *Server) registryScope(u *store.User) string {
	orgs, err := s.Store.OrgsForUser(u.ID)
	if err != nil || len(orgs) == 0 {
		return ""
	}
	return orgs[0].Slug
}

// signRegistryToken returns a compact HMAC-signed token: base64(payload).base64(sig).
func (s *Server) signRegistryToken(c regClaims) string {
	payload, _ := json.Marshal(c)
	b := base64.RawURLEncoding.EncodeToString(payload)
	mac := hmac.New(sha256.New, s.registrySignKey())
	mac.Write([]byte(b))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return b + "." + sig
}

func (s *Server) verifyRegistryToken(tok string) (regClaims, bool) {
	var c regClaims
	if tok == "" {
		return c, false
	}
	i := strings.LastIndexByte(tok, '.')
	if i <= 0 {
		return c, false
	}
	b, sig := tok[:i], tok[i+1:]
	mac := hmac.New(sha256.New, s.registrySignKey())
	mac.Write([]byte(b))
	want := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	if subtle.ConstantTimeCompare([]byte(sig), []byte(want)) != 1 {
		return c, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(b)
	if err != nil || json.Unmarshal(payload, &c) != nil {
		return c, false
	}
	if time.Now().Unix() > c.Exp {
		return c, false
	}
	return c, true
}

func (s *Server) registrySignKey() []byte {
	if s.Cfg.JWTSecret != "" {
		return []byte("registry:" + s.Cfg.JWTSecret)
	}
	return []byte("kapibara-registry-dev-key")
}
