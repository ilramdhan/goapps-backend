package mbactivesync

import (
	"fmt"
	"math/rand"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// ---- fixture helpers ----

func fp(v float64) *float64 { return &v }
func ip(v int) *int         { return &v }
func i64(v int64) *int64    { return &v }

var t0 = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func uid(n int) uuid.UUID {
	return uuid.MustParse(fmt.Sprintf("00000000-0000-0000-0000-%012d", n))
}

func head(n int, sys, shade string, active bool) HeadSnap {
	return HeadSnap{ID: uid(1000 + n), SysID: sys, MBCosting: "MGT " + sys, IsActive: active, ShadeCode: shade, CreatedAt: t0}
}

type spinOpt func(*SpinSnap)

func spin(n int, h HeadSnap, den float64, fil int, cs, shade string, ldr float64, opts ...spinOpt) SpinSnap {
	s := SpinSnap{
		ID: uid(n), HeadID: h.ID, IsActive: true, Denier: fp(den), Filament: ip(fil), CrossSection: cs,
		ShadeCode: shade, CC: shade, RunLDRPct: fp(ldr), LDRType: "ACTUAL", LDRIsActual: true, LDRIsFixed: true,
		CreatedAt: t0.Add(time.Duration(n) * time.Minute),
	}
	for _, o := range opts {
		o(&s)
	}
	return s
}

func inactive(s *SpinSnap) { s.IsActive = false }
func child(parent uuid.UUID) spinOpt {
	return func(s *SpinSnap) { s.ParentID = &parent }
}
func withFG(fg int64) spinOpt { return func(s *SpinSnap) { s.FGProductID = &fg } }
func ldrCalc(s *SpinSnap)     { s.LDRType, s.LDRIsActual, s.LDRIsFixed = "CALC", false, false }

func poy(id int64, code, shade string) POYSnap {
	return POYSnap{SysID: id, ERPItemCode: code, ShadeCode: shade, IsActive: true}
}

func frow(ln int, sys, poyCode, poyName, shade string, doz float64) FileRow {
	r := FileRow{Ln: ln, SysID: sys, POY: poyCode, POYName: poyName, Shade: shade, ShadeName: "N " + shade, Doz: doz, DozOK: true}
	parseSpec(&r)
	return r
}

const spec380 = "POY 380/144/TBL/DBR"

func classify(t *testing.T, rows []FileRow, snap *Snapshot) map[int]*RowPlan {
	t.Helper()
	plans, err := ClassifyRows(rows, snap)
	if err != nil {
		t.Fatal(err)
	}
	GuardFGConflicts(plans, snap, nil)
	m := map[int]*RowPlan{}
	for _, p := range plans {
		m[p.Row.Ln] = p
	}
	return m
}

// withNames sets spin & head shade names equal to the file shade names so tests
// only see shade-code effects.
func named(s *SpinSnap) { s.ShadeName = "N " + s.ShadeCode }

// ---- rules ----

func TestClassifyRows_Rules(t *testing.T) {
	hLive := head(1, "S1", "Y145T", true)
	hLive.ShadeName = "N Y145T"
	hDead := head(2, "S2", "Y145T", false)
	hDead.ShadeName = "N Y145T"
	hEmpty := head(3, "S3", "Y145T", true)
	hEmpty.ShadeName = "N Y145T"
	hFmt := head(4, "S4", "Y-145-T", true)
	hDiff := head(5, "S5", "Z999T", true)

	root1 := spin(1, hLive, 380, 144, "TBL", "Y145T", 1.39, named)
	root2 := spin(2, hDead, 380, 144, "TBL", "Y145T", 1.39, named, inactive)
	root4 := spin(4, hFmt, 380, 144, "TBL", "Y-145-T", 1.39, named)
	root5 := spin(5, hDiff, 380, 144, "TBL", "Y145T", 1.39, named)
	snap := &Snapshot{
		Heads: []HeadSnap{hLive, hDead, hEmpty, hFmt, hDiff},
		Spins: []SpinSnap{root1, root2, root4, root5},
		POYs: []POYSnap{poy(10, "POY1", "Y145T"), poy(11, "POY2", "X438T"), poy(12, "POY3", "Y145T"),
			poy(13, "POY4", "Y145T"), poy(14, "POY5", "Y145T"), poy(15, "POY6", "Y145T"), poy(16, "POY7", "Y145T"),
			poy(17, "POY8", "X438T")},
	}
	rows := []FileRow{
		frow(1, "NOHEAD", "POY1", spec380, "Y145T", 1.39),  // rule 1
		frow(2, "S1", "POYX", spec380, "Y145T", 1.39),      // rule 2 poy
		frow(3, "S1", "POY1", "WEIRD", "Y145T", 1.39),      // rule 2 spec
		frow(4, "S1", "POY1", spec380, "Y145T", 1.39),      // rule 7 KEEP
		frow(5, "S1", "POY1", spec380, "Y-145-T", 1.50),    // rule 3 dup_row of ln 4
		frow(6, "S1", "POY3", "POY 150/48/RD", "Y145T", 2), // rule 5 spec
		frow(7, "S3", "POY4", spec380, "Y145T", 1.39),      // rule 4 ROOT
		frow(8, "S3", "POY2", "POY 150/48/RD", "X438T", 2), // rule 4 DUP
		frow(9, "S2", "POY5", spec380, "Y145T", 1.39),      // rule 10 REACTIVATE
		frow(10, "S4", "POY6", spec380, "Y145T", 1.39),     // KEEP→UPDATE_SHADE(format)+SET_HEAD_SHADE
		frow(11, "S5", "POY7", spec380, "Y145T", 1.39),     // 7b REVIEW head_shade_differs
		frow(12, "S1", "POY8", spec380, "X438T", 1.39),     // rule 6 spin_taken
	}
	got := classify(t, rows, snap)

	type want struct {
		action   Action
		role     Role
		phase    Phase
		decision Decision
		reasons  []string
		subs     []string
		source   string
	}
	cases := map[int]want{
		1:  {ActionNewRecipe, RoleNone, PhaseD, DecisionSkip, nil, nil, ""},
		2:  {ActionReportOnly, RoleNone, PhaseNone, DecisionSkip, []string{ReasonPOYNotFound}, nil, ""},
		3:  {ActionReportOnly, RoleNone, PhaseNone, DecisionSkip, []string{ReasonSpecUnparsed}, nil, ""},
		4:  {ActionKeep, RoleNone, PhaseA, DecisionApply, nil, []string{SubSetFG}, ""},
		5:  {ActionReportOnly, RoleNone, PhaseNone, DecisionSkip, []string{ReasonDupRow}, nil, ""},
		6:  {ActionDuplicateSpin, RoleNone, PhaseC, DecisionApply, []string{ReasonSpec}, []string{SubSetFG, SubSetLDR}, root1.ID.String()},
		7:  {ActionGenerateSpin, RoleRoot, PhaseC, DecisionApply, nil, []string{SubSetFG, SubSetLDR}, ""},
		8:  {ActionGenerateSpin, RoleDup, PhaseC, DecisionApply, nil, []string{SubSetFG, SubSetLDR, SubSetShade}, "ln:7"},
		9:  {ActionReactivate, RoleNone, PhaseD, DecisionApply, nil, []string{SubSetFG, SubActSpin, SubActHead}, ""},
		10: {ActionUpdateShade, RoleNone, PhaseA, DecisionApply, []string{ReasonShadeFormat}, []string{SubSetFG, SubSetShade, SubSetHeadShade}, ""},
		11: {ActionKeep, RoleNone, PhaseA, DecisionReview, []string{ReasonHeadShadeDiffers}, []string{SubSetFG}, ""},
		12: {ActionDuplicateSpin, RoleNone, PhaseC, DecisionApply, []string{ReasonSpinTaken}, []string{SubSetFG, SubSetLDR, SubSetShade}, root1.ID.String()},
	}
	for ln, w := range cases {
		p := got[ln]
		if p.Action != w.action || p.Role != w.role || p.Phase != w.phase || p.Decision != w.decision {
			t.Errorf("ln %d: got %s/%s/%s/%s want %s/%s/%s/%s (reasons %v)", ln,
				p.Action, p.Role, p.Phase, p.Decision, w.action, w.role, w.phase, w.decision, p.Reasons)
		}
		if !sameStrings(p.Reasons, w.reasons) {
			t.Errorf("ln %d: reasons %v want %v", ln, p.Reasons, w.reasons)
		}
		if !sameStrings(p.SubActions, w.subs) {
			t.Errorf("ln %d: subs %v want %v", ln, p.SubActions, w.subs)
		}
		if p.SourceRef() != w.source {
			t.Errorf("ln %d: source %q want %q", ln, p.SourceRef(), w.source)
		}
	}
	if got[12].Alloc == nil || got[12].Alloc.Spin.ID != root1.ID || got[12].Alloc.Rank != 2 {
		t.Errorf("ln 12 must keep its alloc (rank 2 on root1): %+v", got[12].Alloc)
	}
	if got[10].TargetShadeCode != "Y145T" || *got[10].TargetFG != 15 || *got[10].TargetLDR != 1.39 {
		t.Errorf("ln 10 targets: %+v", got[10])
	}
	if got[8].TargetDen == nil || *got[8].TargetDen != 150 || *got[8].TargetFil != 48 || got[8].TargetCS != "RD" {
		t.Errorf("ln 8 spec targets: %+v", got[8])
	} // P2h-4c: the ROOT also targets the ROW spec (the generated root starts with the head spec).
	if got[7].TargetDen == nil || *got[7].TargetDen != 380 || got[7].TargetFil == nil || *got[7].TargetFil != 144 || got[7].TargetCS != "TBL" {
		t.Errorf("ln 7 ROOT spec targets: %+v", got[7])
	}
}

func sameStrings(a, b []string) bool {
	if len(a) == 0 && len(b) == 0 {
		return true
	}
	return reflect.DeepEqual(a, b)
}

func TestClassifyRows_UpdateShadeAndLDR(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	h.ShadeName = "N Y145T"
	sp := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named)
	snap := &Snapshot{Heads: []HeadSnap{h}, Spins: []SpinSnap{sp}, POYs: []POYSnap{poy(10, "POY1", "Y145T"), poy(11, "POY1", "X438T")}}

	t.Run("UPDATE_LDR", func(t *testing.T) {
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.50)}, snap)[1]
		if got.Action != ActionUpdateLDR || !got.HasSub(SubSetLDR) || got.Decision != DecisionApply {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("KEEP within 0.005 still SET_LDR when not exact", func(t *testing.T) {
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.394)}, snap)[1]
		if got.Action != ActionKeep || !got.HasSub(SubSetLDR) {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("UPDATE_SHADE real shade on root → REVIEW head_shade_differs", func(t *testing.T) {
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "X438T", 1.39)}, snap)[1]
		if got.Action != ActionUpdateShade || !got.HasSub(SubSetShade) || got.Decision != DecisionReview || !got.HasReason(ReasonHeadShadeDiffers) {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("Q14 empty head shade on root → SET_HEAD_SHADE + APPLY", func(t *testing.T) {
		he := h
		he.ShadeCode, he.ShadeName = "", ""
		s2 := spin(1, he, 380, 144, "TBL", "Y145T", 1.39, named)
		snap2 := &Snapshot{Heads: []HeadSnap{he}, Spins: []SpinSnap{s2}, POYs: snap.POYs}
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.39)}, snap2)[1]
		if !got.HasSub(SubSetHeadShade) || got.HasReason(ReasonHeadShadeDiffers) || got.Decision != DecisionApply {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("LDR not ACTUAL → SET_LDR even when equal", func(t *testing.T) {
		s2 := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named, ldrCalc)
		snap2 := &Snapshot{Heads: snap.Heads, Spins: []SpinSnap{s2}, POYs: snap.POYs}
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.39)}, snap2)[1]
		if got.Action != ActionKeep || !got.HasSub(SubSetLDR) {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("FG already right → no SET_FG", func(t *testing.T) {
		s2 := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named, withFG(10))
		snap2 := &Snapshot{Heads: snap.Heads, Spins: []SpinSnap{s2}, POYs: snap.POYs}
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.39)}, snap2)[1]
		if got.Action != ActionKeep || len(got.SubActions) != 0 || got.Decision != DecisionApply {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("other FG → REVIEW fg_differs", func(t *testing.T) {
		s2 := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named, withFG(99))
		snap2 := &Snapshot{Heads: snap.Heads, Spins: []SpinSnap{s2}, POYs: snap.POYs}
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.39)}, snap2)[1]
		if !got.HasReason(ReasonFGDiffers) || got.Decision != DecisionReview {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("child spin never gets Q12 head rule", func(t *testing.T) {
		c := spin(2, h, 150, 48, "RD", "X438T", 2, named, child(sp.ID))
		snap2 := &Snapshot{Heads: snap.Heads, Spins: []SpinSnap{sp, c}, POYs: snap.POYs}
		got := classify(t, []FileRow{frow(1, "S1", "POY1", "POY 150/48/RD", "Y145T", 2)}, snap2)[1]
		if got.Spin.ID != c.ID || got.Action != ActionUpdateShade || got.Decision != DecisionApply || got.HasSub(SubSetHeadShade) {
			t.Fatalf("%+v", got)
		}
	})
	t.Run("missing doz → REVIEW", func(t *testing.T) {
		r := frow(1, "S1", "POY1", spec380, "Y145T", 0)
		r.DozOK = false
		got := classify(t, []FileRow{r}, snap)[1]
		if got.Action != ActionUpdateLDR || got.Decision != DecisionReview || !got.HasReason(ReasonDozMissing) {
			t.Fatalf("%+v", got)
		}
	})
}

