package costroute

import (
	"math"
	"testing"
)

const (
	tUpA   int64 = 901
	tUpB   int64 = 902
	tSpd         = "SPD0000006"
	tOil         = "RMP0000087"
	tDelta       = 1e-9
)

func tProduct(up int64, ratio float64) *Rm {
	return &Rm{RmType: RmTypeProduct, RmProductSysID: up, RouteRmRatio: ratio}
}

func tGroup(code string, ratio float64) *Rm {
	return &Rm{RmType: RmTypeGroup, RmGroupCode: code, RouteRmRatio: ratio}
}

func tSrc() RatioSourceData {
	return RatioSourceData{
		Mode:           RMRatioModeDenier,
		UpstreamDenier: map[int64]float64{tUpA: 70, tUpB: 150},
		GroupDenier:    map[string]float64{tSpd: 40},
		SpandexDraft:   3,
	}
}

func TestComputeSeqRMRatios(t *testing.T) {
	spd33 := 40 / 3.3
	tests := []struct {
		name         string
		rms          []*Rm
		mutate       func(*RatioSourceData)
		wantRatios   []float64
		wantDeniers  []float64
		wantSources  []string
		wantNotes    int
		wantLineNote []bool
	}{
		{
			name:        "MEL 75/150",
			rms:         []*Rm{tProduct(tUpA, 1), tProduct(tUpB, 1)},
			mutate:      func(s *RatioSourceData) { s.UpstreamDenier[tUpA] = 75 },
			wantRatios:  []float64{1.0 / 3, 2.0 / 3},
			wantDeniers: []float64{75, 150},
			wantSources: []string{RatioSourceDenier, RatioSourceDenier},
		},
		{
			name:        "ACY PTY150 + spandex 40/3",
			rms:         []*Rm{tProduct(tUpB, 1), tGroup(tSpd, 1)},
			wantRatios:  []float64{150 / (150 + 40.0/3), (40.0 / 3) / (150 + 40.0/3)},
			wantDeniers: []float64{150, 40.0 / 3},
			wantSources: []string{RatioSourceDenier, RatioSourceDenier},
		},
		{
			name:        "draft 3.3",
			rms:         []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1)},
			mutate:      func(s *RatioSourceData) { s.SpandexDraft = 3.3 },
			wantRatios:  []float64{70 / (70 + spd33), spd33 / (70 + spd33)},
			wantDeniers: []float64{70, spd33},
			wantSources: []string{RatioSourceDenier, RatioSourceDenier},
		},
		{
			name:        "duplicate spandex 0.5/0.5",
			rms:         []*Rm{tGroup(tSpd, 1), tGroup(tSpd, 1)},
			wantRatios:  []float64{0.5, 0.5},
			wantDeniers: []float64{40.0 / 3, 40.0 / 3},
			wantSources: []string{RatioSourceDenier, RatioSourceDenier},
		},
		{
			name:        "PTY only -> 1",
			rms:         []*Rm{tProduct(tUpA, 0.4)},
			wantRatios:  []float64{1},
			wantDeniers: []float64{70},
			wantSources: []string{RatioSourceDenier},
		},
		{
			name:         "missing upstream denier -> 0 + note",
			rms:          []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1)},
			mutate:       func(s *RatioSourceData) { delete(s.UpstreamDenier, tUpA) },
			wantRatios:   []float64{0, 1},
			wantDeniers:  []float64{0, 40.0 / 3},
			wantSources:  []string{RatioSourceDenier, RatioSourceDenier},
			wantNotes:    1,
			wantLineNote: []bool{true, false},
		},
		{
			name:         "missing draft -> 0 + note",
			rms:          []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1)},
			mutate:       func(s *RatioSourceData) { s.SpandexDraft = 0 },
			wantRatios:   []float64{1, 0},
			wantDeniers:  []float64{70, 0},
			wantSources:  []string{RatioSourceDenier, RatioSourceDenier},
			wantNotes:    1,
			wantLineNote: []bool{false, true},
		},
		{
			name: "sum denier 0 -> all 0 + notes",
			rms:  []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1)},
			mutate: func(s *RatioSourceData) {
				s.UpstreamDenier = nil
				s.SpandexDraft = 0
			},
			wantRatios:   []float64{0, 0},
			wantDeniers:  []float64{0, 0},
			wantSources:  []string{RatioSourceDenier, RatioSourceDenier},
			wantNotes:    3,
			wantLineNote: []bool{true, true},
		},
		{
			name:        "non-denier group and item keep input",
			rms:         []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1), tGroup(tOil, 0.7), {RmType: RmTypeItem, RmItemCode: "I1", RouteRmRatio: 0.2}},
			wantRatios:  []float64{0.84, 0.16, 0.7, 0.2},
			wantDeniers: []float64{70, 40.0 / 3, 0, 0},
			wantSources: []string{RatioSourceDenier, RatioSourceDenier, RatioSourceInput, RatioSourceInput},
		},
		{
			name:         "group head denier 0 -> note",
			rms:          []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1)},
			mutate:       func(s *RatioSourceData) { s.GroupDenier[tSpd] = 0 },
			wantRatios:   []float64{1, 0},
			wantDeniers:  []float64{70, 0},
			wantSources:  []string{RatioSourceDenier, RatioSourceDenier},
			wantNotes:    1,
			wantLineNote: []bool{false, true},
		},
		{
			name:        "INPUT passthrough",
			rms:         []*Rm{tProduct(tUpA, 0.6), tGroup(tSpd, 0.4)},
			mutate:      func(s *RatioSourceData) { s.Mode = RMRatioModeInput },
			wantRatios:  []float64{0.6, 0.4},
			wantDeniers: []float64{0, 0},
			wantSources: []string{"", ""},
		},
		{
			name:        "empty mode passthrough",
			rms:         []*Rm{tProduct(tUpA, 0.6), nil},
			mutate:      func(s *RatioSourceData) { s.Mode = "" },
			wantRatios:  []float64{0.6, 0},
			wantDeniers: []float64{0, 0},
			wantSources: []string{"", ""},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			src := tSrc()
			if tc.mutate != nil {
				tc.mutate(&src)
			}
			got, notes := ComputeSeqRMRatios(&Seq{RouteLevel: 1, Rms: tc.rms}, src)
			if len(got) != len(tc.rms) {
				t.Fatalf("len = %d, want %d", len(got), len(tc.rms))
			}
			for i := range got {
				if math.Abs(got[i].Ratio-tc.wantRatios[i]) > tDelta {
					t.Errorf("line %d ratio = %v, want %v", i, got[i].Ratio, tc.wantRatios[i])
				}
				if math.Abs(got[i].Denier-tc.wantDeniers[i]) > tDelta {
					t.Errorf("line %d denier = %v, want %v", i, got[i].Denier, tc.wantDeniers[i])
				}
				if got[i].Source != tc.wantSources[i] {
					t.Errorf("line %d source = %q, want %q", i, got[i].Source, tc.wantSources[i])
				}
				wantNote := tc.wantLineNote != nil && tc.wantLineNote[i]
				if (got[i].Note != "") != wantNote {
					t.Errorf("line %d note = %q, want note=%v", i, got[i].Note, wantNote)
				}
			}
			if len(notes) != tc.wantNotes {
				t.Errorf("notes = %v, want %d", notes, tc.wantNotes)
			}
		})
	}
}

