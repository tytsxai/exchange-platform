// Package service 后台服务
package service

import (
	"context"

	"github.com/exchange/admin/internal/repository"
)

// AdminRepository 后台仓储接口
type AdminRepository interface {
	// 交易对管理
	ListSymbolConfigs(ctx context.Context) ([]*repository.SymbolConfig, error)
	GetSymbolConfig(ctx context.Context, symbol string) (*repository.SymbolConfig, error)
	CreateSymbolConfig(ctx context.Context, cfg *repository.SymbolConfig) error
	UpdateSymbolConfig(ctx context.Context, cfg *repository.SymbolConfig) error
	UpdateSymbolStatus(ctx context.Context, symbol string, status int) error
	UpdateAllSymbolStatus(ctx context.Context, status int) error

	// 审计日志
	CreateAuditLog(ctx context.Context, log *repository.AuditLog) error
	ListAuditLogs(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error)

	// RBAC
	ListRoles(ctx context.Context) ([]*repository.Role, error)
	GetUserRoles(ctx context.Context, userID int64) ([]int64, error)
	AssignUserRole(ctx context.Context, userID, roleID int64) error
	RemoveUserRole(ctx context.Context, userID, roleID int64) error
}
