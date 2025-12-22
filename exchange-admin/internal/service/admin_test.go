package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/exchange/admin/internal/repository"
)

// ========== 测试辅助工具 ==========

type mockIDGenerator struct {
	nextID int64
}

func (m *mockIDGenerator) NextID() int64 {
	m.nextID++
	return m.nextID
}

// ========== 常量测试 ==========

func TestStatusConstants(t *testing.T) {
	if StatusTrading != 1 {
		t.Fatalf("expected StatusTrading=1, got %d", StatusTrading)
	}
	if StatusHalt != 2 {
		t.Fatalf("expected StatusHalt=2, got %d", StatusHalt)
	}
	if StatusCancelOnly != 3 {
		t.Fatalf("expected StatusCancelOnly=3, got %d", StatusCancelOnly)
	}
}

// ========== 交易对管理测试 ==========

func TestListSymbols_Success(t *testing.T) {
	mockRepo := &mockRepository{
		listSymbolConfigsFunc: func(ctx context.Context) ([]*repository.SymbolConfig, error) {
			return []*repository.SymbolConfig{
				{Symbol: "BTCUSDT", Status: StatusTrading},
				{Symbol: "ETHUSDT", Status: StatusHalt},
			}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	symbols, err := svc.ListSymbols(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(symbols) != 2 {
		t.Fatalf("expected 2 symbols, got %d", len(symbols))
	}
	if symbols[0].Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT, got %s", symbols[0].Symbol)
	}
}

func TestListSymbols_Error(t *testing.T) {
	mockRepo := &mockRepository{
		listSymbolConfigsFunc: func(ctx context.Context) ([]*repository.SymbolConfig, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	_, err := svc.ListSymbols(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetSymbol_Success(t *testing.T) {
	mockRepo := &mockRepository{
		getSymbolConfigFunc: func(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
			if symbol == "BTCUSDT" {
				return &repository.SymbolConfig{Symbol: "BTCUSDT", Status: StatusTrading}, nil
			}
			return nil, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg, err := svc.GetSymbol(context.Background(), "BTCUSDT")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Symbol != "BTCUSDT" {
		t.Fatalf("expected BTCUSDT, got %s", cfg.Symbol)
	}
}

func TestGetSymbol_NotFound(t *testing.T) {
	mockRepo := &mockRepository{
		getSymbolConfigFunc: func(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
			return nil, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg, err := svc.GetSymbol(context.Background(), "UNKNOWN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil config")
	}
}

func TestCreateSymbol_Success(t *testing.T) {
	var capturedCfg *repository.SymbolConfig
	var capturedLog *repository.AuditLog

	mockRepo := &mockRepository{
		createSymbolConfigFunc: func(ctx context.Context, cfg *repository.SymbolConfig) error {
			capturedCfg = cfg
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			capturedLog = log
			return nil
		},
	}
	idGen := &mockIDGenerator{}
	svc := NewAdminService(mockRepo, idGen)

	cfg := &repository.SymbolConfig{Symbol: "BTCUSDT", Status: 0}
	err := svc.CreateSymbol(context.Background(), 100, "192.168.1.1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证默认状态
	if capturedCfg.Status != StatusHalt {
		t.Fatalf("expected default status=%d, got %d", StatusHalt, capturedCfg.Status)
	}

	// 验证审计日志
	if capturedLog == nil {
		t.Fatal("expected audit log to be created")
	}
	if capturedLog.Action != "CREATE_SYMBOL" {
		t.Fatalf("expected action=CREATE_SYMBOL, got %s", capturedLog.Action)
	}
	if capturedLog.ActorUserID != 100 {
		t.Fatalf("expected actorUserID=100, got %d", capturedLog.ActorUserID)
	}
	if capturedLog.IP != "192.168.1.1" {
		t.Fatalf("expected IP=192.168.1.1, got %s", capturedLog.IP)
	}
}

func TestCreateSymbol_WithStatus(t *testing.T) {
	var capturedCfg *repository.SymbolConfig

	mockRepo := &mockRepository{
		createSymbolConfigFunc: func(ctx context.Context, cfg *repository.SymbolConfig) error {
			capturedCfg = cfg
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg := &repository.SymbolConfig{Symbol: "BTCUSDT", Status: StatusTrading}
	err := svc.CreateSymbol(context.Background(), 100, "192.168.1.1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证状态未被覆盖
	if capturedCfg.Status != StatusTrading {
		t.Fatalf("expected status=%d, got %d", StatusTrading, capturedCfg.Status)
	}
}

func TestCreateSymbol_RepositoryError(t *testing.T) {
	mockRepo := &mockRepository{
		createSymbolConfigFunc: func(ctx context.Context, cfg *repository.SymbolConfig) error {
			return fmt.Errorf("duplicate key")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg := &repository.SymbolConfig{Symbol: "BTCUSDT"}
	err := svc.CreateSymbol(context.Background(), 100, "192.168.1.1", cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestUpdateSymbol_Success(t *testing.T) {
	var capturedLog *repository.AuditLog

	mockRepo := &mockRepository{
		getSymbolConfigFunc: func(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
			return &repository.SymbolConfig{Symbol: "BTCUSDT", Status: StatusHalt}, nil
		},
		updateSymbolConfigFunc: func(ctx context.Context, cfg *repository.SymbolConfig) error {
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			capturedLog = log
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg := &repository.SymbolConfig{Symbol: "BTCUSDT", Status: StatusTrading}
	err := svc.UpdateSymbol(context.Background(), 100, "192.168.1.1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证审计日志包含 before 和 after
	if capturedLog == nil {
		t.Fatal("expected audit log to be created")
	}
	if capturedLog.Action != "UPDATE_SYMBOL" {
		t.Fatalf("expected action=UPDATE_SYMBOL, got %s", capturedLog.Action)
	}
	if len(capturedLog.BeforeJSON) == 0 {
		t.Fatal("expected beforeJSON to be set")
	}
	if len(capturedLog.AfterJSON) == 0 {
		t.Fatal("expected afterJSON to be set")
	}
}

func TestUpdateSymbol_RepositoryError(t *testing.T) {
	mockRepo := &mockRepository{
		getSymbolConfigFunc: func(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
			return &repository.SymbolConfig{Symbol: "BTCUSDT"}, nil
		},
		updateSymbolConfigFunc: func(ctx context.Context, cfg *repository.SymbolConfig) error {
			return fmt.Errorf("not found")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg := &repository.SymbolConfig{Symbol: "BTCUSDT"}
	err := svc.UpdateSymbol(context.Background(), 100, "192.168.1.1", cfg)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ========== Kill Switch 测试 ==========

func TestSetSymbolStatus_Success(t *testing.T) {
	var capturedSymbol string
	var capturedStatus int

	mockRepo := &mockRepository{
		getSymbolConfigFunc: func(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
			return &repository.SymbolConfig{Symbol: symbol, Status: StatusTrading}, nil
		},
		updateSymbolStatusFunc: func(ctx context.Context, symbol string, status int) error {
			capturedSymbol = symbol
			capturedStatus = status
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.SetSymbolStatus(context.Background(), 100, "192.168.1.1", "BTCUSDT", StatusHalt)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedSymbol != "BTCUSDT" {
		t.Fatalf("expected symbol=BTCUSDT, got %s", capturedSymbol)
	}
	if capturedStatus != StatusHalt {
		t.Fatalf("expected status=%d, got %d", StatusHalt, capturedStatus)
	}
}

func TestSetSymbolStatus_Error(t *testing.T) {
	mockRepo := &mockRepository{
		getSymbolConfigFunc: func(ctx context.Context, symbol string) (*repository.SymbolConfig, error) {
			return &repository.SymbolConfig{Symbol: symbol, Status: StatusTrading}, nil
		},
		updateSymbolStatusFunc: func(ctx context.Context, symbol string, status int) error {
			return fmt.Errorf("update failed")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.SetSymbolStatus(context.Background(), 100, "192.168.1.1", "BTCUSDT", StatusHalt)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGlobalHalt_Success(t *testing.T) {
	var capturedStatus int
	var capturedLog *repository.AuditLog

	mockRepo := &mockRepository{
		updateAllSymbolStatusFunc: func(ctx context.Context, status int) error {
			capturedStatus = status
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			capturedLog = log
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.GlobalHalt(context.Background(), 100, "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStatus != StatusHalt {
		t.Fatalf("expected status=%d, got %d", StatusHalt, capturedStatus)
	}

	if capturedLog.Action != "GLOBAL_HALT" {
		t.Fatalf("expected action=GLOBAL_HALT, got %s", capturedLog.Action)
	}
	if capturedLog.TargetType != "SYSTEM" {
		t.Fatalf("expected targetType=SYSTEM, got %s", capturedLog.TargetType)
	}
}

func TestGlobalHalt_Error(t *testing.T) {
	mockRepo := &mockRepository{
		updateAllSymbolStatusFunc: func(ctx context.Context, status int) error {
			return fmt.Errorf("update failed")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.GlobalHalt(context.Background(), 100, "192.168.1.1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGlobalCancelOnly_Success(t *testing.T) {
	var capturedStatus int

	mockRepo := &mockRepository{
		updateAllSymbolStatusFunc: func(ctx context.Context, status int) error {
			capturedStatus = status
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.GlobalCancelOnly(context.Background(), 100, "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStatus != StatusCancelOnly {
		t.Fatalf("expected status=%d, got %d", StatusCancelOnly, capturedStatus)
	}
}

func TestGlobalCancelOnly_Error(t *testing.T) {
	mockRepo := &mockRepository{
		updateAllSymbolStatusFunc: func(ctx context.Context, status int) error {
			return fmt.Errorf("update failed")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.GlobalCancelOnly(context.Background(), 100, "192.168.1.1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGlobalResume_Success(t *testing.T) {
	var capturedStatus int

	mockRepo := &mockRepository{
		updateAllSymbolStatusFunc: func(ctx context.Context, status int) error {
			capturedStatus = status
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.GlobalResume(context.Background(), 100, "192.168.1.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedStatus != StatusTrading {
		t.Fatalf("expected status=%d, got %d", StatusTrading, capturedStatus)
	}
}

func TestGlobalResume_Error(t *testing.T) {
	mockRepo := &mockRepository{
		updateAllSymbolStatusFunc: func(ctx context.Context, status int) error {
			return fmt.Errorf("update failed")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.GlobalResume(context.Background(), 100, "192.168.1.1")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ========== 审计日志测试 ==========

func TestListAuditLogs_Success(t *testing.T) {
	mockRepo := &mockRepository{
		listAuditLogsFunc: func(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error) {
			return []*repository.AuditLog{
				{AuditID: 1, Action: "CREATE_SYMBOL"},
				{AuditID: 2, Action: "UPDATE_SYMBOL"},
			}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	logs, err := svc.ListAuditLogs(context.Background(), "SYMBOL", 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(logs) != 2 {
		t.Fatalf("expected 2 logs, got %d", len(logs))
	}
}

func TestListAuditLogs_DefaultLimit(t *testing.T) {
	var capturedLimit int

	mockRepo := &mockRepository{
		listAuditLogsFunc: func(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error) {
			capturedLimit = limit
			return []*repository.AuditLog{}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	// 测试 limit <= 0
	svc.ListAuditLogs(context.Background(), "", 0)
	if capturedLimit != 100 {
		t.Fatalf("expected default limit=100, got %d", capturedLimit)
	}

	// 测试 limit > 1000
	svc.ListAuditLogs(context.Background(), "", 2000)
	if capturedLimit != 100 {
		t.Fatalf("expected capped limit=100, got %d", capturedLimit)
	}

	// 测试正常 limit
	svc.ListAuditLogs(context.Background(), "", 50)
	if capturedLimit != 50 {
		t.Fatalf("expected limit=50, got %d", capturedLimit)
	}
}

func TestListAuditLogs_Error(t *testing.T) {
	mockRepo := &mockRepository{
		listAuditLogsFunc: func(ctx context.Context, targetType string, limit int) ([]*repository.AuditLog, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	_, err := svc.ListAuditLogs(context.Background(), "", 10)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ========== RBAC 测试 ==========

func TestListRoles_Success(t *testing.T) {
	mockRepo := &mockRepository{
		listRolesFunc: func(ctx context.Context) ([]*repository.Role, error) {
			return []*repository.Role{
				{RoleID: 1, Name: "Admin"},
				{RoleID: 2, Name: "Operator"},
			}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	roles, err := svc.ListRoles(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roles) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roles))
	}
}

func TestListRoles_Error(t *testing.T) {
	mockRepo := &mockRepository{
		listRolesFunc: func(ctx context.Context) ([]*repository.Role, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	_, err := svc.ListRoles(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestGetUserRoles_Success(t *testing.T) {
	mockRepo := &mockRepository{
		getUserRolesFunc: func(ctx context.Context, userID int64) ([]int64, error) {
			return []int64{1, 2}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	roleIDs, err := svc.GetUserRoles(context.Background(), 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(roleIDs) != 2 {
		t.Fatalf("expected 2 role IDs, got %d", len(roleIDs))
	}
}

func TestGetUserRoles_Error(t *testing.T) {
	mockRepo := &mockRepository{
		getUserRolesFunc: func(ctx context.Context, userID int64) ([]int64, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	_, err := svc.GetUserRoles(context.Background(), 100)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestAssignRole_Success(t *testing.T) {
	var capturedUserID, capturedRoleID int64

	mockRepo := &mockRepository{
		assignUserRoleFunc: func(ctx context.Context, userID, roleID int64) error {
			capturedUserID = userID
			capturedRoleID = roleID
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.AssignRole(context.Background(), 100, "192.168.1.1", 200, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedUserID != 200 {
		t.Fatalf("expected userID=200, got %d", capturedUserID)
	}
	if capturedRoleID != 1 {
		t.Fatalf("expected roleID=1, got %d", capturedRoleID)
	}
}

func TestAssignRole_Error(t *testing.T) {
	mockRepo := &mockRepository{
		assignUserRoleFunc: func(ctx context.Context, userID, roleID int64) error {
			return fmt.Errorf("constraint violation")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.AssignRole(context.Background(), 100, "192.168.1.1", 200, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestRemoveRole_Success(t *testing.T) {
	var capturedUserID, capturedRoleID int64

	mockRepo := &mockRepository{
		removeUserRoleFunc: func(ctx context.Context, userID, roleID int64) error {
			capturedUserID = userID
			capturedRoleID = roleID
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.RemoveRole(context.Background(), 100, "192.168.1.1", 200, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if capturedUserID != 200 {
		t.Fatalf("expected userID=200, got %d", capturedUserID)
	}
	if capturedRoleID != 1 {
		t.Fatalf("expected roleID=1, got %d", capturedRoleID)
	}
}

func TestRemoveRole_Error(t *testing.T) {
	mockRepo := &mockRepository{
		removeUserRoleFunc: func(ctx context.Context, userID, roleID int64) error {
			return fmt.Errorf("not found")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	err := svc.RemoveRole(context.Background(), 100, "192.168.1.1", 200, 1)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ========== 系统状态测试 ==========

func TestGetSystemStatus_Success(t *testing.T) {
	mockRepo := &mockRepository{
		listSymbolConfigsFunc: func(ctx context.Context) ([]*repository.SymbolConfig, error) {
			return []*repository.SymbolConfig{
				{Symbol: "BTCUSDT", Status: StatusTrading},
				{Symbol: "ETHUSDT", Status: StatusTrading},
				{Symbol: "BNBUSDT", Status: StatusHalt},
				{Symbol: "ADAUSDT", Status: StatusCancelOnly},
			}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	status, err := svc.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.TradingSymbols != 2 {
		t.Fatalf("expected 2 trading symbols, got %d", status.TradingSymbols)
	}
	if status.HaltedSymbols != 1 {
		t.Fatalf("expected 1 halted symbol, got %d", status.HaltedSymbols)
	}
	if status.CancelOnlySymbols != 1 {
		t.Fatalf("expected 1 cancel-only symbol, got %d", status.CancelOnlySymbols)
	}
	if status.ServerTimeMs == 0 {
		t.Fatal("expected non-zero server time")
	}
}

func TestGetSystemStatus_EmptyList(t *testing.T) {
	mockRepo := &mockRepository{
		listSymbolConfigsFunc: func(ctx context.Context) ([]*repository.SymbolConfig, error) {
			return []*repository.SymbolConfig{}, nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	status, err := svc.GetSystemStatus(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if status.TradingSymbols != 0 {
		t.Fatalf("expected 0 trading symbols, got %d", status.TradingSymbols)
	}
}

func TestGetSystemStatus_Error(t *testing.T) {
	mockRepo := &mockRepository{
		listSymbolConfigsFunc: func(ctx context.Context) ([]*repository.SymbolConfig, error) {
			return nil, fmt.Errorf("database error")
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	_, err := svc.GetSystemStatus(context.Background())
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ========== 审计日志 JSON 序列化测试 ==========

func TestAuditLogJSONSerialization(t *testing.T) {
	var capturedLog *repository.AuditLog

	mockRepo := &mockRepository{
		createSymbolConfigFunc: func(ctx context.Context, cfg *repository.SymbolConfig) error {
			return nil
		},
		createAuditLogFunc: func(ctx context.Context, log *repository.AuditLog) error {
			capturedLog = log
			return nil
		},
	}
	svc := NewAdminService(mockRepo, &mockIDGenerator{})

	cfg := &repository.SymbolConfig{
		Symbol:     "BTCUSDT",
		BaseAsset:  "BTC",
		QuoteAsset: "USDT",
		Status:     StatusTrading,
	}

	err := svc.CreateSymbol(context.Background(), 100, "192.168.1.1", cfg)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 验证 JSON 可以反序列化
	var afterData map[string]interface{}
	if err := json.Unmarshal(capturedLog.AfterJSON, &afterData); err != nil {
		t.Fatalf("failed to unmarshal afterJSON: %v", err)
	}

	if afterData["symbol"] != "BTCUSDT" {
		t.Fatalf("expected symbol=BTCUSDT in JSON, got %v", afterData["symbol"])
	}
}

// ========== ID 生成器测试 ==========

func TestIDGenerator(t *testing.T) {
	idGen := &mockIDGenerator{}

	id1 := idGen.NextID()
	id2 := idGen.NextID()

	if id1 >= id2 {
		t.Fatalf("expected id1 < id2, got id1=%d, id2=%d", id1, id2)
	}
}

func TestNewAdminService(t *testing.T) {
	repo := &mockRepository{}
	idGen := &mockIDGenerator{}

	svc := NewAdminService(repo, idGen)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}
