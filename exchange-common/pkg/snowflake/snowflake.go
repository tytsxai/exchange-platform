// Package snowflake 雪花 ID 生成器
package snowflake

import (
	"errors"
	"sync"
	"time"
)

const (
	// 起始时间戳 (2024-01-01 00:00:00 UTC)
	epoch int64 = 1704067200000

	// 位数分配
	workerIDBits     = 10 // 机器 ID 位数
	sequenceBits     = 12 // 序列号位数

	// 最大值
	maxWorkerID   = -1 ^ (-1 << workerIDBits) // 1023
	maxSequence   = -1 ^ (-1 << sequenceBits) // 4095

	// 位移
	workerIDShift  = sequenceBits
	timestampShift = sequenceBits + workerIDBits
)

var (
	ErrInvalidWorkerID = errors.New("worker ID must be between 0 and 1023")
	ErrClockMovedBack  = errors.New("clock moved backwards")
)

// Generator 雪花 ID 生成器
type Generator struct {
	mu        sync.Mutex
	workerID  int64
	sequence  int64
	lastTime  int64
}

// New 创建生成器
func New(workerID int64) (*Generator, error) {
	if workerID < 0 || workerID > maxWorkerID {
		return nil, ErrInvalidWorkerID
	}
	return &Generator{
		workerID: workerID,
	}, nil
}

// Generate 生成 ID
func (g *Generator) Generate() (int64, error) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := time.Now().UnixMilli()

	if now < g.lastTime {
		return 0, ErrClockMovedBack
	}

	if now == g.lastTime {
		g.sequence = (g.sequence + 1) & maxSequence
		if g.sequence == 0 {
			// 序列号用尽，等待下一毫秒
			for now <= g.lastTime {
				now = time.Now().UnixMilli()
			}
		}
	} else {
		g.sequence = 0
	}

	g.lastTime = now

	id := ((now - epoch) << timestampShift) |
		(g.workerID << workerIDShift) |
		g.sequence

	return id, nil
}

// MustGenerate 生成 ID，panic on error
func (g *Generator) MustGenerate() int64 {
	id, err := g.Generate()
	if err != nil {
		panic(err)
	}
	return id
}

// Parse 解析 ID
func Parse(id int64) (timestamp int64, workerID int64, sequence int64) {
	timestamp = (id >> timestampShift) + epoch
	workerID = (id >> workerIDShift) & maxWorkerID
	sequence = id & maxSequence
	return
}

// Time 获取 ID 的生成时间
func Time(id int64) time.Time {
	ts, _, _ := Parse(id)
	return time.UnixMilli(ts)
}

// 全局生成器
var defaultGenerator *Generator

// Init 初始化全局生成器
func Init(workerID int64) error {
	g, err := New(workerID)
	if err != nil {
		return err
	}
	defaultGenerator = g
	return nil
}

// NextID 使用全局生成器生成 ID
func NextID() (int64, error) {
	if defaultGenerator == nil {
		return 0, errors.New("snowflake not initialized")
	}
	return defaultGenerator.Generate()
}

// MustNextID 使用全局生成器生成 ID，panic on error
func MustNextID() int64 {
	id, err := NextID()
	if err != nil {
		panic(err)
	}
	return id
}
