package withdraw

import (
	"testing"
)

func TestWithdrawManager_Submit(t *testing.T) {
	m := NewWithdrawManager()

	// 正常提交
	req, err := m.Submit(1, "USDT", "TRX", "TAddr123", 1000)
	if err != nil {
		t.Fatalf("Submit failed: %v", err)
	}
	if req.Status != WithdrawStatusPending {
		t.Errorf("expected status PENDING, got %s", req.Status)
	}
	if req.WithdrawID != 1 {
		t.Errorf("expected withdrawID 1, got %d", req.WithdrawID)
	}

	// 无效参数
	_, err = m.Submit(0, "USDT", "TRX", "TAddr", 100)
	if err == nil {
		t.Error("expected error for invalid userID")
	}
	_, err = m.Submit(1, "", "TRX", "TAddr", 100)
	if err == nil {
		t.Error("expected error for empty asset")
	}
}

func TestWithdrawManager_Approve(t *testing.T) {
	m := NewWithdrawManager()
	req, _ := m.Submit(1, "USDT", "TRX", "TAddr", 1000)

	// 正常审批
	approved, err := m.Approve(req.WithdrawID)
	if err != nil {
		t.Fatalf("Approve failed: %v", err)
	}
	if approved.Status != WithdrawStatusApproved {
		t.Errorf("expected APPROVED, got %s", approved.Status)
	}

	// 重复审批
	_, err = m.Approve(req.WithdrawID)
	if err == nil {
		t.Error("expected error for duplicate approve")
	}

	// 不存在的ID
	_, err = m.Approve(9999)
	if err == nil {
		t.Error("expected error for non-existent withdraw")
	}
}

func TestWithdrawManager_Reject(t *testing.T) {
	m := NewWithdrawManager()
	req, _ := m.Submit(1, "USDT", "TRX", "TAddr", 1000)

	rejected, err := m.Reject(req.WithdrawID, "risk detected")
	if err != nil {
		t.Fatalf("Reject failed: %v", err)
	}
	if rejected.Status != WithdrawStatusRejected {
		t.Errorf("expected REJECTED, got %s", rejected.Status)
	}
	if rejected.Reason != "risk detected" {
		t.Errorf("expected reason 'risk detected', got %s", rejected.Reason)
	}
}

func TestWithdrawManager_StartProcessing(t *testing.T) {
	m := NewWithdrawManager()
	req, _ := m.Submit(1, "USDT", "TRX", "TAddr", 1000)

	// 未审批不能开始处理
	_, err := m.StartProcessing(req.WithdrawID)
	if err == nil {
		t.Error("expected error for non-approved withdraw")
	}

	// 审批后可以开始处理
	m.Approve(req.WithdrawID)
	processing, err := m.StartProcessing(req.WithdrawID)
	if err != nil {
		t.Fatalf("StartProcessing failed: %v", err)
	}
	if processing.Status != WithdrawStatusProcessing {
		t.Errorf("expected PROCESSING, got %s", processing.Status)
	}
}

func TestWithdrawManager_Complete(t *testing.T) {
	m := NewWithdrawManager()
	req, _ := m.Submit(1, "USDT", "TRX", "TAddr", 1000)
	m.Approve(req.WithdrawID)
	m.StartProcessing(req.WithdrawID)

	completed, err := m.Complete(req.WithdrawID)
	if err != nil {
		t.Fatalf("Complete failed: %v", err)
	}
	if completed.Status != WithdrawStatusCompleted {
		t.Errorf("expected COMPLETED, got %s", completed.Status)
	}
}

func TestWithdrawManager_Fail(t *testing.T) {
	m := NewWithdrawManager()
	req, _ := m.Submit(1, "USDT", "TRX", "TAddr", 1000)
	m.Approve(req.WithdrawID)
	m.StartProcessing(req.WithdrawID)

	failed, err := m.Fail(req.WithdrawID, "chain error")
	if err != nil {
		t.Fatalf("Fail failed: %v", err)
	}
	if failed.Status != WithdrawStatusFailed {
		t.Errorf("expected FAILED, got %s", failed.Status)
	}
	if failed.Reason != "chain error" {
		t.Errorf("expected reason 'chain error', got %s", failed.Reason)
	}
}
