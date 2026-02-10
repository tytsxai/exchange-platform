package config

import (
	"os"
	"testing"
	"time"
)

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue string
		want         string
	}{
		{
			name:         "returns env value when set",
			key:          "TEST_GET_ENV_SET",
			envValue:     "custom_value",
			defaultValue: "default",
			want:         "custom_value",
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_UNSET",
			envValue:     "",
			defaultValue: "default_value",
			want:         "default_value",
		},
		{
			name:         "returns default when empty string",
			key:          "TEST_GET_ENV_EMPTY",
			envValue:     "",
			defaultValue: "fallback",
			want:         "fallback",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnv(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Fatalf("GetEnv(%q, %q) = %q, want %q", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestIsInsecureDevSecret(t *testing.T) {
	tests := []struct {
		name   string
		value  string
		unsafe bool
	}{
		{
			name:   "internal token placeholder",
			value:  "dev-internal-token-change-me",
			unsafe: true,
		},
		{
			name:   "admin token placeholder",
			value:  "dev-admin-token-change-me",
			unsafe: true,
		},
		{
			name:   "auth token placeholder",
			value:  "dev-auth-token-secret-32-bytes-minimum",
			unsafe: true,
		},
		{
			name:   "api key secret placeholder",
			value:  "dev-api-key-secret-32-bytes-minimum",
			unsafe: true,
		},
		{
			name:   "non-placeholder secret",
			value:  "prod-very-strong-random-secret-value",
			unsafe: false,
		},
		{
			name:   "empty secret",
			value:  "",
			unsafe: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsInsecureDevSecret(tt.value)
			if got != tt.unsafe {
				t.Fatalf("IsInsecureDevSecret(%q) = %v, want %v", tt.value, got, tt.unsafe)
			}
		})
	}
}

func TestMinSecretLength(t *testing.T) {
	if MinSecretLength != 32 {
		t.Fatalf("MinSecretLength = %d, want 32", MinSecretLength)
	}
}

func TestGetEnvInt(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue int
		want         int
	}{
		{
			name:         "returns parsed int when valid",
			key:          "TEST_GET_ENV_INT_VALID",
			envValue:     "42",
			defaultValue: 0,
			want:         42,
		},
		{
			name:         "returns negative int",
			key:          "TEST_GET_ENV_INT_NEG",
			envValue:     "-100",
			defaultValue: 0,
			want:         -100,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_INT_UNSET",
			envValue:     "",
			defaultValue: 10,
			want:         10,
		},
		{
			name:         "returns default when invalid",
			key:          "TEST_GET_ENV_INT_INVALID",
			envValue:     "not_a_number",
			defaultValue: 5,
			want:         5,
		},
		{
			name:         "returns default when float string",
			key:          "TEST_GET_ENV_INT_FLOAT",
			envValue:     "3.14",
			defaultValue: 7,
			want:         7,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnvInt(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Fatalf("GetEnvInt(%q, %d) = %d, want %d", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvInt64(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue int64
		want         int64
	}{
		{
			name:         "returns parsed int64 when valid",
			key:          "TEST_GET_ENV_INT64_VALID",
			envValue:     "9223372036854775807",
			defaultValue: 0,
			want:         9223372036854775807,
		},
		{
			name:         "returns negative int64",
			key:          "TEST_GET_ENV_INT64_NEG",
			envValue:     "-9223372036854775808",
			defaultValue: 0,
			want:         -9223372036854775808,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_INT64_UNSET",
			envValue:     "",
			defaultValue: 100,
			want:         100,
		},
		{
			name:         "returns default when invalid",
			key:          "TEST_GET_ENV_INT64_INVALID",
			envValue:     "invalid",
			defaultValue: 50,
			want:         50,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnvInt64(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Fatalf("GetEnvInt64(%q, %d) = %d, want %d", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue bool
		want         bool
	}{
		{
			name:         "returns true for 'true'",
			key:          "TEST_GET_ENV_BOOL_TRUE",
			envValue:     "true",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "returns true for '1'",
			key:          "TEST_GET_ENV_BOOL_ONE",
			envValue:     "1",
			defaultValue: false,
			want:         true,
		},
		{
			name:         "returns false for 'false'",
			key:          "TEST_GET_ENV_BOOL_FALSE",
			envValue:     "false",
			defaultValue: true,
			want:         false,
		},
		{
			name:         "returns false for '0'",
			key:          "TEST_GET_ENV_BOOL_ZERO",
			envValue:     "0",
			defaultValue: true,
			want:         false,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_BOOL_UNSET",
			envValue:     "",
			defaultValue: true,
			want:         true,
		},
		{
			name:         "returns default when invalid",
			key:          "TEST_GET_ENV_BOOL_INVALID",
			envValue:     "yes",
			defaultValue: false,
			want:         false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnvBool(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Fatalf("GetEnvBool(%q, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvFloat64(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue float64
		want         float64
	}{
		{
			name:         "returns parsed float when valid",
			key:          "TEST_GET_ENV_FLOAT_VALID",
			envValue:     "3.14159",
			defaultValue: 0,
			want:         3.14159,
		},
		{
			name:         "returns negative float",
			key:          "TEST_GET_ENV_FLOAT_NEG",
			envValue:     "-2.5",
			defaultValue: 0,
			want:         -2.5,
		},
		{
			name:         "returns integer as float",
			key:          "TEST_GET_ENV_FLOAT_INT",
			envValue:     "42",
			defaultValue: 0,
			want:         42.0,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_FLOAT_UNSET",
			envValue:     "",
			defaultValue: 1.5,
			want:         1.5,
		},
		{
			name:         "returns default when invalid",
			key:          "TEST_GET_ENV_FLOAT_INVALID",
			envValue:     "not_float",
			defaultValue: 2.0,
			want:         2.0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnvFloat64(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Fatalf("GetEnvFloat64(%q, %f) = %f, want %f", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvDuration(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue time.Duration
		want         time.Duration
	}{
		{
			name:         "returns parsed duration seconds",
			key:          "TEST_GET_ENV_DUR_SEC",
			envValue:     "30s",
			defaultValue: 0,
			want:         30 * time.Second,
		},
		{
			name:         "returns parsed duration minutes",
			key:          "TEST_GET_ENV_DUR_MIN",
			envValue:     "5m",
			defaultValue: 0,
			want:         5 * time.Minute,
		},
		{
			name:         "returns parsed duration hours",
			key:          "TEST_GET_ENV_DUR_HOUR",
			envValue:     "2h",
			defaultValue: 0,
			want:         2 * time.Hour,
		},
		{
			name:         "returns parsed duration milliseconds",
			key:          "TEST_GET_ENV_DUR_MS",
			envValue:     "500ms",
			defaultValue: 0,
			want:         500 * time.Millisecond,
		},
		{
			name:         "returns parsed complex duration",
			key:          "TEST_GET_ENV_DUR_COMPLEX",
			envValue:     "1h30m",
			defaultValue: 0,
			want:         90 * time.Minute,
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_DUR_UNSET",
			envValue:     "",
			defaultValue: 10 * time.Second,
			want:         10 * time.Second,
		},
		{
			name:         "returns default when invalid",
			key:          "TEST_GET_ENV_DUR_INVALID",
			envValue:     "invalid",
			defaultValue: 5 * time.Second,
			want:         5 * time.Second,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnvDuration(tt.key, tt.defaultValue)
			if got != tt.want {
				t.Fatalf("GetEnvDuration(%q, %v) = %v, want %v", tt.key, tt.defaultValue, got, tt.want)
			}
		})
	}
}

func TestGetEnvSlice(t *testing.T) {
	tests := []struct {
		name         string
		key          string
		envValue     string
		defaultValue []string
		want         []string
	}{
		{
			name:         "returns parsed slice",
			key:          "TEST_GET_ENV_SLICE_VALID",
			envValue:     "a,b,c",
			defaultValue: nil,
			want:         []string{"a", "b", "c"},
		},
		{
			name:         "trims whitespace",
			key:          "TEST_GET_ENV_SLICE_SPACE",
			envValue:     " a , b , c ",
			defaultValue: nil,
			want:         []string{"a", "b", "c"},
		},
		{
			name:         "filters empty parts",
			key:          "TEST_GET_ENV_SLICE_EMPTY",
			envValue:     "a,,b,  ,c",
			defaultValue: nil,
			want:         []string{"a", "b", "c"},
		},
		{
			name:         "returns single element",
			key:          "TEST_GET_ENV_SLICE_SINGLE",
			envValue:     "single",
			defaultValue: nil,
			want:         []string{"single"},
		},
		{
			name:         "returns default when not set",
			key:          "TEST_GET_ENV_SLICE_UNSET",
			envValue:     "",
			defaultValue: []string{"default"},
			want:         []string{"default"},
		},
		{
			name:         "returns default when only commas",
			key:          "TEST_GET_ENV_SLICE_COMMAS",
			envValue:     ",,,",
			defaultValue: []string{"fallback"},
			want:         []string{"fallback"},
		},
		{
			name:         "returns default when only spaces",
			key:          "TEST_GET_ENV_SLICE_SPACES",
			envValue:     "  ,  ,  ",
			defaultValue: []string{"default"},
			want:         []string{"default"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.envValue != "" {
				os.Setenv(tt.key, tt.envValue)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := GetEnvSlice(tt.key, tt.defaultValue)

			if len(got) != len(tt.want) {
				t.Fatalf("GetEnvSlice(%q, %v) length = %d, want %d", tt.key, tt.defaultValue, len(got), len(tt.want))
			}

			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("GetEnvSlice(%q, %v)[%d] = %q, want %q", tt.key, tt.defaultValue, i, got[i], tt.want[i])
				}
			}
		})
	}
}
