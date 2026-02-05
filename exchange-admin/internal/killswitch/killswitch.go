// Package killswitch 交易开关（Kill Switch）控制
package killswitch

import "sync"

// KillSwitch 交易开关控制器（内存存储 + 读写锁）
type KillSwitch struct {
	mu           sync.RWMutex
	globalHalted bool
	symbolHalted map[string]bool
}

// New 创建一个 KillSwitch
func New() *KillSwitch {
	return &KillSwitch{
		symbolHalted: make(map[string]bool),
	}
}

// SetGlobalHalt 设置全局暂停状态
func (k *KillSwitch) SetGlobalHalt(halt bool) {
	k.mu.Lock()
	k.globalHalted = halt
	k.mu.Unlock()
}

// IsGlobalHalted 查询全局是否暂停
func (k *KillSwitch) IsGlobalHalted() bool {
	k.mu.RLock()
	halted := k.globalHalted
	k.mu.RUnlock()
	return halted
}

// SetSymbolHalt 设置指定交易对暂停状态
func (k *KillSwitch) SetSymbolHalt(symbol string, halt bool) {
	k.mu.Lock()
	if halt {
		k.symbolHalted[symbol] = true
	} else {
		delete(k.symbolHalted, symbol)
	}
	k.mu.Unlock()
}

// IsSymbolHalted 查询指定交易对是否暂停（全局暂停优先生效）
func (k *KillSwitch) IsSymbolHalted(symbol string) bool {
	k.mu.RLock()
	halted := k.globalHalted || k.symbolHalted[symbol]
	k.mu.RUnlock()
	return halted
}
