// Package config 配置
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	envconfig "github.com/exchange/common/pkg/config"
	commondecimal "github.com/exchange/common/pkg/decimal"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int
	AppEnv      string

	// PostgreSQL
	DBHost            string
	DBPort            int
	DBUser            string
	DBPassword        string
	DBName            string
	DBSSLMode         string
	DBMaxOpenConns    int
	DBMaxIdleConns    int
	DBConnMaxLifetime time.Duration
	DBConnMaxIdleTime time.Duration

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
	InternalToken   string

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
		HTTPPort:    envconfig.GetEnvInt("ORDER_HTTP_PORT", envconfig.GetEnvInt("HTTP_PORT", 8081)),
		AppEnv:      strings.ToLower(envconfig.GetEnv("APP_ENV", "dev")),

		DBHost:            envconfig.GetEnv("DB_HOST", "localhost"),
		DBPort:            envconfig.GetEnvInt("DB_PORT", 5436), // 默认使用5436避免与其他项目冲突
		DBUser:            envconfig.GetEnv("DB_USER", "exchange"),
		DBPassword:        envconfig.GetEnv("DB_PASSWORD", "exchange123"),
		DBName:            envconfig.GetEnv("DB_NAME", "exchange"),
		DBSSLMode:         envconfig.GetEnv("DB_SSL_MODE", "disable"),
		DBMaxOpenConns:    envconfig.GetEnvInt("DB_MAX_OPEN_CONNS", 50),
		DBMaxIdleConns:    envconfig.GetEnvInt("DB_MAX_IDLE_CONNS", 10),
		DBConnMaxLifetime: envconfig.GetEnvDuration("DB_CONN_MAX_LIFETIME", 30*time.Minute),
		DBConnMaxIdleTime: envconfig.GetEnvDuration("DB_CONN_MAX_IDLE_TIME", 5*time.Minute),

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
		InternalToken:   envconfig.GetEnv("INTERNAL_TOKEN", ""),

		PriceProtection: PriceProtectionConfig{
			Enabled:          envconfig.GetEnvBool("PRICE_PROTECTION_ENABLED", true),
			DefaultLimitRate: getEnvDecimal("PRICE_PROTECTION_DEFAULT_LIMIT_RATE", defaultLimitRate),
		},
	}
}

func (c *Config) Validate() error {
	if c.InternalToken == "" {
		return fmt.Errorf("INTERNAL_TOKEN is required")
	}

	// 生产/预发必须显式配置，禁止使用 dev 默认值
	if c.AppEnv != "dev" {
		if envconfig.IsInsecureDevSecret(c.InternalToken) {
			return fmt.Errorf("INTERNAL_TOKEN must not be a dev placeholder (APP_ENV=%s)", c.AppEnv)
		}
		if c.RedisPassword == "" {
			return fmt.Errorf("REDIS_PASSWORD is required (APP_ENV=%s)", c.AppEnv)
		}
		if strings.TrimSpace(c.MatchingConsumerGroup) == "" || strings.TrimSpace(c.MatchingConsumerName) == "" {
			return fmt.Errorf("MATCHING_CONSUMER_GROUP and MATCHING_CONSUMER_NAME are required (APP_ENV=%s)", c.AppEnv)
		}
		if c.DBPassword == "" || c.DBPassword == "exchange123" {
			return fmt.Errorf("DB_PASSWORD must be explicitly set (APP_ENV=%s)", c.AppEnv)
		}
		if strings.EqualFold(c.DBSSLMode, "disable") {
			return fmt.Errorf("DB_SSL_MODE must not be disable (APP_ENV=%s)", c.AppEnv)
		}
	}
	return nil
}

// DSN 返回数据库连接字符串
func (c *Config) DSN() string {
	return "host=" + c.DBHost +
		" port=" + strconv.Itoa(c.DBPort) +
		" user=" + c.DBUser +
		" password=" + c.DBPassword +
		" dbname=" + c.DBName +
		" sslmode=" + c.DBSSLMode
}

func getEnvDecimal(key string, defaultValue commondecimal.Decimal) commondecimal.Decimal {
	if value := os.Getenv(key); value != "" {
		if v, err := commondecimal.New(value); err == nil && v.Cmp(commondecimal.Zero) > 0 {
			return *v
		}
	}
	return defaultValue
}
