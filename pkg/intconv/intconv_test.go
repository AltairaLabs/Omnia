package intconv

import (
	"math"
	"testing"
)

func TestClampInt32(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   int64
		want int32
	}{
		{"zero", 0, 0},
		{"positive within range", 1 << 30, 1 << 30},
		{"negative within range", -(1 << 30), -(1 << 30)},
		{"max int32", math.MaxInt32, math.MaxInt32},
		{"min int32", math.MinInt32, math.MinInt32},
		{"just over max clamps", int64(math.MaxInt32) + 1, math.MaxInt32},
		{"just under min clamps", int64(math.MinInt32) - 1, math.MinInt32},
		{"max int64 clamps", math.MaxInt64, math.MaxInt32},
		{"min int64 clamps", math.MinInt64, math.MinInt32},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClampInt32(tc.in); got != tc.want {
				t.Errorf("ClampInt32(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestClampInt(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		in   int64
		want int
	}{
		{"zero", 0, 0},
		{"small positive", 42, 42},
		{"small negative", -42, -42},
		{"max int", math.MaxInt, math.MaxInt},
		{"min int", math.MinInt, math.MinInt},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ClampInt(tc.in); got != tc.want {
				t.Errorf("ClampInt(%d) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}
