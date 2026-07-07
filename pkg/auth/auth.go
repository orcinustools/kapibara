// Package auth handles password hashing, JWT session tokens, and API tokens.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/pquerna/otp/totp"
	"golang.org/x/crypto/bcrypt"
)

// ErrInvalidToken is returned when a session token fails validation.
var ErrInvalidToken = errors.New("invalid token")

// Manager issues and verifies session JWTs.
type Manager struct {
	secret []byte
	ttl    time.Duration
}

// NewManager returns a token manager. If secret is empty a random one is
// generated (sessions won't survive a restart — dev only).
func NewManager(secret string) *Manager {
	b := []byte(secret)
	if len(b) == 0 {
		b = make([]byte, 32)
		_, _ = rand.Read(b)
	}
	return &Manager{secret: b, ttl: 7 * 24 * time.Hour}
}

// Claims is the JWT payload for a kapibara session.
type Claims struct {
	UserID string `json:"uid"`
	Email  string `json:"email"`
	jwt.RegisteredClaims
}

// Issue creates a signed session token for a user.
func (m *Manager) Issue(userID, email string) (string, error) {
	now := time.Now()
	claims := Claims{
		UserID: userID,
		Email:  email,
		RegisteredClaims: jwt.RegisteredClaims{
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(m.ttl)),
			Subject:   userID,
		},
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return tok.SignedString(m.secret)
}

// Verify parses and validates a session token, returning its claims.
func (m *Manager) Verify(tokenStr string) (*Claims, error) {
	claims := &Claims{}
	tok, err := jwt.ParseWithClaims(tokenStr, claims, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return m.secret, nil
	})
	if err != nil || !tok.Valid {
		return nil, ErrInvalidToken
	}
	return claims, nil
}

// HashPassword returns a bcrypt hash of the password.
func HashPassword(password string) (string, error) {
	b, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(b), err
}

// CheckPassword reports whether password matches the bcrypt hash.
func CheckPassword(hash, password string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) == nil
}

// GenerateAPIToken returns a random API token (shown once) and its SHA-256
// hash (stored). The token is prefixed "kap_" for recognizability.
func GenerateAPIToken() (token, hash string) {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	token = "kap_" + hex.EncodeToString(b)
	return token, HashToken(token)
}

// HashToken returns the hex SHA-256 of an API token, for storage/lookup.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// GenerateTOTP creates a new TOTP secret for a user and returns the secret plus
// an otpauth:// URL for QR provisioning.
func GenerateTOTP(email string) (secret, otpauthURL string, err error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Kapibara",
		AccountName: email,
	})
	if err != nil {
		return "", "", err
	}
	return key.Secret(), key.URL(), nil
}

// ValidateTOTP reports whether code is valid for the secret at the current time.
func ValidateTOTP(secret, code string) bool {
	return totp.Validate(code, secret)
}
