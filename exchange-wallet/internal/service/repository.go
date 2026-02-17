package service

import (
	"context"

	"github.com/exchange/wallet/internal/repository"
)

// WalletRepository 钱包仓储接口
type WalletRepository interface {
	// 资产与网络
	ListAssets(ctx context.Context) ([]*repository.Asset, error)
	ListNetworks(ctx context.Context, asset string) ([]*repository.Network, error)
	GetNetwork(ctx context.Context, asset, network string) (*repository.Network, error)

	// 充值地址
	GetOrCreateDepositAddress(ctx context.Context, userID int64, asset, network, address string) (*repository.DepositAddress, error)
	ListDepositAddresses(ctx context.Context, asset, network string, limit int) ([]*repository.DepositAddress, error)

	// 充值记录
	CreateDeposit(ctx context.Context, d *repository.Deposit) error
	UpsertDeposit(ctx context.Context, d *repository.Deposit) (*repository.Deposit, error)
	UpdateDepositStatus(ctx context.Context, depositID int64, status, confirmations int) error
	ListDeposits(ctx context.Context, userID int64, limit int) ([]*repository.Deposit, error)

	// 提现记录
	CreateWithdrawal(ctx context.Context, w *repository.Withdrawal) error
	GetWithdrawalByIdempotencyKey(ctx context.Context, key string) (*repository.Withdrawal, error)
	GetWithdrawal(ctx context.Context, withdrawID int64) (*repository.Withdrawal, error)
	UpdateWithdrawalStatusCAS(ctx context.Context, withdrawID int64, expectedStatuses []int, status int, approvedBy int64, txid string) (bool, error)
	ListWithdrawals(ctx context.Context, userID int64, limit int) ([]*repository.Withdrawal, error)
	ListPendingWithdrawals(ctx context.Context, limit int) ([]*repository.Withdrawal, error)
}
