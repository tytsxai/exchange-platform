// Package service 用户服务
package service

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/pquerna/otp"
	"github.com/pquerna/otp/totp"
)

var (
	ErrTOTPNotInitialized           = errors.New("totp not initialized")
	ErrTOTPNotEnabled               = errors.New("totp not enabled")
	ErrInvalidTOTPCode              = errors.New("invalid totp code")
	ErrInvalidPassword              = errors.New("invalid password")
	ErrPasswordVerificationDisabled = errors.New("password verification disabled")
)

const (
	totpPeriod = 30
	totpSkew   = 1
)

// PasswordVerifier 验证用户密码
type PasswordVerifier interface {
	VerifyPassword(userID int64, password string) bool
}

// TOTPService 管理用户的 TOTP 配置
type TOTPService struct {
	mu               sync.Mutex
	secrets          map[int64]string
	enabled          map[int64]bool
	passwordVerifier PasswordVerifier
	now              func() time.Time
}

// NewTOTPService 创建 TOTP 服务
func NewTOTPService(passwordVerifier PasswordVerifier) *TOTPService {
	return &TOTPService{
		secrets:          make(map[int64]string),
		enabled:          make(map[int64]bool),
		passwordVerifier: passwordVerifier,
		now:              time.Now,
	}
}

// GenerateSecret 生成 TOTP 密钥
func (s *TOTPService) GenerateSecret(userID int64) (string, error) {
	key, err := totp.Generate(totp.GenerateOpts{
		Issuer:      "Exchange",
		AccountName: fmt.Sprintf("user:%d", userID),
	})
	if err != nil {
		return "", fmt.Errorf("generate totp secret: %w", err)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	s.secrets[userID] = key.Secret()
	s.enabled[userID] = false

	return key.Secret(), nil
}

// VerifyCode 验证 6 位数字码
func (s *TOTPService) VerifyCode(userID int64, code string) (bool, error) {
	if !isValidTOTPCode(code) {
		return false, ErrInvalidTOTPCode
	}

	s.mu.Lock()
	secret, ok := s.secrets[userID]
	s.mu.Unlock()
	if !ok || secret == "" {
		return false, ErrTOTPNotInitialized
	}

	valid, err := totp.ValidateCustom(code, secret, s.now(), totpValidateOpts())
	if err != nil {
		return false, fmt.Errorf("validate totp code: %w", err)
	}
	if !valid {
		return false, ErrInvalidTOTPCode
	}
	return true, nil
}

// EnableTOTP 启用 2FA（需验证首次码）
func (s *TOTPService) EnableTOTP(userID int64, code string) error {
	valid, err := s.VerifyCode(userID, code)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidTOTPCode
	}

	s.mu.Lock()
	s.enabled[userID] = true
	s.mu.Unlock()
	return nil
}

// DisableTOTP 禁用 2FA
func (s *TOTPService) DisableTOTP(userID int64, code, password string) error {
	s.mu.Lock()
	enabled := s.enabled[userID]
	s.mu.Unlock()
	if !enabled {
		return ErrTOTPNotEnabled
	}

	if s.passwordVerifier == nil {
		return ErrPasswordVerificationDisabled
	}
	if !s.passwordVerifier.VerifyPassword(userID, password) {
		return ErrInvalidPassword
	}

	valid, err := s.VerifyCode(userID, code)
	if err != nil {
		return err
	}
	if !valid {
		return ErrInvalidTOTPCode
	}

	s.mu.Lock()
	s.enabled[userID] = false
	s.mu.Unlock()
	return nil
}

func isValidTOTPCode(code string) bool {
	if len(code) != 6 {
		return false
	}
	for i := 0; i < len(code); i++ {
		if code[i] < '0' || code[i] > '9' {
			return false
		}
	}
	return true
}

func totpValidateOpts() totp.ValidateOpts {
	return totp.ValidateOpts{
		Period:    totpPeriod,
		Skew:      totpSkew,
		Digits:    otp.DigitsSix,
		Algorithm: otp.AlgorithmSHA1,
	}
}
