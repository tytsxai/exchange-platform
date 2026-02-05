// Package service 用户服务
package service

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/exchange/common/pkg/audit"
	"github.com/exchange/user/internal/repository"
)

var (
	ErrTokenIssuerNotConfigured = errors.New("token issuer not configured")
	ErrUserFrozen               = errors.New("user frozen")
	ErrUserDisabled             = errors.New("user disabled")
)

const (
	minPasswordLength = 8
	maxPasswordLength = 128
	maxEmailLength    = 254
)

// UserRepository 用户仓储接口
type UserRepository interface {
	CreateUser(ctx context.Context, user *repository.User, password string) error
	GetUserByEmail(ctx context.Context, email string) (*repository.User, error)
	VerifyPassword(user *repository.User, password string) bool
	CreateApiKey(ctx context.Context, apiKey *repository.ApiKey) (secret string, err error)
	ListApiKeys(ctx context.Context, userID int64) ([]*repository.ApiKey, error)
	DeleteApiKey(ctx context.Context, userID, apiKeyID int64) error
	GetApiKeyByKey(ctx context.Context, key string) (*repository.ApiKey, error)
	GetUserByID(ctx context.Context, userID int64) (*repository.User, error)
}

// UserService 用户服务
type UserService struct {
	repo        UserRepository
	idGen       IDGenerator
	token       TokenIssuer
	auditLogger audit.Logger
}

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// TokenIssuer token issuer interface.
type TokenIssuer interface {
	Issue(userID int64) (string, error)
}

// NewUserService 创建用户服务
func NewUserService(repo UserRepository, idGen IDGenerator, tokenIssuer TokenIssuer) *UserService {
	return &UserService{
		repo:  repo,
		idGen: idGen,
		token: tokenIssuer,
	}
}

// SetAuditLogger 设置审计日志 logger。
func (s *UserService) SetAuditLogger(logger audit.Logger) {
	s.auditLogger = logger
}

// RegisterRequest 注册请求
type RegisterRequest struct {
	Email    string
	Password string
}

// RegisterResponse 注册响应
type RegisterResponse struct {
	User      *repository.User
	ErrorCode string
}

// Register 注册
func (s *UserService) Register(ctx context.Context, req *RegisterRequest) (*RegisterResponse, error) {
	email := strings.TrimSpace(req.Email)
	if code := validateRegisterInput(email, req.Password); code != "" {
		return &RegisterResponse{ErrorCode: code}, nil
	}

	now := time.Now().UnixMilli()
	user := &repository.User{
		UserID:      s.idGen.NextID(),
		Email:       email,
		Status:      repository.UserStatusActive,
		KycStatus:   1, // NOT_STARTED
		CreatedAtMs: now,
		UpdatedAtMs: now,
	}

	err := s.repo.CreateUser(ctx, user, req.Password)
	if err != nil {
		if err == repository.ErrEmailExists {
			return &RegisterResponse{ErrorCode: "EMAIL_EXISTS"}, nil
		}
		return nil, err
	}

	return &RegisterResponse{User: user}, nil
}

// LoginRequest 登录请求
type LoginRequest struct {
	Email    string
	Password string
}

// LoginResponse 登录响应
type LoginResponse struct {
	User      *repository.User
	Token     string
	ErrorCode string
}

// Login 登录
func (s *UserService) Login(ctx context.Context, req *LoginRequest) (*LoginResponse, error) {
	email := strings.TrimSpace(req.Email)
	if email == "" {
		s.writeAudit(ctx, audit.NewLog(audit.EventLoginFailed, 0).
			WithIP("").
			WithParams(map[string]interface{}{"email": email}).
			WithResult(false, "INVALID_CREDENTIALS"))
		return &LoginResponse{ErrorCode: "INVALID_CREDENTIALS"}, nil
	}

	user, err := s.repo.GetUserByEmail(ctx, email)
	if err != nil {
		if err == repository.ErrUserNotFound {
			s.writeAudit(ctx, audit.NewLog(audit.EventLoginFailed, 0).
				WithIP("").
				WithParams(map[string]interface{}{"email": email}).
				WithResult(false, "INVALID_CREDENTIALS"))
			return &LoginResponse{ErrorCode: "INVALID_CREDENTIALS"}, nil
		}
		return nil, err
	}

	if !s.repo.VerifyPassword(user, req.Password) {
		s.writeAudit(ctx, audit.NewLog(audit.EventLoginFailed, user.UserID).
			WithIP("").
			WithParams(map[string]interface{}{"email": email}).
			WithResult(false, "INVALID_CREDENTIALS"))
		return &LoginResponse{ErrorCode: "INVALID_CREDENTIALS"}, nil
	}

	if user.Status != repository.UserStatusActive {
		code := "USER_FROZEN"
		if user.Status == repository.UserStatusDisabled {
			code = "USER_DISABLED"
		}
		s.writeAudit(ctx, audit.NewLog(audit.EventLoginFailed, user.UserID).
			WithIP("").
			WithParams(map[string]interface{}{"email": email, "status": user.Status}).
			WithResult(false, code))
		return &LoginResponse{ErrorCode: code}, nil
	}

	if s.token == nil {
		return nil, ErrTokenIssuerNotConfigured
	}

	token, err := s.token.Issue(user.UserID)
	if err != nil {
		return nil, err
	}

	s.writeAudit(ctx, audit.NewLog(audit.EventLogin, user.UserID).
		WithIP("").
		WithParams(map[string]interface{}{"email": email}).
		WithResult(true, ""))
	return &LoginResponse{User: user, Token: token}, nil
}

