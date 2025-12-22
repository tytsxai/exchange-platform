// Package service 用户服务
package service

import (
	"context"
	"time"

	"github.com/exchange/user/internal/repository"
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
	repo  UserRepository
	idGen IDGenerator
}

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// NewUserService 创建用户服务
func NewUserService(repo UserRepository, idGen IDGenerator) *UserService {
	return &UserService{
		repo:  repo,
		idGen: idGen,
	}
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
	now := time.Now().UnixMilli()
	user := &repository.User{
		UserID:      s.idGen.NextID(),
		Email:       req.Email,
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
	user, err := s.repo.GetUserByEmail(ctx, req.Email)
	if err != nil {
		if err == repository.ErrUserNotFound {
			return &LoginResponse{ErrorCode: "INVALID_CREDENTIALS"}, nil
		}
		return nil, err
	}

	if !s.repo.VerifyPassword(user, req.Password) {
		return &LoginResponse{ErrorCode: "INVALID_CREDENTIALS"}, nil
	}

	if user.Status != repository.UserStatusActive {
		return &LoginResponse{ErrorCode: "USER_FROZEN"}, nil
	}

	// 生成 token（简化实现）
	token := "token_" + string(rune(user.UserID))

	return &LoginResponse{User: user, Token: token}, nil
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

	return &CreateApiKeyResponse{ApiKey: apiKey, Secret: secret}, nil
}

// ListApiKeys 列出 API Keys
func (s *UserService) ListApiKeys(ctx context.Context, userID int64) ([]*repository.ApiKey, error) {
	return s.repo.ListApiKeys(ctx, userID)
}

// DeleteApiKey 删除 API Key
func (s *UserService) DeleteApiKey(ctx context.Context, userID, apiKeyID int64) error {
	return s.repo.DeleteApiKey(ctx, userID, apiKeyID)
}

// GetApiKeyInfo 获取 API Key 信息（用于网关鉴权）
func (s *UserService) GetApiKeyInfo(ctx context.Context, apiKey string) (secret string, userID int64, permissions int, err error) {
	key, err := s.repo.GetApiKeyByKey(ctx, apiKey)
	if err != nil {
		return "", 0, 0, err
	}
	return key.SecretHash, key.UserID, key.Permissions, nil
}

// GetUser 获取用户
func (s *UserService) GetUser(ctx context.Context, userID int64) (*repository.User, error) {
	return s.repo.GetUserByID(ctx, userID)
}