func TestAllocate_OrderAndRank(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	// lvl beats shade: a lvl2 spin with other shade wins over a lvl3 spin with same shade.
	lvl2 := spin(1, h, 380, 144, "TBL", "X438T", 1)
	lvl3 := spin(2, h, 380, 144, "RD", "Y145T", 1)
	// same lvl & shade: smaller ldr_d wins; then live; then id.
	near := spin(3, h, 380, 144, "TBL", "Y145T", 1.40, inactive)
	far := spin(4, h, 380, 144, "TBL", "Y145T", 2.00)
	spins := map[uuid.UUID][]*SpinSnap{h.ID: {&lvl2, &lvl3, &near, &far}}

	r1 := frow(1, "S1", "P", spec380, "Y145T", 1.39)
	r2 := frow(2, "S1", "P", spec380, "Y145T", 1.40)
	r3 := frow(3, "S1", "P", "POY 380/144/RD", "Q1", 1.0)
	out := Allocate([]AllocInput{{Row: r1, HeadID: h.ID}, {Row: r2, HeadID: h.ID}, {Row: r3, HeadID: h.ID}}, spins)
	if out[1].Spin.ID != near.ID || out[2].Spin.ID != near.ID {
		t.Fatalf("rows 1,2 must pick near (lvl2, sh_eq, min ldr_d): %v %v", out[1].Spin.ID, out[2].Spin.ID)
	}
	if out[2].Rank != 1 || out[1].Rank != 2 {
		t.Errorf("rank by ldr_d: ln2 (d=0) rank %d, ln1 rank %d", out[2].Rank, out[1].Rank)
	}
	// r3 cs=RD: the RD spin matches at lvl 2 and wins over the TBL spins (lvl 3) despite its shade.
	if out[3].Spin.ID != lvl3.ID || out[3].Lvl != AllocLvlDenFilCS {
		t.Errorf("row 3 picked %v lvl %d", out[3].Spin.ID, out[3].Lvl)
	}
	// r1 vs the lvl2 TBL spin with another shade: lvl ties at 2, sh_eq decides → near.
	if out[1].Lvl != AllocLvlDenFilCS {
		t.Errorf("row 1 lvl %d", out[1].Lvl)
	}
	// live breaks a tie of lvl/sh_eq/ldr_d; id breaks a full tie.
	a := spin(10, h, 380, 144, "TBL", "Y145T", 1.39, inactive)
	b := spin(11, h, 380, 144, "TBL", "Y145T", 1.39)
	c := spin(12, h, 380, 144, "TBL", "Y145T", 1.39)
	out = Allocate([]AllocInput{{Row: r1, HeadID: h.ID}}, map[uuid.UUID][]*SpinSnap{h.ID: {&c, &a, &b}})
	if out[1].Spin.ID != b.ID {
		t.Errorf("tie → live then lowest id; got %v", out[1].Spin.ID)
	}
}

