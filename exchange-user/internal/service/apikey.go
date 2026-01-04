// Package service API Key signature verification.
package service

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"time"

	"github.com/exchange/common/pkg/signature"
	"github.com/exchange/user/internal/repository"
)

const (
	apiKeyStatusEnabled = 1
)

var (
	ErrInvalidTimestamp = errors.New("invalid timestamp")
	ErrInvalidSignature = errors.New("invalid signature")
	ErrAPIKeyDisabled   = errors.New("api key disabled")
)

type SignaturePayload struct {
	Method   string
	Path     string
	Nonce    string
	Query    url.Values
	BodyHash string
	Body     []byte
}

type APIKeyRepository interface {
	GetApiKeyByKey(ctx context.Context, key string) (*repository.ApiKey, error)
}

type APIKeyService struct {
	repo APIKeyRepository
	now  func() time.Time
}

func NewAPIKeyService(repo APIKeyRepository) *APIKeyService {
	return &APIKeyService{
		repo: repo,
		now:  time.Now,
	}
}

func (s *APIKeyService) VerifySignature(apiKey string, timestamp int64, signature string, payload SignaturePayload) (int64, error) {
	nowMs := s.now().UnixMilli()
	diff := nowMs - timestamp
	if diff < 0 {
		diff = -diff
	}
	if diff > int64(30*time.Second/time.Millisecond) {
		return 0, ErrInvalidTimestamp
	}

	key, err := s.repo.GetApiKeyByKey(context.Background(), apiKey)
	if err != nil {
		return 0, err
	}
	if key.Status != apiKeyStatusEnabled {
		return 0, ErrAPIKeyDisabled
	}

	canonical := buildSignaturePayload(timestamp, payload)
	expectedSig := signWithSecret(key.SecretHash, canonical)
	if !hmac.Equal([]byte(signature), []byte(expectedSig)) {
		return 0, ErrInvalidSignature
	}

	return key.UserID, nil
}

func buildSignaturePayload(timestamp int64, payload SignaturePayload) string {
	bodyHash := payload.BodyHash
	if bodyHash == "" && len(payload.Body) > 0 {
		bodyHash = computeBodyHash(payload.Body)
	}
	return signature.BuildCanonicalStringWithBodyHash(timestamp, payload.Nonce, payload.Method, payload.Path, payload.Query, bodyHash)
}

func signWithSecret(secret, data string) string {
	h := hmac.New(sha256.New, []byte(secret))
	h.Write([]byte(data))
	return hex.EncodeToString(h.Sum(nil))
}

func computeBodyHash(body []byte) string {
	if len(body) == 0 {
		return ""
	}
	sum := sha256.Sum256(body)
	return hex.EncodeToString(sum[:])
}
