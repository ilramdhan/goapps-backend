package grpc

import (
	"context"
	"testing"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

type fakeErpAttrUpdater struct {
	got *app.UpdateErpAttributesCommand
}

func (f *fakeErpAttrUpdater) Handle(_ context.Context, c app.UpdateErpAttributesCommand) (*domain.CostProductMaster, error) {
	f.got = &c
	return nil, nil
}

func TestApplyErpAttrs(t *testing.T) {
	f := &fakeErpAttrUpdater{}
	h := &CostProductMasterHandler{}
	cur := &domain.CostProductMaster{}
	h.WithErpAttributes(f)
	if _, err := h.applyErpAttrs(context.Background(), cur, erpAttrPatch("", "", "", "", ""), "u"); err != nil || f.got != nil {
		t.Fatalf("empty patch must not call updater (err=%v)", err)
	}
	if _, err := h.applyErpAttrs(context.Background(), cur, erpAttrPatch("FG", "", "", "", "1.5"), "u"); err != nil {
		t.Fatal(err)
	}
	if f.got == nil || f.got.ProductSysID != 0 || f.got.Patch.FgType == nil || *f.got.Patch.FgType != "FG" ||
		f.got.Patch.ChpItemCode != nil || f.got.Patch.PrdPerDay == nil || f.got.ActorUserID != "u" {
		t.Fatalf("bad command %+v", f.got)
	}
}