func TestAllocate_NullShadeSortsLast(t *testing.T) {
	h := head(1, "S1", "", true)
	noShade := spin(1, h, 380, 144, "TBL", "", 1.39)
	other := spin(2, h, 380, 144, "TBL", "X438T", 5)
	out := Allocate([]AllocInput{{Row: frow(1, "S1", "P", spec380, "Y145T", 1.39), HeadID: h.ID}},
		map[uuid.UUID][]*SpinSnap{h.ID: {&noShade, &other}})
	if out[1].Spin.ID != other.ID {
		t.Errorf("sh_eq false must beat sh_eq NULL; got %v", out[1].Spin.ID)
	}
	if out[1].ShEq == nil || *out[1].ShEq {
		t.Errorf("sh_eq: %v", out[1].ShEq)
	}
}

func TestSelectDuplicateSource(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	rootFar := spin(1, h, 75, 36, "RD", "Y145T", 1)
	rootNear := spin(2, h, 150, 72, "RD", "X438T", 1)
	rootNearSh := spin(3, h, 150, 72, "RD", "Y145T", 1, inactive)
	kid := spin(4, h, 150, 48, "RD", "Y145T", 1, child(rootFar.ID))
	r := frow(1, "S1", "P", "POY 150/48/RD", "Y145T", 1)
	got := SelectDuplicateSource([]*SpinSnap{&rootFar, &rootNear, &rootNearSh, &kid}, r)
	if got.ID != rootNearSh.ID {
		t.Errorf("want root with nearest den/fil then sh_eq (even inactive), got %v", got.ID)
	}
	if SelectDuplicateSource([]*SpinSnap{&kid}, r) != nil {
		t.Error("no root → nil")
	}
	// live before created_at when everything else ties.
	a := spin(5, h, 150, 72, "RD", "Y145T", 1, inactive)
	b := spin(6, h, 150, 72, "RD", "Y145T", 1)
	if got := SelectDuplicateSource([]*SpinSnap{&a, &b}, r); got.ID != b.ID {
		t.Errorf("live first, got %v", got.ID)
	}
}

