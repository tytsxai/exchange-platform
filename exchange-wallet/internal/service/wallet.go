// Package service 钱包服务
package service

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/repository"
)

// WalletService 钱包服务
type WalletService struct {
	repo        WalletRepository
	idGen       IDGenerator
	clearingCli ClearingClient
	tronCli     TronClient
}

var (
	ErrInvalidWithdrawRequest = errors.New("invalid withdraw request")
	ErrInvalidWithdrawState   = errors.New("invalid withdraw state")
)

// IDGenerator ID 生成器接口
type IDGenerator interface {
	NextID() int64
}

// TronClient 链客户端（MVP: TRON）
type TronClient interface {
	GetTransactions(address string, limit int) ([]client.Transaction, error)
	GetTRC20Transfers(address string, limit int, contractAddress string) ([]client.TRC20Transfer, error)
	GetTransactionInfo(txid string) (*client.TransactionInfo, error)
	GetNowBlockNumber() (int64, error)
}

// NewWalletService 创建钱包服务
func NewWalletService(repo WalletRepository, idGen IDGenerator, clearingCli ClearingClient, tronCli TronClient) *WalletService {
	return &WalletService{
		repo:        repo,
		idGen:       idGen,
		clearingCli: clearingCli,
		tronCli:     tronCli,
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

// ProcessDeposit 处理充值（链上监听调用，amount 为最小单位整数）
func (s *WalletService) ProcessDeposit(ctx context.Context, userID int64, asset, network, txid string, vout int, amount int64, confirmations int) error {
	if userID <= 0 {
		return fmt.Errorf("invalid userID")
	}
	if amount <= 0 {
		return fmt.Errorf("invalid amount")
	}
	if txid == "" {
		return fmt.Errorf("txid required")
	}

	// 检查网络配置
	net, err := s.repo.GetNetwork(ctx, asset, network)
	if err != nil {
		return err
	}
	if net == nil {
		return fmt.Errorf("network not found")
	}

	status := repository.DepositStatusPending
	if confirmations >= net.ConfirmationsRequired {
		status = repository.DepositStatusConfirmed
	}

	deposit, err := s.repo.UpsertDeposit(ctx, &repository.Deposit{
		DepositID:     s.idGen.NextID(),
		UserID:        userID,
		Asset:         asset,
		Network:       network,
		Amount:        amount,
		Txid:          txid,
		Vout:          vout,
		Confirmations: confirmations,
		Status:        status,
	})
	if err != nil {
		return err
	}

	// 未达到确认数：仅记录
	if deposit.Confirmations < net.ConfirmationsRequired {
		return nil
	}

	// 已入账：幂等
	if deposit.Status == repository.DepositStatusCredited {
		return nil
	}

	// 达到确认数 -> 记账入可用余额
	creditReq := &client.CreditRequest{
		IdempotencyKey: fmt.Sprintf("deposit:%s:%s:%s:%d", asset, network, txid, vout),
		UserID:         deposit.UserID,
		Asset:          deposit.Asset,
		Amount:         deposit.Amount,
		RefType:        "DEPOSIT",
		RefID:          fmt.Sprintf("%d", deposit.DepositID),
	}
	if err := s.clearingCli.Credit(ctx, creditReq); err != nil {
		return fmt.Errorf("credit deposit: %w", err)
	}

	if err := s.repo.UpdateDepositStatus(ctx, deposit.DepositID, repository.DepositStatusCredited, deposit.Confirmations); err != nil {
		return fmt.Errorf("update deposit status: %w", err)
	}
	return nil
}

// ========== 提现 ==========

// WithdrawRequest 提现请求
type WithdrawRequest struct {
	IdempotencyKey string
	UserID         int64
	Asset          string
	Network        string
	Amount         int64
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
	if req == nil {
		return &WithdrawResponse{ErrorCode: "INVALID_PARAM"}, nil
	}
	req.IdempotencyKey = strings.TrimSpace(req.IdempotencyKey)
	req.Asset = strings.TrimSpace(req.Asset)
	req.Network = strings.TrimSpace(req.Network)
	req.Address = strings.TrimSpace(req.Address)
	if req.IdempotencyKey == "" || req.UserID <= 0 || req.Asset == "" || req.Network == "" || req.Address == "" || req.Amount <= 0 {
		return &WithdrawResponse{ErrorCode: "INVALID_PARAM"}, nil
	}

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
		Amount:         req.Amount,
		RefType:        "WITHDRAW",
		RefID:          req.IdempotencyKey,
	}
	if err := s.clearingCli.Freeze(ctx, freezeReq); err != nil {
		return nil, fmt.Errorf("freeze funds: %w", err)
	}

	// 2. 创建提现记录

	if err := s.repo.CreateWithdrawal(ctx, withdrawal); err != nil {
		// 并发竞态下可能已由其他请求写入；回查后按幂等成功处理。
		existing, lookupErr := s.repo.GetWithdrawalByIdempotencyKey(ctx, req.IdempotencyKey)
		if lookupErr != nil {
			return nil, fmt.Errorf("create withdrawal: %w (lookup idempotency key failed: %v)", err, lookupErr)
		}
		if existing != nil {
			return &WithdrawResponse{Withdrawal: existing}, nil
		}
		// 不能在此处自动解冻：冻结幂等键已被占用，自动解冻会导致同幂等键重试时
		// 可能出现“提现单创建成功但资金未冻结”的不一致状态。
		log.Printf("[CRITICAL] create withdrawal failed after freeze, funds remain frozen: userID=%d key=%s err=%v", req.UserID, req.IdempotencyKey, err)
		return nil, fmt.Errorf("create withdrawal after freeze: %w", err)
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
	if withdrawID <= 0 || approverID <= 0 {
		return ErrInvalidWithdrawRequest
	}
	withdraw, err := s.repo.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("get withdraw: %w", err)
	}
	if withdraw == nil {
		return fmt.Errorf("withdraw not found")
	}
	switch withdraw.Status {
	case repository.WithdrawStatusApproved:
		return nil // 幂等
	case repository.WithdrawStatusPending:
		// ok
	default:
		return fmt.Errorf("%w: current=%d target=%d", ErrInvalidWithdrawState, withdraw.Status, repository.WithdrawStatusApproved)
	}

	return s.transitionWithdrawalStatus(ctx, withdrawID, []int{repository.WithdrawStatusPending}, repository.WithdrawStatusApproved, approverID, "")
}

// RejectWithdraw 拒绝提现
func (s *WalletService) RejectWithdraw(ctx context.Context, withdrawID, approverID int64) error {
	if withdrawID <= 0 || approverID <= 0 {
		return ErrInvalidWithdrawRequest
	}
	// 1. 获取提现记录
	withdraw, err := s.repo.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("get withdraw: %w", err)
	}
	if withdraw == nil {
		return fmt.Errorf("withdraw not found")
	}

	// 2. 先通过 CAS 抢占状态（PENDING -> REJECTED），避免并发下先解冻后被审批覆盖。
	switch withdraw.Status {
	case repository.WithdrawStatusPending:
		if err := s.transitionWithdrawalStatus(
			ctx,
			withdrawID,
			[]int{repository.WithdrawStatusPending},
			repository.WithdrawStatusRejected,
			approverID,
			"",
		); err != nil {
			return err
		}
	case repository.WithdrawStatusRejected:
		// 幂等重试：继续执行解冻，利用 idempotency key 保证不会重复扣改。
	default:
		return fmt.Errorf("%w: current=%d target=%d", ErrInvalidWithdrawState, withdraw.Status, repository.WithdrawStatusRejected)
	}

	// 3. 解冻资金（幂等）
	unfreezeReq := &client.UnfreezeRequest{
		IdempotencyKey: fmt.Sprintf("reject:%d", withdrawID),
		UserID:         withdraw.UserID,
		Asset:          withdraw.Asset,
		Amount:         withdraw.Amount,
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
	if withdrawID <= 0 || strings.TrimSpace(txid) == "" {
		return ErrInvalidWithdrawRequest
	}
	// 1. 获取提现记录
	withdraw, err := s.repo.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("get withdraw: %w", err)
	}
	if withdraw == nil {
		return fmt.Errorf("withdraw not found")
	}
	switch withdraw.Status {
	case repository.WithdrawStatusCompleted:
		return nil // 幂等
	case repository.WithdrawStatusApproved, repository.WithdrawStatusProcessing:
		// ok
	default:
		return fmt.Errorf("%w: current=%d target=%d", ErrInvalidWithdrawState, withdraw.Status, repository.WithdrawStatusCompleted)
	}

	deductReq := &client.DeductRequest{
		IdempotencyKey: fmt.Sprintf("complete:%d", withdrawID),
		UserID:         withdraw.UserID,
		Asset:          withdraw.Asset,
		Amount:         withdraw.Amount,
		RefType:        "WITHDRAW_COMPLETE",
		RefID:          fmt.Sprintf("%d", withdrawID),
	}
	if err := s.clearingCli.Deduct(ctx, deductReq); err != nil {
		log.Printf("[CRITICAL] Failed to deduct frozen funds for completed withdraw %d: %v", withdrawID, err)
		return fmt.Errorf("deduct funds: %w", err)
	}

	// 2. 标记完成
	if err := s.transitionWithdrawalStatus(
		ctx,
		withdrawID,
		[]int{repository.WithdrawStatusApproved, repository.WithdrawStatusProcessing},
		repository.WithdrawStatusCompleted,
		0,
		strings.TrimSpace(txid),
	); err != nil {
		log.Printf("[CRITICAL] Funds deducted but failed to mark withdrawal completed: withdrawID=%d err=%v", withdrawID, err)
		return err
	}
	return nil
}

func (s *WalletService) transitionWithdrawalStatus(ctx context.Context, withdrawID int64, expected []int, target int, approvedBy int64, txid string) error {
	updated, err := s.repo.UpdateWithdrawalStatusCAS(ctx, withdrawID, expected, target, approvedBy, txid)
	if err != nil {
		return err
	}
	if updated {
		return nil
	}

	latest, err := s.repo.GetWithdrawal(ctx, withdrawID)
	if err != nil {
		return fmt.Errorf("reload withdraw: %w", err)
	}
	if latest == nil {
		return fmt.Errorf("withdraw not found")
	}
	if latest.Status == target {
		return nil
	}
	return fmt.Errorf("%w: current=%d target=%d", ErrInvalidWithdrawState, latest.Status, target)
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
