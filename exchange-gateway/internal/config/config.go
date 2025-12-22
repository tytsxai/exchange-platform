// Package config 配置
package config

import (
	"os"
	"strconv"
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

	// 限流
	IPRateLimit   int // 每秒请求数
	UserRateLimit int
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: getEnv("SERVICE_NAME", "exchange-gateway"),
		HTTPPort:    getEnvInt("HTTP_PORT", 8080),
		WSPort:      getEnvInt("WS_PORT", 8090),

		OrderServiceURL:    getEnv("ORDER_SERVICE_URL", "http://localhost:8081"),
		ClearingServiceURL: getEnv("CLEARING_SERVICE_URL", "http://localhost:8083"),
		UserServiceURL:     getEnv("USER_SERVICE_URL", "http://localhost:8085"),
		MatchingServiceURL: getEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		PrivateUserEventChannel: getEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		IPRateLimit:   getEnvInt("IP_RATE_LIMIT", 100),
		UserRateLimit: getEnvInt("USER_RATE_LIMIT", 50),
	}
}

func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}
