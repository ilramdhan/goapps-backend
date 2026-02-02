package safeconv

import (
	"math"
	"testing"
)

func TestInt64ToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    int64
		expected int32
	}{
		{"normal positive", 100, 100},
		{"normal negative", -100, -100},
		{"zero", 0, 0},
		{"max int32", math.MaxInt32, math.MaxInt32},
		{"min int32", math.MinInt32, math.MinInt32},
		{"overflow positive", math.MaxInt32 + 1, math.MaxInt32},
		{"overflow negative", math.MinInt32 - 1, math.MinInt32},
		{"large positive", math.MaxInt64, math.MaxInt32},
		{"large negative", math.MinInt64, math.MinInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Int64ToInt32(tt.input)
			if result != tt.expected {
				t.Errorf("Int64ToInt32(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}

func TestIntToInt32(t *testing.T) {
	tests := []struct {
		name     string
		input    int
		expected int32
	}{
		{"normal positive", 100, 100},
		{"normal negative", -100, -100},
		{"zero", 0, 0},
		{"max int32", math.MaxInt32, math.MaxInt32},
		{"min int32", math.MinInt32, math.MinInt32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := IntToInt32(tt.input)
			if result != tt.expected {
				t.Errorf("IntToInt32(%d) = %d, want %d", tt.input, result, tt.expected)
			}
		})
	}
}
