package commonlot_test

import (
	"errors"
	"testing"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/commonlot"
)

func TestNewComponentValidation(t *testing.T) {
	tests := []struct {
		name    string
		lot     string
		bobbins int32
		qty     float64
		wantErr error
	}{
		{"valid", "LOT1", 10, 25.5, nil},
		{"empty lot", "  ", 10, 25.5, commonlot.ErrEmptyOriginalLot},
		{"negative bobbins", "LOT1", -1, 25.5, commonlot.ErrNegativeBobbin},
		{"negative qty", "LOT1", 10, -0.1, commonlot.ErrNegativeQty},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := commonlot.NewComponent(tt.lot, "SH1", tt.bobbins, tt.qty)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewCommonLotRequiresComponents(t *testing.T) {
	if _, err := commonlot.NewCommonLot("CL1", "ITEM", "SH", "AX", nil); !errors.Is(err, commonlot.ErrNoComponents) {
		t.Fatalf("err = %v, want ErrNoComponents", err)
	}
	if _, err := commonlot.NewCommonLot("  ", "ITEM", "SH", "AX", []commonlot.Component{{}}); !errors.Is(err, commonlot.ErrEmptyLotNo) {
		t.Fatalf("err = %v, want ErrEmptyLotNo", err)
	}
}

func TestCommonLotTotalsAndIntegrity(t *testing.T) {
	c1, err := commonlot.NewComponent("LOT_A", "SH_DARK", 30, 120.5)
	if err != nil {
		t.Fatalf("NewComponent c1: %v", err)
	}
	c2, err := commonlot.NewComponent("LOT_B", "SH_LIGHT", 10, 40.0)
	if err != nil {
		t.Fatalf("NewComponent c2: %v", err)
	}
	lot, err := commonlot.NewCommonLot("CL_NEW", "ITEM_X", "SH_MIX", "AX", []commonlot.Component{c1, c2})
	if err != nil {
		t.Fatalf("NewCommonLot: %v", err)
	}
	if got := lot.TotalBobbins(); got != 40 {
		t.Errorf("TotalBobbins = %d, want 40", got)
	}
	if got := lot.TotalQtyKg(); got != 160.5 {
		t.Errorf("TotalQtyKg = %g, want 160.5", got)
	}
	if len(lot.Components()) != 2 {
		t.Errorf("Components len = %d, want 2", len(lot.Components()))
	}
	// Component provenance preserved: original lots retained across shades.
	if lot.Components()[0].OriginalLotNo() != "LOT_A" || lot.Components()[1].OriginalLotNo() != "LOT_B" {
		t.Errorf("component original lots not preserved: %+v", lot.Components())
	}
	if lot.Components()[0].OriginalShadeCode() != "SH_DARK" {
		t.Errorf("component shade not preserved")
	}
}