func TestClassifyRows_NoRoot(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	other := uid(77)
	orphan := spin(1, h, 75, 36, "RD", "Y145T", 1, child(other))
	snap := &Snapshot{Heads: []HeadSnap{h}, Spins: []SpinSnap{orphan}, POYs: []POYSnap{poy(10, "POY1", "Y145T")}}
	got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1)}, snap)[1]
	if got.Action != ActionReportOnly || !got.HasReason(ReasonNoRoot) || got.Decision != DecisionSkip {
		t.Fatalf("%+v", got)
	}
}

func TestClassifyRows_POYAmbiguousAndInactive(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	h.ShadeName = "N Y145T"
	sp := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named)
	p3 := poy(30, "POY1", "Y-145-T")
	p2 := poy(20, "POY1", "Y145T")
	p1 := poy(5, "POY1", "Y145T")
	p1.IsActive = false
	snap := &Snapshot{Heads: []HeadSnap{h}, Spins: []SpinSnap{sp}, POYs: []POYSnap{p1, p2, p3}}
	got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", 1.39)}, snap)[1]
	if got.POY == nil || got.POY.SysID != 20 || !got.HasReason(ReasonPOYAmbiguous) || got.Decision != DecisionApply {
		t.Fatalf("want min active sys_id 20 + poy_ambiguous (non-blocking): %+v", got)
	}
}

