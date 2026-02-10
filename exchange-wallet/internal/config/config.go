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

	// Streams
	OrderStream             string
	EventStream             string
	PrivateUserEventChannel string

	// Tron
	TronNodeURL    string
	TronGridAPIKey string

	// Dependencies
	ClearingServiceURL string
	InternalToken      string

	// Jobs
	DepositScannerEnabled      bool
	DepositScannerIntervalSecs int
	DepositScannerMaxAddresses int
	AllowMVPDepositScanner     bool

	// Auth
	AuthTokenSecret string
	AuthTokenTTL    time.Duration
	AdminToken      string

	// Docs
	EnableDocs        bool
	AllowDocsInNonDev bool

	WorkerID int64
}

// Load 加载配置
func Load() *Config {
	appEnv := strings.ToLower(envconfig.GetEnv("APP_ENV", "dev"))
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-wallet"),
		HTTPPort:    envconfig.GetEnvInt("WALLET_HTTP_PORT", envconfig.GetEnvInt("HTTP_PORT", 8086)),
		AppEnv:      appEnv,

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

		TronNodeURL:    envconfig.GetEnv("TRON_NODE_URL", "https://api.trongrid.io"),
		TronGridAPIKey: envconfig.GetEnv("TRON_GRID_API_KEY", ""),

		ClearingServiceURL: envconfig.GetEnv("CLEARING_SERVICE_URL", "http://localhost:8083"),
		InternalToken:      envconfig.GetEnv("INTERNAL_TOKEN", ""),

		DepositScannerEnabled:      envconfig.GetEnvBool("DEPOSIT_SCANNER_ENABLED", false),
		DepositScannerIntervalSecs: envconfig.GetEnvInt("DEPOSIT_SCANNER_INTERVAL_SECS", 15),
		DepositScannerMaxAddresses: envconfig.GetEnvInt("DEPOSIT_SCANNER_MAX_ADDRESSES", 200),
		AllowMVPDepositScanner:     envconfig.GetEnvBool("ALLOW_MVP_DEPOSIT_SCANNER", false),

		AuthTokenSecret: envconfig.GetEnv("AUTH_TOKEN_SECRET", ""),
		AuthTokenTTL:    envconfig.GetEnvDuration("AUTH_TOKEN_TTL", 24*time.Hour),
		AdminToken:      envconfig.GetEnv("ADMIN_TOKEN", ""),

		EnableDocs:        envconfig.GetEnvBool("ENABLE_DOCS", appEnv == "dev"),
		AllowDocsInNonDev: envconfig.GetEnvBool("ALLOW_DOCS_IN_NONDEV", false),

		WorkerID: envconfig.GetEnvInt64("WORKER_ID", 7),
	}
}

func (c *Config) Validate() error {
	if c.InternalToken == "" {
		return fmt.Errorf("INTERNAL_TOKEN is required")
	}
	if c.AuthTokenSecret == "" {
		return fmt.Errorf("AUTH_TOKEN_SECRET is required")
	}
	if len(c.AuthTokenSecret) < envconfig.MinSecretLength {
		return fmt.Errorf("AUTH_TOKEN_SECRET must be at least %d characters", envconfig.MinSecretLength)
	}
	if c.AdminToken == "" {
		return fmt.Errorf("ADMIN_TOKEN is required")
	}
	if c.AppEnv != "dev" {
		if c.EnableDocs && !c.AllowDocsInNonDev {
			return fmt.Errorf("ENABLE_DOCS must be false unless ALLOW_DOCS_IN_NONDEV=true (APP_ENV=%s)", c.AppEnv)
		}
		if len(c.InternalToken) < envconfig.MinSecretLength {
			return fmt.Errorf("INTERNAL_TOKEN must be at least %d characters (APP_ENV=%s)", envconfig.MinSecretLength, c.AppEnv)
		}
		if envconfig.IsInsecureDevSecret(c.InternalToken) {
			return fmt.Errorf("INTERNAL_TOKEN must not be a dev placeholder (APP_ENV=%s)", c.AppEnv)
		}
		if envconfig.IsInsecureDevSecret(c.AuthTokenSecret) {
			return fmt.Errorf("AUTH_TOKEN_SECRET must not be a dev placeholder (APP_ENV=%s)", c.AppEnv)
		}
		if len(c.AdminToken) < envconfig.MinSecretLength {
			return fmt.Errorf("ADMIN_TOKEN must be at least %d characters (APP_ENV=%s)", envconfig.MinSecretLength, c.AppEnv)
		}
		if envconfig.IsInsecureDevSecret(c.AdminToken) {
			return fmt.Errorf("ADMIN_TOKEN must not be a dev placeholder (APP_ENV=%s)", c.AppEnv)
		}
		if c.DBPassword == "" || c.DBPassword == "exchange123" {
			return fmt.Errorf("DB_PASSWORD must be explicitly set (APP_ENV=%s)", c.AppEnv)
		}
		if strings.EqualFold(c.DBSSLMode, "disable") {
			return fmt.Errorf("DB_SSL_MODE must not be disable (APP_ENV=%s)", c.AppEnv)
		}
		if c.DepositScannerEnabled && !c.AllowMVPDepositScanner {
			return fmt.Errorf("DEPOSIT_SCANNER_ENABLED is high-risk MVP; set ALLOW_MVP_DEPOSIT_SCANNER=true to override (APP_ENV=%s)", c.AppEnv)
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
