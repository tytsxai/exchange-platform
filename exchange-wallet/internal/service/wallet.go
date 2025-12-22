// Package service 钱包服务
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"time"

	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/repository"
)

// WalletService 钱包服务
type WalletService struct {
	repo         *repository.WalletRepository
	idGen        IDGenerator
	clearingCli  *client.ClearingClient
}

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// NewWalletService 创建钱包服务
func NewWalletService(repo *repository.WalletRepository, idGen IDGenerator, clearingCli *client.ClearingClient) *WalletService {
	return &WalletService{
		repo:        repo,
		idGen:       idGen,
		clearingCli: clearingCli,
	}
}

// ========== 资产与网络 ==========

// ListAssets 列出资产
func (s *WalletService) ListAssets(ctx context.Context) ([]*repository.Asset, error) {
	return s.repo.ListAssets(ctx)
}

// ListNetworks 列出网络
func (s *WalletService) ListNetworks(ctx context.Context, asset string) ([]*repository.Network, error) {
	return s.repo.ListNetworks(ctx, asset)
}

// ========== 充值 ==========

// GetDepositAddress 获取充值地址
func (s *WalletService) GetDepositAddress(ctx context.Context, userID int64, asset, network string) (*repository.DepositAddress, error) {
	// 检查网络配置
	net, err := s.repo.GetNetwork(ctx, asset, network)
	if err != nil {
		return nil, err
	}
	if net == nil {
		return nil, fmt.Errorf("network not found")
	}
	if !net.DepositEnabled {
		return nil, fmt.Errorf("deposit disabled")
	}

	// 生成地址（简化实现：实际应该调用钱包服务）
	address := generateAddress()

	return s.repo.GetOrCreateDepositAddress(ctx, userID, asset, network, address)
}

// ListDeposits 列出充值记录
func (s *WalletService) ListDeposits(ctx context.Context, userID int64, limit int) ([]*repository.Deposit, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListDeposits(ctx, userID, limit)
}

// ProcessDeposit 处理充值（链上监听调用）
func (s *WalletService) ProcessDeposit(ctx context.Context, userID int64, asset, network, txid string, vout int, amount float64, confirmations int) error {
	// 检查网络配置
	net, err := s.repo.GetNetwork(ctx, asset, network)
	if err != nil {
		return err
	}
	if net == nil {
		return fmt.Errorf("network not found")
	}

	// 创建充值记录
	deposit := &repository.Deposit{
		DepositID:     s.idGen.NextID(),
		UserID:        userID,
		Asset:         asset,
		Network:       network,
		Amount:        amount,
		Txid:          txid,
		Vout:          vout,
		Confirmations: confirmations,
		Status:        repository.DepositStatusPending,
	}

	if confirmations >= net.ConfirmationsRequired {
		deposit.Status = repository.DepositStatusConfirmed
	}

	return s.repo.CreateDeposit(ctx, deposit)
}

// ========== 提现 ==========

// WithdrawRequest 提现请求
type WithdrawRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Network        string
	Amount         float64
	Address        string
	Tag            string
}

// WithdrawResponse 提现响应
type WithdrawResponse struct {
	Withdrawal *repository.Withdrawal
	ErrorCode  string
}

// RequestWithdraw 申请提现
func (s *WalletService) RequestWithdraw(ctx context.Context, req *WithdrawRequest) (*WithdrawResponse, error) {
	// 幂等检查
	existing, err := s.repo.GetWithdrawalByIdempotencyKey(ctx, req.IdempotencyKey)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return &WithdrawResponse{Withdrawal: existing}, nil
	}

	// 检查网络配置
	net, err := s.repo.GetNetwork(ctx, req.Asset, req.Network)
	if err != nil {
		return nil, err
	}
	if net == nil {
		return &WithdrawResponse{ErrorCode: "NETWORK_NOT_FOUND"}, nil
	}
	if !net.WithdrawEnabled {
		return &WithdrawResponse{ErrorCode: "WITHDRAW_DISABLED"}, nil
	}
	if req.Amount < net.MinWithdraw {
		return &WithdrawResponse{ErrorCode: "AMOUNT_TOO_SMALL"}, nil
	}

	// 创建提现记录
	withdrawal := &repository.Withdrawal{
		WithdrawID:     s.idGen.NextID(),
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		Network:        req.Network,
		Amount:         req.Amount,
		Fee:            net.WithdrawFee,
		Address:        req.Address,
		Tag:            req.Tag,
		Status:         repository.WithdrawStatusPending,
	}

	// 1. 调用 clearing 服务冻结资金
	freezeReq := &client.FreezeRequest{
		IdempotencyKey: req.IdempotencyKey,
		UserID:         req.UserID,
		Asset:          req.Asset,
		Amount:         int64(req.Amount * 1e8),
		RefType:        "WITHDRAW",
		RefID:          req.IdempotencyKey,
	}
	if err := s.clearingCli.Freeze(ctx, freezeReq); err != nil {
		return nil, fmt.Errorf("freeze funds: %w", err)
	}

	// 2. 创建提现记录

	if err := s.repo.CreateWithdrawal(ctx, withdrawal); err != nil {
		return nil, err
	}

	return &WithdrawResponse{Withdrawal: withdrawal}, nil
}

