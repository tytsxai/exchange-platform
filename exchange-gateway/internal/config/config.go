// Package config 配置
package config

import (
	"fmt"
	"strings"

	envconfig "github.com/exchange/common/pkg/config"
)

// Config 服务配置
type Config struct {
	ServiceName string
	HTTPPort    int
	WSPort      int
	AppEnv      string

	// 后端服务
	OrderServiceURL      string
	ClearingServiceURL   string
	UserServiceURL       string
	MatchingServiceURL   string
	MarketDataServiceURL string

	// Redis
	RedisAddr     string
	RedisPassword string

	// Private events (pub/sub)
	PrivateUserEventChannel string

	// Internal Auth
	InternalToken string

	// CORS
	CORSAllowOrigins  []string
	TrustedProxyCIDRs []string

	// Docs
	EnableDocs        bool
	AllowDocsInNonDev bool

	// 限流
	IPRateLimit   int // 每秒请求数
	UserRateLimit int
}

// Load 加载配置
func Load() *Config {
	appEnv := strings.ToLower(envconfig.GetEnv("APP_ENV", "dev"))
	defaultOrigins := []string{"*"}
	if appEnv != "dev" {
		defaultOrigins = []string{}
	}
	return &Config{
		ServiceName: envconfig.GetEnv("SERVICE_NAME", "exchange-gateway"),
		HTTPPort:    envconfig.GetEnvInt("GATEWAY_HTTP_PORT", envconfig.GetEnvInt("HTTP_PORT", 8080)),
		WSPort:      envconfig.GetEnvInt("GATEWAY_WS_PORT", envconfig.GetEnvInt("WS_PORT", 8090)),
		AppEnv:      appEnv,

		OrderServiceURL:      envconfig.GetEnv("ORDER_SERVICE_URL", "http://localhost:8081"),
		ClearingServiceURL:   envconfig.GetEnv("CLEARING_SERVICE_URL", "http://localhost:8083"),
		UserServiceURL:       envconfig.GetEnv("USER_SERVICE_URL", "http://localhost:8085"),
		MatchingServiceURL:   envconfig.GetEnv("MATCHING_SERVICE_URL", "http://localhost:8082"),
		MarketDataServiceURL: envconfig.GetEnv("MARKETDATA_SERVICE_URL", "http://localhost:8084"),

		RedisAddr:     envconfig.GetEnv("REDIS_ADDR", "localhost:6380"), // 默认使用6380避免与本地Redis冲突
		RedisPassword: envconfig.GetEnv("REDIS_PASSWORD", ""),

		PrivateUserEventChannel: envconfig.GetEnv("PRIVATE_USER_EVENT_CHANNEL", "private:user:{userId}:events"),

		InternalToken: envconfig.GetEnv("INTERNAL_TOKEN", ""),

		CORSAllowOrigins:  envconfig.GetEnvSlice("CORS_ALLOW_ORIGINS", defaultOrigins),
		TrustedProxyCIDRs: envconfig.GetEnvSlice("TRUSTED_PROXY_CIDRS", nil),
		EnableDocs:        envconfig.GetEnvBool("ENABLE_DOCS", appEnv == "dev"),
		AllowDocsInNonDev: envconfig.GetEnvBool("ALLOW_DOCS_IN_NONDEV", false),

		IPRateLimit:   envconfig.GetEnvInt("IP_RATE_LIMIT", 100),
		UserRateLimit: envconfig.GetEnvInt("USER_RATE_LIMIT", 50),
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
		if c.RedisPassword == "" {
			return fmt.Errorf("REDIS_PASSWORD is required (APP_ENV=%s)", c.AppEnv)
		}
	}
	if c.AppEnv != "dev" {
		if c.EnableDocs && !c.AllowDocsInNonDev {
			return fmt.Errorf("ENABLE_DOCS must be false unless ALLOW_DOCS_IN_NONDEV=true (APP_ENV=%s)", c.AppEnv)
		}
		for _, o := range c.CORSAllowOrigins {
			if o == "*" {
				return fmt.Errorf("CORS_ALLOW_ORIGINS must not include '*' when APP_ENV=%s", c.AppEnv)
			}
		}
	}
	return nil
}
