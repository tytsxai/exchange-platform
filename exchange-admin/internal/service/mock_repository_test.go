package service

import (
	"context"
	"fmt"

	"github.com/exchange/admin/internal/repository"
)

// mockRepository 模拟仓储实现
type mockRepository struct {
	// 交易对管理
	listSymbolConfigsFunc     func(ctx context.Context) ([]*repository.SymbolConfig, error)
	getSymbolConfigFunc       func(ctx context.Context, symbol string) (*repository.SymbolConfig, error)
	createSymbolConfigFunc    func(ctx context.Context, cfg *repository.SymbolConfig) error
	updateSymbolConfigFunc    func(ctx context.Context, cfg *repository.SymbolConfig) error
	updateSymbolStatusFunc    func(ctx context.Context, symbol string, status int) error
	updateAllSymbolStatusFunc func(ctx context.Context, status int) error

	// 审计日志
	createAuditLogFunc func(ctx context.Context, log *repository.AuditLog) error
	listAuditLogsFunc  func(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error)

	// RBAC
	listRolesFunc      func(ctx context.Context) ([]*repository.Role, error)
	getUserRolesFunc   func(ctx context.Context, userID int64) ([]int64, error)
	assignUserRoleFunc func(ctx context.Context, userID, roleID int64) error
	removeUserRoleFunc func(ctx context.Context, userID, roleID int64) error
}

func (m *mockRepository) ListSymbolConfigs(ctx context.Context) ([]*repository.SymbolConfig, error) {
	if m.listSymbolConfigsFunc != nil {
		return m.listSymbolConfigsFunc(ctx)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRepository) GetSymbolConfig(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
	if m.getSymbolConfigFunc != nil {
		return m.getSymbolConfigFunc(ctx, symbol)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRepository) CreateSymbolConfig(ctx context.Context, cfg *repository.SymbolConfig) error {
	if m.createSymbolConfigFunc != nil {
		return m.createSymbolConfigFunc(ctx, cfg)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockRepository) UpdateSymbolConfig(ctx context.Context, cfg *repository.SymbolConfig) error {
	if m.updateSymbolConfigFunc != nil {
		return m.updateSymbolConfigFunc(ctx, cfg)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockRepository) UpdateSymbolStatus(ctx context.Context, symbol string, status int) error {
	if m.updateSymbolStatusFunc != nil {
		return m.updateSymbolStatusFunc(ctx, symbol, status)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockRepository) UpdateAllSymbolStatus(ctx context.Context, status int) error {
	if m.updateAllSymbolStatusFunc != nil {
		return m.updateAllSymbolStatusFunc(ctx, status)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockRepository) CreateAuditLog(ctx context.Context, log *repository.AuditLog) error {
	if m.createAuditLogFunc != nil {
		return m.createAuditLogFunc(ctx, log)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockRepository) ListAuditLogs(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error) {
	if m.listAuditLogsFunc != nil {
		return m.listAuditLogsFunc(ctx, targetType, limit)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRepository) ListRoles(ctx context.Context) ([]*repository.Role, error) {
	if m.listRolesFunc != nil {
		return m.listRolesFunc(ctx)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRepository) GetUserRoles(ctx context.Context, userID int64) ([]int64, error) {
	if m.getUserRolesFunc != nil {
		return m.getUserRolesFunc(ctx, userID)
	}
	return nil, fmt.Errorf("not implemented")
}

func (m *mockRepository) AssignUserRole(ctx context.Context, userID, roleID int64) error {
	if m.assignUserRoleFunc != nil {
		return m.assignUserRoleFunc(ctx, userID, roleID)
	}
	return fmt.Errorf("not implemented")
}

func (m *mockRepository) RemoveUserRole(ctx context.Context, userID, roleID int64) error {
	if m.removeUserRoleFunc != nil {
		return m.removeUserRoleFunc(ctx, userID, roleID)
	}
	return fmt.Errorf("not implemented")
}
