// Package config 提供环境变量配置工具函数
package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

var insecureDevSecrets = map[string]struct{}{
	"dev-internal-token-change-me":           {},
	"dev-admin-token-change-me":              {},
	"dev-auth-token-secret-32-bytes-minimum": {},
	"dev-api-key-secret-32-bytes-minimum":    {},
}

const MinSecretLength = 32

// IsInsecureDevSecret returns true when the value matches a known dev placeholder secret.
// It is intended to prevent accidental production deployments with default credentials.
func IsInsecureDevSecret(value string) bool {
	_, ok := insecureDevSecrets[value]
	return ok
}

// GetEnv 获取环境变量，如果不存在则返回默认值
func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

// GetEnvInt 获取整数类型的环境变量
func GetEnvInt(key string, defaultValue int) int {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.Atoi(value); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetEnvInt64 获取int64类型的环境变量
func GetEnvInt64(key string, defaultValue int64) int64 {
	if value := os.Getenv(key); value != "" {
		if i, err := strconv.ParseInt(value, 10, 64); err == nil {
			return i
		}
	}
	return defaultValue
}

// GetEnvBool 获取布尔类型的环境变量
func GetEnvBool(key string, defaultValue bool) bool {
	if value := os.Getenv(key); value != "" {
		if b, err := strconv.ParseBool(value); err == nil {
			return b
		}
	}
	return defaultValue
}

// GetEnvFloat64 获取float64类型的环境变量
func GetEnvFloat64(key string, defaultValue float64) float64 {
	if value := os.Getenv(key); value != "" {
		if f, err := strconv.ParseFloat(value, 64); err == nil {
			return f
		}
	}
	return defaultValue
}

// GetEnvDuration 获取时间间隔类型的环境变量
func GetEnvDuration(key string, defaultValue time.Duration) time.Duration {
	if value := os.Getenv(key); value != "" {
		if d, err := time.ParseDuration(value); err == nil {
			return d
		}
	}
	return defaultValue
}

// GetEnvSlice 获取字符串切片类型的环境变量，使用逗号分隔
func GetEnvSlice(key string, defaultValue []string) []string {
	if value := os.Getenv(key); value != "" {
		parts := strings.Split(value, ",")
		result := make([]string, 0, len(parts))
		for _, part := range parts {
			trimmed := strings.TrimSpace(part)
			if trimmed != "" {
				result = append(result, trimmed)
			}
		}
		if len(result) > 0 {
			return result
		}
	}
	return defaultValue
}
