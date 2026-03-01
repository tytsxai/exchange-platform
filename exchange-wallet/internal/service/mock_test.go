package service

import (
	"context"
	"fmt"

	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/repository"
)

// mockIDGen mock ID 生成器
type mockIDGen struct {
	id int64
}

func (m *mockIDGen) NextID() int64 {
	m.id++
	return m.id
}

// mockWalletRepository mock 钱包仓储
type mockWalletRepository struct {
	assets              []*repository.Asset
	networks            []*repository.Network
	depositAddresses    map[string]*repository.DepositAddress
	deposits            map[int64]*repository.Deposit
	withdrawals         map[int64]*repository.Withdrawal
	withdrawalsByKey    map[string]*repository.Withdrawal
	listAssetsErr       error
	listNetworksErr     error
	getNetworkErr       error
	createDepositErr    error
	listDepositsErr     error
	createWithdrawalErr error
	// createWithdrawalPersistBeforeErr simulates "written but returned error"
	// scenarios such as race-induced conflict/timeout on create.
	createWithdrawalPersistBeforeErr bool
	getWithdrawalErr    error
	updateStatusErr     error
	forceCASNoUpdate    bool
	casNoUpdateStatus   int
	listWithdrawalsErr  error
}

func newMockWalletRepository() *mockWalletRepository {
	return &mockWalletRepository{
		depositAddresses: make(map[string]*repository.DepositAddress),
		deposits:         make(map[int64]*repository.Deposit),
		withdrawals:      make(map[int64]*repository.Withdrawal),
		withdrawalsByKey: make(map[string]*repository.Withdrawal),
	}
}

func (m *mockWalletRepository) ListAssets(ctx context.Context) ([]*repository.Asset, error) {
	if m.listAssetsErr != nil {
		return nil, m.listAssetsErr
	}
	return m.assets, nil
}

func (m *mockWalletRepository) ListNetworks(ctx context.Context, asset string) ([]*repository.Network, error) {
	if m.listNetworksErr != nil {
		return nil, m.listNetworksErr
	}
	if asset == "" {
		return m.networks, nil
	}
	var result []*repository.Network
	for _, n := range m.networks {
		if n.Asset == asset {
			result = append(result, n)
		}
	}
	return result, nil
}

func (m *mockWalletRepository) GetNetwork(ctx context.Context, asset, network string) (*repository.Network, error) {
	if m.getNetworkErr != nil {
		return nil, m.getNetworkErr
	}
	for _, n := range m.networks {
		if n.Asset == asset && n.Network == network {
			return n, nil
		}
	}
	return nil, nil
}

func (m *mockWalletRepository) GetOrCreateDepositAddress(ctx context.Context, userID int64, asset, network, address string) (*repository.DepositAddress, error) {
	key := fmt.Sprintf("%d:%s:%s", userID, asset, network)
	if addr, ok := m.depositAddresses[key]; ok {
		return addr, nil
	}
	addr := &repository.DepositAddress{
		UserID:      userID,
		Asset:       asset,
		Network:     network,
		Address:     address,
		CreatedAtMs: 1234567890000,
	}
	m.depositAddresses[key] = addr
	return addr, nil
}

func (m *mockWalletRepository) ListDepositAddresses(ctx context.Context, asset, network string, limit int) ([]*repository.DepositAddress, error) {
	var result []*repository.DepositAddress
	for _, addr := range m.depositAddresses {
		if addr.Asset == asset && addr.Network == network {
			result = append(result, addr)
		}
	}
	if limit > 0 && len(result) > limit {
		return result[:limit], nil
	}
	return result, nil
}

func (m *mockWalletRepository) CreateDeposit(ctx context.Context, d *repository.Deposit) error {
	if m.createDepositErr != nil {
		return m.createDepositErr
	}
	m.deposits[d.DepositID] = d
	return nil
}

func (m *mockWalletRepository) UpsertDeposit(ctx context.Context, d *repository.Deposit) (*repository.Deposit, error) {
	if err := m.CreateDeposit(ctx, d); err != nil {
		return nil, err
	}
	return d, nil
}

func (m *mockWalletRepository) UpdateDepositStatus(ctx context.Context, depositID int64, status, confirmations int) error {
	if m.updateStatusErr != nil {
		return m.updateStatusErr
	}
	if d, ok := m.deposits[depositID]; ok {
		d.Status = status
		d.Confirmations = confirmations
	}
	return nil
}

