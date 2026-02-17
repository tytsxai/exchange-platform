package recovery

import "testing"

func TestParseScaledInt(t *testing.T) {
	tests := []struct {
		name      string
		value     string
		precision int
		want      int64
		wantErr   bool
	}{
		{name: "int", value: "123", precision: 8, want: 123},
		{name: "decimal", value: "1.23", precision: 2, want: 123},
		{name: "empty", value: "", precision: 8, want: 0},
		{name: "invalid", value: "abc", precision: 8, wantErr: true},
	}

	for _, tc := range tests {
		got, err := parseScaledInt(tc.value, tc.precision)
		if tc.wantErr {
			if err == nil {
				t.Fatalf("%s: expected error", tc.name)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: unexpected error: %v", tc.name, err)
		}
		if got != tc.want {
			t.Fatalf("%s: got %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestEnumMappers(t *testing.T) {
	if sideToString(1) != "BUY" || sideToString(2) != "SELL" {
		t.Fatal("side mapper failed")
	}
	if orderTypeToString(1) != "LIMIT" || orderTypeToString(2) != "MARKET" {
		t.Fatal("order type mapper failed")
	}
	if timeInForceToString(1) != "GTC" || timeInForceToString(4) != "POST_ONLY" {
		t.Fatal("tif mapper failed")
	}
}
