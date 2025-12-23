package service

import (
	"context"
	"errors"
	"net/url"
	"testing"
	"time"

	"github.com/exchange/user/internal/repository"
)

type mockRepo struct {
	createUserFn     func(ctx context.Context, user *repository.User, password string) error
	getUserByEmailFn func(ctx context.Context, email string) (*repository.User, error)
	verifyPasswordFn func(user *repository.User, password string) bool
	createApiKeyFn   func(ctx context.Context, apiKey *repository.ApiKey) (string, error)
	listApiKeysFn    func(ctx context.Context, userID int64) ([]*repository.ApiKey, error)
	deleteApiKeyFn   func(ctx context.Context, userID, apiKeyID int64) error
	getApiKeyByKeyFn func(ctx context.Context, key string) (*repository.ApiKey, error)
	getUserByIDFn    func(ctx context.Context, userID int64) (*repository.User, error)
}

func (m *mockRepo) CreateUser(ctx context.Context, user *repository.User, password string) error {
	if m.createUserFn != nil {
		return m.createUserFn(ctx, user, password)
	}
	return nil
}

func (m *mockRepo) GetUserByEmail(ctx context.Context, email string) (*repository.User, error) {
	if m.getUserByEmailFn != nil {
		return m.getUserByEmailFn(ctx, email)
	}
	return nil, repository.ErrUserNotFound
}

func (m *mockRepo) VerifyPassword(user *repository.User, password string) bool {
	if m.verifyPasswordFn != nil {
		return m.verifyPasswordFn(user, password)
	}
	return false
}

func (m *mockRepo) CreateApiKey(ctx context.Context, apiKey *repository.ApiKey) (string, error) {
	if m.createApiKeyFn != nil {
		return m.createApiKeyFn(ctx, apiKey)
	}
	return "", nil
}

func (m *mockRepo) ListApiKeys(ctx context.Context, userID int64) ([]*repository.ApiKey, error) {
	if m.listApiKeysFn != nil {
		return m.listApiKeysFn(ctx, userID)
	}
	return nil, nil
}

func (m *mockRepo) DeleteApiKey(ctx context.Context, userID, apiKeyID int64) error {
	if m.deleteApiKeyFn != nil {
		return m.deleteApiKeyFn(ctx, userID, apiKeyID)
	}
	return nil
}

func (m *mockRepo) GetApiKeyByKey(ctx context.Context, key string) (*repository.ApiKey, error) {
	if m.getApiKeyByKeyFn != nil {
		return m.getApiKeyByKeyFn(ctx, key)
	}
	return nil, repository.ErrApiKeyNotFound
}

func (m *mockRepo) GetUserByID(ctx context.Context, userID int64) (*repository.User, error) {
	if m.getUserByIDFn != nil {
		return m.getUserByIDFn(ctx, userID)
	}
	return nil, repository.ErrUserNotFound
}

func TestTOTPAPIKeyVerifySignatureSuccess(t *testing.T) {
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	repo := &mockRepo{
		getApiKeyByKeyFn: func(ctx context.Context, key string) (*repository.ApiKey, error) {
			return &repository.ApiKey{
				UserID:     99,
				SecretHash: "secret-key",
				Status:     apiKeyStatusEnabled,
			}, nil
		},
	}
	svc := NewAPIKeyService(repo)
	svc.now = func() time.Time { return now }

	payload := SignaturePayload{
		Method: "POST",
		Path:   "/orders",
		Nonce:  "nonce-1",
		Query:  url.Values{},
	}
	timestamp := now.UnixMilli()
	sig := signWithSecret("secret-key", buildSignaturePayload(timestamp, payload))

	userID, err := svc.VerifySignature("api-key", timestamp, sig, payload)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if userID != 99 {
		t.Fatalf("expected userID=99, got %d", userID)
	}
}

func TestTOTPAPIKeyVerifySignatureInvalidTimestamp(t *testing.T) {
	repo := &mockRepo{}
	svc := NewAPIKeyService(repo)
	svc.now = func() time.Time { return time.Unix(0, 0) }

	_, err := svc.VerifySignature("api-key", time.Now().UnixMilli(), "sig", SignaturePayload{})
	if !errors.Is(err, ErrInvalidTimestamp) {
		t.Fatalf("expected ErrInvalidTimestamp, got %v", err)
	}
}

