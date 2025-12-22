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

	// PostgreSQL
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

	// Streams
	OrderStream             string
	EventStream             string
	PrivateUserEventChannel string

	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: getEnv("SERVICE_NAME", "exchange-wallet"),
		HTTPPort:    getEnvInt("HTTP_PORT", 8087),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5436), // 默认使用5436避免与其他项目冲突
		DBUser:     getEnv("DB_USER", "exchange"),
		DBPassword: getEnv("DB_PASSWORD", "exchange123"),
		DBName:     getEnv("DB_NAME", "exchange"),

		OrderStream:             getEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:             getEnv("EVENT_STREAM", "exchange:events"),
		PrivateUserEventChannel: getEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		WorkerID: int64(getEnvInt("WORKER_ID", 7)),
	}
}

// DSN 返回数据库连接字符串
func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" port=" + strconv.Itoa(c.DBPort) +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" sslmode=disable"
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
