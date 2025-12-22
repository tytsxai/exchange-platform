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

	// Redis
	RedisAddr     string
	RedisPassword string

	// Streams
	OrderStream   string
	EventStream   string
	ConsumerGroup string
	ConsumerName  string

	// Private events (pub/sub)
	PrivateUserEventChannel string
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: getEnv("SERVICE_NAME", "exchange-marketdata"),
		HTTPPort:    getEnvInt("HTTP_PORT", 8084),
		WSPort:      getEnvInt("WS_PORT", 8094),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		OrderStream:   getEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:   getEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup: getEnv("CONSUMER_GROUP", "marketdata-group"),
		ConsumerName:  getEnv("CONSUMER_NAME", "marketdata-1"),

		PrivateUserEventChannel: getEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),
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
