package costroute

import (
	"math"
	"reflect"
	"testing"
)

// --- RM denier override (rm-denier-override, migration 000578) -------------

func assertNear(t *testing.T, want, got float64, what string) {
	t.Helper()
	if math.Abs(want-got) > tDelta {
		t.Fatalf("%s = %v, want %v", what, got, want)
	}
}

func TestComputeSeqRMRatios_ProductOverride(t *testing.T) {
	seq := &Seq{RouteLevel: 1, Rms: []*Rm{tProduct(tUpA, 1), tProduct(tUpB, 1)}}
	src := tSrc()
	src.ProductDenierOverride = map[int64]float64{tUpA: 30}

	lines, notes := ComputeSeqRMRatios(seq, src)
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none", notes)
	}
	assertNear(t, 30, lines[0].Denier, "overridden denier")
	assertNear(t, 30.0/180, lines[0].Ratio, "overridden ratio")
	if !lines[0].Overridden || lines[0].Source != RatioSourceDenier {
		t.Fatalf("line 0 = %+v, want overridden DENIER line", lines[0])
	}
	assertNear(t, 150, lines[1].Denier, "master denier")
	assertNear(t, 150.0/180, lines[1].Ratio, "master ratio")
	if lines[1].Overridden {
		t.Fatal("line 1 must not be overridden")
	}
}

func TestComputeSeqRMRatios_ProductOverrideNonPositiveIgnored(t *testing.T) {
	seq := &Seq{Rms: []*Rm{tProduct(tUpA, 1), tProduct(tUpB, 1)}}
	src := tSrc()
	src.ProductDenierOverride = map[int64]float64{tUpA: 0, tUpB: -5}
	got, _ := ComputeSeqRMRatios(seq, src)
	want, _ := ComputeSeqRMRatios(seq, tSrc())
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("non-positive overrides must be ignored: got %+v want %+v", got, want)
	}
}

func TestComputeSeqRMRatios_SpandexGroupOverrideNotDividedByDraft(t *testing.T) {
	seq := &Seq{Rms: []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1)}}
	src := tSrc()
	src.SpandexDraft = 0 // SPANDEX_DRAFT missing: not needed when overridden
	src.GroupDenierOverride = map[string]float64{tSpd: 10}

	lines, notes := ComputeSeqRMRatios(seq, src)
	if len(notes) != 0 {
		t.Fatalf("notes = %v, want none (draft not required)", notes)
	}
	assertNear(t, 10, lines[1].Denier, "spandex override is the final denier")
	assertNear(t, 10.0/80, lines[1].Ratio, "spandex ratio")
	assertNear(t, 70.0/80, lines[0].Ratio, "product ratio")
	if !lines[1].Overridden || lines[1].Source != RatioSourceDenier {
		t.Fatalf("line 1 = %+v, want overridden DENIER line", lines[1])
	}
}

func TestComputeSeqRMRatios_GroupWithoutHeadDenierOverride(t *testing.T) {
	seq := &Seq{Rms: []*Rm{tProduct(tUpA, 1), tGroup(tOil, 0.02)}}
	base, _ := ComputeSeqRMRatios(seq, tSrc())
	if base[1].Source != RatioSourceInput {
		t.Fatalf("precondition: oil group without head denier is INPUT, got %+v", base[1])
	}

	src := tSrc()
	src.GroupDenierOverride = map[string]float64{tOil: 30}
	lines, _ := ComputeSeqRMRatios(seq, src)
	if lines[1].Source != RatioSourceDenier || !lines[1].Overridden {
		t.Fatalf("override makes the group a denier line, got %+v", lines[1])
	}
	assertNear(t, 30.0/100, lines[1].Ratio, "group ratio")
	assertNear(t, 70.0/100, lines[0].Ratio, "product ratio")
}

func TestComputeSeqRMRatios_ItemLineNeverOverridden(t *testing.T) {
	item := &Rm{RmType: RmTypeItem, RmItemCode: "IT1", RouteRmRatio: 0.5}
	seq := &Seq{Rms: []*Rm{tProduct(tUpA, 1), item}}
	src := tSrc()
	src.GroupDenierOverride = map[string]float64{"IT1": 99}
	lines, _ := ComputeSeqRMRatios(seq, src)
	if lines[1].Source != RatioSourceInput || lines[1].Overridden || lines[1].Ratio != 0.5 {
		t.Fatalf("ITEM line = %+v, want INPUT passthrough", lines[1])
	}
}

func TestComputeSeqRMRatios_NilOverridesIdentical(t *testing.T) {
	seq := &Seq{RouteLevel: 2, Rms: []*Rm{tProduct(tUpA, 1), tProduct(999, 1), tGroup(tSpd, 1), tGroup(tOil, 0.1), nil}}
	want, wantNotes := ComputeSeqRMRatios(seq, tSrc())
	src := tSrc()
	src.ProductDenierOverride = map[int64]float64{}
	src.GroupDenierOverride = map[string]float64{}
	got, gotNotes := ComputeSeqRMRatios(seq, src)
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotNotes, wantNotes) {
		t.Fatalf("empty overrides changed result: got %+v %v want %+v %v", got, gotNotes, want, wantNotes)
	}
}

func TestComputeSeqRMRatios_InputModeIgnoresOverrides(t *testing.T) {
	seq := &Seq{Rms: []*Rm{tProduct(tUpA, 0.4), tGroup(tSpd, 0.6)}}
	src := tSrc()
	src.Mode = RMRatioModeInput
	src.ProductDenierOverride = map[int64]float64{tUpA: 30}
	src.GroupDenierOverride = map[string]float64{tSpd: 10}
	lines, notes := ComputeSeqRMRatios(seq, src)
	if notes != nil {
		t.Fatalf("notes = %v", notes)
	}
	want := []LineRatio{{Ratio: 0.4}, {Ratio: 0.6}}
	if !reflect.DeepEqual(lines, want) {
		t.Fatalf("INPUT mode lines = %+v, want %+v", lines, want)
	}
}

func TestGraph_AnnotateRMRatios_OverridePerSeqScope(t *testing.T) {
	const prodX, prodY int64 = 10, 20
	g := &Graph{Seqs: []*Seq{
		{ProductSysID: prodX, Rms: []*Rm{tProduct(tUpA, 1), tProduct(tUpB, 1)}},
		{ProductSysID: prodY, Rms: []*Rm{tProduct(tUpA, 1), tProduct(tUpB, 1)}},
	}}
	g.AnnotateRMRatios(RatioSources{
		Modes:          map[int64]string{prodX: RMRatioModeDenier, prodY: RMRatioModeDenier},
		UpstreamDenier: map[int64]float64{tUpA: 70, tUpB: 150},
		Overrides:      map[int64]DenierOverrides{prodX: {Product: map[int64]float64{tUpA: 30}}},
	})

	x, y := g.Seqs[0].Rms, g.Seqs[1].Rms
	if !x[0].DenierOverridden || x[1].DenierOverridden {
		t.Fatalf("seq X override flags = %v/%v, want true/false", x[0].DenierOverridden, x[1].DenierOverridden)
	}
	assertNear(t, 30, x[0].Denier, "X overridden denier")
	assertNear(t, 30.0/180, x[0].ComputedRatio, "X computed ratio")
	assertNear(t, 30.0/180, x[0].EffectiveRatio, "X effective ratio")

	if y[0].DenierOverridden || y[1].DenierOverridden {
		t.Fatal("product X's override must not affect product Y's seq")
	}
	assertNear(t, 70, y[0].Denier, "Y master denier")
	assertNear(t, 70.0/220, y[0].ComputedRatio, "Y computed ratio")
}