// ---- FG conflict guard ----

func TestGuardFGConflicts(t *testing.T) {
	hA := head(1, "SA", "Y145T", true)
	hA.ShadeName = "N Y145T"
	hB := head(2, "SB", "Y145T", true)
	hB.ShadeName = "N Y145T"
	hC := head(3, "SC", "Y145T", true) // not in file
	spA := spin(1, hA, 380, 144, "TBL", "Y145T", 1.39, named)
	spB := spin(2, hB, 380, 144, "TBL", "Y145T", 1.39, named)
	poys := []POYSnap{poy(10, "POY1", "Y145T")}
	rows := []FileRow{
		frow(1, "SA", "POY1", spec380, "Y145T", 1.39),
		frow(2, "SB", "POY1", spec380, "Y145T", 1.39),
	}

	t.Run("two rows across SYSIDs → second is fg_conflict", func(t *testing.T) {
		snap := &Snapshot{Heads: []HeadSnap{hA, hB}, Spins: []SpinSnap{spA, spB}, POYs: poys}
		got := classify(t, rows, snap)
		if got[1].HasReason(ReasonFGConflict) || got[1].Decision != DecisionApply || !got[1].HasSub(SubSetFG) {
			t.Errorf("ln1 owner: %+v", got[1])
		}
		if !got[2].HasReason(ReasonFGConflict) || got[2].HasSub(SubSetFG) || got[2].Decision != DecisionReview {
			t.Errorf("ln2 conflict: %+v", got[2])
		}
	})
	t.Run("held by live spin outside the plan → conflict unless exempt", func(t *testing.T) {
		holder := spin(3, hC, 380, 144, "TBL", "Y145T", 1, withFG(10))
		snap := &Snapshot{Heads: []HeadSnap{hA, hC}, Spins: []SpinSnap{spA, holder}, POYs: poys}
		got := classify(t, rows[:1], snap)
		if !got[1].HasReason(ReasonFGConflict) || got[1].Decision != DecisionReview {
			t.Errorf("want conflict: %+v", got[1])
		}
		plans, err := ClassifyRows(rows[:1], snap)
		if err != nil {
			t.Fatal(err)
		}
		GuardFGConflicts(plans, snap, func(_ *RowPlan, h *SpinSnap) bool { return h.ID == holder.ID })
		if plans[0].HasReason(ReasonFGConflict) || plans[0].Decision != DecisionApply {
			t.Errorf("supersede-exempt holder must not conflict: %+v", plans[0])
		}
	})
	t.Run("holder is the row's own spin → no conflict", func(t *testing.T) {
		own := spin(1, hA, 380, 144, "TBL", "Y145T", 1.39, named, withFG(10))
		snap := &Snapshot{Heads: []HeadSnap{hA}, Spins: []SpinSnap{own}, POYs: poys}
		got := classify(t, rows[:1], snap)
		if got[1].HasReason(ReasonFGConflict) || got[1].Decision != DecisionApply {
			t.Errorf("%+v", got[1])
		}
	})
	t.Run("holder is another row's target spin → that row owns the pair", func(t *testing.T) {
		ownB := spin(2, hB, 380, 144, "TBL", "Y145T", 1.39, named, withFG(10))
		snap := &Snapshot{Heads: []HeadSnap{hA, hB}, Spins: []SpinSnap{spA, ownB}, POYs: poys}
		got := classify(t, rows, snap)
		if !got[1].HasReason(ReasonFGConflict) {
			t.Errorf("ln1 must yield to the holder's row: %+v", got[1])
		}
		if got[2].HasReason(ReasonFGConflict) {
			t.Errorf("ln2 holds the pair: %+v", got[2])
		}
	})
	t.Run("inactive holder is ignored; REPORT rows are ignored", func(t *testing.T) {
		dead := spin(3, hC, 380, 144, "TBL", "Y145T", 1, withFG(10), inactive)
		snap := &Snapshot{Heads: []HeadSnap{hA, hC}, Spins: []SpinSnap{spA, dead}, POYs: poys}
		got := classify(t, append([]FileRow{frow(0, "SA", "POY1", "WEIRD", "Y145T", 1)}, rows[0]), snap)
		if got[1].HasReason(ReasonFGConflict) {
			t.Errorf("%+v", got[1])
		}
	})
}

