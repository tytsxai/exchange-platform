// Package repository 用户数据访问层
package repository

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

var (
	ErrUserNotFound     = errors.New("user not found")
	ErrEmailExists      = errors.New("email already exists")
	ErrInvalidPassword  = errors.New("invalid password")
	ErrApiKeyNotFound   = errors.New("api key not found")
)

// UserStatus 用户状态
const (
	UserStatusActive   = 1
	UserStatusFrozen   = 2
	UserStatusDisabled = 3
)

// User 用户
type User struct {
	UserID       int64
	Email        string
	Phone        string
	PasswordHash string
	Status       int
	KycStatus    int
	CreatedAtMs  int64
	UpdatedAtMs  int64
}

// ApiKey API Key
type ApiKey struct {
	ApiKeyID    int64
	UserID      int64
	ApiKey      string
	SecretHash  string
	Label       string
	Permissions int // bitmask: 1=READ, 2=TRADE, 4=WITHDRAW
	IPWhitelist []string
	Status      int
	CreatedAtMs int64
	UpdatedAtMs int64
}

// UserRepository 用户仓储
type UserRepository struct {
	db *sql.DB
}

// NewUserRepository 创建仓储
func NewUserRepository(db *sql.DB) *UserRepository {
	return &UserRepository{db: db}
}

// CreateUser 创建用户
func (r *UserRepository) CreateUser(ctx context.Context, user *User, password string) error {
	// 密码哈希
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return fmt.Errorf("hash password: %w", err)
	}
	user.PasswordHash = string(hash)

	query := `
		INSERT INTO exchange_user.users
		(user_id, email, phone, password_hash, status, kyc_status, created_at_ms, updated_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err = r.db.ExecContext(ctx, query,
		user.UserID, nullString(user.Email), nullString(user.Phone),
		user.PasswordHash, user.Status, user.KycStatus,
		user.CreatedAtMs, user.UpdatedAtMs,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return ErrEmailExists
		}
		return fmt.Errorf("insert user: %w", err)
	}
	return nil
}

// GetUserByEmail 通过邮箱获取用户
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	query := `
		SELECT user_id, email, phone, password_hash, status, kyc_status, created_at_ms, updated_at_ms
		FROM exchange_user.users
		WHERE email = $1
	`
	return r.scanUser(r.db.QueryRowContext(ctx, query, email))
}

// GetUserByID 通过 ID 获取用户
func (r *UserRepository) GetUserByID(ctx context.Context, userID int64) (*User, error) {
	query := `
		SELECT user_id, email, phone, password_hash, status, kyc_status, created_at_ms, updated_at_ms
		FROM exchange_user.users
		WHERE user_id = $1
	`
	return r.scanUser(r.db.QueryRowContext(ctx, query, userID))
}

// VerifyPassword 验证密码
func (r *UserRepository) VerifyPassword(user *User, password string) bool {
	err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password))
	return err == nil
}

// CreateApiKey 创建 API Key
func (r *UserRepository) CreateApiKey(ctx context.Context, apiKey *ApiKey) (secret string, err error) {
	// 生成 API Key 和 Secret
	apiKey.ApiKey, err = generateRandomString(32)
	if err != nil {
		return "", fmt.Errorf("generate api key: %w", err)
	}

	secret, err = generateRandomString(64)
	if err != nil {
		return "", fmt.Errorf("generate secret: %w", err)
	}

	// 存储原始 secret（用于签名验证）
	apiKey.SecretHash = secret

	query := `
		INSERT INTO exchange_user.api_keys
		(api_key_id, user_id, api_key, secret_hash, label, permissions, ip_whitelist, status, created_at_ms, updated_at_ms)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err = r.db.ExecContext(ctx, query,
		apiKey.ApiKeyID, apiKey.UserID, apiKey.ApiKey, apiKey.SecretHash,
		apiKey.Label, apiKey.Permissions, pqArray(apiKey.IPWhitelist),
		apiKey.Status, apiKey.CreatedAtMs, apiKey.UpdatedAtMs,
	)
	if err != nil {
		return "", fmt.Errorf("insert api key: %w", err)
	}

	return secret, nil
}

// GetApiKeyByKey 通过 API Key 获取
func (r *UserRepository) GetApiKeyByKey(ctx context.Context, key string) (*ApiKey, error) {
	query := `
		SELECT api_key_id, user_id, api_key, secret_hash, label, permissions, ip_whitelist, status, created_at_ms, updated_at_ms
		FROM exchange_user.api_keys
		WHERE api_key = $1 AND status = 1
	`
	return r.scanApiKey(r.db.QueryRowContext(ctx, query, key))
}