// ListWithdrawals 列出提现记录
func (s *WalletService) ListWithdrawals(ctx context.Context, userID int64, limit int) ([]*repository.Withdrawal, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListWithdrawals(ctx, userID, limit)
}

// ListPendingWithdrawals 列出待审核提现
func (s *WalletService) ListPendingWithdrawals(ctx context.Context, limit int) ([]*repository.Withdrawal, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	return s.repo.ListPendingWithdrawals(ctx, limit)
}

// ApproveWithdraw 审批提现
func (s *WalletService) ApproveWithdraw(ctx context.Context, withdrawID, approverID int64) error {
	return s.repo.UpdateWithdrawalStatus(ctx, withdrawID, repository.WithdrawStatusApproved, approverID, "")
}

// RejectWithdraw 拒绝提现
func (s *WalletService) RejectWithdraw(ctx context.Context, withdrawID, approverID int64) error {
	// 1. 拒绝提现
	if err := s.repo.UpdateWithdrawalStatus(ctx, withdrawID, repository.WithdrawStatusRejected, approverID, ""); err != nil {
		return err
	}

	// 2. 调用 clearing 服务解冻资金
	withdraw, err := s.repo.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		// Log error but proceed? Or return error?
		return fmt.Errorf("get withdraw: %w", err)
	}

	unfreezeReq := &client.UnfreezeRequest{
		IdempotencyKey: fmt.Sprintf("reject:%d", withdrawID),
		UserID:         withdraw.UserID,
		Asset:          withdraw.Asset,
		Amount:         int64(withdraw.Amount * 1e8),
		RefType:        "WITHDRAW_REJECT",
		RefID:          fmt.Sprintf("%d", withdrawID),
	}
	if err := s.clearingCli.Unfreeze(ctx, unfreezeReq); err != nil {
		log.Printf("[CRITICAL] Failed to unfreeze funds for rejected withdraw %d: %v", withdrawID, err)
		return fmt.Errorf("unfreeze funds: %w", err)
	}

	return nil
}

// CompleteWithdraw 完成提现（出款后调用）
func (s *WalletService) CompleteWithdraw(ctx context.Context, withdrawID int64, txid string) error {
	// 1. 完成提现
	if err := s.repo.UpdateWithdrawalStatus(ctx, withdrawID, repository.WithdrawStatusCompleted, 0, txid); err != nil {
		return err
	}

	// 2. 扣除冻结资金
	withdraw, err := s.repo.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("get withdraw: %w", err)
	}

	deductReq := &client.DeductRequest{
		IdempotencyKey: fmt.Sprintf("complete:%d", withdrawID),
		UserID:         withdraw.UserID,
		Asset:          withdraw.Asset,
		Amount:         int64(withdraw.Amount * 1e8),
		RefType:        "WITHDRAW_COMPLETE",
		RefID:          fmt.Sprintf("%d", withdrawID),
	}
	if err := s.clearingCli.Deduct(ctx, deductReq); err != nil {
		log.Printf("[CRITICAL] Failed to deduct frozen funds for completed withdraw %d: %v", withdrawID, err)
		return fmt.Errorf("deduct funds: %w", err)
	}

	return nil
}

// 生成地址（简化实现）
func generateAddress() string {
	bytes := make([]byte, 20)
	if _, err := rand.Read(bytes); err != nil {
		// 回退到时间戳+随机数
		return fmt.Sprintf("0x%x%d", time.Now().UnixNano(), time.Now().UnixNano()%1000000)
	}
	return "0x" + hex.EncodeToString(bytes)
}
