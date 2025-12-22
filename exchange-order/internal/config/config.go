// Package config 配置
package config

import (
	"os"
	"strconv"

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
		ServiceName: getEnv("SERVICE_NAME", "exchange-order"),
		HTTPPort:    getEnvInt("HTTP_PORT", 8081),

		DBHost:     getEnv("DB_HOST", "localhost"),
		DBPort:     getEnvInt("DB_PORT", 5436), // 默认使用5436避免与其他项目冲突
		DBUser:     getEnv("DB_USER", "exchange"),
		DBPassword: getEnv("DB_PASSWORD", "exchange123"),
		DBName:     getEnv("DB_NAME", "exchange"),

		RedisAddr:     getEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: getEnv("REDIS_PASSWORD", ""),

		OrderStream: getEnv("ORDER_STREAM", "exchange:orders"),

		EventStream:           getEnv("EVENT_STREAM", "exchange:events"),
		MatchingConsumerGroup: getEnv("MATCHING_CONSUMER_GROUP", "order-updater-group"),
		MatchingConsumerName:  getEnv("MATCHING_CONSUMER_NAME", "order-updater-1"),

		PrivateUserEventChannel: getEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		WorkerID: int64(getEnvInt("WORKER_ID", 3)),

		MatchingServiceURL: getEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),

		ClearingBaseURL: getEnv("CLEARING_BASE_URL", "http://localhost:8083"),

		PriceProtection: PriceProtectionConfig{
			Enabled:          getEnvBool("PRICE_PROTECTION_ENABLED", true),
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

func getEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if value == "true" || value == "1" || value == "TRUE" {
			return true
		}
		if value == "false" || value == "0" || value == "FALSE" {
			return false
		}
	}
	return defaultValue
}

func getEnvDecimal(key string, defaultValue commondecimal.Decimal) commondecimal.Decimal {
	if value := os.Getenv(key); value != "" {
		if v, err := commondecimal.New(value); err == nil && v.Cmp(commondecimal.Zero) > 0 {
			return *v
		}
	}
	return defaultValue
}