func TestTOTPAPIKeyVerifySignatureRepoError(t *testing.T) {
	repo := &mockRepo{
		getApiKeyByKeyFn: func(ctx context.Context, key string) (*repository.ApiKey, error) {
			return nil, repository.ErrApiKeyNotFound
		},
	}
	svc := NewAPIKeyService(repo)
	svc.now = func() time.Time { return time.UnixMilli(1000) }

	_, err := svc.VerifySignature("api-key", 1000, "sig", SignaturePayload{})
	if !errors.Is(err, repository.ErrApiKeyNotFound) {
		t.Fatalf("expected ErrApiKeyNotFound, got %v", err)
	}
}

func TestTOTPAPIKeyVerifySignatureDisabled(t *testing.T) {
	repo := &mockRepo{
		getApiKeyByKeyFn: func(ctx context.Context, key string) (*repository.ApiKey, error) {
			return &repository.ApiKey{
				UserID:     1,
				SecretHash: "secret-key",
				Status:     2,
			}, nil
		},
	}
	svc := NewAPIKeyService(repo)
	svc.now = func() time.Time { return time.UnixMilli(1000) }

	_, err := svc.VerifySignature("api-key", 1000, "sig", SignaturePayload{})
	if !errors.Is(err, ErrAPIKeyDisabled) {
		t.Fatalf("expected ErrAPIKeyDisabled, got %v", err)
	}
}

func TestTOTPAPIKeyVerifySignatureInvalidSignature(t *testing.T) {
	repo := &mockRepo{
		getApiKeyByKeyFn: func(ctx context.Context, key string) (*repository.ApiKey, error) {
			return &repository.ApiKey{
				UserID:     1,
				SecretHash: "secret-key",
				Status:     apiKeyStatusEnabled,
			}, nil
		},
	}
	svc := NewAPIKeyService(repo)
	svc.now = func() time.Time { return time.UnixMilli(1000) }

	_, err := svc.VerifySignature("api-key", 1000, "bad", SignaturePayload{
		Method: "GET",
		Path:   "/orders",
		Nonce:  "nonce-1",
		Query:  url.Values{},
	})
	if !errors.Is(err, ErrInvalidSignature) {
		t.Fatalf("expected ErrInvalidSignature, got %v", err)
	}
}

