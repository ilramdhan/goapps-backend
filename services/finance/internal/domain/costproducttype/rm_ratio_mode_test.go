package costproducttype

import (
	"errors"
	"testing"
	"time"
)

func TestNew_RMRatioMode(t *testing.T) {
	tests := []struct {
		name    string
		in      string
		want    string
		wantErr error
	}{
		{"empty defaults to INPUT", "", RMRatioModeInput, nil},
		{"INPUT", "INPUT", RMRatioModeInput, nil},
		{"DENIER", "DENIER", RMRatioModeDenier, nil},
		{"lowercase normalised", " denier ", RMRatioModeDenier, nil},
		{"invalid", "OTHER", "", ErrInvalidRMRatioMode},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := New("MEL", "Melange", tc.in)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("err = %v, want %v", err, tc.wantErr)
			}
			if err == nil && got.RMRatioMode() != tc.want {
				t.Fatalf("mode = %q, want %q", got.RMRatioMode(), tc.want)
			}
		})
	}
}

func TestUpdate_RMRatioMode(t *testing.T) {
	c, err := New("MEL", "Melange", RMRatioModeDenier)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.Update("Melange 2", true, ""); err != nil || c.RMRatioMode() != RMRatioModeDenier {
		t.Fatalf("empty must keep existing: err=%v mode=%q", err, c.RMRatioMode())
	}
	if err := c.Update("Melange 2", true, "INPUT"); err != nil || c.RMRatioMode() != RMRatioModeInput {
		t.Fatalf("expected INPUT: err=%v mode=%q", err, c.RMRatioMode())
	}
	if err := c.Update("Melange 2", true, "BAD"); !errors.Is(err, ErrInvalidRMRatioMode) {
		t.Fatalf("expected ErrInvalidRMRatioMode, got %v", err)
	}
	if c.RMRatioMode() != RMRatioModeInput {
		t.Fatalf("failed update must not mutate mode, got %q", c.RMRatioMode())
	}
}

func TestRMRatioMode_UnsetReadsAsInput(t *testing.T) {
	c := Reconstruct(1, "X", "X", true, time.Time{}, time.Time{})
	if c.RMRatioMode() != RMRatioModeInput {
		t.Fatalf("got %q", c.RMRatioMode())
	}
}
