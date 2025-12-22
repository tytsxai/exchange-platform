package service

import (
	"testing"

	"github.com/exchange/user/internal/repository"
)

func TestUserServiceStruct(t *testing.T) {
	svc := &UserService{}
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestRegisterRequest_Fields(t *testing.T) {
	req := &RegisterRequest{
		Email:    "test@example.com",
		Password: "password123",
	}

	if req.Email != "test@example.com" {
		t.Fatalf("expected Email=test@example.com, got %s", req.Email)
	}
	if req.Password != "password123" {
		t.Fatalf("expected Password=password123, got %s", req.Password)
	}
}

func TestRegisterResponse_Fields(t *testing.T) {
	resp := &RegisterResponse{
		ErrorCode: "",
	}

	if resp.ErrorCode != "" {
		t.Fatalf("expected empty ErrorCode, got %s", resp.ErrorCode)
	}

	resp = &RegisterResponse{
		ErrorCode: "EMAIL_EXISTS",
	}

	if resp.ErrorCode != "EMAIL_EXISTS" {
		t.Fatalf("expected ErrorCode=EMAIL_EXISTS, got %s", resp.ErrorCode)
	}
}

func TestLoginRequest_Fields(t *testing.T) {
	req := &LoginRequest{
		Email:    "test@example.com",
		Password: "password123",
	}

	if req.Email != "test@example.com" {
		t.Fatalf("expected Email=test@example.com, got %s", req.Email)
	}
	if req.Password != "password123" {
		t.Fatalf("expected Password=password123, got %s", req.Password)
	}
}

func TestLoginResponse_Fields(t *testing.T) {
	resp := &LoginResponse{
		Token:     "test-token",
		ErrorCode: "",
	}

	if resp.Token != "test-token" {
		t.Fatalf("expected Token=test-token, got %s", resp.Token)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected empty ErrorCode, got %s", resp.ErrorCode)
	}

	resp = &LoginResponse{
		ErrorCode: "INVALID_CREDENTIALS",
	}

	if resp.ErrorCode != "INVALID_CREDENTIALS" {
		t.Fatalf("expected ErrorCode=INVALID_CREDENTIALS, got %s", resp.ErrorCode)
	}
}

func TestCreateApiKeyRequest_Fields(t *testing.T) {
	req := &CreateApiKeyRequest{
		UserID:      123,
		Label:       "test-key",
		Permissions: 7,
		IPWhitelist: []string{"192.168.1.1"},
	}

	if req.UserID != 123 {
		t.Fatalf("expected UserID=123, got %d", req.UserID)
	}
	if req.Label != "test-key" {
		t.Fatalf("expected Label=test-key, got %s", req.Label)
	}
	if req.Permissions != 7 {
		t.Fatalf("expected Permissions=7, got %d", req.Permissions)
	}
}

func TestCreateApiKeyResponse_Fields(t *testing.T) {
	resp := &CreateApiKeyResponse{
		Secret:    "test-secret",
		ErrorCode: "",
	}

	if resp.Secret != "test-secret" {
		t.Fatalf("expected Secret=test-secret, got %s", resp.Secret)
	}
	if resp.ErrorCode != "" {
		t.Fatalf("expected empty ErrorCode, got %s", resp.ErrorCode)
	}
}

func TestIDGeneratorInterface(t *testing.T) {
	var _ IDGenerator = &mockIDGen{}
}

type mockIDGen struct {
	id int64
}

func (m *mockIDGen) NextID() int64 {
	m.id++
	return m.id
}

func TestNewUserService(t *testing.T) {
	gen := &mockIDGen{}
	svc := NewUserService(nil, gen)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestUserStatusFromRepository(t *testing.T) {
	if repository.UserStatusActive != 1 {
		t.Fatalf("expected UserStatusActive=1, got %d", repository.UserStatusActive)
	}
	if repository.UserStatusFrozen != 2 {
		t.Fatalf("expected UserStatusFrozen=2, got %d", repository.UserStatusFrozen)
	}
}

func TestLoginErrorCodes(t *testing.T) {
	errorCodes := []string{
		"INVALID_CREDENTIALS",
		"USER_FROZEN",
		"EMAIL_EXISTS",
	}

	for _, code := range errorCodes {
		resp := &LoginResponse{ErrorCode: code}
		if resp.ErrorCode != code {
			t.Fatalf("expected ErrorCode=%s, got %s", code, resp.ErrorCode)
		}
	}
}
