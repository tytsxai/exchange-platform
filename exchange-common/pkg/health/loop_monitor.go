package health

import (
	"sync/atomic"
	"time"
)

// LoopMonitor tracks whether a background loop is still ticking.
// It is intentionally lightweight and dependency-free (std only).
type LoopMonitor struct {
	lastTickUnixNano atomic.Int64
	lastErr          atomic.Value // string
}

func (m *LoopMonitor) Tick() {
	m.lastTickUnixNano.Store(time.Now().UnixNano())
}

func (m *LoopMonitor) SetError(err error) {
	if err == nil {
		return
	}
	m.lastErr.Store(err.Error())
}

func (m *LoopMonitor) LastError() string {
	if v := m.lastErr.Load(); v != nil {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return ""
}

// Healthy returns whether the loop has ticked recently.
// If Tick() has never been called, it returns ok=false.
func (m *LoopMonitor) Healthy(now time.Time, maxAge time.Duration) (ok bool, age time.Duration, lastErr string) {
	lastErr = m.LastError()
	last := m.lastTickUnixNano.Load()
	if last <= 0 {
		return false, 0, lastErr
	}
	t := time.Unix(0, last)
	if now.Before(t) {
		return true, 0, lastErr
	}
	age = now.Sub(t)
	if maxAge <= 0 {
		maxAge = 10 * time.Second
	}
	return age <= maxAge, age, lastErr
}
