package config

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/exchange/admin/internal/repository"
)

type symbolRepo interface {
	GetSymbolConfig(ctx context.Context, symbol string) (*repository.SymbolConfig, error)
	UpdateSymbolStatus(ctx context.Context, symbol string, status int) error
	UpdateSymbolConfig(ctx context.Context, c *repository.SymbolConfig) error
	CreateAuditLog(ctx context.Context, log *repository.AuditLog) error
}

type idGenerator interface {
	NextID() int64
}

// SymbolManager 交易对配置管理
type SymbolManager struct {
	repo  symbolRepo
	idGen idGenerator
}

func NewSymbolManager(repo symbolRepo, idGen idGenerator) *SymbolManager {
	return &SymbolManager{repo: repo, idGen: idGen}
}

// UpdateSymbolStatus 上下架/状态切换交易对
func (m *SymbolManager) UpdateSymbolStatus(ctx context.Context, actorID int64, ip, symbol string, status int) error {
	before, err := m.repo.GetSymbolConfig(ctx, symbol)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("symbol not found")
	}
	if err := m.repo.UpdateSymbolStatus(ctx, symbol, status); err != nil {
		return err
	}

	beforeStatus := before.Status
	beforeJSON, _ := json.Marshal(map[string]any{"status": beforeStatus})
	afterJSON, _ := json.Marshal(map[string]any{"status": status})
	_ = m.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     m.idGen.NextID(),
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

// UpdateFeeRate 更新交易对费率
func (m *SymbolManager) UpdateFeeRate(ctx context.Context, actorID int64, ip, symbol string, makerFeeRate, takerFeeRate float64) error {
	if makerFeeRate < 0 || takerFeeRate < 0 {
		return fmt.Errorf("fee rate must be >= 0")
	}
	before, err := m.repo.GetSymbolConfig(ctx, symbol)
	if err != nil {
		return err
	}
	if before == nil {
		return fmt.Errorf("symbol not found")
	}

	after := *before
	after.MakerFeeRate = makerFeeRate
	after.TakerFeeRate = takerFeeRate
	if err := m.repo.UpdateSymbolConfig(ctx, &after); err != nil {
		return err
	}

	beforeJSON, _ := json.Marshal(map[string]any{
		"makerFeeRate": before.MakerFeeRate,
		"takerFeeRate": before.TakerFeeRate,
	})
	afterJSON, _ := json.Marshal(map[string]any{
		"makerFeeRate": makerFeeRate,
		"takerFeeRate": takerFeeRate,
	})
	_ = m.repo.CreateAuditLog(ctx, &repository.AuditLog{
		AuditID:     m.idGen.NextID(),
		ActorUserID: actorID,
		Action:      "UPDATE_FEE_RATE",
		TargetType:  "SYMBOL",
		TargetID:    symbol,
		BeforeJSON:  beforeJSON,
		AfterJSON:   afterJSON,
		IP:          ip,
	})
	return nil
}
