// Package config 配置
package config

import (
	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int
	WSPort      int

	// 后端服务
	OrderServiceURL    string
	ClearingServiceURL string
	UserServiceURL     string
	MatchingServiceURL string

	// Redis
	RedisAddr     string
	RedisPassword string

	// Private events (pub/sub)
	PrivateUserEventChannel string

	// Internal Auth
	InternalToken string

	// 限流
	IPRateLimit   int // 每秒请求数
	UserRateLimit int
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-gateway"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8080),
		WSPort:      envconfig.GetEnvInt("WS_PORT", 8090),

		OrderServiceURL:    envconfig.GetEnv("ORDER_SERVICE_URL", "http://localhost:8081"),
		ClearingServiceURL: envconfig.GetEnv("CLEARING_SERVICE_URL", "http://localhost:8083"),
		UserServiceURL:     envconfig.GetEnv("USER_SERVICE_URL", "http://localhost:8085"),
		MatchingServiceURL: envconfig.GetEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		InternalToken: envconfig.GetEnv("INTERNAL_TOKEN", ""),

		IPRateLimit:   envconfig.GetEnvInt("IP_RATE_LIMIT", 100),
		UserRateLimit: envconfig.GetEnvInt("USER_RATE_LIMIT", 50),
	}
}
