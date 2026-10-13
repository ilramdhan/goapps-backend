package grpc

import (
	"testing"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

func TestCostProductTypeToProto_RMRatioMode(t *testing.T) {
	pt, err := domain.New("MEL", "Melange", domain.RMRatioModeDenier)
	if err != nil {
		t.Fatal(err)
	}
	var got *financev1.CostProductType = costProductTypeToProto(pt)
	if got.GetRmRatioMode() != domain.RMRatioModeDenier {
		t.Fatalf("rm_ratio_mode = %q, want DENIER", got.GetRmRatioMode())
	}
	def, _ := domain.New("POY", "Poy", "")
	if costProductTypeToProto(def).GetRmRatioMode() != domain.RMRatioModeInput {
		t.Fatal("default must map to INPUT")
	}
}

func TestProductTypeErrToBase_InvalidRMRatioMode(t *testing.T) {
	if b := productTypeErrToBase(domain.ErrInvalidRMRatioMode); b.GetStatusCode() != "400" {
		t.Fatalf("status = %q, want 400", b.GetStatusCode())
	}
}