// ---- determinism ----

func buildBigFixture() ([]FileRow, *Snapshot) {
	snap := &Snapshot{}
	var rows []FileRow
	shades := []string{"Y145T", "Y-145-T", "X438T", "", "Z1"}
	specs := []string{spec380, "POY 380/144/RD", "POY 150/48/RD", "POY 75/36/TBL", "WEIRD"}
	for i := 1; i <= 40; i++ {
		poyShade := shades[i%3]
		snap.POYs = append(snap.POYs, poy(int64(i), fmt.Sprintf("POY%d", i%7), poyShade))
	}
	ln := 1
	for h := 1; h <= 25; h++ {
		sys := fmt.Sprintf("S%02d", h)
		hd := head(h, sys, shades[h%len(shades)], h%3 != 0)
		snap.Heads = append(snap.Heads, hd)
		var root *SpinSnap
		for s := 0; s < h%4; s++ {
			opts := []spinOpt{}
			if s%2 == 1 {
				opts = append(opts, inactive)
			}
			if root != nil && s == 3 {
				opts = append(opts, child(root.ID))
			}
			if (h+s)%5 == 0 {
				opts = append(opts, withFG(int64((h+s)%40+1)))
			}
			den := []float64{380, 150, 75}[s%3]
			fil := []int{144, 48, 36}[s%3]
			sp := spin(h*10+s, hd, den, fil, []string{"TBL", "RD"}[s%2], shades[(h+s)%len(shades)], 1+float64(s)/10, opts...)
			sp.CreatedAt = t0 // force ties so id ordering matters
			snap.Spins = append(snap.Spins, sp)
			if s == 0 {
				root = &snap.Spins[len(snap.Spins)-1]
			}
		}
		for r := 0; r < 1+h%5; r++ {
			rows = append(rows, frow(ln, sys, fmt.Sprintf("POY%d", (h+r)%7), specs[(h+r)%len(specs)], shades[(h*r)%len(shades)], 1+float64(r)/10))
			ln++
		}
	}
	rows = append(rows, frow(ln, "UNKNOWN", "POY1", spec380, "Y145T", 1))
	return rows, snap
}

func renderPlans(plans []*RowPlan) string {
	var b strings.Builder
	for _, p := range plans {
		spinID, rank := "", 0
		if p.Spin != nil {
			spinID = p.Spin.ID.String()
		}
		if p.Alloc != nil {
			rank = p.Alloc.Rank
		}
		fmt.Fprintf(&b, "%d|%s|%s|%s|%s|%s|%s|%s|%s|%d|%v\n", p.Row.Ln, p.Action, p.Role, p.Phase, p.Decision,
			strings.Join(p.Reasons, ";"), strings.Join(p.SubActions, ";"), spinID, p.SourceRef(), rank, p.TargetFG != nil)
	}
	return b.String()
}

func TestClassifyRows_Deterministic(t *testing.T) {
	rows, snap := buildBigFixture()
	run := func(rs []FileRow, s *Snapshot) string {
		plans, err := ClassifyRows(rs, s)
		if err != nil {
			t.Fatal(err)
		}
		GuardFGConflicts(plans, s, nil)
		return renderPlans(plans)
	}
	want := run(rows, snap)
	if !strings.Contains(want, string(ActionDuplicateSpin)) || !strings.Contains(want, string(ActionKeep)) ||
		!strings.Contains(want, string(ActionGenerateSpin)) || !strings.Contains(want, string(ActionReactivate)) {
		t.Fatalf("fixture does not exercise enough actions:\n%s", want)
	}
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // test shuffle
	for i := 0; i < 20; i++ {
		rs := append([]FileRow(nil), rows...)
		rng.Shuffle(len(rs), func(a, b int) { rs[a], rs[b] = rs[b], rs[a] })
		s := *snap
		s.Spins = append([]SpinSnap(nil), snap.Spins...)
		s.Heads = append([]HeadSnap(nil), snap.Heads...)
		s.POYs = append([]POYSnap(nil), snap.POYs...)
		rng.Shuffle(len(s.Spins), func(a, b int) { s.Spins[a], s.Spins[b] = s.Spins[b], s.Spins[a] })
		rng.Shuffle(len(s.Heads), func(a, b int) { s.Heads[a], s.Heads[b] = s.Heads[b], s.Heads[a] })
		rng.Shuffle(len(s.POYs), func(a, b int) { s.POYs[a], s.POYs[b] = s.POYs[b], s.POYs[a] })
		if got := run(rs, &s); got != want {
			t.Fatalf("shuffle %d changed the plan:\nwant\n%s\ngot\n%s", i, want, got)
		}
	}
}

