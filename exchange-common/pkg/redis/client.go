// Package redis Redis 客户端封装
package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Config Redis 配置
type Config struct {
	Addr         string        `json:"addr" yaml:"addr"`
	Password     string        `json:"password" yaml:"password"`
	DB           int           `json:"db" yaml:"db"`
	PoolSize     int           `json:"poolSize" yaml:"poolSize"`
	MinIdleConns int           `json:"minIdleConns" yaml:"minIdleConns"`
	DialTimeout  time.Duration `json:"dialTimeout" yaml:"dialTimeout"`
	ReadTimeout  time.Duration `json:"readTimeout" yaml:"readTimeout"`
	WriteTimeout time.Duration `json:"writeTimeout" yaml:"writeTimeout"`
}

// DefaultConfig 默认配置
var DefaultConfig = Config{
	Addr:         "localhost:6379",
	PoolSize:     100,
	MinIdleConns: 10,
	DialTimeout:  5 * time.Second,
	ReadTimeout:  3 * time.Second,
	WriteTimeout: 3 * time.Second,
}

// Client Redis 客户端封装
type Client struct {
	*redis.Client
}

// NewClient 创建客户端
func NewClient(cfg *Config) (*Client, error) {
	if cfg == nil {
		cfg = &DefaultConfig
	}

	client := redis.NewClient(&redis.Options{
		Addr:         cfg.Addr,
		Password:     cfg.Password,
		DB:           cfg.DB,
		PoolSize:     cfg.PoolSize,
		MinIdleConns: cfg.MinIdleConns,
		DialTimeout:  cfg.DialTimeout,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
	})

	// 测试连接
	ctx, cancel := context.WithTimeout(context.Background(), cfg.DialTimeout)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, err
	}

	return &Client{Client: client}, nil
}

// NonceStore 基于 Redis 的 nonce 存储
type NonceStore struct {
	client *Client
	prefix string
}

// NewNonceStore 创建 nonce 存储
func NewNonceStore(client *Client, prefix string) *NonceStore {
	if prefix == "" {
		prefix = "nonce:"
	}
	return &NonceStore{
		client: client,
		prefix: prefix,
	}
}

// Exists 检查 nonce 是否存在
func (s *NonceStore) Exists(apiKey, nonce string, expireAt time.Time) (bool, error) {
	key := s.prefix + apiKey + ":" + nonce
	ttl := time.Until(expireAt)
	if ttl <= 0 {
		ttl = time.Minute
	}

	// SETNX + EXPIRE
	ctx := context.Background()
	ok, err := s.client.SetNX(ctx, key, "1", ttl).Result()
	if err != nil {
		return false, err
	}

	// ok=true 表示设置成功（nonce 不存在）
	// ok=false 表示 key 已存在（nonce 重复）
	return !ok, nil
}

// Lock 分布式锁
type Lock struct {
	client *Client
	key    string
	value  string
	ttl    time.Duration
}

// NewLock 创建锁
func NewLock(client *Client, key, value string, ttl time.Duration) *Lock {
	return &Lock{
		client: client,
		key:    key,
		value:  value,
		ttl:    ttl,
	}
}

// Acquire 获取锁
func (l *Lock) Acquire(ctx context.Context) (bool, error) {
	return l.client.SetNX(ctx, l.key, l.value, l.ttl).Result()
}

// Release 释放锁（仅释放自己持有的锁）
func (l *Lock) Release(ctx context.Context) error {
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("del", KEYS[1])
		else
			return 0
		end
	`
	return l.client.Eval(ctx, script, []string{l.key}, l.value).Err()
}

// Extend 延长锁时间
func (l *Lock) Extend(ctx context.Context, ttl time.Duration) (bool, error) {
	script := `
		if redis.call("get", KEYS[1]) == ARGV[1] then
			return redis.call("pexpire", KEYS[1], ARGV[2])
		else
			return 0
		end
	`
	result, err := l.client.Eval(ctx, script, []string{l.key}, l.value, ttl.Milliseconds()).Int()
	if err != nil {
		return false, err
	}
	return result == 1, nil
}

// RateLimiter 限流器（滑动窗口）
type RateLimiter struct {
	client *Client
	prefix string
}

// NewRateLimiter 创建限流器
func NewRateLimiter(client *Client, prefix string) *RateLimiter {
	if prefix == "" {
		prefix = "ratelimit:"
	}
	return &RateLimiter{
		client: client,
		prefix: prefix,
	}
}

// Allow 检查是否允许请求
func (r *RateLimiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, int64, error) {
	now := time.Now().UnixMilli()
	windowStart := now - window.Milliseconds()
	fullKey := r.prefix + key

	// 使用 Lua 脚本保证原子性
	script := `
		local key = KEYS[1]
		local now = tonumber(ARGV[1])
		local window_start = tonumber(ARGV[2])
		local limit = tonumber(ARGV[3])
		local window_ms = tonumber(ARGV[4])

		-- 移除过期的请求
		redis.call("zremrangebyscore", key, "-inf", window_start)

		-- 获取当前请求数
		local count = redis.call("zcard", key)

		if count < limit then
			-- 添加当前请求
			redis.call("zadd", key, now, now .. "-" .. math.random())
			redis.call("pexpire", key, window_ms)
			return {1, limit - count - 1}
		else
			return {0, 0}
		end
	`

	result, err := r.client.Eval(ctx, script, []string{fullKey}, now, windowStart, limit, window.Milliseconds()).Slice()
	if err != nil {
		return false, 0, err
	}

	allowed := result[0].(int64) == 1
	remaining := result[1].(int64)
	return allowed, remaining, nil
}
