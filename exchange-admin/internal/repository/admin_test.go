package repository

import (
	"encoding/json"
	"testing"
)

func TestSymbolConfigStruct(t *testing.T) {
	cfg := &SymbolConfig{
		Symbol:         "BTCUSDT",
		BaseAsset:      "BTC",
		QuoteAsset:     "USDT",
		PriceTick:      1,
		QtyStep:        1,
		PricePrecision: 2,
		QtyPrecision:   3,
		MinQty:         1,
		MaxQty:         10000,
		MinNotional:    1000,
		PriceLimitRate: 0.1,
		MakerFeeRate:   0.001,
		TakerFeeRate:   0.001,
		Status:         1,
		CreatedAtMs:    1000,
		UpdatedAtMs:    2000,
	}

	if cfg.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", cfg.Symbol)
	}
	if cfg.BaseAsset != "BTC" {
		t.Fatalf("expected BaseAsset=BTC, got %s", cfg.BaseAsset)
	}
	if cfg.QuoteAsset != "USDT" {
		t.Fatalf("expected QuoteAsset=USDT, got %s", cfg.QuoteAsset)
	}
	if cfg.PricePrecision != 2 {
		t.Fatalf("expected PricePrecision=2, got %d", cfg.PricePrecision)
	}
	if cfg.Status != 1 {
		t.Fatalf("expected Status=1, got %d", cfg.Status)
	}
}

func TestSymbolConfigJSON(t *testing.T) {
	cfg := &SymbolConfig{
		Symbol:     "BTCUSDT",
		BaseAsset:  "BTC",
		QuoteAsset: "USDT",
		Status:     1,
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded SymbolConfig
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Symbol != "BTCUSDT" {
		t.Fatalf("expected Symbol=BTCUSDT, got %s", decoded.Symbol)
	}
}

func TestAuditLogStruct(t *testing.T) {
	log := &AuditLog{
		AuditID:     1,
		ActorUserID: 100,
		Action:      "CREATE_SYMBOL",
		TargetType:  "SYMBOL",
		TargetID:    "BTCUSDT",
		BeforeJSON:  json.RawMessage(`{"status":1}`),
		AfterJSON:   json.RawMessage(`{"status":2}`),
		IP:          "192.168.1.1",
		CreatedAtMs: 1000,
	}

	if log.AuditID != 1 {
		t.Fatalf("expected AuditID=1, got %d", log.AuditID)
	}
	if log.Action != "CREATE_SYMBOL" {
		t.Fatalf("expected Action=CREATE_SYMBOL, got %s", log.Action)
	}
	if log.TargetType != "SYMBOL" {
		t.Fatalf("expected TargetType=SYMBOL, got %s", log.TargetType)
	}
	if log.IP != "192.168.1.1" {
		t.Fatalf("expected IP=192.168.1.1, got %s", log.IP)
	}
}

func TestAuditLogJSON(t *testing.T) {
	log := &AuditLog{
		AuditID:    1,
		Action:     "UPDATE_SYMBOL",
		TargetType: "SYMBOL",
		TargetID:   "BTCUSDT",
	}

	data, err := json.Marshal(log)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded AuditLog
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Action != "UPDATE_SYMBOL" {
		t.Fatalf("expected Action=UPDATE_SYMBOL, got %s", decoded.Action)
	}
}

func TestRoleStruct(t *testing.T) {
	role := &Role{
		RoleID:      1,
		Name:        "admin",
		Permissions: []string{"read", "write", "delete"},
		CreatedAtMs: 1000,
		UpdatedAtMs: 2000,
	}

	if role.RoleID != 1 {
		t.Fatalf("expected RoleID=1, got %d", role.RoleID)
	}
	if role.Name != "admin" {
		t.Fatalf("expected Name=admin, got %s", role.Name)
	}
	if len(role.Permissions) != 3 {
		t.Fatalf("expected 3 permissions, got %d", len(role.Permissions))
	}
}

func TestRoleJSON(t *testing.T) {
	role := &Role{
		RoleID:      1,
		Name:        "trader",
		Permissions: []string{"read", "trade"},
	}

	data, err := json.Marshal(role)
	if err != nil {
		t.Fatalf("failed to marshal: %v", err)
	}

	var decoded Role
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("failed to unmarshal: %v", err)
	}

	if decoded.Name != "trader" {
		t.Fatalf("expected Name=trader, got %s", decoded.Name)
	}
}

func TestNewAdminRepository(t *testing.T) {
	repo := NewAdminRepository(nil)
	if repo == nil {
		t.Fatal("expected non-nil repository")
	}
}

func TestSymbolStatusValues(t *testing.T) {
	// Status: 1=TRADING, 2=HALT, 3=CANCEL_ONLY
	cfg := &SymbolConfig{Status: 1}
	if cfg.Status != 1 {
		t.Fatalf("expected Status=1 (TRADING), got %d", cfg.Status)
	}

	cfg.Status = 2
	if cfg.Status != 2 {
		t.Fatalf("expected Status=2 (HALT), got %d", cfg.Status)
	}

	cfg.Status = 3
	if cfg.Status != 3 {
		t.Fatalf("expected Status=3 (CANCEL_ONLY), got %d", cfg.Status)
	}
}

func TestAuditLogActions(t *testing.T) {
	actions := []string{
		"CREATE_SYMBOL",
		"UPDATE_SYMBOL",
		"SET_SYMBOL_STATUS",
		"GLOBAL_HALT",
		"GLOBAL_CANCEL_ONLY",
		"GLOBAL_RESUME",
		"ASSIGN_ROLE",
		"REMOVE_ROLE",
	}

	for _, action := range actions {
		log := &AuditLog{Action: action}
		if log.Action != action {
			t.Fatalf("expected Action=%s, got %s", action, log.Action)
		}
	}
}

func TestAuditLogTargetTypes(t *testing.T) {
	targetTypes := []string{
		"SYMBOL",
		"SYSTEM",
		"USER_ROLE",
	}

	for _, tt := range targetTypes {
		log := &AuditLog{TargetType: tt}
		if log.TargetType != tt {
			t.Fatalf("expected TargetType=%s, got %s", tt, log.TargetType)
		}
	}
}
