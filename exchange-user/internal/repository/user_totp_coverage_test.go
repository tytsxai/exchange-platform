package repository

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"golang.org/x/crypto/bcrypt"
)

func TestTOTPUserCreateAndGet(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	user := &User{
		UserID:      1,
		Email:       "user@example.com",
		Phone:       "13800138000",
		Status:      UserStatusActive,
		KycStatus:   1,
		CreatedAtMs: 1000,
		UpdatedAtMs: 2000,
	}

	mock.ExpectExec("INSERT INTO exchange_user.users").
		WithArgs(user.UserID, sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
			user.Status, user.KycStatus, user.CreatedAtMs, user.UpdatedAtMs).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.CreateUser(context.Background(), user, "password123"); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if user.PasswordHash == "" {
		t.Fatal("expected password hash to be set")
	}
	if !repo.VerifyPassword(user, "password123") {
		t.Fatal("expected password to verify")
	}

	rows := sqlmock.NewRows([]string{
		"user_id", "email", "phone", "password_hash", "status", "kyc_status", "created_at_ms", "updated_at_ms",
	}).AddRow(user.UserID, user.Email, user.Phone, user.PasswordHash, user.Status, user.KycStatus, user.CreatedAtMs, user.UpdatedAtMs)

	mock.ExpectQuery("SELECT user_id, email, phone, password_hash").
		WithArgs(user.Email).
		WillReturnRows(rows)

	got, err := repo.GetUserByEmail(context.Background(), user.Email)
	if err != nil {
		t.Fatalf("get user by email: %v", err)
	}
	if got.UserID != user.UserID {
		t.Fatalf("expected %d, got %d", user.UserID, got.UserID)
	}

	rowsByID := sqlmock.NewRows([]string{
		"user_id", "email", "phone", "password_hash", "status", "kyc_status", "created_at_ms", "updated_at_ms",
	}).AddRow(user.UserID, user.Email, user.Phone, user.PasswordHash, user.Status, user.KycStatus, user.CreatedAtMs, user.UpdatedAtMs)

	mock.ExpectQuery("SELECT user_id, email, phone, password_hash").
		WithArgs(user.UserID).
		WillReturnRows(rowsByID)

	got, err = repo.GetUserByID(context.Background(), user.UserID)
	if err != nil {
		t.Fatalf("get user by id: %v", err)
	}
	if got.Email != user.Email {
		t.Fatalf("expected %s, got %s", user.Email, got.Email)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTOTPUserCreateUserDuplicate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	user := &User{UserID: 2, Email: "dup@example.com", Status: UserStatusActive, KycStatus: 1}

	mock.ExpectExec("INSERT INTO exchange_user.users").
		WillReturnError(errors.New("duplicate key value"))

	if err := repo.CreateUser(context.Background(), user, "password123"); err != ErrEmailExists {
		t.Fatalf("expected ErrEmailExists, got %v", err)
	}
}

func TestTOTPUserCreateUserError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	user := &User{UserID: 3, Email: "err@example.com", Status: UserStatusActive, KycStatus: 1}

	mock.ExpectExec("INSERT INTO exchange_user.users").
		WillReturnError(errors.New("insert error"))

	if err := repo.CreateUser(context.Background(), user, "password123"); err == nil || err == ErrEmailExists {
		t.Fatalf("expected insert error, got %v", err)
	}
}

func TestTOTPUserNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)

	mock.ExpectQuery("SELECT user_id, email, phone, password_hash").
		WithArgs("missing@example.com").
		WillReturnError(sql.ErrNoRows)

	if _, err := repo.GetUserByEmail(context.Background(), "missing@example.com"); err != ErrUserNotFound {
		t.Fatalf("expected ErrUserNotFound, got %v", err)
	}
}

func TestTOTPUserQueryError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)

	mock.ExpectQuery("SELECT user_id, email, phone, password_hash").
		WithArgs(int64(99)).
		WillReturnError(errors.New("query error"))

	if _, err := repo.GetUserByID(context.Background(), 99); err == nil || err == ErrUserNotFound {
		t.Fatalf("expected query error, got %v", err)
	}
}

