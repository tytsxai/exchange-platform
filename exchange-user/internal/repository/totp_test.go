package repository

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/base64"
	"errors"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
)

const (
	testTOTPKey    = "12345678901234567890123456789012"
	testTOTPSecret = "totp-secret"
)

type secretMatcher struct {
	key       []byte
	plaintext string
}

func (m secretMatcher) Match(v driver.Value) bool {
	secret, ok := v.(string)
	if !ok {
		return false
	}
	decrypted, err := decryptSecret(m.key, secret)
	if err != nil {
		return false
	}
	return decrypted == m.plaintext
}

func TestTOTPNewRepositoryInvalidKey(t *testing.T) {
	repo, err := NewTOTPRepository(nil, []byte("short"))
	if err != ErrInvalidTOTPKeyLength {
		t.Fatalf("expected ErrInvalidTOTPKeyLength, got %v", err)
	}
	if repo != nil {
		t.Fatalf("expected nil repo, got %v", repo)
	}
}

func TestTOTPEncryptDecryptSecret(t *testing.T) {
	encrypted, err := encryptSecret([]byte(testTOTPKey), testTOTPSecret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}
	if encrypted == testTOTPSecret {
		t.Fatal("expected encrypted secret to differ from plaintext")
	}

	decrypted, err := decryptSecret([]byte(testTOTPKey), encrypted)
	if err != nil {
		t.Fatalf("decrypt secret: %v", err)
	}
	if decrypted != testTOTPSecret {
		t.Fatalf("expected %s, got %s", testTOTPSecret, decrypted)
	}
}

func TestTOTPDecryptSecretErrors(t *testing.T) {
	if _, err := decryptSecret([]byte(testTOTPKey), "not-base64"); err == nil {
		t.Fatal("expected error for invalid base64")
	}

	shortPayload := base64.StdEncoding.EncodeToString([]byte("short"))
	if _, err := decryptSecret([]byte(testTOTPKey), shortPayload); err == nil {
		t.Fatal("expected error for short ciphertext")
	}
}

func TestTOTPSaveAndGetSecret(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectExec("INSERT INTO exchange_user.user_totp_secrets").
		WithArgs(int64(1), secretMatcher{key: []byte(testTOTPKey), plaintext: testTOTPSecret}).
		WillReturnResult(sqlmock.NewResult(1, 1))

	if err := repo.SaveSecret(context.Background(), 1, testTOTPSecret); err != nil {
		t.Fatalf("save secret: %v", err)
	}

	encrypted, err := encryptSecret([]byte(testTOTPKey), testTOTPSecret)
	if err != nil {
		t.Fatalf("encrypt secret: %v", err)
	}

	mock.ExpectQuery("SELECT secret, enabled").
		WithArgs(int64(1)).
		WillReturnRows(sqlmock.NewRows([]string{"secret", "enabled"}).AddRow(encrypted, true))

	secret, enabled, err := repo.GetSecret(context.Background(), 1)
	if err != nil {
		t.Fatalf("get secret: %v", err)
	}
	if secret != testTOTPSecret {
		t.Fatalf("expected %s, got %s", testTOTPSecret, secret)
	}
	if !enabled {
		t.Fatal("expected enabled=true")
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTOTPGetSecretNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectQuery("SELECT secret, enabled").
		WithArgs(int64(2)).
		WillReturnError(sql.ErrNoRows)

	_, _, err = repo.GetSecret(context.Background(), 2)
	if err != ErrTOTPNotFound {
		t.Fatalf("expected ErrTOTPNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTOTPGetSecretDecryptError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectQuery("SELECT secret, enabled").
		WithArgs(int64(5)).
		WillReturnRows(sqlmock.NewRows([]string{"secret", "enabled"}).AddRow("not-base64", true))

	if _, _, err := repo.GetSecret(context.Background(), 5); err == nil {
		t.Fatal("expected decrypt error")
	}
}

func TestTOTPSaveSecretError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectExec("INSERT INTO exchange_user.user_totp_secrets").
		WithArgs(int64(6), sqlmock.AnyArg()).
		WillReturnError(errors.New("db error"))

	if err := repo.SaveSecret(context.Background(), 6, testTOTPSecret); err == nil {
		t.Fatal("expected save error")
	}
}

func TestTOTPUpdateEnabled(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectExec("UPDATE exchange_user.user_totp_secrets").
		WithArgs(true, int64(3)).
		WillReturnResult(sqlmock.NewResult(0, 1))

	if err := repo.UpdateEnabled(context.Background(), 3, true); err != nil {
		t.Fatalf("update enabled: %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTOTPUpdateEnabledNotFound(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectExec("UPDATE exchange_user.user_totp_secrets").
		WithArgs(false, int64(4)).
		WillReturnResult(sqlmock.NewResult(0, 0))

	if err := repo.UpdateEnabled(context.Background(), 4, false); err != ErrTOTPNotFound {
		t.Fatalf("expected ErrTOTPNotFound, got %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatalf("expectations: %v", err)
	}
}

func TestTOTPUpdateEnabledError(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatalf("sqlmock new: %v", err)
	}
	defer db.Close()

	repo, err := NewTOTPRepository(db, []byte(testTOTPKey))
	if err != nil {
		t.Fatalf("new repo: %v", err)
	}

	mock.ExpectExec("UPDATE exchange_user.user_totp_secrets").
		WithArgs(true, int64(7)).
		WillReturnError(errors.New("db error"))

	if err := repo.UpdateEnabled(context.Background(), 7, true); err == nil {
		t.Fatal("expected update error")
	}
}
