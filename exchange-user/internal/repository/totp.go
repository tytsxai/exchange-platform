// Package repository 用户 TOTP 数据访问层
package repository

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

var (
	ErrTOTPNotFound        = errors.New("totp secret not found")
	ErrInvalidTOTPKeyLength = errors.New("invalid TOTP encryption key length")
)

// TOTPRepository TOTP 仓储
type TOTPRepository struct {
	db  *sql.DB
	key []byte
}

// NewTOTPRepository 创建仓储，key 必须为 32 字节（AES-256）
func NewTOTPRepository(db *sql.DB, key []byte) (*TOTPRepository, error) {
	if len(key) != 32 {
		return nil, ErrInvalidTOTPKeyLength
	}
	secretKey := make([]byte, 32)
	copy(secretKey, key)
	return &TOTPRepository{db: db, key: secretKey}, nil
}

// SaveSecret 保存 TOTP 密钥（加密存储）
func (r *TOTPRepository) SaveSecret(ctx context.Context, userID int64, secret string) error {
	encryptedSecret, err := encryptSecret(r.key, secret)
	if err != nil {
		return fmt.Errorf("encrypt secret: %w", err)
	}

	query := `
		INSERT INTO exchange_user.user_totp_secrets (user_id, secret, enabled, created_at)
		VALUES ($1, $2, FALSE, NOW())
		ON CONFLICT (user_id) DO UPDATE
		SET secret = EXCLUDED.secret,
		    enabled = EXCLUDED.enabled
	`
	_, err = r.db.ExecContext(ctx, query, userID, encryptedSecret)
	if err != nil {
		return fmt.Errorf("save totp secret: %w", err)
	}
	return nil
}

// GetSecret 获取 TOTP 密钥（解密返回）
func (r *TOTPRepository) GetSecret(ctx context.Context, userID int64) (secret string, enabled bool, err error) {
	query := `
		SELECT secret, enabled
		FROM exchange_user.user_totp_secrets
		WHERE user_id = $1
	`
	var encryptedSecret string
	err = r.db.QueryRowContext(ctx, query, userID).Scan(&encryptedSecret, &enabled)
	if err == sql.ErrNoRows {
		return "", false, ErrTOTPNotFound
	}
	if err != nil {
		return "", false, fmt.Errorf("get totp secret: %w", err)
	}

	secret, err = decryptSecret(r.key, encryptedSecret)
	if err != nil {
		return "", false, fmt.Errorf("decrypt secret: %w", err)
	}
	return secret, enabled, nil
}

// UpdateEnabled 更新 TOTP 启用状态
func (r *TOTPRepository) UpdateEnabled(ctx context.Context, userID int64, enabled bool) error {
	query := `
		UPDATE exchange_user.user_totp_secrets
		SET enabled = $1
		WHERE user_id = $2
	`
	result, err := r.db.ExecContext(ctx, query, enabled, userID)
	if err != nil {
		return fmt.Errorf("update totp enabled: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrTOTPNotFound
	}
	return nil
}

func encryptSecret(key []byte, plaintext string) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nil, nonce, []byte(plaintext), nil)
	payload := append(nonce, ciphertext...)
	return base64.StdEncoding.EncodeToString(payload), nil
}

func decryptSecret(key []byte, encoded string) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	if len(payload) < gcm.NonceSize() {
		return "", errors.New("ciphertext too short")
	}
	nonce := payload[:gcm.NonceSize()]
	ciphertext := payload[gcm.NonceSize():]
	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", err
	}
	return string(plaintext), nil
}
