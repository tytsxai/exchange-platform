// Package redis Redis 客户端封装
package redis

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strconv"
	"strings"
)

const (
	envRedisTLS        = "REDIS_TLS"
	envRedisCACert     = "REDIS_CACERT"
	envRedisCert       = "REDIS_CERT"
	envRedisKey        = "REDIS_KEY"
	envRedisServerName = "REDIS_SERVER_NAME"
)

// TLSConfigFromEnv builds a Redis TLS config from environment variables.
//
// Supported envs:
// - REDIS_TLS=true/false
// - REDIS_CACERT=/path/to/ca.pem
// - REDIS_CERT=/path/to/client-cert.pem
// - REDIS_KEY=/path/to/client-key.pem
// - REDIS_SERVER_NAME=redis.example.com
func TLSConfigFromEnv() (*tls.Config, error) {
	enabled, err := envBool(envRedisTLS, false)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %w", envRedisTLS, err)
	}
	if !enabled {
		return nil, nil
	}

	caCertPath := strings.TrimSpace(os.Getenv(envRedisCACert))
	certPath := strings.TrimSpace(os.Getenv(envRedisCert))
	keyPath := strings.TrimSpace(os.Getenv(envRedisKey))
	serverName := strings.TrimSpace(os.Getenv(envRedisServerName))

	if (certPath == "") != (keyPath == "") {
		return nil, fmt.Errorf("%s and %s must be set together", envRedisCert, envRedisKey)
	}

	cfg := &tls.Config{
		MinVersion: tls.VersionTLS12,
		ServerName: serverName,
	}

	if caCertPath != "" {
		caBytes, err := os.ReadFile(caCertPath)
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", envRedisCACert, err)
		}
		pool, err := x509.SystemCertPool()
		if err != nil || pool == nil {
			pool = x509.NewCertPool()
		}
		if ok := pool.AppendCertsFromPEM(caBytes); !ok {
			return nil, fmt.Errorf("append %s: no valid certificates found", envRedisCACert)
		}
		cfg.RootCAs = pool
	}

	if certPath != "" {
		cert, err := tls.LoadX509KeyPair(certPath, keyPath)
		if err != nil {
			return nil, fmt.Errorf("load %s/%s: %w", envRedisCert, envRedisKey, err)
		}
		cfg.Certificates = []tls.Certificate{cert}
	}

	return cfg, nil
}

func envBool(key string, defaultValue bool) (bool, error) {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return defaultValue, nil
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		return false, err
	}
	return v, nil
}