func TestClassifyRows_DuplicateLn(t *testing.T) {
	if _, err := ClassifyRows([]FileRow{{Ln: 1, SysID: "A"}, {Ln: 1, SysID: "B"}}, &Snapshot{}); err == nil {
		t.Fatal("want error on duplicate ln")
	}
}

// L-14 computes ldr_d in exact NUMERIC: a difference of exactly 0.005 is KEEP,
// even where float64 subtraction lands just above 0.005 (1.39 vs 1.395).
func TestClassifyRows_KeepBoundaryExact(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	h.ShadeName = "N Y145T"
	for _, tc := range []struct {
		ldr, doz float64
		want     Action
	}{
		{1.39, 1.395, ActionKeep},
		{1.00, 1.005, ActionKeep},
		{1.395, 1.39, ActionKeep},
		{1.39, 1.3951, ActionUpdateLDR},
		{1.39, 1.40, ActionUpdateLDR},
	} {
		sp := spin(1, h, 380, 144, "TBL", "Y145T", tc.ldr, named)
		snap := &Snapshot{Heads: []HeadSnap{h}, Spins: []SpinSnap{sp}, POYs: []POYSnap{poy(10, "POY1", "Y145T")}}
		got := classify(t, []FileRow{frow(1, "S1", "POY1", spec380, "Y145T", tc.doz)}, snap)[1]
		if got.Action != tc.want {
			t.Errorf("ldr %v doz %v: got %s want %s", tc.ldr, tc.doz, got.Action, tc.want)
		}
	}
	// sweep: every exact 0.005 difference must be KEEP
	for i := 0; i <= 2000; i++ {
		ldr := float64(i) / 1000
		doz := ldr + 0.005
		d := ldrDiff(&SpinSnap{RunLDRPct: &ldr}, FileRow{Doz: doz, DozOK: true})
		if *d != keepLDRDiff4 {
			t.Fatalf("ldr %v doz %v: ldr_d %d want %d", ldr, doz, *d, keepLDRDiff4)
		}
	}
}

// Equal ldr_d (|1.00-0.93| == |1.07-1.00| in NUMERIC) must fall back to live, then mbs_id
// in "best", and to ln in the per-spin rank.
func TestAllocate_LDRDTieExact(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	lo := spin(1, h, 380, 144, "TBL", "Y145T", 0.93) // |0.93-1.00| = 0.07
	hi := spin(2, h, 380, 144, "TBL", "Y145T", 1.07) // |1.07-1.00| = 0.07
	r := frow(1, "S1", "P", spec380, "Y145T", 1.00)
	out := Allocate([]AllocInput{{Row: r, HeadID: h.ID}}, map[uuid.UUID][]*SpinSnap{h.ID: {&hi, &lo}})
	if out[1].Spin.ID != lo.ID {
		t.Errorf("tie on ldr_d → lowest mbs_id; got %v", out[1].Spin.ID)
	}
	loIn := spin(1, h, 380, 144, "TBL", "Y145T", 0.93, inactive)
	out = Allocate([]AllocInput{{Row: r, HeadID: h.ID}}, map[uuid.UUID][]*SpinSnap{h.ID: {&loIn, &hi}})
	if out[1].Spin.ID != hi.ID {
		t.Errorf("tie on ldr_d → live first; got %v", out[1].Spin.ID)
	}
	// rank: two rows on the same spin with equal ldr_d (1.07-1.00 vs 1.07-1.14) → ln order.
	r2 := frow(2, "S1", "P", spec380, "Y145T", 1.00)
	r1 := frow(1, "S1", "P", spec380, "Y145T", 1.14)
	out = Allocate([]AllocInput{{Row: r2, HeadID: h.ID}, {Row: r1, HeadID: h.ID}}, map[uuid.UUID][]*SpinSnap{h.ID: {&hi}})
	if out[1].Rank != 1 || out[2].Rank != 2 {
		t.Errorf("rank tie → ln ASC: ln1 %d ln2 %d", out[1].Rank, out[2].Rank)
	}
}

func TestSelectDuplicateSource_DenTieExact(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	// |150.07-150.00| == |149.93-150.00| exactly; created_at then decides (lower n = earlier).
	a := spin(1, h, 149.93, 48, "RD", "Y145T", 1)
	b := spin(2, h, 150.07, 48, "RD", "Y145T", 1)
	r := frow(1, "S1", "P", "POY 150/48/RD", "Y145T", 1)
	if got := SelectDuplicateSource([]*SpinSnap{&b, &a}, r); got.ID != a.ID {
		t.Errorf("den tie → created_at; got %v", got.ID)
	}
}

