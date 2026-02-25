package redis

import (
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTLSConfigFromEnv_DisabledByDefault(t *testing.T) {
	t.Setenv("REDIS_TLS", "")
	t.Setenv("REDIS_CACERT", "")
	t.Setenv("REDIS_CERT", "")
	t.Setenv("REDIS_KEY", "")
	t.Setenv("REDIS_SERVER_NAME", "")

	cfg, err := TLSConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil tls config when REDIS_TLS is disabled")
	}
}

func TestTLSConfigFromEnv_InvalidBool(t *testing.T) {
	t.Setenv("REDIS_TLS", "not-bool")

	if _, err := TLSConfigFromEnv(); err == nil {
		t.Fatal("expected error for invalid REDIS_TLS")
	}
}

func TestTLSConfigFromEnv_CertKeyPairValidation(t *testing.T) {
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_CERT", "/tmp/redis-client-cert.pem")
	t.Setenv("REDIS_KEY", "")

	if _, err := TLSConfigFromEnv(); err == nil {
		t.Fatal("expected error when REDIS_CERT is set without REDIS_KEY")
	}
}

func TestTLSConfigFromEnv_BasicTLSConfig(t *testing.T) {
	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_CERT", "")
	t.Setenv("REDIS_KEY", "")
	t.Setenv("REDIS_CACERT", "")
	t.Setenv("REDIS_SERVER_NAME", "redis.internal")

	cfg, err := TLSConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected non-nil tls config")
	}
	if cfg.MinVersion != tls.VersionTLS12 {
		t.Fatalf("unexpected min tls version: %d", cfg.MinVersion)
	}
	if cfg.ServerName != "redis.internal" {
		t.Fatalf("unexpected server name: %s", cfg.ServerName)
	}
}

func TestTLSConfigFromEnv_InvalidCACert(t *testing.T) {
	tmpDir := t.TempDir()
	caPath := filepath.Join(tmpDir, "invalid-ca.pem")
	if err := os.WriteFile(caPath, []byte("not-a-certificate"), 0o600); err != nil {
		t.Fatalf("write temp ca file: %v", err)
	}

	t.Setenv("REDIS_TLS", "true")
	t.Setenv("REDIS_CACERT", caPath)
	t.Setenv("REDIS_CERT", "")
	t.Setenv("REDIS_KEY", "")

	_, err := TLSConfigFromEnv()
	if err == nil {
		t.Fatal("expected error for invalid REDIS_CACERT")
	}
	if !strings.Contains(err.Error(), "REDIS_CACERT") {
		t.Fatalf("unexpected error: %v", err)
	}
}
