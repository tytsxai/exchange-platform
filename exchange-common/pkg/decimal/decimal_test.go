package decimal

import (
	"math/big"
	"testing"
)

func TestNew(t *testing.T) {
	tests := []struct {
		input     string
		wantVal   int64
		wantScale int
		wantErr   bool
	}{
		{"0", 0, 0, false},
		{"10", 10, 0, false},
		{"12.34", 1234, 2, false},
		{"-0.001", -1, 3, false},
		{"0.00000001", 1, 8, false},
		{"invalid", 0, 0, true},
	}

	for _, tt := range tests {
		got, err := New(tt.input)
		if (err != nil) != tt.wantErr {
			t.Errorf("New(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			continue
		}
		if !tt.wantErr {
			if got.value.Cmp(big.NewInt(tt.wantVal)) != 0 {
				t.Errorf("New(%q) value = %s, want %d", tt.input, got.value.String(), tt.wantVal)
			}
			if got.scale != tt.wantScale {
				t.Errorf("New(%q) scale = %d, want %d", tt.input, got.scale, tt.wantScale)
			}
		}
	}
}

func TestDecimal_Add(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"1", "2", "3"},
		{"1.1", "2.2", "3.3"},
		{"1.001", "0.002", "1.003"},
		{"1", "0.1", "1.1"},
	}

	for _, tt := range tests {
		da := MustNew(tt.a)
		db := MustNew(tt.b)
		got := da.Add(db)
		if got.String() != tt.want {
			t.Errorf("%s + %s = %s, want %s", tt.a, tt.b, got.String(), tt.want)
		}
	}
}

func TestDecimal_Mul(t *testing.T) {
	tests := []struct {
		a, b string
		want string
	}{
		{"2", "3", "6"},
		{"2.0", "3.00", "6"},
		{"0.5", "0.5", "0.25"},
		{"-2", "3", "-6"},
	}

	for _, tt := range tests {
		da := MustNew(tt.a)
		db := MustNew(tt.b)
		got := da.Mul(db)
		if got.String() != tt.want {
			t.Errorf("%s * %s = %s, want %s", tt.a, tt.b, got.String(), tt.want)
		}
	}
}

func TestDecimal_Div_Truncate(t *testing.T) {
	tests := []struct {
		a, b  string
		scale int
		want  string
	}{
		{"1.234", "1", 2, "1.23"},
		{"123.456", "100", 1, "1.2"},
		{"12.34", "100", 2, "0.12"},
	}

	for _, tt := range tests {
		da := MustNew(tt.a)
		db := MustNew(tt.b)
		got := da.Div(db, tt.scale)
		if got.String() != tt.want {
			t.Errorf("%s / %s scale=%d = %s, want %s", tt.a, tt.b, tt.scale, got.String(), tt.want)
		}
	}
}