func TestClassifyRows_EmptyHeadShadeMultiRoot(t *testing.T) {
	he := head(1, "S1", "", true)
	he.ShadeName = ""
	pys := []POYSnap{poy(1, "POY1", "Y145T"), poy(2, "POY2", "X438T"), poy(3, "POY3", "Y-145-T")}
	run := func(sh2, poy2 string) map[int]*RowPlan {
		s1 := spin(1, he, 380, 144, "TBL", "Y145T", 1.39, named)
		s2 := spin(2, he, 150, 48, "RD", sh2, 1.39, named)
		snap := &Snapshot{Heads: []HeadSnap{he}, Spins: []SpinSnap{s1, s2}, POYs: pys}
		return classify(t, []FileRow{
			frow(1, "S1", "POY1", spec380, "Y145T", 1.39),
			frow(2, "S1", poy2, "POY 150/48/RD", sh2, 1.39),
		}, snap)
	}
	t.Run("different shades → both REVIEW", func(t *testing.T) {
		for ln, p := range run("X438T", "POY2") {
			if p.HasSub(SubSetHeadShade) || p.Decision != DecisionReview || !p.HasReason(ReasonHeadShadeDiffers) {
				t.Fatalf("ln %d: %+v", ln, p)
			}
		}
	})
	t.Run("same loose shade → APPLY with SET_HEAD_SHADE", func(t *testing.T) {
		for ln, p := range run("Y-145-T", "POY3") {
			if p.HasReason(ReasonHeadShadeDiffers) || !p.HasSub(SubSetHeadShade) || p.Decision != DecisionApply {
				t.Fatalf("ln %d: %+v", ln, p)
			}
		}
	})
}

// P2h-4a bug 1: after phase C a head holds the source plus clones identical on
// den/fil/cs/shade/LDR that differ only by FG. Each row must own the clone that
// already carries its target FG (fg_rank first), leaving nothing unallocated.
func TestAllocate_FGRankFirst(t *testing.T) {
	h := head(1, "S1", "Y145T", true)
	src := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named)
	// ids chosen so the uuid tiebreak would favour cloneB for both rows without fg_rank
	cloneA := spin(3, h, 380, 144, "TBL", "Y145T", 1.39, named, child(src.ID), withFG(10))
	cloneB := spin(2, h, 380, 144, "TBL", "Y145T", 1.39, named, child(src.ID), withFG(11))
	snap := &Snapshot{
		Heads: []HeadSnap{h},
		Spins: []SpinSnap{src, cloneA, cloneB},
		POYs:  []POYSnap{poy(10, "PA", "Y145T"), poy(11, "PB", "Y145T")},
	}
	rows := []FileRow{
		frow(1, "S1", "PA", spec380, "Y145T", 1.39),
		frow(2, "S1", "PB", spec380, "Y145T", 1.39),
	}
	spins := snap.SpinsByHead()
	out := Allocate([]AllocInput{{Row: rows[0], HeadID: h.ID, TargetFG: i64(10)}, {Row: rows[1], HeadID: h.ID, TargetFG: i64(11)}}, spins)
	if out[1].Spin.ID != cloneA.ID || out[1].Rank != 1 || out[1].FGRank != 0 {
		t.Errorf("row A: %+v", out[1])
	}
	if out[2].Spin.ID != cloneB.ID || out[2].Rank != 1 || out[2].FGRank != 0 {
		t.Errorf("row B: %+v", out[2])
	}
	// NULL FG beats another FG, even when the other-FG spin is better on lvl/LDR.
	other := spin(20, h, 380, 144, "TBL", "Y145T", 1.39, named, withFG(99))
	nullFG := spin(21, h, 380, 144, "RD", "Y145T", 2.5, named)
	out = Allocate([]AllocInput{{Row: rows[0], HeadID: h.ID, TargetFG: i64(10)}}, map[uuid.UUID][]*SpinSnap{h.ID: {&other, &nullFG}})
	if out[1].Spin.ID != nullFG.ID || out[1].FGRank != 1 {
		t.Errorf("NULL FG must beat other FG: %+v", out[1])
	}

	// Full plan: both rows KEEP on their own clone, no unallocated clone deactivated.
	plans, hp := planAll(t, rows, snap)
	for ln, want := range map[int]uuid.UUID{1: cloneA.ID, 2: cloneB.ID} {
		p := plans[ln]
		if p.Spin == nil || p.Spin.ID != want || p.Action != ActionKeep || p.Decision != DecisionApply {
			t.Errorf("ln %d: action %s spin %v reasons %v", ln, p.Action, p.Spin, p.Reasons)
		}
	}
	for _, a := range hp.Actions {
		if a.OldSpin != nil && (a.OldSpin.ID == cloneA.ID || a.OldSpin.ID == cloneB.ID) {
			t.Errorf("clone must not get a head action: %s %v", a.Kind, a.Reasons)
		}
	}
}