// ListApiKeys 列出用户的 API Keys
func (r *UserRepository) ListApiKeys(ctx context.Context, userID int64) ([]*ApiKey, error) {
	query := `
		SELECT api_key_id, user_id, api_key, secret_hash, label, permissions, ip_whitelist, status, created_at_ms, updated_at_ms
		FROM exchange_user.api_keys
		WHERE user_id = $1
		ORDER BY created_at_ms DESC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, fmt.Errorf("query api keys: %w", err)
	}
	defer rows.Close()

	var keys []*ApiKey
	for rows.Next() {
		key, err := r.scanApiKeyRow(rows)
		if err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, nil
}

// DeleteApiKey 删除 API Key
func (r *UserRepository) DeleteApiKey(ctx context.Context, userID, apiKeyID int64) error {
	query := `
		UPDATE exchange_user.api_keys
		SET status = 2
		WHERE api_key_id = $1 AND user_id = $2
	`
	result, err := r.db.ExecContext(ctx, query, apiKeyID, userID)
	if err != nil {
		return fmt.Errorf("delete api key: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return ErrApiKeyNotFound
	}
	return nil
}

// VerifyApiKeySecret 验证 API Key Secret
func (r *UserRepository) VerifyApiKeySecret(apiKey *ApiKey, secret string) bool {
	return hmac.Equal([]byte(apiKey.SecretHash), []byte(secret))
}

func (r *UserRepository) scanUser(row *sql.Row) (*User, error) {
	var u User
	var email, phone sql.NullString

	err := row.Scan(
		&u.UserID, &email, &phone, &u.PasswordHash,
		&u.Status, &u.KycStatus, &u.CreatedAtMs, &u.UpdatedAtMs,
	)
	if err == sql.ErrNoRows {
		return nil, ErrUserNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan user: %w", err)
	}

	u.Email = email.String
	u.Phone = phone.String
	return &u, nil
}

func (r *UserRepository) scanApiKey(row *sql.Row) (*ApiKey, error) {
	var k ApiKey
	var label sql.NullString
	var ipWhitelist []string

	err := row.Scan(
		&k.ApiKeyID, &k.UserID, &k.ApiKey, &k.SecretHash,
		&label, &k.Permissions, pqArrayScan(&ipWhitelist),
		&k.Status, &k.CreatedAtMs, &k.UpdatedAtMs,
	)
	if err == sql.ErrNoRows {
		return nil, ErrApiKeyNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}

	k.Label = label.String
	k.IPWhitelist = ipWhitelist
	return &k, nil
}

func (r *UserRepository) scanApiKeyRow(rows *sql.Rows) (*ApiKey, error) {
	var k ApiKey
	var label sql.NullString
	var ipWhitelist []string

	err := rows.Scan(
		&k.ApiKeyID, &k.UserID, &k.ApiKey, &k.SecretHash,
		&label, &k.Permissions, pqArrayScan(&ipWhitelist),
		&k.Status, &k.CreatedAtMs, &k.UpdatedAtMs,
	)
	if err != nil {
		return nil, fmt.Errorf("scan api key: %w", err)
	}

	k.Label = label.String
	k.IPWhitelist = ipWhitelist
	return &k, nil
}

func generateRandomString(length int) (string, error) {
	bytes := make([]byte, length/2)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}
	return hex.EncodeToString(bytes), nil
}

func nullString(s string) sql.NullString {
	if s == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: s, Valid: true}
}

func isUniqueViolation(err error) bool {
	return err != nil && (contains(err.Error(), "unique") || contains(err.Error(), "duplicate"))
}

func contains(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// pqArray 转换为 PostgreSQL 数组
func pqArray(arr []string) interface{} {
	if len(arr) == 0 {
		return "{}"
	}
	result := "{"
	for i, s := range arr {
		if i > 0 {
			result += ","
		}
		result += "\"" + s + "\""
	}
	result += "}"
	return result
}

// pqArrayScan 扫描 PostgreSQL 数组
func pqArrayScan(arr *[]string) interface{} {
	return &pqStringArray{arr: arr}
}

type pqStringArray struct {
	arr *[]string
}

func (p *pqStringArray) Scan(src interface{}) error {
	if src == nil {
		*p.arr = nil
		return nil
	}
	// 简化实现
	*p.arr = []string{}
	return nil
}
