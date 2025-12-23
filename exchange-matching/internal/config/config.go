// Package config 配置
package config

import (
	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	// 服务
	ServiceName string
	HTTPPort    int
	MetricsPort int

	// Redis
	RedisAddr     string
	RedisPassword string
	RedisDB       int

	// Streams
	OrderStream   string
	EventStream   string
	ConsumerGroup string
	ConsumerName  string

	// Private events (pub/sub)
	PrivateUserEventChannel string

	// Worker
	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-matching"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8082),
		MetricsPort: envconfig.GetEnvInt("METRICS_PORT", 9082),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),
		RedisDB:       envconfig.GetEnvInt("REDIS_DB", 0),

		OrderStream:   envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:   envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup: envconfig.GetEnv("CONSUMER_GROUP", "matching-group"),
		ConsumerName:  envconfig.GetEnv("CONSUMER_NAME", "matching-1"),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 1),
	}
}
