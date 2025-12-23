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

	// Tron
	TronNodeURL    string
	TronGridAPIKey string

	// Dependencies
	ClearingServiceURL string

	// Jobs
	DepositScannerEnabled      bool
	DepositScannerIntervalSecs int
	DepositScannerMaxAddresses int

	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-wallet"),
		HTTPPort:    envconfig.GetEnvInt("HTTP_PORT", 8086),

		DBHost:     envconfig.GetEnv("DB_HOST", "localhost"),
		DBPort:     envconfig.GetEnvInt("DB_PORT", 5436), // 默认使用5436避免与其他项目冲突
		DBUser:     envconfig.GetEnv("DB_USER", "exchange"),
		DBPassword: envconfig.GetEnv("DB_PASSWORD", "exchange123"),
		DBName:     envconfig.GetEnv("DB_NAME", "exchange"),

		OrderStream:             envconfig.GetEnv("ORDER_STREAM", "exchange:orders"),
		EventStream:             envconfig.GetEnv("EVENT_STREAM", "exchange:events"),
		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		TronNodeURL:    envconfig.GetEnv("TRON_NODE_URL", "https://api.trongrid.io"),
		TronGridAPIKey: envconfig.GetEnv("TRON_GRID_API_KEY", ""),

		ClearingServiceURL: envconfig.GetEnv("CLEARING_SERVICE_URL", "http://localhost:8083"),

		DepositScannerEnabled:      envconfig.GetEnvBool("DEPOSIT_SCANNER_ENABLED", false),
		DepositScannerIntervalSecs: envconfig.GetEnvInt("DEPOSIT_SCANNER_INTERVAL_SECS", 15),
		DepositScannerMaxAddresses: envconfig.GetEnvInt("DEPOSIT_SCANNER_MAX_ADDRESSES", 200),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 7),
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
