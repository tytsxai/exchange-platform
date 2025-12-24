package config

import (
	"testing"

	envconfig "github.com/exchange/common/pkg/config"
	commondecimal "github.com/exchange/common/pkg/decimal"
)

func TestPriceProtectionDefaults(t *testing.T) {
	t.Setenv("PRICE_PROTECTION_ENABLED", "")
	t.Setenv("PRICE_PROTECTION_DEFAULT_LIMIT_RATE", "")

	cfg := Load()
	if !cfg.PriceProtection.Enabled {
		t.Fatal("expected price protection enabled by default")
	}
	if cfg.PriceProtection.DefaultLimitRate.String() != "0.05" {
		t.Fatalf("expected default limit rate 0.05, got %s", cfg.PriceProtection.DefaultLimitRate.String())
	}
}

func TestPriceProtectionFromEnv(t *testing.T) {
	t.Setenv("PRICE_PROTECTION_ENABLED", "false")
	t.Setenv("PRICE_PROTECTION_DEFAULT_LIMIT_RATE", "0.1")

	cfg := Load()
	if cfg.PriceProtection.Enabled {
		t.Fatal("expected price protection disabled from env")
	}
	if cfg.PriceProtection.DefaultLimitRate.String() != "0.1" {
		t.Fatalf("expected limit rate 0.1, got %s", cfg.PriceProtection.DefaultLimitRate.String())
	}
}

func TestPriceProtectionInvalidLimitRate(t *testing.T) {
	t.Setenv("PRICE_PROTECTION_DEFAULT_LIMIT_RATE", "-0.01")

	cfg := Load()
	if cfg.PriceProtection.DefaultLimitRate.String() != "0.05" {
		t.Fatalf("expected default limit rate on invalid env, got %s", cfg.PriceProtection.DefaultLimitRate.String())
	}
}

func TestConfigHelpers(t *testing.T) {
	t.Setenv("TEST_ENV", "value")
	if envconfig.GetEnv("TEST_ENV", "default") != "value" {
		t.Fatal("expected getEnv to return value")
	}
	if envconfig.GetEnv("MISSING_ENV", "default") != "default" {
		t.Fatal("expected getEnv default")
	}

	t.Setenv("INT_ENV", "abc")
	if envconfig.GetEnvInt("INT_ENV", 5) != 5 {
		t.Fatal("expected getEnvInt default on invalid")
	}
	t.Setenv("INT_ENV", "6")
	if envconfig.GetEnvInt("INT_ENV", 5) != 6 {
		t.Fatal("expected getEnvInt parsed value")
	}

	t.Setenv("BOOL_ENV", "TRUE")
	if !envconfig.GetEnvBool("BOOL_ENV", false) {
		t.Fatal("expected getEnvBool true")
	}
	t.Setenv("BOOL_ENV", "0")
	if envconfig.GetEnvBool("BOOL_ENV", true) {
		t.Fatal("expected getEnvBool false")
	}
	t.Setenv("BOOL_ENV", "invalid")
	if !envconfig.GetEnvBool("BOOL_ENV", true) {
		t.Fatal("expected getEnvBool default")
	}

	t.Setenv("DEC_ENV", "invalid")
	val := getEnvDecimal("DEC_ENV", cfgDecimal("0.05"))
	if val.String() != "0.05" {
		t.Fatal("expected getEnvDecimal default on invalid")
	}
}

func TestConfigDSN(t *testing.T) {
	cfg := &Config{
		DBHost:     "localhost",
		DBPort:     5432,
		DBUser:     "user",
		DBPassword: "pass",
		DBName:     "db",
		DBSSLMode:  "require",
	}
	expected := "host=localhost port=5432 user=user password=pass dbname=db sslmode=require"
	if cfg.DSN() != expected {
		t.Fatalf("expected DSN %s, got %s", expected, cfg.DSN())
	}
}

func cfgDecimal(value string) commondecimal.Decimal {
	dec := commondecimal.MustNew(value)
	return *dec
}
