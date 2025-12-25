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

	// Public WS security/stability
	WSAllowOrigins     []string
	WSMaxSubscriptions int
}

// Load 加载配置
func Load() *Config {
	appEnv := strings.ToLower(envconfig.GetEnv("APP_ENV", "dev"))
	wsDefaultOrigins := []string{"*"}
	if appEnv != "dev" {
		wsDefaultOrigins = []string{}
	}
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-marketdata"),
		HTTPPort:    envconfig.GetEnvInt("MARKETDATA_HTTP_PORT", envconfig.GetEnvInt("HTTP_PORT", 8084)),
		WSPort:      envconfig.GetEnvInt("MARKETDATA_WS_PORT", envconfig.GetEnvInt("WS_PORT", 8094)),
		AppEnv:      appEnv,

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),

		OrderStream:   envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:   envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup: envconfig.GetEnv("CONSUMER_GROUP", "marketdata-group"),
		ConsumerName:  envconfig.GetEnv("CONSUMER_NAME", "marketdata-1"),
		ReplayCount:   envconfig.GetEnvInt("EVENT_REPLAY_COUNT", 1000),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		InternalToken: envconfig.GetEnv("INTERNAL_TOKEN", ""),

		WSAllowOrigins:     envconfig.GetEnvSlice("MARKETDATA_WS_ALLOW_ORIGINS", wsDefaultOrigins),
		WSMaxSubscriptions: envconfig.GetEnvInt("MARKETDATA_WS_MAX_SUBSCRIPTIONS", 50),
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
		for _, o := range c.WSAllowOrigins {
			if o == "*" {
				return fmt.Errorf("MARKETDATA_WS_ALLOW_ORIGINS must not include '*' when APP_ENV=%s", c.AppEnv)
			}
		}
	}
	if c.WSMaxSubscriptions <= 0 || c.WSMaxSubscriptions > 200 {
		return fmt.Errorf("MARKETDATA_WS_MAX_SUBSCRIPTIONS must be between 1 and 200")
	}
	return nil
}