func (m *mockWalletRepository) ListDeposits(ctx context.Context, userID int64, limit int) ([]*repository.Deposit, error) {
	if m.listDepositsErr != nil {
		return nil, m.listDepositsErr
	}
	var result []*repository.Deposit
	for _, d := range m.deposits {
		if d.UserID == userID {
			result = append(result, d)
		}
	}
	return result, nil
}

func (m *mockWalletRepository) CreateWithdrawal(ctx context.Context, w *repository.Withdrawal) error {
	if m.createWithdrawalPersistBeforeErr {
		m.withdrawals[w.WithdrawID] = w
		m.withdrawalsByKey[w.IdempotencyKey] = w
	}
	if m.createWithdrawalErr != nil {
		return m.createWithdrawalErr
	}
	m.withdrawals[w.WithdrawID] = w
	m.withdrawalsByKey[w.IdempotencyKey] = w
	return nil
}

func (m *mockWalletRepository) GetWithdrawalByIdempotencyKey(ctx context.Context, key string) (*repository.Withdrawal, error) {
	if m.getWithdrawalErr != nil {
		return nil, m.getWithdrawalErr
	}
	return m.withdrawalsByKey[key], nil
}

func (m *mockWalletRepository) GetWithdrawal(ctx context.Context, withdrawID int64) (*repository.Withdrawal, error) {
	if m.getWithdrawalErr != nil {
		return nil, m.getWithdrawalErr
	}
	return m.withdrawals[withdrawID], nil
}

func (m *mockWalletRepository) UpdateWithdrawalStatusCAS(ctx context.Context, withdrawID int64, expectedStatuses []int, status int, approvedBy int64, txid string) (bool, error) {
	if m.updateStatusErr != nil {
		return false, m.updateStatusErr
	}
	if m.forceCASNoUpdate {
		if w, ok := m.withdrawals[withdrawID]; ok && m.casNoUpdateStatus > 0 {
			w.Status = m.casNoUpdateStatus
		}
		return false, nil
	}

	if w, ok := m.withdrawals[withdrawID]; ok {
		match := false
		for _, current := range expectedStatuses {
			if w.Status == current {
				match = true
				break
			}
		}
		if !match {
			return false, nil
		}
		w.Status = status
		w.ApprovedBy = approvedBy
		if txid != "" {
			w.Txid = txid
		}
		return true, nil
	}
	return false, nil
}

func (m *mockWalletRepository) ListWithdrawals(ctx context.Context, userID int64, limit int) ([]*repository.Withdrawal, error) {
	if m.listWithdrawalsErr != nil {
		return nil, m.listWithdrawalsErr
	}
	var result []*repository.Withdrawal
	for _, w := range m.withdrawals {
		if w.UserID == userID {
			result = append(result, w)
		}
	}
	return result, nil
}

func (m *mockWalletRepository) ListPendingWithdrawals(ctx context.Context, limit int) ([]*repository.Withdrawal, error) {
	if m.listWithdrawalsErr != nil {
		return nil, m.listWithdrawalsErr
	}
	var result []*repository.Withdrawal
	for _, w := range m.withdrawals {
		if w.Status == repository.WithdrawStatusPending {
			result = append(result, w)
		}
	}
	return result, nil
}

// mockClearingClient mock 清算客户端
type mockClearingClient struct {
	freezeErr     error
	unfreezeErr   error
	deductErr     error
	creditErr     error
	freezeCalls   []client.FreezeRequest
	unfreezeCalls []client.UnfreezeRequest
	deductCalls   []client.DeductRequest
	creditCalls   []client.CreditRequest
}

func newMockClearingClient() *mockClearingClient {
	return &mockClearingClient{}
}

func (m *mockClearingClient) Freeze(ctx context.Context, req *client.FreezeRequest) error {
	if m.freezeErr != nil {
		return m.freezeErr
	}
	m.freezeCalls = append(m.freezeCalls, *req)
	return nil
}

func (m *mockClearingClient) Unfreeze(ctx context.Context, req *client.UnfreezeRequest) error {
	if m.unfreezeErr != nil {
		return m.unfreezeErr
	}
	m.unfreezeCalls = append(m.unfreezeCalls, *req)
	return nil
}

func (m *mockClearingClient) Deduct(ctx context.Context, req *client.DeductRequest) error {
	if m.deductErr != nil {
		return m.deductErr
	}
	m.deductCalls = append(m.deductCalls, *req)
	return nil
}

func (m *mockClearingClient) Credit(ctx context.Context, req *client.CreditRequest) error {
	if m.creditErr != nil {
		return m.creditErr
	}
	m.creditCalls = append(m.creditCalls, *req)
	return nil
}
