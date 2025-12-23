// Package auth provides token issuance and verification helpers.
package auth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	tokenVersion = "v1"
	minSecretLen = 32
)

var (
	ErrMissingSecret    = errors.New("auth token secret is required")
	ErrSecretTooShort   = errors.New("auth token secret is too short")
	ErrInvalidTTL       = errors.New("auth token ttl must be positive")
	ErrInvalidToken     = errors.New("invalid auth token")
	ErrInvalidSignature = errors.New("invalid auth token signature")
	ErrTokenExpired     = errors.New("auth token expired")
	ErrInvalidUserID    = errors.New("invalid user id")
)

type tokenPayload struct {
	UserID    int64  `json:"uid"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
	Nonce     string `json:"nonce"`
}

// TokenManager issues and verifies signed tokens.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	clock  func() time.Time
}

// NewTokenManager creates a token manager with HMAC signing.
func NewTokenManager(secret string, ttl time.Duration) (*TokenManager, error) {
	if strings.TrimSpace(secret) == "" {
		return nil, ErrMissingSecret
	}
	if len(secret) < minSecretLen {
		return nil, ErrSecretTooShort
	}
	if ttl <= 0 {
		return nil, ErrInvalidTTL
	}

	return &TokenManager{
		secret: []byte(secret),
		ttl:    ttl,
		clock:  time.Now,
	}, nil
}

// Issue creates a signed token for the user.
func (m *TokenManager) Issue(userID int64) (string, error) {
	if userID <= 0 {
		return "", ErrInvalidUserID
	}

	now := m.clock().UTC()
	nonce, err := randomNonce(16)
	if err != nil {
		return "", fmt.Errorf("generate nonce: %w", err)
	}

	payload := tokenPayload{
		UserID:    userID,
		IssuedAt:  now.Unix(),
		ExpiresAt: now.Add(m.ttl).Unix(),
		Nonce:     nonce,
	}

	payloadBytes, err := json.Marshal(payload)
	if err != nil {
		return "", fmt.Errorf("marshal payload: %w", err)
	}

	encodedPayload := base64.RawURLEncoding.EncodeToString(payloadBytes)
	signature := signPayload(m.secret, encodedPayload)

	return strings.Join([]string{tokenVersion, encodedPayload, signature}, "."), nil
}

// Verify validates the token and returns the user id.
func (m *TokenManager) Verify(token string) (int64, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 || parts[0] != tokenVersion {
		return 0, ErrInvalidToken
	}

	payloadEncoded := parts[1]
	signature := parts[2]

	expectedSig := signPayload(m.secret, payloadEncoded)
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return 0, ErrInvalidSignature
	}

	payloadBytes, err := base64.RawURLEncoding.DecodeString(payloadEncoded)
	if err != nil {
		return 0, ErrInvalidToken
	}

	var payload tokenPayload
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return 0, ErrInvalidToken
	}

	if payload.UserID <= 0 {
		return 0, ErrInvalidToken
	}

	now := m.clock().UTC().Unix()
	if payload.ExpiresAt < now {
		return 0, ErrTokenExpired
	}

	return payload.UserID, nil
}

func signPayload(secret []byte, payload string) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(payload))
	return hex.EncodeToString(mac.Sum(nil))
}

func randomNonce(size int) (string, error) {
	buf := make([]byte, size)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
