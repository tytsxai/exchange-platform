package service

import (
	"errors"
	"testing"
	"time"

	"github.com/pquerna/otp/totp"
)

type mockPasswordVerifier struct {
	ok bool
}

func (m mockPasswordVerifier) VerifyPassword(userID int64, password string) bool {
	return m.ok && password == "pass"
}

func TestTOTPVerifyCodeErrors(t *testing.T) {
	svc := NewTOTPService(nil)
	svc.now = func() time.Time { return time.Unix(0, 0) }

	if _, err := svc.VerifyCode(1, "12ab"); !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("expected ErrInvalidTOTPCode, got %v", err)
	}

	if _, err := svc.VerifyCode(1, "123456"); !errors.Is(err, ErrTOTPNotInitialized) {
		t.Fatalf("expected ErrTOTPNotInitialized, got %v", err)
	}
}

func TestTOTPFlow(t *testing.T) {
	svc := NewTOTPService(mockPasswordVerifier{ok: true})
	now := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.now = func() time.Time { return now }

	secret, err := svc.GenerateSecret(1)
	if err != nil || secret == "" {
		t.Fatalf("expected secret, got err=%v secret=%s", err, secret)
	}

	code, err := totp.GenerateCodeCustom(secret, now, totpValidateOpts())
	if err != nil {
		t.Fatalf("expected code, got %v", err)
	}

	valid, err := svc.VerifyCode(1, code)
	if err != nil || !valid {
		t.Fatalf("expected valid code, got err=%v valid=%v", err, valid)
	}

	if err := svc.EnableTOTP(1, code); err != nil {
		t.Fatalf("expected enable success, got %v", err)
	}

	if err := svc.DisableTOTP(1, code, "wrong"); !errors.Is(err, ErrInvalidPassword) {
		t.Fatalf("expected ErrInvalidPassword, got %v", err)
	}

	if err := svc.DisableTOTP(1, "000000", "pass"); !errors.Is(err, ErrInvalidTOTPCode) {
		t.Fatalf("expected ErrInvalidTOTPCode, got %v", err)
	}

	if err := svc.DisableTOTP(1, code, "pass"); err != nil {
		t.Fatalf("expected disable success, got %v", err)
	}
}

func TestTOTPDisableErrors(t *testing.T) {
	svc := NewTOTPService(nil)
	if err := svc.DisableTOTP(1, "123456", "pass"); !errors.Is(err, ErrTOTPNotEnabled) {
		t.Fatalf("expected ErrTOTPNotEnabled, got %v", err)
	}

	secret, err := svc.GenerateSecret(1)
	if err != nil {
		t.Fatalf("expected secret, got %v", err)
	}
	code, err := totp.GenerateCodeCustom(secret, svc.now(), totpValidateOpts())
	if err != nil {
		t.Fatalf("expected code, got %v", err)
	}
	if err := svc.EnableTOTP(1, code); err != nil {
		t.Fatalf("expected enable success, got %v", err)
	}

	if err := svc.DisableTOTP(1, code, "pass"); !errors.Is(err, ErrPasswordVerificationDisabled) {
		t.Fatalf("expected ErrPasswordVerificationDisabled, got %v", err)
	}
}
