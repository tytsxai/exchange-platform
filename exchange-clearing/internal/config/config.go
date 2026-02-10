// Package config 配置
package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int
	GRPCPort    int
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
	OrderStream   string
	EventStream   string
	ConsumerGroup string
	ConsumerName  string

	// Private events (pub/sub)
	PrivateUserEventChannel string

	// Matching
	MatchingServiceURL string

	// Internal Auth
	InternalToken string

	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-clearing"),
		HTTPPort:    envconfig.GetEnvInt("CLEARING_HTTP_PORT", envconfig.GetEnvInt("HTTP_PORT", 8083)),
		GRPCPort:    envconfig.GetEnvInt("CLEARING_GRPC_PORT", envconfig.GetEnvInt("GRPC_PORT", 9083)),
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

		OrderStream:   envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:   envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		ConsumerGroup: envconfig.GetEnv("CONSUMER_GROUP", "clearing-group"),
		ConsumerName:  envconfig.GetEnv("CONSUMER_NAME", "clearing-1"),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		MatchingServiceURL: envconfig.GetEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),

		InternalToken: envconfig.GetEnv("INTERNAL_TOKEN", ""),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 2),
	}
}

func (c *Config) Validate() error {
	if c.InternalToken == "" {
		return fmt.Errorf("INTERNAL_TOKEN is required")
	}
	if c.AppEnv != "dev" {
		if envconfig.IsInsecureDevSecret(c.InternalToken) {
			return fmt.Errorf("INTERNAL_TOKEN must not be a dev placeholder (APP_ENV=%s)", c.AppEnv)
		}
		if len(c.InternalToken) < envconfig.MinSecretLength {
			return fmt.Errorf("INTERNAL_TOKEN must be at least %d characters (APP_ENV=%s)", envconfig.MinSecretLength, c.AppEnv)
		}
		if c.RedisPassword == "" {
			return fmt.Errorf("REDIS_PASSWORD is required (APP_ENV=%s)", c.AppEnv)
		}
		if strings.TrimSpace(c.ConsumerGroup) == "" || strings.TrimSpace(c.ConsumerName) == "" {
			return fmt.Errorf("CONSUMER_GROUP and CONSUMER_NAME are required (APP_ENV=%s)", c.AppEnv)
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
