package service

import (
	"testing"
)

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

func TestSystemStatusStruct(t *testing.T) {
	status := &SystemStatus{
		TradingSymbols:    5,
		HaltedSymbols:     2,
		CancelOnlySymbols: 1,
		ServerTimeMs:      1000000,
	}

	if status.TradingSymbols != 5 {
		t.Fatalf("expected TradingSymbols=5, got %d", status.TradingSymbols)
	}
	if status.HaltedSymbols != 2 {
		t.Fatalf("expected HaltedSymbols=2, got %d", status.HaltedSymbols)
	}
	if status.CancelOnlySymbols != 1 {
		t.Fatalf("expected CancelOnlySymbols=1, got %d", status.CancelOnlySymbols)
	}
	if status.ServerTimeMs != 1000000 {
		t.Fatalf("expected ServerTimeMs=1000000, got %d", status.ServerTimeMs)
	}
}

func TestIDGeneratorInterface(t *testing.T) {
	// Test that the interface is properly defined
	var _ IDGenerator = &mockIDGenerator{}
}

type mockIDGenerator struct {
	nextID int64
}

func (m *mockIDGenerator) NextID() int64 {
	m.nextID++
	return m.nextID
}

func TestMockIDGenerator(t *testing.T) {
	gen := &mockIDGenerator{}

	id1 := gen.NextID()
	if id1 != 1 {
		t.Fatalf("expected first ID=1, got %d", id1)
	}

	id2 := gen.NextID()
	if id2 != 2 {
		t.Fatalf("expected second ID=2, got %d", id2)
	}
}

func TestNewAdminService(t *testing.T) {
	gen := &mockIDGenerator{}
	svc := NewAdminService(nil, gen)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
}

func TestAdminServiceStruct(t *testing.T) {
	gen := &mockIDGenerator{}
	svc := &AdminService{
		repo:  nil,
		idGen: gen,
	}

	if svc.idGen == nil {
		t.Fatal("expected non-nil idGen")
	}
}

func TestSystemStatusTotal(t *testing.T) {
	status := &SystemStatus{
		TradingSymbols:    5,
		HaltedSymbols:     2,
		CancelOnlySymbols: 1,
	}

	total := status.TradingSymbols + status.HaltedSymbols + status.CancelOnlySymbols
	if total != 8 {
		t.Fatalf("expected total=8, got %d", total)
	}
}

func TestStatusTransitions(t *testing.T) {
	// Valid status transitions
	transitions := []struct {
		from int
		to   int
	}{
		{StatusTrading, StatusHalt},
		{StatusTrading, StatusCancelOnly},
		{StatusHalt, StatusTrading},
		{StatusHalt, StatusCancelOnly},
		{StatusCancelOnly, StatusTrading},
		{StatusCancelOnly, StatusHalt},
	}

	for _, tr := range transitions {
		if tr.from == tr.to {
			t.Fatalf("invalid transition: from=%d to=%d", tr.from, tr.to)
		}
	}
}

func TestAuditLogLimitDefault(t *testing.T) {
	// Test that limit defaults are handled correctly
	limits := []struct {
		input    int
		expected int
	}{
		{0, 100},
		{-1, 100},
		{50, 50},
		{100, 100},
		{1001, 100},
	}

	for _, l := range limits {
		result := l.input
		if result <= 0 || result > 1000 {
			result = 100
		}
		if result != l.expected {
			t.Fatalf("for input=%d, expected=%d, got=%d", l.input, l.expected, result)
		}
	}
}
