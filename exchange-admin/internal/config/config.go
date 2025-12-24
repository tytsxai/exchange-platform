// Package config 配置
package config

import (
	"strconv"
	"time"

	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int

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

	// Streams
	OrderStream             string
	EventStream             string
	PrivateUserEventChannel string

	// Auth
	AuthTokenSecret string
	AuthTokenTTL    time.Duration
	AdminToken      string

	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-admin"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8087),

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

		OrderStream:             envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:             envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		AuthTokenSecret: envconfig.GetEnv("AUTH_TOKEN_SECRET", ""),
		AuthTokenTTL:    envconfig.GetEnvDuration("AUTH_TOKEN_TTL", 24*time.Hour),
		AdminToken:      envconfig.GetEnv("ADMIN_TOKEN", ""),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 6),
	}
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
