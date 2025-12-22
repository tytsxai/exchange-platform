// Package service 后台服务
package service

import (
	"context"
	"encoding/json"
	"strconv"
	"time"

	"github.com/exchange/admin/internal/repository"
)

// AdminService 后台服务
type AdminService struct {
	repo  AdminRepository
	idGen IDGenerator
}

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// NewAdminService 创建后台服务
func NewAdminService(repo AdminRepository, idGen IDGenerator) *AdminService {
	return &AdminService{
		repo:  repo,
		idGen: idGen,
	}
}

// ========== 交易对管理 ==========

// ListSymbols 列出交易对
func (s *AdminService) ListSymbols(ctx context.Context) ([]*repository.SymbolConfig, error) {
	return s.repo.ListSymbolConfigs(ctx)
}

// GetSymbol 获取交易对
func (s *AdminService) GetSymbol(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
	return s.repo.GetSymbolConfig(ctx, symbol)
}

// CreateSymbol 创建交易对
func (s *AdminService) CreateSymbol(ctx context.Context, actorID int64, ip string, cfg *repository.SymbolConfig) error {
	if cfg.Status == 0 {
		cfg.Status = 2 // 默认 HALT
	}

	if err := s.repo.CreateSymbolConfig(ctx, cfg); err != nil {
		return err
	}

	// 审计日志
	afterJSON, _ := json.Marshal(cfg)
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "CREATE_SYMBOL",
		TargetType:  "SYMBOL",
		TargetID:    cfg.Symbol,
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// UpdateSymbol 更新交易对
func (s *AdminService) UpdateSymbol(ctx context.Context, actorID int64, ip string, cfg *repository.SymbolConfig) error {
	// 获取旧配置
	before, _ := s.repo.GetSymbolConfig(ctx, cfg.Symbol)
	beforeJSON, _ := json.Marshal(before)

	if err := s.repo.UpdateSymbolConfig(ctx, cfg); err != nil {
		return err
	}

	// 审计日志
	afterJSON, _ := json.Marshal(cfg)
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "UPDATE_SYMBOL",
		TargetType:  "SYMBOL",
		TargetID:    cfg.Symbol,
		BeforeJSON:  beforeJSON,
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// ========== Kill Switch ==========

// SymbolStatus 交易对状态
const (
	StatusTrading    = 1
	StatusHalt       = 2
	StatusCancelOnly = 3
)

// SetSymbolStatus 设置交易对状态
func (s *AdminService) SetSymbolStatus(ctx context.Context, actorID int64, ip, symbol string, status int) error {
	before, _ := s.repo.GetSymbolConfig(ctx, symbol)

	if err := s.repo.UpdateSymbolStatus(ctx, symbol, status); err != nil {
		return err
	}

	// 审计日志
	beforeStatus := 0
	if before != nil {
		beforeStatus = before.Status
	}
	beforeJSON, _ := json.Marshal(map[string]interface{}{"status": beforeStatus})
	afterJSON, _ := json.Marshal(map[string]interface{}{"status": status})
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "SET_SYMBOL_STATUS",
		TargetType:  "SYMBOL",
		TargetID:    symbol,
		BeforeJSON:  beforeJSON,
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// GlobalHalt 全局暂停交易
func (s *AdminService) GlobalHalt(ctx context.Context, actorID int64, ip string) error {
	if err := s.repo.UpdateAllSymbolStatus(ctx, StatusHalt); err != nil {
		return err
	}

	// 审计日志
	afterJSON, _ := json.Marshal(map[string]interface{}{"status": StatusHalt, "scope": "ALL"})
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "GLOBAL_HALT",
		TargetType:  "SYSTEM",
		TargetID:    "ALL_SYMBOLS",
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// GlobalCancelOnly 全局只允许撤单
func (s *AdminService) GlobalCancelOnly(ctx context.Context, actorID int64, ip string) error {
	if err := s.repo.UpdateAllSymbolStatus(ctx, StatusCancelOnly); err != nil {
		return err
	}

	// 审计日志
	afterJSON, _ := json.Marshal(map[string]interface{}{"status": StatusCancelOnly, "scope": "ALL"})
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "GLOBAL_CANCEL_ONLY",
		TargetType:  "SYSTEM",
		TargetID:    "ALL_SYMBOLS",
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// GlobalResume 全局恢复交易
func (s *AdminService) GlobalResume(ctx context.Context, actorID int64, ip string) error {
	if err := s.repo.UpdateAllSymbolStatus(ctx, StatusTrading); err != nil {
		return err
	}

	// 审计日志
	afterJSON, _ := json.Marshal(map[string]interface{}{"status": StatusTrading, "scope": "ALL"})
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "GLOBAL_RESUME",
		TargetType:  "SYSTEM",
		TargetID:    "ALL_SYMBOLS",
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// ========== 审计日志 ==========

// ListAuditLogs 查询审计日志
func (s *AdminService) ListAuditLogs(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error) {
	if limit <= 0 || limit > 1000 {
		limit = 100
	}
	return s.repo.ListAuditLogs(ctx, targetType, limit)
}

// ========== RBAC ==========

// ListRoles 列出角色
func (s *AdminService) ListRoles(ctx context.Context) ([]*repository.Role, error) {
	return s.repo.ListRoles(ctx)
}

// GetUserRoles 获取用户角色
func (s *AdminService) GetUserRoles(ctx context.Context, userID int64) ([]int64, error) {
	return s.repo.GetUserRoles(ctx, userID)
}

// AssignRole 分配角色
func (s *AdminService) AssignRole(ctx context.Context, actorID int64, ip string, userID, roleID int64) error {
	if err := s.repo.AssignUserRole(ctx, userID, roleID); err != nil {
		return err
	}

	// 审计日志
	afterJSON, _ := json.Marshal(map[string]interface{}{"userId": userID, "roleId": roleID})
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "ASSIGN_ROLE",
		TargetType:  "USER_ROLE",
		TargetID:    strconv.FormatInt(userID, 10),
		AfterJSON:   afterJSON,
		IP:          ip,
	})

	return nil
}

// RemoveRole 移除角色
func (s *AdminService) RemoveRole(ctx context.Context, actorID int64, ip string, userID, roleID int64) error {
	if err := s.repo.RemoveUserRole(ctx, userID, roleID); err != nil {
		return err
	}

	// 审计日志
	beforeJSON, _ := json.Marshal(map[string]interface{}{"userId": userID, "roleId": roleID})
	s.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     s.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "REMOVE_ROLE",
		TargetType:  "USER_ROLE",
		TargetID:    strconv.FormatInt(userID, 10),
		BeforeJSON:  beforeJSON,
		IP:          ip,
	})

	return nil
}

// ========== 系统状态 ==========

// SystemStatus 系统状态
type SystemStatus struct {
	TradingSymbols    int   `json:"tradingSymbols"`
	HaltedSymbols     int   `json:"haltedSymbols"`
	CancelOnlySymbols int   `json:"cancelOnlySymbols"`
	ServerTimeMs      int64 `json:"serverTimeMs"`
}

// GetSystemStatus 获取系统状态
func (s *AdminService) GetSystemStatus(ctx context.Context) (*SystemStatus, error) {
	symbols, err := s.repo.ListSymbolConfigs(ctx)
	if err != nil {
		return nil, err
	}

	status := &SystemStatus{
		ServerTimeMs: time.Now().UnixMilli(),
	}

	for _, sym := range symbols {
		switch sym.Status {
		case StatusTrading:
			status.TradingSymbols++
		case StatusHalt:
			status.HaltedSymbols++
		case StatusCancelOnly:
			status.CancelOnlySymbols++
		}
	}

	return status, nil
}
