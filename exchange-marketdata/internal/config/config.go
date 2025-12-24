// Package config 配置
package config

import (
	"fmt"
	"strings"

	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int
	WSPort      int
	AppEnv      string

	// Redis
	RedisAddr     string
	RedisPassword string

	// Streams
	OrderStream   string
	EventStream   string
	ConsumerGroup string
	ConsumerName  string
	ReplayCount   int

	// Private events (pub/sub)
	PrivateUserEventChannel string

	// Internal Auth
	InternalToken string
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-marketdata"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8084),
		WSPort:      envconfig.GetEnvInt("WS_PORT", 8094),
		AppEnv:      strings.ToLower(envconfig.GetEnv("APP_ENV", "dev")),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),

		OrderStream:   envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:   envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup: envconfig.GetEnv("CONSUMER_GROUP", "marketdata-group"),
		ConsumerName:  envconfig.GetEnv("CONSUMER_NAME", "marketdata-1"),
		ReplayCount:   envconfig.GetEnvInt("EVENT_REPLAY_COUNT", 1000),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		InternalToken: envconfig.GetEnv("INTERNAL_TOKEN", ""),
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
	return nil
}
