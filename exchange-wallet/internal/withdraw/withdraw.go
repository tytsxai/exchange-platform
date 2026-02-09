// Package withdraw 提现流程管理模块（状态机）
package withdraw

import (
	"fmt"
	"sync"
	"time"
)

type WithdrawStatus string

const (
	WithdrawStatusPending    WithdrawStatus = "PENDING"
	WithdrawStatusApproved   WithdrawStatus = "APPROVED"
	WithdrawStatusRejected   WithdrawStatus = "REJECTED"
	WithdrawStatusProcessing WithdrawStatus = "PROCESSING"
	WithdrawStatusCompleted  WithdrawStatus = "COMPLETED"
	WithdrawStatusFailed     WithdrawStatus = "FAILED"
)

type WithdrawRequest struct {
	WithdrawID int64
	UserID     int64
	Asset      string
	Network    string
	Address    string
	Amount     int64 // 最小单位整数

	Status    WithdrawStatus
	Reason    string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type WithdrawManager struct {
	mu       sync.Mutex
	nextID   int64
	requests map[int64]*WithdrawRequest
}

func NewWithdrawManager() *WithdrawManager {
	return &WithdrawManager{nextID: 1, requests: make(map[int64]*WithdrawRequest)}
}

func (m *WithdrawManager) Submit(userID int64, asset, network, address string, amount int64) (*WithdrawRequest, error) {
	if userID <= 0 || amount <= 0 {
		return nil, fmt.Errorf("invalid userID/amount")
	}
	if asset == "" || network == "" || address == "" {
		return nil, fmt.Errorf("asset/network/address required")
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	id := m.nextID
	m.nextID++

	req := &WithdrawRequest{
		WithdrawID: id,
		UserID:     userID,
		Asset:      asset,
		Network:    network,
		Address:    address,
		Amount:     amount,
		Status:     WithdrawStatusPending,
		CreatedAt:  now,
		UpdatedAt:  now,
	}
	m.requests[id] = req
	return req, nil
}

func (m *WithdrawManager) Approve(withdrawID int64) (*WithdrawRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := m.requests[withdrawID]
	if req == nil {
		return nil, fmt.Errorf("withdraw not found")
	}
	if req.Status != WithdrawStatusPending {
		return nil, fmt.Errorf("invalid transition: %s -> %s", req.Status, WithdrawStatusApproved)
	}
	req.Status = WithdrawStatusApproved
	req.UpdatedAt = time.Now()
	req.Reason = ""
	return req, nil
}

func (m *WithdrawManager) Reject(withdrawID int64, reason string) (*WithdrawRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := m.requests[withdrawID]
	if req == nil {
		return nil, fmt.Errorf("withdraw not found")
	}
	if req.Status != WithdrawStatusPending {
		return nil, fmt.Errorf("invalid transition: %s -> %s", req.Status, WithdrawStatusRejected)
	}
	if reason == "" {
		reason = "rejected"
	}
	req.Status = WithdrawStatusRejected
	req.Reason = reason
	req.UpdatedAt = time.Now()
	return req, nil
}

// StartProcessing 开始处理提现（APPROVED -> PROCESSING）
func (m *WithdrawManager) StartProcessing(withdrawID int64) (*WithdrawRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := m.requests[withdrawID]
	if req == nil {
		return nil, fmt.Errorf("withdraw not found")
	}
	if req.Status != WithdrawStatusApproved {
		return nil, fmt.Errorf("invalid transition: %s -> %s", req.Status, WithdrawStatusProcessing)
	}
	req.Status = WithdrawStatusProcessing
	req.UpdatedAt = time.Now()
	return req, nil
}

// Complete 完成提现（PROCESSING -> COMPLETED）
func (m *WithdrawManager) Complete(withdrawID int64) (*WithdrawRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := m.requests[withdrawID]
	if req == nil {
		return nil, fmt.Errorf("withdraw not found")
	}
	if req.Status != WithdrawStatusProcessing {
		return nil, fmt.Errorf("invalid transition: %s -> %s", req.Status, WithdrawStatusCompleted)
	}
	req.Status = WithdrawStatusCompleted
	req.UpdatedAt = time.Now()
	return req, nil
}

// Fail 提现失败（PROCESSING -> FAILED）
func (m *WithdrawManager) Fail(withdrawID int64, reason string) (*WithdrawRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := m.requests[withdrawID]
	if req == nil {
		return nil, fmt.Errorf("withdraw not found")
	}
	if req.Status != WithdrawStatusProcessing {
		return nil, fmt.Errorf("invalid transition: %s -> %s", req.Status, WithdrawStatusFailed)
	}
	if reason == "" {
		reason = "failed"
	}
	req.Status = WithdrawStatusFailed
	req.Reason = reason
	req.UpdatedAt = time.Now()
	return req, nil
}

// Get 获取提现请求
func (m *WithdrawManager) Get(withdrawID int64) (*WithdrawRequest, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	req := m.requests[withdrawID]
	if req == nil {
		return nil, fmt.Errorf("withdraw not found")
	}
	return req, nil
}