func TestComputeSeqRMRatios_NoteTextsAndDedupe(t *testing.T) {
	src := tSrc()
	src.SpandexDraft = 0
	delete(src.UpstreamDenier, tUpA)
	seq := &Seq{RouteLevel: 2, Rms: []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1), tGroup(tSpd, 1)}}
	_, notes := ComputeSeqRMRatios(seq, src)
	want := []string{
		"DENIER ratio: upstream product 901 has no DENIER; treated as 0",
		"DENIER ratio: SPANDEX_DRAFT missing or not positive; RM group SPD0000006 treated as 0",
		"DENIER ratio: total denier is 0 at route level 2; denier-based RM lines treated as ratio 0",
	}
	if len(notes) != len(want) {
		t.Fatalf("notes = %v", notes)
	}
	for i := range want {
		if notes[i] != want[i] {
			t.Errorf("note %d = %q, want %q", i, notes[i], want[i])
		}
	}
}

func TestComputeSeqRMRatios_NilSeq(t *testing.T) {
	got, notes := ComputeSeqRMRatios(nil, tSrc())
	if got != nil || notes != nil {
		t.Fatalf("got %v %v", got, notes)
	}
}

func TestIsDenierMode(t *testing.T) {
	for in, want := range map[string]bool{"DENIER": true, " denier ": true, "INPUT": false, "": false} {
		if IsDenierMode(in) != want {
			t.Errorf("IsDenierMode(%q) != %v", in, want)
		}
	}
}