func TestTOTPUserApiKeyFlow(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	apiKey := &ApiKey{
		ApiKeyID:    10,
		UserID:      1,
		Label:       "test",
		Permissions: 7,
		IPWhitelist: []string{"127.0.0.1"},
		Status:      1,
		CreatedAtMs: 1111,
		UpdatedAtMs: 2222,
	}

	mock.ExpectExec("INSERT INTO exchange_user.api_keys").
		WithArgs(apiKey.ApiKeyID, apiKey.UserID, sqlmock.AnyArg(), sqlmock.AnyArg(),
			apiKey.Label, apiKey.Permissions, sqlmock.AnyArg(), apiKey.Status, apiKey.CreatedAtMs, apiKey.UpdatedAtMs).
		WillReturnResult(sqlmock.NewResult(1, 1))

	secret, err := repo.CreateApiKey(context.Background(), apiKey)
	if err != nil {
		t.Fatalf("create api key: %v", err)
	}
	if len(secret) != 64 {
		t.Fatalf("expected secret length 64, got %d", len(secret))
	}
	if len(apiKey.ApiKey) != 32 {
		t.Fatalf("expected api key length 32, got %d", len(apiKey.ApiKey))
	}

	rows := sqlmock.NewRows([]string{
		"api_key_id", "user_id", "api_key", "secret_hash", "label", "permissions", "ip_whitelist", "status", "created_at_ms", "updated_at_ms",
	}).AddRow(apiKey.ApiKeyID, apiKey.UserID, apiKey.ApiKey, apiKey.SecretHash, apiKey.Label, apiKey.Permissions, nil, apiKey.Status, apiKey.CreatedAtMs, apiKey.UpdatedAtMs)

	mock.ExpectQuery("SELECT api_key_id, user_id, api_key, secret_hash").
		WithArgs(apiKey.ApiKey).
		WillReturnRows(rows)

	gotKey, err := repo.GetApiKeyByKey(context.Background(), apiKey.ApiKey)
	if err != nil {
		t.Fatalf("get api key: %v", err)
	}
	if gotKey.ApiKeyID != apiKey.ApiKeyID {
		t.Fatalf("expected %d, got %d", apiKey.ApiKeyID, gotKey.ApiKeyID)
	}
	if !repo.VerifyApiKeySecret(gotKey, secret) {
		t.Fatal("expected api key secret to verify")
	}

	listRows := sqlmock.NewRows([]string{
		"api_key_id", "user_id", "api_key", "secret_hash", "label", "permissions", "ip_whitelist", "status", "created_at_ms", "updated_at_ms",
	}).AddRow(apiKey.ApiKeyID, apiKey.UserID, apiKey.ApiKey, apiKey.SecretHash, apiKey.Label, apiKey.Permissions, nil, apiKey.Status, apiKey.CreatedAtMs, apiKey.UpdatedAtMs)

	mock.ExpectQuery("SELECT api_key_id, user_id, api_key, secret_hash").
		WithArgs(apiKey.UserID).
		WillReturnRows(listRows)

	keys, err := repo.ListApiKeys(context.Background(), apiKey.UserID)
	if err != nil {
		t.Fatalf("list api keys: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("expected 1 key, got %d", len(keys))
	}

	mock.ExpectExec("UPDATE exchange_user.api_keys").
		WithArgs(apiKey.ApiKeyID, apiKey.UserID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.DeleteApiKey(context.Background(), apiKey.UserID, apiKey.ApiKeyID); err != nil {
		t.Fatalf("delete api key: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTOTPUserApiKeyNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)

	mock.ExpectQuery("SELECT api_key_id, user_id, api_key, secret_hash").
		WithArgs("missing").
		WillReturnError(sql.ErrNoRows)

	if _, err := repo.GetApiKeyByKey(context.Background(), "missing"); err != ErrApiKeyNotFound {
		t.Fatalf("expected ErrApiKeyNotFound, got %v", err)
	}

	mock.ExpectExec("UPDATE exchange_user.api_keys").
		WithArgs(int64(1), int64(1)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.DeleteApiKey(context.Background(), 1, 1); err != ErrApiKeyNotFound {
		t.Fatalf("expected ErrApiKeyNotFound, got %v", err)
	}
}

func TestTOTPUserApiKeyErrors(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo := NewUserRepository(db)
	apiKey := &ApiKey{ApiKeyID: 11, UserID: 2, Status: 1}

	mock.ExpectExec("INSERT INTO exchange_user.api_keys").
		WillReturnError(errors.New("insert error"))

	if _, err := repo.CreateApiKey(context.Background(), apiKey); err == nil {
		t.Fatal("expected create api key error")
	}

	mock.ExpectQuery("SELECT api_key_id, user_id, api_key, secret_hash").
		WithArgs("boom").
		WillReturnError(errors.New("query error"))

	if _, err := repo.GetApiKeyByKey(context.Background(), "boom"); err == nil || err == ErrApiKeyNotFound {
		t.Fatalf("expected query error, got %v", err)
	}

	mock.ExpectQuery("SELECT api_key_id, user_id, api_key, secret_hash").
		WithArgs(int64(2)).
		WillReturnError(errors.New("list error"))

	if _, err := repo.ListApiKeys(context.Background(), 2); err == nil {
		t.Fatal("expected list api keys error")
	}

	badRows := sqlmock.NewRows([]string{"api_key_id", "user_id"}).AddRow(1, 2)
	mock.ExpectQuery("SELECT api_key_id, user_id, api_key, secret_hash").
		WithArgs(int64(3)).
		WillReturnRows(badRows)

	if _, err := repo.ListApiKeys(context.Background(), 3); err == nil {
		t.Fatal("expected scan error")
	}
}

func TestTOTPUserHelpers(t *testing.T) {
	if null := nullString(""); null.Valid {
		t.Fatal("expected null string to be invalid")
	}
	if null := nullString("x"); !null.Valid || null.String != "x" {
		t.Fatal("expected valid null string")
	}

	if !isUniqueViolation(errors.New("unique constraint")) {
		t.Fatal("expected unique violation to be true")
	}
	if isUniqueViolation(nil) {
		t.Fatal("expected unique violation to be false")
	}
	if contains("abcd", "bc") == false {
		t.Fatal("expected contains to be true")
	}

	if pqArray([]string{}) != "{}" {
		t.Fatal("expected empty array literal")
	}
	if pqArray([]string{"a", "b"}) == "{}" {
		t.Fatal("expected non-empty array literal")
	}

	arr := []string{}
	scanner := pqArrayScan(&arr).(*pqStringArray)
	if err := scanner.Scan(nil); err != nil {
		t.Fatalf("scan nil: %v", err)
	}
	if arr != nil {
		t.Fatal("expected nil array")
	}

	arr = []string{"before"}
	if err := scanner.Scan("value"); err != nil {
		t.Fatalf("scan value: %v", err)
	}
	if len(arr) != 1 || arr[0] != "value" {
		t.Fatalf("expected parsed array, got %v", arr)
	}

	hash, err := bcrypt.GenerateFromPassword([]byte("secret"), bcrypt.DefaultCost)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	user := &User{PasswordHash: string(hash)}
	repo := NewUserRepository(nil)
	if repo.VerifyPassword(user, "wrong") {
		t.Fatal("expected verify password to fail")
	}
}
