// Package config 配置
package config

import (
	"os"
	"strconv"
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
	InputStream  string
	OutputStream string
	ConsumerGroup string
	ConsumerName  string

	// Worker
	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: getEnv("SERVICE_NAME", "exchange-matching"),
		HTTPPort:    getEnvInt("HTTP_PORT", 8082),
		MetricsPort: getEnvInt("METRICS_PORT", 9082),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6380"),  // 默认使用6380避免与本地Redis冲突
		RedisPassword: getEnv("REDIS_PASSWORD", ""),
		RedisDB:       getEnvInt("REDIS_DB", 0),

		InputStream:   getEnv("INPUT_STREAM", "exchange:orders"),
		OutputStream:  getEnv("OUTPUT_STREAM", "exchange:events"),
		ConsumerGroup: getEnv("CONSUMER_GROUP", "matching-group"),
		ConsumerName:  getEnv("CONSUMER_NAME", "matching-1"),

		WorkerID: int64(getEnvInt("WORKER_ID", 1)),
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
