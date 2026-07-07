package api

import (
	"encoding/json"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/orcinustools/kapibara/pkg/auth"
	"github.com/orcinustools/kapibara/pkg/store"
)

type registerReq struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Password string `json:"password"`
	OrgName  string `json:"orgName"`
}

type authResp struct {
	Token string      `json:"token"`
	User  *store.User `json:"user"`
}

// handleRegister creates a user. The first user becomes the platform admin and
// gets a default organization they own.
func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req registerReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if req.Email == "" || len(req.Password) < 8 {
		writeError(w, http.StatusBadRequest, "email required and password must be >= 8 chars")
		return
	}
	if _, err := s.Store.UserByEmail(req.Email); err == nil {
		writeError(w, http.StatusConflict, "email already registered")
		return
	}

	count, err := s.Store.CountUsers()
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	user := &store.User{
		Email:        req.Email,
		Name:         req.Name,
		PasswordHash: hash,
		IsAdmin:      count == 0, // first user is platform admin
	}
	if err := s.Store.CreateUser(user); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Give the new user a default organization they own.
	orgName := req.OrgName
	if orgName == "" {
		orgName = defaultOrgName(req.Name, req.Email)
	}
	org := &store.Organization{Name: orgName, Slug: uniqueSlug(s, slugify(orgName))}
	if err := s.Store.CreateOrg(org); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err := s.Store.CreateMembership(&store.Membership{
		UserID: user.ID, OrganizationID: org.ID, Role: store.RoleOwner,
	}); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	tok, err := s.Auth.Issue(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, authResp{Token: tok, User: user})
}

type loginReq struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	TOTPCode string `json:"totpCode"`
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req loginReq
	if !decodeJSON(w, r, &req) {
		return
	}
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	user, err := s.Store.UserByEmail(req.Email)
	if err != nil || !auth.CheckPassword(user.PasswordHash, req.Password) {
		writeError(w, http.StatusUnauthorized, "invalid credentials")
		return
	}
	// Second factor, when enabled.
	if user.TwoFAEnabled {
		if req.TOTPCode == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"error": "totp required", "totpRequired": true})
			return
		}
		if !auth.ValidateTOTP(user.TOTPSecret, req.TOTPCode) {
			writeError(w, http.StatusUnauthorized, "invalid totp code")
			return
		}
	}
	tok, err := s.Auth.Issue(user.ID, user.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, authResp{Token: tok, User: user})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, currentUser(r))
}

// --- 2FA (TOTP) ---

// handle2FAEnroll generates a TOTP secret (not yet enabled) and returns the
// otpauth URL for QR provisioning.
func (s *Server) handle2FAEnroll(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	secret, url, err := auth.GenerateTOTP(u.Email)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Store the pending secret; 2FA stays disabled until verified.
	u.TOTPSecret = secret
	u.TwoFAEnabled = false
	if err := s.Store.UpdateUser(u); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"secret": secret, "otpauthUrl": url})
}

// handle2FAVerify enables 2FA after confirming the user can generate valid codes.
func (s *Server) handle2FAVerify(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if u.TOTPSecret == "" || !auth.ValidateTOTP(u.TOTPSecret, req.Code) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	u.TwoFAEnabled = true
	if err := s.Store.UpdateUser(u); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"twoFAEnabled": true})
}

// handle2FADisable turns off 2FA (requires a valid current code).
func (s *Server) handle2FADisable(w http.ResponseWriter, r *http.Request) {
	u := currentUser(r)
	var req struct {
		Code string `json:"code"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	if !u.TwoFAEnabled || !auth.ValidateTOTP(u.TOTPSecret, req.Code) {
		writeError(w, http.StatusBadRequest, "invalid code")
		return
	}
	u.TwoFAEnabled = false
	u.TOTPSecret = ""
	if err := s.Store.UpdateUser(u); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"twoFAEnabled": false})
}

// --- API tokens ---

func (s *Server) handleListTokens(w http.ResponseWriter, r *http.Request) {
	toks, err := s.Store.APITokensForUser(currentUser(r).ID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tokens": toks})
}

func (s *Server) handleCreateToken(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	_ = decodeJSON(w, r, &req)
	token, hash := auth.GenerateAPIToken()
	rec := &store.ApiToken{UserID: currentUser(r).ID, Name: req.Name, TokenHash: hash}
	if err := s.Store.CreateAPIToken(rec); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	// The plaintext token is returned exactly once.
	writeJSON(w, http.StatusCreated, map[string]any{"token": token, "id": rec.ID, "name": rec.Name})
}

// --- helpers ---

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

// decodeJSONOptional decodes a JSON body if present, tolerating an empty body.
func decodeJSONOptional(r *http.Request, v any) error {
	if r.Body == nil {
		return nil
	}
	err := json.NewDecoder(r.Body).Decode(v)
	if err == io.EOF {
		return nil
	}
	return err
}

var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = slugRE.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		s = "org"
	}
	return s
}

// uniqueSlug appends a numeric suffix until the slug is free.
func uniqueSlug(s *Server, base string) string {
	slug := base
	for i := 2; ; i++ {
		var n int64
		s.Store.DB.Model(&store.Organization{}).Where("slug = ?", slug).Count(&n)
		if n == 0 {
			return slug
		}
		slug = base + "-" + strconv.Itoa(i)
	}
}

func defaultOrgName(name, email string) string {
	if name != "" {
		return name + "'s Org"
	}
	return strings.SplitN(email, "@", 2)[0] + "'s Org"
}
