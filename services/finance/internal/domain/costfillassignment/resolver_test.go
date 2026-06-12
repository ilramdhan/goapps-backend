package costfillassignment

import "testing"

func strp(s string) *string { return &s }
func i32p(i int32) *int32   { return &i }
func boolp(b bool) *bool    { return &b }

func TestResolve_GlobalOnly(t *testing.T) {
	global := Config{
		Tier: TierGlobal, RouteLevel: 7,
		FillerType: strp("DEPT"), FillerValue: strp("RND"),
		ApproverType: strp("USER"), ApproverValue: strp("u-boss"),
		ReapproveOnChange: boolp(false), SLAFillHours: i32p(48), SLAApproveHours: i32p(24),
	}
	got, err := Resolve(&global, nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FillerType != "DEPT" || got.FillerValue != "RND" {
		t.Fatalf("filler not from global: %+v", got)
	}
}

func TestResolve_ProductOverridesFillerOnly(t *testing.T) {
	global := Config{
		Tier: TierGlobal, RouteLevel: 7,
		FillerType: strp("DEPT"), FillerValue: strp("RND"),
		ApproverType: strp("USER"), ApproverValue: strp("u-boss"),
		ReapproveOnChange: boolp(false), SLAFillHours: i32p(48), SLAApproveHours: i32p(24),
	}
	product := Config{Tier: TierProduct, RouteLevel: 7, FillerValue: strp("TQM")}
	got, err := Resolve(&global, &product, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.FillerValue != "TQM" {
		t.Fatalf("expected product filler override TQM, got %s", got.FillerValue)
	}
	if got.ApproverValue != "u-boss" {
		t.Fatalf("approver should still come from global, got %s", got.ApproverValue)
	}
}

func TestResolve_RequestWinsOverProduct(t *testing.T) {
	global := Config{
		Tier: TierGlobal, RouteLevel: 7,
		FillerType: strp("DEPT"), FillerValue: strp("RND"),
		ReapproveOnChange: boolp(false), SLAFillHours: i32p(48), SLAApproveHours: i32p(24),
	}
	product := Config{Tier: TierProduct, RouteLevel: 7, FillerValue: strp("TQM")}
	request := Config{Tier: TierRequest, RouteLevel: 7, FillerValue: strp("FIN")}
	got, _ := Resolve(&global, &product, &request)
	if got.FillerValue != "FIN" {
		t.Fatalf("expected request filler FIN, got %s", got.FillerValue)
	}
}

func TestResolve_NoGlobalIsError(t *testing.T) {
	if _, err := Resolve(nil, nil, nil); err == nil {
		t.Fatal("expected ErrConfigNotFound when no global config")
	}
}
