// Package config 配置
package config

import (
	"strconv"

	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int
	GRPCPort    int

	// PostgreSQL
	DBHost     string
	DBPort     int
	DBUser     string
	DBPassword string
	DBName     string

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

	// Matching
	MatchingServiceURL string

	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-clearing"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8083),
		GRPCPort:    envconfig.GetEnvInt("GRPC_PORT", 9083),

		DBHost:     envconfig.GetEnv("DB_HOST", "localhost"),
		DBPort:     envconfig.GetEnvInt("DB_PORT", 5436), // 默认使用5436避免与其他项目冲突
		DBUser:     envconfig.GetEnv("DB_USER", "exchange"),
		DBPassword: envconfig.GetEnv("DB_PASSWORD", "exchange123"),
		DBName:     envconfig.GetEnv("DB_NAME", "exchange"),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),

		OrderStream:   envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:   envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup: envconfig.GetEnv("CONSUMER_GROUP", "clearing-group"),
		ConsumerName:  envconfig.GetEnv("CONSUMER_NAME", "clearing-1"),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		MatchingServiceURL: envconfig.GetEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 2),
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
