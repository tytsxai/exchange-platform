package repository

import (
	"testing"
)

func TestUserStruct(t *testing.T) {
	user := &User{
		UserID:       123,
		Email:        "test@example.com",
		Phone:        "13800138000",
		PasswordHash: "hashed-password",
		Status:       1,
		KycStatus:    1,
		CreatedAtMs:  1000,
		UpdatedAtMs:  2000,
	}

	if user.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", user.UserID)
	}
	if user.Email != "test@example.com" {
		t.Fatalf("expected Email=test@example.com, got %s", user.Email)
	}
	if user.Phone != "13800138000" {
		t.Fatalf("expected Phone=13800138000, got %s", user.Phone)
	}
	if user.Status != 1 {
		t.Fatalf("expected Status=1, got %d", user.Status)
	}
}

func TestApiKeyStruct(t *testing.T) {
	apiKey := &ApiKey{
		ApiKeyID:    456,
		UserID:      123,
		ApiKey:      "api-key-value",
		SecretHash:  "secret-hash",
		Label:       "test-label",
		Permissions: 7,
		Status:      1,
		CreatedAtMs: 1000,
		UpdatedAtMs: 2000,
	}

	if apiKey.ApiKeyID != 456 {
		t.Fatalf("expected ApiKeyID=456, got %d", apiKey.ApiKeyID)
	}
	if apiKey.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", apiKey.UserID)
	}
	if apiKey.ApiKey != "api-key-value" {
		t.Fatalf("expected ApiKey=api-key-value, got %s", apiKey.ApiKey)
	}
	if apiKey.Permissions != 7 {
		t.Fatalf("expected Permissions=7, got %d", apiKey.Permissions)
	}
}

func TestNewUserRepository(t *testing.T) {
	repo := NewUserRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestErrUserNotFound(t *testing.T) {
	if ErrUserNotFound == nil {
		t.Fatal("ErrUserNotFound should not be nil")
	}
	if ErrUserNotFound.Error() != "user not found" {
		t.Fatalf("expected 'user not found', got %s", ErrUserNotFound.Error())
	}
}

func TestErrEmailExists(t *testing.T) {
	if ErrEmailExists == nil {
		t.Fatal("ErrEmailExists should not be nil")
	}
	if ErrEmailExists.Error() != "email already exists" {
		t.Fatalf("expected 'email already exists', got %s", ErrEmailExists.Error())
	}
}

func TestErrApiKeyNotFound(t *testing.T) {
	if ErrApiKeyNotFound == nil {
		t.Fatal("ErrApiKeyNotFound should not be nil")
	}
	if ErrApiKeyNotFound.Error() != "api key not found" {
		t.Fatalf("expected 'api key not found', got %s", ErrApiKeyNotFound.Error())
	}
}

func TestUserStatusConstants(t *testing.T) {
	if UserStatusActive != 1 {
		t.Fatalf("expected UserStatusActive=1, got %d", UserStatusActive)
	}
	if UserStatusFrozen != 2 {
		t.Fatalf("expected UserStatusFrozen=2, got %d", UserStatusFrozen)
	}
	if UserStatusDisabled != 3 {
		t.Fatalf("expected UserStatusDisabled=3, got %d", UserStatusDisabled)
	}
}

func TestErrInvalidPassword(t *testing.T) {
	if ErrInvalidPassword == nil {
		t.Fatal("ErrInvalidPassword should not be nil")
	}
	if ErrInvalidPassword.Error() != "invalid password" {
		t.Fatalf("expected 'invalid password', got %s", ErrInvalidPassword.Error())
	}
}

func TestApiKeyIPWhitelist(t *testing.T) {
	apiKey := &ApiKey{
		IPWhitelist: []string{"192.168.1.1", "10.0.0.1"},
	}

	if len(apiKey.IPWhitelist) != 2 {
		t.Fatalf("expected 2 IPs, got %d", len(apiKey.IPWhitelist))
	}
	if apiKey.IPWhitelist[0] != "192.168.1.1" {
		t.Fatalf("expected first IP=192.168.1.1, got %s", apiKey.IPWhitelist[0])
	}
}
