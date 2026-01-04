// Package config 配置
package config

import (
	"fmt"
	"strings"
	"time"

	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	// 服务
	ServiceName string
	HTTPPort    int
	MetricsPort int
	AppEnv      string

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Streams
	OrderStream    string
	EventStream    string
	ConsumerGroup  string
	ConsumerName   string
	OrderDedupeTTL time.Duration

	// Private events (pub/sub)
	PrivateUserEventChannel string

	// Internal Auth
	InternalToken string

	// Worker
	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-matching"),
		HTTPPort:    envconfig.GetEnvInt("MATCHING_HTTP_PORT", envconfig.GetEnvInt("HTTP_PORT", 8082)),
		MetricsPort: envconfig.GetEnvInt("MATCHING_METRICS_PORT", envconfig.GetEnvInt("METRICS_PORT", 9082)),
		AppEnv:      strings.ToLower(envconfig.GetEnv("APP_ENV", "dev")),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),
		RedisDB:       envconfig.GetEnvInt("REDIS_DB", 0),

		OrderStream:    envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:    envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup:  envconfig.GetEnv("CONSUMER_GROUP", "matching-group"),
		ConsumerName:   envconfig.GetEnv("CONSUMER_NAME", "matching-1"),
		OrderDedupeTTL: envconfig.GetEnvDuration("MATCHING_ORDER_DEDUP_TTL", 24*time.Hour),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		InternalToken: envconfig.GetEnv("INTERNAL_TOKEN", ""),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 1),
	}
}

func (c *Config) Validate() error {
	if c.InternalToken == "" {
		return fmt.Errorf("INTERNAL_TOKEN is required")
	}
	if c.AppEnv != "dev" {
		if envconfig.IsInsecureDevSecret(c.InternalToken) {
			return fmt.Errorf("INTERNAL_TOKEN must not be a dev placeholder (APP_ENV=%s)", c.AppEnv)
		}
		if c.RedisPassword == "" {
			return fmt.Errorf("REDIS_PASSWORD is required (APP_ENV=%s)", c.AppEnv)
		}
		if strings.TrimSpace(c.ConsumerGroup) == "" || strings.TrimSpace(c.ConsumerName) == "" {
			return fmt.Errorf("CONSUMER_GROUP and CONSUMER_NAME are required (APP_ENV=%s)", c.AppEnv)
		}
	}
	if c.OrderDedupeTTL <= 0 {
		return fmt.Errorf("MATCHING_ORDER_DEDUP_TTL must be positive")
	}
	return nil
}