func TestTOTPUserServiceRegisterSuccess(t *testing.T) {
	repo := &mockRepo{}
	gen := &mockIDGen{}
	svc := NewUserService(repo, gen, &stubTokenIssuer{token: "token_test"})

	resp, err := svc.Register(context.Background(), &RegisterRequest{
		Email:    "user@example.com",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.User == nil || resp.User.UserID == 0 {
		t.Fatal("expected user to be created")
	}
}

func TestTOTPUserServiceRegisterEmailExists(t *testing.T) {
	repo := &mockRepo{
		createUserFn: func(ctx context.Context, user *repository.User, password string) error {
			return repository.ErrEmailExists
		},
	}
	svc := NewUserService(repo, &mockIDGen{}, &stubTokenIssuer{token: "token_test"})

	resp, err := svc.Register(context.Background(), &RegisterRequest{
		Email:    "user@example.com",
		Password: "pass",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.ErrorCode != "EMAIL_EXISTS" {
		t.Fatalf("expected EMAIL_EXISTS, got %s", resp.ErrorCode)
	}
}

func TestTOTPUserServiceRegisterError(t *testing.T) {
	repo := &mockRepo{
		createUserFn: func(ctx context.Context, user *repository.User, password string) error {
			return errors.New("boom")
		},
	}
	svc := NewUserService(repo, &mockIDGen{}, &stubTokenIssuer{token: "token_test"})

	_, err := svc.Register(context.Background(), &RegisterRequest{
		Email:    "user@example.com",
		Password: "pass",
	})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestTOTPUserServiceLoginScenarios(t *testing.T) {
	user := &repository.User{
		UserID: 1,
		Status: repository.UserStatusActive,
	}
	repo := &mockRepo{
		getUserByEmailFn: func(ctx context.Context, email string) (*repository.User, error) {
			if email == "missing@example.com" {
				return nil, repository.ErrUserNotFound
			}
			return user, nil
		},
		verifyPasswordFn: func(user *repository.User, password string) bool {
			return password == "ok"
		},
	}
	svc := NewUserService(repo, &mockIDGen{}, &stubTokenIssuer{token: "token_test"})

	resp, err := svc.Login(context.Background(), &LoginRequest{Email: "missing@example.com", Password: "ok"})
	if err != nil || resp.ErrorCode != "INVALID_CREDENTIALS" {
		t.Fatalf("expected INVALID_CREDENTIALS, got err=%v code=%s", err, resp.ErrorCode)
	}

	resp, err = svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "bad"})
	if err != nil || resp.ErrorCode != "INVALID_CREDENTIALS" {
		t.Fatalf("expected INVALID_CREDENTIALS, got err=%v code=%s", err, resp.ErrorCode)
	}

	user.Status = repository.UserStatusFrozen
	resp, err = svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "ok"})
	if err != nil || resp.ErrorCode != "USER_FROZEN" {
		t.Fatalf("expected USER_FROZEN, got err=%v code=%s", err, resp.ErrorCode)
	}

	user.Status = repository.UserStatusActive
	resp, err = svc.Login(context.Background(), &LoginRequest{Email: "user@example.com", Password: "ok"})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Token == "" {
		t.Fatal("expected token to be set")
	}
}

func TestTOTPUserServiceApiKeyOps(t *testing.T) {
	repo := &mockRepo{
		createApiKeyFn: func(ctx context.Context, apiKey *repository.ApiKey) (string, error) {
			return "secret", nil
		},
		listApiKeysFn: func(ctx context.Context, userID int64) ([]*repository.ApiKey, error) {
			return []*repository.ApiKey{{ApiKeyID: 1}}, nil
		},
		deleteApiKeyFn: func(ctx context.Context, userID, apiKeyID int64) error {
			return nil
		},
		getApiKeyByKeyFn: func(ctx context.Context, key string) (*repository.ApiKey, error) {
			return &repository.ApiKey{
				UserID:      7,
				SecretHash:  "secret",
				Permissions: 3,
				Status:      apiKeyStatusEnabled,
			}, nil
		},
		getUserByIDFn: func(ctx context.Context, userID int64) (*repository.User, error) {
			return &repository.User{UserID: userID}, nil
		},
	}
	svc := NewUserService(repo, &mockIDGen{}, &stubTokenIssuer{token: "token_test"})

	resp, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{
		UserID:      1,
		Label:       "test",
		Permissions: 7,
	})
	if err != nil || resp.Secret != "secret" {
		t.Fatalf("expected secret, got err=%v secret=%s", err, resp.Secret)
	}

	keys, err := svc.ListApiKeys(context.Background(), 1)
	if err != nil || len(keys) != 1 {
		t.Fatalf("expected keys, got err=%v len=%d", err, len(keys))
	}

	if err := svc.DeleteApiKey(context.Background(), 1, 2); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	secret, userID, perms, err := svc.GetApiKeyInfo(context.Background(), "key")
	if err != nil || secret == "" || userID != 7 || perms != 3 {
		t.Fatalf("unexpected api key info: err=%v secret=%s userID=%d perms=%d", err, secret, userID, perms)
	}

	user, err := svc.GetUser(context.Background(), 9)
	if err != nil || user.UserID != 9 {
		t.Fatalf("expected userID=9, got err=%v userID=%d", err, user.UserID)
	}
}

func TestTOTPUserServiceApiKeyErrors(t *testing.T) {
	repo := &mockRepo{
		createApiKeyFn: func(ctx context.Context, apiKey *repository.ApiKey) (string, error) {
			return "", errors.New("bad")
		},
		listApiKeysFn: func(ctx context.Context, userID int64) ([]*repository.ApiKey, error) {
			return nil, errors.New("bad")
		},
		deleteApiKeyFn: func(ctx context.Context, userID, apiKeyID int64) error {
			return errors.New("bad")
		},
		getApiKeyByKeyFn: func(ctx context.Context, key string) (*repository.ApiKey, error) {
			return nil, errors.New("bad")
		},
		getUserByIDFn: func(ctx context.Context, userID int64) (*repository.User, error) {
			return nil, errors.New("bad")
		},
	}
	svc := NewUserService(repo, &mockIDGen{}, &stubTokenIssuer{token: "token_test"})

	if _, err := svc.CreateApiKey(context.Background(), &CreateApiKeyRequest{UserID: 1}); err == nil {
		t.Fatal("expected create api key error")
	}

	if _, err := svc.ListApiKeys(context.Background(), 1); err == nil {
		t.Fatal("expected list api keys error")
	}

	if err := svc.DeleteApiKey(context.Background(), 1, 1); err == nil {
		t.Fatal("expected delete api key error")
	}

	if _, _, _, err := svc.GetApiKeyInfo(context.Background(), "key"); err == nil {
		t.Fatal("expected get api key info error")
	}

	if _, err := svc.GetUser(context.Background(), 1); err == nil {
		t.Fatal("expected get user error")
	}
}