func TestGraph_DenierInputKeys(t *testing.T) {
	g := &Graph{Seqs: []*Seq{
		{ProductSysID: 1, Rms: []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1), tGroup(tSpd, 1), nil, {RmType: RmTypeItem, RmItemCode: "I"}}},
		{ProductSysID: 1, Rms: []*Rm{tProduct(tUpA, 1)}},
		{ProductSysID: 2, Rms: []*Rm{tProduct(tUpB, 1), tGroup(tOil, 1)}},
		nil,
	}}
	up, grp, prods := g.DenierInputKeys(map[int64]string{1: RMRatioModeDenier, 2: RMRatioModeInput})
	if len(up) != 1 || up[0] != tUpA || len(grp) != 1 || grp[0] != tSpd || len(prods) != 1 || prods[0] != 1 {
		t.Fatalf("got %v %v %v", up, grp, prods)
	}
	up, grp, prods = g.DenierInputKeys(nil)
	if up != nil || grp != nil || prods != nil {
		t.Fatalf("INPUT-only graph should need nothing, got %v %v %v", up, grp, prods)
	}
	var nilG *Graph
	if a, b, c := nilG.DenierInputKeys(nil); a != nil || b != nil || c != nil {
		t.Fatal("nil graph")
	}
}

func TestGraph_AnnotateRMRatios(t *testing.T) {
	g := &Graph{
		Head: &Head{ProductSysID: 1},
		Seqs: []*Seq{
			{ProductSysID: 1, RouteLevel: 1, Rms: []*Rm{tProduct(tUpA, 1), tGroup(tSpd, 1), nil}},
			{ProductSysID: 2, RouteLevel: 2, Rms: []*Rm{tProduct(tUpB, 0.9)}},
			nil,
		},
	}
	g.AnnotateRMRatios(RatioSources{
		Modes:          map[int64]string{1: "denier"},
		UpstreamDenier: map[int64]float64{tUpA: 70},
		GroupDenier:    map[string]float64{tSpd: 40},
		SpandexDraft:   map[int64]float64{1: 3},
	})
	if g.Head.RMRatioMode != RMRatioModeDenier || g.Seqs[0].RMRatioMode != RMRatioModeDenier || g.Seqs[1].RMRatioMode != RMRatioModeInput {
		t.Fatalf("modes %q %q %q", g.Head.RMRatioMode, g.Seqs[0].RMRatioMode, g.Seqs[1].RMRatioMode)
	}
	pty := g.Seqs[0].Rms[0]
	if math.Abs(pty.ComputedRatio-0.84) > tDelta || math.Abs(pty.EffectiveRatio-0.84) > tDelta || pty.RatioSource != RatioSourceDenier || pty.Denier != 70 || pty.RouteRmRatio != 1 {
		t.Fatalf("pty %+v", pty)
	}
	in := g.Seqs[1].Rms[0]
	if in.ComputedRatio != 0.9 || in.RatioSource != "" || in.EffectiveRatio != 0 {
		t.Fatalf("input line %+v", in)
	}
	var nilG *Graph
	nilG.AnnotateRMRatios(RatioSources{})
}
