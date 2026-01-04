package service

import (
	"context"
	"log"
	"runtime/debug"
	"time"

	"github.com/exchange/common/pkg/health"
	"github.com/exchange/wallet/internal/client"
	"github.com/exchange/wallet/internal/repository"
)

// DepositScanner 周期性扫描链上入账（MVP：TRON + TRX/TRC20）
//
// 注意：该实现仅用于 MVP/内测。生产环境推荐：
// - 使用按区块高度的扫描（scanBlock）或可靠的 webhook/event source
// - 使用更严格的确认数/重组处理、以及 outbox/重试策略
type DepositScanner struct {
	svc          *WalletService
	interval     time.Duration
	maxAddresses int
	txLimit      int
	loop         health.LoopMonitor
}

func NewDepositScanner(svc *WalletService, interval time.Duration, maxAddresses int) *DepositScanner {
	if interval <= 0 {
		interval = 15 * time.Second
	}
	if maxAddresses <= 0 {
		maxAddresses = 200
	}
	return &DepositScanner{
		svc:          svc,
		interval:     interval,
		maxAddresses: maxAddresses,
		txLimit:      50,
	}
}

func (s *DepositScanner) Start(ctx context.Context) {
	if s.svc == nil || s.svc.tronCli == nil {
		log.Println("[DepositScanner] disabled: tron client not configured")
		return
	}

	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	log.Printf("[DepositScanner] started (interval=%s maxAddresses=%d)", s.interval, s.maxAddresses)
	for {
		select {
		case <-ctx.Done():
			log.Println("[DepositScanner] stopped")
			return
		case <-ticker.C:
			s.loop.Tick()
			s.safeScanOnce(ctx)
		}
	}
}

func (s *DepositScanner) safeScanOnce(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("[DepositScanner] panic recovered: %v\n%s", r, string(debug.Stack()))
		}
	}()
	s.scanOnce(ctx)
}

func (s *DepositScanner) Interval() time.Duration {
	return s.interval
}

func (s *DepositScanner) Healthy(now time.Time, maxAge time.Duration) (bool, time.Duration, string) {
	return s.loop.Healthy(now, maxAge)
}

func (s *DepositScanner) scanOnce(ctx context.Context) {
	networks, err := s.svc.ListNetworks(ctx, "")
	if err != nil {
		log.Printf("[DepositScanner] list networks failed: %v", err)
		return
	}

	currentBlock, err := s.svc.tronCli.GetNowBlockNumber()
	if err != nil {
		log.Printf("[DepositScanner] get tron now block failed: %v", err)
		currentBlock = 0
	}

	blockCache := make(map[string]int64) // txid -> blockNumber
	confirmationsFor := func(txid string) int {
		if currentBlock <= 0 {
			return 0
		}
		if bn, ok := blockCache[txid]; ok {
			if currentBlock >= bn {
				return int(currentBlock - bn + 1)
			}
			return 0
		}
		info, err := s.svc.tronCli.GetTransactionInfo(txid)
		if err != nil {
			return 0
		}
		blockCache[txid] = info.BlockNumber
		if currentBlock >= info.BlockNumber {
			return int(currentBlock - info.BlockNumber + 1)
		}
		return 0
	}

	for _, n := range networks {
		if !n.DepositEnabled {
			continue
		}
		if n.Network != "TRON" {
			continue // MVP 仅实现 TRON
		}

		addrs, err := s.svc.repo.ListDepositAddresses(ctx, n.Asset, n.Network, s.maxAddresses)
		if err != nil {
			log.Printf("[DepositScanner] list deposit addresses failed: asset=%s network=%s err=%v", n.Asset, n.Network, err)
			continue
		}

		for _, addr := range addrs {
			switch n.Asset {
			case "TRX":
				s.scanTRXAddress(ctx, addr, confirmationsFor)
			default:
				if n.ContractAddress == "" {
					continue
				}
				s.scanTRC20Address(ctx, addr, n.Asset, n.ContractAddress, confirmationsFor)
			}
		}
	}
}

func (s *DepositScanner) scanTRXAddress(ctx context.Context, addr *repository.DepositAddress, confirmationsFor func(txid string) int) {
	txs, err := s.svc.tronCli.GetTransactions(addr.Address, s.txLimit)
	if err != nil {
		log.Printf("[DepositScanner] tron txs failed: address=%s err=%v", addr.Address, err)
		return
	}

	for _, tx := range txs {
		if len(tx.RawData.Contract) == 0 {
			continue
		}
		contract := tx.RawData.Contract[0]
		if contract.Type != "TransferContract" {
			continue
		}
		val := contract.Parameter.Value
		if val.ToAddress != addr.Address {
			continue
		}

		conf := confirmationsFor(tx.TxID)
		if err := s.svc.ProcessDeposit(ctx, addr.UserID, "TRX", "TRON", tx.TxID, 0, val.Amount, conf); err != nil {
			log.Printf("[DepositScanner] process deposit failed: txid=%s user=%d err=%v", tx.TxID, addr.UserID, err)
		}
	}
}

func (s *DepositScanner) scanTRC20Address(ctx context.Context, addr *repository.DepositAddress, asset, contractAddress string, confirmationsFor func(txid string) int) {
	transfers, err := s.svc.tronCli.GetTRC20Transfers(addr.Address, s.txLimit, contractAddress)
	if err != nil {
		log.Printf("[DepositScanner] tron trc20 failed: address=%s err=%v", addr.Address, err)
		return
	}

	for _, t := range transfers {
		if t.To != addr.Address {
			continue
		}
		amount, err := client.ParseTRC20Value(t.Value)
		if err != nil {
			continue
		}
		vout := t.EventIndex
		conf := confirmationsFor(t.TransactionID)

		if err := s.svc.ProcessDeposit(ctx, addr.UserID, asset, "TRON", t.TransactionID, vout, amount, conf); err != nil {
			log.Printf("[DepositScanner] process trc20 deposit failed: txid=%s user=%d err=%v", t.TransactionID, addr.UserID, err)
		}
	}
}
