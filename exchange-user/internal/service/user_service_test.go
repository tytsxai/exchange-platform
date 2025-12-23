package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/exchange/user/internal/repository"
	"golang.org/x/crypto/bcrypt"
)

type stubIDGen struct {
	next int64
}

func (s *stubIDGen) NextID() int64 {
	s.next++
	return s.next
}

func TestTOTPUserService_Register_Login_ApiKey(t *testing.T) {
	ctx := context.Background()

	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock.New error: %v", err)
	}
	defer db.Close()

	repo := repository.NewUserRepository(db)
	idGen := &stubIDGen{next: 100}
	tokenIssuer := &stubTokenIssuer{token: "token_test"}
	svc := NewUserService(repo, idGen, tokenIssuer)

	mock.ExpectExec("INSERT INTO exchange_user.users").
		WithArgs(int64(101), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), repository.UserStatusActive, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	regResp, err := svc.Register(ctx, &RegisterRequest{Email: "a@b.com", Password: "pw"})
	if err != nil {
		t.Fatalf("Register error: %v", err)
	}
	if regResp.ErrorCode != "" || regResp.User == nil {
		t.Fatalf("unexpected Register response: %+v", regResp)
	}

	mock.ExpectExec("INSERT INTO exchange_user.users").
		WithArgs(int64(102), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), repository.UserStatusActive, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(errors.New("duplicate key value violates unique constraint"))

	regResp, err = svc.Register(ctx, &RegisterRequest{Email: "dup@b.com", Password: "pw"})
	if err != nil || regResp.ErrorCode != "EMAIL_EXISTS" {
		t.Fatalf("expected EMAIL_EXISTS, got resp=%+v err=%v", regResp, err)
	}

	mock.ExpectExec("INSERT INTO exchange_user.users").
		WithArgs(int64(103), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), repository.UserStatusActive, 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnError(sql.ErrConnDone)

	if _, err := svc.Register(ctx, &RegisterRequest{Email: "err@b.com", Password: "pw"}); err == nil {
		t.Fatal("expected Register error")
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("good-pass"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("GenerateFromPassword error: %v", err)
	}

	mock.ExpectQuery("FROM exchange_user.users").
		WithArgs("missing@b.com").
		WillReturnError(sql.ErrNoRows)

	loginResp, err := svc.Login(ctx, &LoginRequest{Email: "missing@b.com", Password: "x"})
	if err != nil || loginResp.ErrorCode != "INVALID_CREDENTIALS" {
		t.Fatalf("expected INVALID_CREDENTIALS, got resp=%+v err=%v", loginResp, err)
	}

	rows := sqlmock.NewRows([]string{
		"user_id", "email", "phone", "password_hash", "status", "kyc_status", "created_at_ms", "updated_at_ms",
	}).AddRow(int64(200), "user@b.com", nil, string(hash), repository.UserStatusActive, 1, int64(1), int64(1))

	mock.ExpectQuery("FROM exchange_user.users").
		WithArgs("user@b.com").
		WillReturnRows(rows)

	loginResp, err = svc.Login(ctx, &LoginRequest{Email: "user@b.com", Password: "bad-pass"})
	if err != nil || loginResp.ErrorCode != "INVALID_CREDENTIALS" {
		t.Fatalf("expected INVALID_CREDENTIALS, got resp=%+v err=%v", loginResp, err)
	}

	rows = sqlmock.NewRows([]string{
		"user_id", "email", "phone", "password_hash", "status", "kyc_status", "created_at_ms", "updated_at_ms",
	}).AddRow(int64(201), "user@b.com", nil, string(hash), repository.UserStatusFrozen, 1, int64(1), int64(1))

	mock.ExpectQuery("FROM exchange_user.users").
		WithArgs("user@b.com").
		WillReturnRows(rows)

	loginResp, err = svc.Login(ctx, &LoginRequest{Email: "user@b.com", Password: "good-pass"})
	if err != nil || loginResp.ErrorCode != "USER_FROZEN" {
		t.Fatalf("expected USER_FROZEN, got resp=%+v err=%v", loginResp, err)
	}

	rows = sqlmock.NewRows([]string{
		"user_id", "email", "phone", "password_hash", "status", "kyc_status", "created_at_ms", "updated_at_ms",
	}).AddRow(int64(65), "user@b.com", nil, string(hash), repository.UserStatusActive, 1, int64(1), int64(1))

	mock.ExpectQuery("FROM exchange_user.users").
		WithArgs("user@b.com").
		WillReturnRows(rows)

	loginResp, err = svc.Login(ctx, &LoginRequest{Email: "user@b.com", Password: "good-pass"})
	if err != nil || loginResp.Token != "token_test" {
		t.Fatalf("expected token_test, got resp=%+v err=%v", loginResp, err)
	}

	mock.ExpectExec("INSERT INTO exchange_user.api_keys").
		WithArgs(int64(104), int64(999), sqlmock.AnyArg(), sqlmock.AnyArg(), "label", 3, sqlmock.AnyArg(), 1, sqlmock.AnyArg(), sqlmock.AnyArg()).
		WillReturnResult(sqlmock.NewResult(1, 1))

	apiResp, err := svc.CreateApiKey(ctx, &CreateApiKeyRequest{
		UserID:      999,
		Label:       "label",
		Permissions: 3,
		IPWhitelist: []string{"127.0.0.1"},
	})
	if err != nil || apiResp.Secret == "" || apiResp.ApiKey == nil {
		t.Fatalf("CreateApiKey failed: resp=%+v err=%v", apiResp, err)
	}

	keyRows := sqlmock.NewRows([]string{
		"api_key_id", "user_id", "api_key", "secret_hash", "label", "permissions", "ip_whitelist", "status", "created_at_ms", "updated_at_ms",
	}).AddRow(int64(1), int64(999), "api_key", "hash", "label", 3, nil, 1, int64(1), int64(1))

	mock.ExpectQuery("FROM exchange_user.api_keys").
		WithArgs(int64(999)).
		WillReturnRows(keyRows)

	keys, err := svc.ListApiKeys(ctx, 999)
	if err != nil || len(keys) != 1 {
		t.Fatalf("ListApiKeys failed: keys=%v err=%v", keys, err)
	}

	mock.ExpectExec("UPDATE exchange_user.api_keys").
		WithArgs(int64(10), int64(999)).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := svc.DeleteApiKey(ctx, 999, 10); err != nil {
		t.Fatalf("DeleteApiKey error: %v", err)
	}

	mock.ExpectExec("UPDATE exchange_user.api_keys").
		WithArgs(int64(11), int64(999)).
		WillReturnResult(sqlmock.NewResult(1, 0))

	if err := svc.DeleteApiKey(ctx, 999, 11); err != repository.ErrApiKeyNotFound {
		t.Fatalf("expected ErrApiKeyNotFound, got %v", err)
	}

	keyRows = sqlmock.NewRows([]string{
		"api_key_id", "user_id", "api_key", "secret_hash", "label", "permissions", "ip_whitelist", "status", "created_at_ms", "updated_at_ms",
	}).AddRow(int64(2), int64(999), "api_key2", "hash2", "label2", 1, nil, 1, int64(1), int64(1))

	mock.ExpectQuery("FROM exchange_user.api_keys").
		WithArgs("api_key2").
		WillReturnRows(keyRows)

	secretHash, apiUserID, perms, err := svc.GetApiKeyInfo(ctx, "api_key2")
	if err != nil || secretHash != "hash2" || apiUserID != 999 || perms != 1 {
		t.Fatalf("GetApiKeyInfo failed: secret=%s user=%d perms=%d err=%v", secretHash, apiUserID, perms, err)
	}

	userRows := sqlmock.NewRows([]string{
		"user_id", "email", "phone", "password_hash", "status", "kyc_status", "created_at_ms", "updated_at_ms",
	}).AddRow(int64(300), "get@b.com", nil, string(hash), repository.UserStatusActive, 1, int64(1), int64(1))

	mock.ExpectQuery("FROM exchange_user.users").
		WithArgs(int64(300)).
		WillReturnRows(userRows)

	u, err := svc.GetUser(ctx, 300)
	if err != nil || u == nil || u.UserID != 300 {
		t.Fatalf("GetUser failed: user=%+v err=%v", u, err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("unmet expectations: %v", err)
	}
}
