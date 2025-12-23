// Package config 配置
package config

import (
	"os"
	"strconv"

	envconfig "github.com/exchange/common/pkg/config"
	commondecimal "github.com/exchange/common/pkg/decimal"
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

	// Redis
	RedisAddr     string
	RedisPassword string

	// Streams
	OrderStream string

	// Matching events
	EventStream           string
	MatchingConsumerGroup string
	MatchingConsumerName  string

	// Private events (pub/sub)
	PrivateUserEventChannel string

	WorkerID int64

	// Matching
	MatchingServiceURL string

	// Clearing
	ClearingBaseURL string

	// Price protection
	PriceProtection PriceProtectionConfig
}

// PriceProtectionConfig 价格保护配置
type PriceProtectionConfig struct {
	Enabled          bool
	DefaultLimitRate commondecimal.Decimal
}

// Load 加载配置
func Load() *Config {
	defaultLimitRate := *commondecimal.MustNew("0.05")
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-order"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8081),

		DBHost:     envconfig.GetEnv("DB_HOST", "localhost"),
		DBPort:     envconfig.GetEnvInt("DB_PORT", 5436), // 默认使用5436避免与其他项目冲突
		DBUser:     envconfig.GetEnv("DB_USER", "exchange"),
		DBPassword: envconfig.GetEnv("DB_PASSWORD", "exchange123"),
		DBName:     envconfig.GetEnv("DB_NAME", "exchange"),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),

		OrderStream: envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),

		EventStream:           envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		MatchingConsumerGroup: envconfig.GetEnv("MATCHING_CONSUMER_GROUP", "order-updater-group"),
		MatchingConsumerName:  envconfig.GetEnv("MATCHING_CONSUMER_NAME", "order-updater-1"),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 3),

		MatchingServiceURL: envconfig.GetEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),

		ClearingBaseURL: envconfig.GetEnv("CLEARING_BASE_URL", "http://localhost:8083"),

		PriceProtection: PriceProtectionConfig{
			Enabled:          envconfig.GetEnvBool("PRICE_PROTECTION_ENABLED", true),
			DefaultLimitRate: getEnvDecimal("PRICE_PROTECTION_DEFAULT_LIMIT_RATE", defaultLimitRate),
		},
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

func getEnvDecimal(key string, defaultValue commondecimal.Decimal) commondecimal.Decimal {
	if value := os.Getenv(key); value != "" {
		if v, err := commondecimal.New(value); err == nil && v.Cmp(commondecimal.Zero) > 0 {
			return *v
		}
	}
	return defaultValue
}