func validateRegisterInput(email, password string) string {
	if email == "" || len(email) > maxEmailLength || strings.ContainsAny(email, " \t\r\n") || !strings.Contains(email, "@") {
		return "INVALID_PARAM"
	}
	if strings.TrimSpace(password) == "" {
		return "INVALID_PASSWORD"
	}
	if len(password) < minPasswordLength || len(password) > maxPasswordLength {
		return "INVALID_PASSWORD"
	}
	return ""
}

// CreateApiKeyRequest 创建 API Key 请求
type CreateApiKeyRequest struct {
	UserID      int64
	Label       string
	Permissions int
	IPWhitelist []string
}

// CreateApiKeyResponse 创建 API Key 响应
type CreateApiKeyResponse struct {
	ApiKey    *repository.ApiKey
	Secret    string
	ErrorCode string
}

// CreateApiKey 创建 API Key
func (s *UserService) CreateApiKey(ctx context.Context, req *CreateApiKeyRequest) (*CreateApiKeyResponse, error) {
	now := time.Now().UnixMilli()
	apiKey := &repository.ApiKey{
		ApiKeyID:    s.idGen.NextID(),
		UserID:      req.UserID,
		Label:       req.Label,
		Permissions: req.Permissions,
		IPWhitelist: req.IPWhitelist,
		Status:      1,
		CreatedAtMs: now,
		UpdatedAtMs: now,
	}

	secret, err := s.repo.CreateApiKey(ctx, apiKey)
	if err != nil {
		return nil, err
	}

	s.writeAudit(ctx, audit.NewLog(audit.EventAPIKeyCreated, req.UserID).
		WithIP("").
		WithParams(map[string]interface{}{
			"apiKeyId":    apiKey.ApiKeyID,
			"label":       req.Label,
			"permissions": req.Permissions,
			"ipWhitelist": req.IPWhitelist,
		}).
		WithResult(true, ""))
	return &CreateApiKeyResponse{ApiKey: apiKey, Secret: secret}, nil
}

// ListApiKeys 列出 API Keys
func (s *UserService) ListApiKeys(ctx context.Context, userID int64) ([]*repository.ApiKey, error) {
	return s.repo.ListApiKeys(ctx, userID)
}

// DeleteApiKey 删除 API Key
func (s *UserService) DeleteApiKey(ctx context.Context, userID, apiKeyID int64) error {
	err := s.repo.DeleteApiKey(ctx, userID, apiKeyID)
	if err == nil {
		s.writeAudit(ctx, audit.NewLog(audit.EventAPIKeyDeleted, userID).
			WithIP("").
			WithParams(map[string]interface{}{"apiKeyId": apiKeyID}).
			WithResult(true, ""))
	}
	return err
}

// GetApiKeyInfo 获取 API Key 信息（用于网关鉴权）
func (s *UserService) GetApiKeyInfo(ctx context.Context, apiKey string) (secret string, userID int64, permissions int, ipWhitelist []string, err error) {
	key, err := s.repo.GetApiKeyByKey(ctx, apiKey)
	if err != nil {
		return "", 0, 0, nil, err
	}
	user, err := s.repo.GetUserByID(ctx, key.UserID)
	if err != nil {
		return "", 0, 0, nil, err
	}
	switch user.Status {
	case repository.UserStatusActive:
		// ok
	case repository.UserStatusFrozen:
		return "", 0, 0, nil, ErrUserFrozen
	case repository.UserStatusDisabled:
		return "", 0, 0, nil, ErrUserDisabled
	default:
		return "", 0, 0, nil, ErrUserDisabled
	}
	return key.SecretHash, key.UserID, key.Permissions, key.IPWhitelist, nil
}

// GetUser 获取用户
func (s *UserService) GetUser(ctx context.Context, userID int64) (*repository.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}

func (s *UserService) writeAudit(ctx context.Context, log *audit.AuditLog) {
	if s == nil || s.auditLogger == nil || log == nil {
		return
	}
	_ = s.auditLogger.Log(ctx, log)
}
