package mbactivesync

import (
	"math/rand"
	"reflect"
	"testing"
)

func namedHead(n int, sys, name, shade string, active bool) HeadSnap {
	h := head(n, sys, shade, active)
	h.MBCosting = name
	h.ShadeName = "N " + shade
	return h
}

func withRefs(ptr, text int) spinOpt {
	return func(s *SpinSnap) { s.PointerProducts, s.TextProducts = ptr, text }
}

func dyeRow(ln int, sys, poyCode, poyName, shade string, doz float64, dye string) FileRow {
	r := frow(ln, sys, poyCode, poyName, shade, doz)
	r.DyeName = dye
	return r
}

func planAll(t *testing.T, rows []FileRow, snap *Snapshot) (map[int]*RowPlan, *HeadPlan) {
	t.Helper()
	plans, err := ClassifyRows(rows, snap)
	if err != nil {
		t.Fatal(err)
	}
	hp := PlanHeads(plans, snap)
	GuardFGConflicts(plans, snap, hp.FGExempt())
	hp.SyncDecisions()
	m := map[int]*RowPlan{}
	for _, p := range plans {
		m[p.Row.Ln] = p
	}
	return m, hp
}

func actionsByKey(hp *HeadPlan) map[string]*HeadAction {
	m := map[string]*HeadAction{}
	for _, a := range hp.Actions {
		m[a.ItemKey()] = a
	}
	return m
}

type headWant struct {
	kind     HeadKind
	phase    Phase
	decision Decision
	reasons  []string
	newRef   string
}

func checkHeadActions(t *testing.T, hp *HeadPlan, want map[string]headWant) {
	t.Helper()
	got := actionsByKey(hp)
	if len(got) != len(want) {
		keys := make([]string, 0, len(got))
		for k, a := range got {
			keys = append(keys, k+"="+string(a.Kind))
		}
		t.Fatalf("got %d head actions %v, want %d", len(got), keys, len(want))
	}
	for k, w := range want {
		a := got[k]
		if a == nil {
			t.Errorf("%s: missing", k)
			continue
		}
		if a.Kind != w.kind || a.Phase != w.phase || a.Decision != w.decision {
			t.Errorf("%s: got %s/%s/%s want %s/%s/%s (reasons %v)", k, a.Kind, a.Phase, a.Decision,
				w.kind, w.phase, w.decision, a.Reasons)
		}
		if !sameStrings(a.Reasons, w.reasons) {
			t.Errorf("%s: reasons %v want %v", k, a.Reasons, w.reasons)
		}
		if a.NewSpinRef() != w.newRef {
			t.Errorf("%s: new_spin_ref %q want %q", k, a.NewSpinRef(), w.newRef)
		}
	}
}

// ORCHID-like (old live head not in file, CS successor without spins → SUPERSEDE
// phase C on the generated ROOT), RICH GOLD-like (successor live spin → phase B),
// BERING-like reversed case (successor inactive → REACTIVATE row, phase D) and a
// family pair whose old head has no live spin (head-only SUPERSEDE_HEAD, Q15).
func TestPlanHeads_SupersedePairs(t *testing.T) {
	orchOld := namedHead(1, "O-OLD", "MGT ORCHID MN 3299-B", "3299-01", true)
	orchCS := namedHead(2, "O-CS", "MGT ORCHID MN 3299-CS-D-05094-B", "3299-01", true)
	goldOld := namedHead(3, "G-OLD", "MGT RICH GOLD TR-375-D-00100-C", "Y375T", true)
	goldN := namedHead(4, "G-N", "MGT RICH GOLD TR-375 N-D-00200-B", "Y375T", true)
	berOld := namedHead(5, "B-OLD", "MGT BERING BL 5086-D-00962-C", "5086-01", true)
	berND := namedHead(6, "B-ND", "MGT BERING BL 5086-ND-D-04253-B", "5086-01", false)
	heavOld := namedHead(7, "H-OLD", "MGT HEAVEN BLUE TR-451-N-D-00832-C", "Z451T", true)
	heavCS := namedHead(8, "H-CS", "MGT HEAVEN BLUE TR-451-CS-D-05067-B", "Z451T", true)

	sOrch := spin(1, orchOld, 500, 96, "TBL", "3299-01", 2.5, named, withRefs(17, 0))
	sGoldOld := spin(2, goldOld, 380, 144, "TBL", "Y375T", 1.2, named, withRefs(5, 1))
	sGoldN := spin(3, goldN, 380, 144, "TBL", "Y375T", 1.2, named)
	sBerOld := spin(4, berOld, 250, 48, "TBL", "5086-01", 3.41, named, withRefs(13, 0))
	sBerND := spin(5, berND, 250, 48, "TBL", "5086-01", 3.41, named, inactive)
	sHeavOld := spin(6, heavOld, 250, 48, "TBL", "Z451T", 1, named, inactive)

	snap := &Snapshot{
		Heads: []HeadSnap{orchOld, orchCS, goldOld, goldN, berOld, berND, heavOld, heavCS},
		Spins: []SpinSnap{sOrch, sGoldOld, sGoldN, sBerOld, sBerND, sHeavOld},
		POYs: []POYSnap{poy(10, "P-O1", "3299-01"), poy(11, "P-O2", "3299-01"), poy(12, "P-G", "Y375T"),
			poy(13, "P-B", "5086-01"), poy(14, "P-H", "Z451T")},
	}
	rows := []FileRow{
		frow(1, "O-CS", "P-O1", "POY 500/96/TBL/DBR", "3299-01", 2.5),  // GENERATE ROOT
		frow(2, "O-CS", "P-O2", "POY 380/72/TBL/DBR", "3299-01", 2.48), // GENERATE DUP
		frow(3, "G-N", "P-G", spec380, "Y375T", 1.2),                   // KEEP
		frow(4, "B-ND", "P-B", "POY 250/48/TBL/DBR", "5086-01", 3.41),  // REACTIVATE
		frow(5, "H-CS", "P-H", spec380, "Z451T", 1),                    // GENERATE ROOT
	}
	got, hp := planAll(t, rows, snap)

	if got[1].Action != ActionGenerateSpin || got[3].Action != ActionKeep || got[4].Action != ActionReactivate {
		t.Fatalf("unexpected row actions: %s %s %s", got[1].Action, got[3].Action, got[4].Action)
	}
	sup := func(h HeadSnap, s SpinSnap) string { return "sup:" + h.ID.String() + ":" + s.ID.String() }
	checkHeadActions(t, hp, map[string]headWant{
		// ORCHID: old spin 500/96 → the row of same shade & den/fil (ln 1, created in phase C).
		sup(orchOld, sOrch):    {KindSupersede, PhaseC, DecisionApply, nil, "ln:1"},
		sup(goldOld, sGoldOld): {KindSupersede, PhaseB, DecisionApply, nil, "spin:" + sGoldN.ID.String()},
		sup(berOld, sBerOld):   {KindSupersede, PhaseD, DecisionApply, []string{ReasonNewHeadInactive}, "spin:" + sBerND.ID.String()},
		// HEAVEN: pair found, old head has no live spin, new head live → head-only supersede in B.
		"suph:" + heavOld.ID.String(): {KindSupersedeHead, PhaseB, DecisionApply, nil, ""},
	})
	if len(hp.Pairs) != 4 {
		t.Errorf("pairs = %d, want 4", len(hp.Pairs))
	}
	a := actionsByKey(hp)[sup(orchOld, sOrch)]
	if a.RefPointer != 17 || a.NewHead.ID != orchCS.ID {
		t.Errorf("orchid refs/new head: %+v", a)
	}
}

// The old spin holding the (shade, FG) pair the new row targets is exempt from
// the FG-conflict guard: the supersede deactivates it.
func TestPlanHeads_FGExemptForSupersededSpin(t *testing.T) {
	old := namedHead(1, "OLD", "MGT MILIM BL 5262-D-01730-02-C", "5262-01", true)
	cs := namedHead(2, "CS", "MGT MILIM BL 5262-CS-D-05064-B", "5262-01", true)
	other := namedHead(3, "X", "MGT OTHER 1", "5262-01", true)
	sOld := spin(1, old, 380, 144, "TBL", "5262-01", 1, named, withFG(10), withRefs(14, 0))
	sCS := spin(2, cs, 380, 144, "TBL", "5262-01", 1, named)
	sOther := spin(3, other, 160, 36, "TBL", "5262-01", 1, named, withFG(11))
	snap := &Snapshot{
		Heads: []HeadSnap{old, cs, other},
		Spins: []SpinSnap{sOld, sCS, sOther},
		POYs:  []POYSnap{poy(10, "P1", "5262-01"), poy(11, "P2", "5262-01")},
	}
	rows := []FileRow{
		frow(1, "CS", "P1", spec380, "5262-01", 1),
		frow(2, "CS", "P2", "POY 160/36/TBL", "5262-01", 1), // pair held by sOther, not superseded
	}
	got, hp := planAll(t, rows, snap)
	if got[1].HasReason(ReasonFGConflict) || got[1].Decision != DecisionApply {
		t.Errorf("ln 1 must be exempt: %v %s", got[1].Reasons, got[1].Decision)
	}
	if !got[2].HasReason(ReasonFGConflict) || got[2].Decision != DecisionReview {
		t.Errorf("ln 2 must conflict: %v %s", got[2].Reasons, got[2].Decision)
	}
	a := actionsByKey(hp)["sup:"+old.ID.String()+":"+sOld.ID.String()]
	if a == nil || a.Decision != DecisionApply || a.NewSpinRef() != "spin:"+sCS.ID.String() {
		t.Fatalf("supersede: %+v", a)
	}
	// Without the exemption the row is a conflict and the pair follows it to REVIEW.
	plans, err := ClassifyRows(rows, snap)
	if err != nil {
		t.Fatal(err)
	}
	hp2 := PlanHeads(plans, snap)
	GuardFGConflicts(plans, snap, nil)
	hp2.SyncDecisions()
	a2 := actionsByKey(hp2)["sup:"+old.ID.String()+":"+sOld.ID.String()]
	if a2.Decision != DecisionReview || !a2.HasReason(ReasonTargetRowReview) {
		t.Errorf("pair must follow its row to REVIEW: %s %v", a2.Decision, a2.Reasons)
	}
	// Back to APPLY when the row becomes APPLY again (reason removed).
	plans[0].Reasons, plans[0].Decision = nil, DecisionApply
	hp2.SyncDecisions()
	if a2.Decision != DecisionApply || a2.HasReason(ReasonTargetRowReview) {
		t.Errorf("pair must return to APPLY: %s %v", a2.Decision, a2.Reasons)
	}
}

func TestPlanHeads_AmbiguousAndTargetSelection(t *testing.T) {
	// Two file heads of the same family share the old shade → ambiguous_family.
	amb := namedHead(1, "A-OLD", "MGT DUSK BLUE TR-342-D-00001-C", "Z342T", true)
	ambN1 := namedHead(2, "A-N1", "MGT DUSK BLUE TR-342 N-D-00002-B", "Z342T", true)
	ambN2 := namedHead(3, "A-N2", "MGT DUSK BLUE TR-342 ND-D-00003-B", "Z342T", true)
	sAmb := spin(1, amb, 380, 144, "TBL", "Z342T", 1, named, withRefs(2, 0))
	sAmbN1 := spin(2, ambN1, 380, 144, "TBL", "Z342T", 1, named)
	sAmbN2 := spin(3, ambN2, 380, 144, "TBL", "Z342T", 1, named)

	// One successor, several same-shade rows: den/fil picks one (old 380/144);
	// a second old spin (160/36) has two rows with its den/fil → ambiguous_target;
	// a third (Y-OTHER shade) has no row → no_target_spin.
	old := namedHead(4, "T-OLD", "MGT LAVENDER VIOLET TR-392-D-00001-C", "Z392T", true)
	nw := namedHead(5, "T-N", "MGT LAVENDER VIOLET TR-392 N-D-00002-B", "Z392T", true)
	s380 := spin(4, old, 380, 144, "TBL", "Z392T", 1, named)
	s160 := spin(5, old, 160, 36, "TBL", "Z392T", 1, named)
	sOther := spin(6, old, 250, 48, "TBL", "Y999T", 1, named, withRefs(0, 3))
	nwRoot := spin(7, nw, 380, 144, "TBL", "Z392T", 1, named)

	snap := &Snapshot{
		Heads: []HeadSnap{amb, ambN1, ambN2, old, nw},
		Spins: []SpinSnap{sAmb, sAmbN1, sAmbN2, s380, s160, sOther, nwRoot},
		POYs: []POYSnap{poy(10, "P1", "Z342T"), poy(11, "P2", "Z342T"), poy(12, "P3", "Z392T"),
			poy(13, "P4", "Z392T"), poy(14, "P5", "Z392T"), poy(15, "P6", "Z392T")},
	}
	rows := []FileRow{
		frow(1, "A-N1", "P1", spec380, "Z342T", 1),
		frow(2, "A-N2", "P2", spec380, "Z342T", 1),
		frow(3, "T-N", "P3", spec380, "Z392T", 1),             // KEEP on nwRoot
		frow(4, "T-N", "P4", "POY 160/36/TBL", "Z392T", 1),    // DUPLICATE
		frow(5, "T-N", "P5", "POY 160/36/TBL", "Z392T", 1),    // DUPLICATE (same den/fil/cs as ln 4)
		frow(6, "T-N", "P6", "POY 75/36/TBL", "Z-392-T", 1.1), // DUPLICATE (other spec)
	}
	_, hp := planAll(t, rows, snap)
	checkHeadActions(t, hp, map[string]headWant{
		"rh:" + amb.ID.String():                           {KindReportHead, PhaseNone, DecisionSkip, []string{ReasonAmbiguousFamily}, ""},
		"sup:" + old.ID.String() + ":" + s380.ID.String(): {KindSupersede, PhaseB, DecisionApply, nil, "spin:" + nwRoot.ID.String()},
		"rs:" + s160.ID.String():                          {KindReportSpin, PhaseNone, DecisionSkip, []string{ReasonAmbiguousTarget}, ""},
		"rs:" + sOther.ID.String():                        {KindReportSpin, PhaseNone, DecisionSkip, []string{ReasonNoTargetSpin}, ""},
	})
	if a := actionsByKey(hp)["rh:"+amb.ID.String()]; a.RefPointer != 2 {
		t.Errorf("ambiguous head refs = %d, want 2", a.RefPointer)
	}
	if a := actionsByKey(hp)["rs:"+sOther.ID.String()]; a.RefText != 3 {
		t.Errorf("no_target_spin text refs = %d, want 3", a.RefText)
	}
}

// Live heads not in the file without a successor: DEACTIVATE_HEAD with 0 referrers,
// REPORT_HEAD(has_refs) otherwise (pointer or text). Q5 unallocated live spins of
// file heads and Q6 live spins under dead heads.
func TestPlanHeads_DeactivateAndReport(t *testing.T) {
	plain := namedHead(1, "D1", "MGT PLAIN 1", "A1", true)
	refd := namedHead(2, "D2", "MGT REFERENCED 2", "A2", true)
	empty := namedHead(3, "D3", "MGT EMPTY 3", "", true)
	fileHead := namedHead(4, "F1", "MGT FILE 4", "A4", true)
	deadHead := namedHead(5, "X1", "MGT DEAD 5", "A5", false)
	deadInFile := namedHead(6, "F2", "MGT DEAD IN FILE 6", "A6", false)

	sPlain := spin(1, plain, 380, 144, "TBL", "A1", 1, named)
	sPlainDead := spin(2, plain, 380, 144, "TBL", "A1", 1, named, inactive, withRefs(9, 0)) // non-live refs ignored
	sRefd := spin(3, refd, 380, 144, "TBL", "A2", 1, named, withRefs(0, 1))
	sKeep := spin(4, fileHead, 380, 144, "TBL", "A4", 1, named)
	sFree := spin(5, fileHead, 160, 36, "TBL", "A4", 1, named)
	sFreeRef := spin(6, fileHead, 250, 48, "TBL", "A4", 1, named, withRefs(4, 0))
	sFreeDead := spin(7, fileHead, 75, 36, "TBL", "A4", 1, named, inactive)
	sDead := spin(8, deadHead, 380, 144, "TBL", "A5", 1, named, withRefs(3, 0))
	sDeadNoRef := spin(9, deadHead, 160, 36, "TBL", "A5", 1, named)
	sDeadInFile := spin(10, deadInFile, 380, 144, "TBL", "A6", 1, named)

	snap := &Snapshot{
		Heads: []HeadSnap{plain, refd, empty, fileHead, deadHead, deadInFile},
		Spins: []SpinSnap{sPlain, sPlainDead, sRefd, sKeep, sFree, sFreeRef, sFreeDead, sDead, sDeadNoRef, sDeadInFile},
		POYs:  []POYSnap{poy(10, "P1", "A4"), poy(11, "P2", "A6")},
	}
	rows := []FileRow{
		frow(1, "F1", "P1", spec380, "A4", 1),
		frow(2, "F2", "P2", "POY 75/36/TBL", "A6", 1), // DUPLICATE: the live spin of the dead file head stays unallocated
	}
	_, hp := planAll(t, rows, snap)
	checkHeadActions(t, hp, map[string]headWant{
		"dh:" + plain.ID.String():       {KindDeactivateHead, PhaseB, DecisionApply, nil, ""},
		"dh:" + empty.ID.String():       {KindDeactivateHead, PhaseB, DecisionApply, nil, ""},
		"rh:" + refd.ID.String():        {KindReportHead, PhaseNone, DecisionSkip, []string{ReasonHasRefs}, ""},
		"ds:" + sFree.ID.String():       {KindDeactivateSpin, PhaseB, DecisionApply, []string{ReasonUnallocated}, ""},
		"ds:" + sDeadInFile.ID.String(): {KindDeactivateSpin, PhaseB, DecisionApply, []string{ReasonUnallocated}, ""},
		"rs:" + sFreeRef.ID.String():    {KindReportSpin, PhaseNone, DecisionSkip, []string{ReasonUnallocated, ReasonHasRefs}, ""},
		"rs:" + sDead.ID.String():       {KindReportSpin, PhaseNone, DecisionSkip, []string{ReasonDeadHead, ReasonHasRefs}, ""},
		"rs:" + sDeadNoRef.ID.String():  {KindReportSpin, PhaseNone, DecisionSkip, []string{ReasonDeadHead}, ""},
	})
	if a := actionsByKey(hp)["rh:"+refd.ID.String()]; a.RefText != 1 || a.RefPointer != 0 {
		t.Errorf("has_refs counts: %+v", a)
	}
	if a := actionsByKey(hp)["dh:"+plain.ID.String()]; a.RefPointer != 0 {
		t.Errorf("inactive spin refs must not count: %d", a.RefPointer)
	}
}

// MERLION (Q7): the successor is a SYSID without head → REPORT_HEAD(pending_new_recipe),
// not deactivated. FOURTH: the old version is itself in the file and the new
// version (other shade) is NEW_RECIPE → no head action at all; both stay active.
func TestPlanHeads_MerlionPendingAndFourthNotPaired(t *testing.T) {
	mer := namedHead(1, "20260805402", "MGT MERLION BL 5850-D-04585-B", "5850-03", true)
	fgMer := namedHead(2, "20251104983", "MGT FG MERLION BL 5850-D-04577-A-B", "5850-01", true)
	fourth := namedHead(3, "20260905473", "MGT AT FOURTH BG  6914-D-05176-B", "6914-01", true)
	sMer := spin(1, mer, 250, 48, "RND", "5850-03", 4, named, withRefs(2, 0))
	sFg := spin(2, fgMer, 250, 48, "RND", "5850-01", 6.15, named)
	sFourth := spin(3, fourth, 250, 48, "RND", "6914-01", 2.15, named)
	snap := &Snapshot{
		Heads: []HeadSnap{mer, fgMer, fourth},
		Spins: []SpinSnap{sMer, sFg, sFourth},
		POYs:  []POYSnap{poy(10, "P1", "5850-03"), poy(11, "P1", "5850-01"), poy(12, "P1", "6914-01"), poy(13, "P1", "6914-00")},
	}
	spec := "POY 250/48/RND/DSD"
	rows := []FileRow{
		dyeRow(3346, "20251104983", "P1", spec, "5850-01", 6.15, "MGT FG MERLION BL 5850-D-04577-A-B"),
		dyeRow(3889, "20261005519", "P1", spec, "5850-03", 4.10, "MGT MERLION BL 5850-D-04627-B"),
		dyeRow(3975, "20260905473", "P1", spec, "6914-01", 2.15, "MGT AT FOURTH BG  6914-D-05176-B"),
		dyeRow(4089, "20261005573", "P1", spec, "6914-00", 1.50, "MGT AT FOURTH BG  6914-D-05213-B"),
	}
	got, hp := planAll(t, rows, snap)
	if got[3889].Action != ActionNewRecipe || got[4089].Action != ActionNewRecipe {
		t.Fatalf("MERLION/FOURTH new versions must be NEW_RECIPE")
	}
	checkHeadActions(t, hp, map[string]headWant{
		"rh:" + mer.ID.String(): {KindReportHead, PhaseNone, DecisionSkip, []string{ReasonPendingNewRecipe}, ""},
	})
	a := actionsByKey(hp)["rh:"+mer.ID.String()]
	if a.NewSysID != "20261005519" || a.RefPointer != 2 {
		t.Errorf("pending successor: %q refs %d", a.NewSysID, a.RefPointer)
	}
	if len(hp.Pairs) != 0 {
		t.Errorf("no pairs expected, got %d", len(hp.Pairs))
	}
}

// The family+shade rule uses the old head's shade set (head shade, head code,
// spin shade/cc, loose) against the successor's file shades.
func TestPlanHeads_ShadeSetUsesHeadCodeAndLoose(t *testing.T) {
	old := namedHead(1, "OLD", "MGT TURMERIC 2101-D-00001-C", "", true)
	old.Code = "Y-2101-T"
	nw := namedHead(2, "NEW", "MGT TURMERIC 2101 N-D-00002-B", "Y2101T", true)
	sOld := spin(1, old, 380, 144, "TBL", "", 1, func(s *SpinSnap) { s.CC = "Y 2101 T" })
	sNew := spin(2, nw, 380, 144, "TBL", "Y2101T", 1, named)
	snap := &Snapshot{
		Heads: []HeadSnap{old, nw},
		Spins: []SpinSnap{sOld, sNew},
		POYs:  []POYSnap{poy(10, "P1", "Y2101T")},
	}
	rows := []FileRow{frow(1, "NEW", "P1", spec380, "Y2101T", 1)}
	_, hp := planAll(t, rows, snap)
	checkHeadActions(t, hp, map[string]headWant{
		"sup:" + old.ID.String() + ":" + sOld.ID.String(): {KindSupersede, PhaseB, DecisionApply, nil, "spin:" + sNew.ID.String()},
	})
	// Different family → no pair, plain deactivate.
	snap.Heads[1].MBCosting = "MGT SAFFRON 2101-B"
	_, hp = planAll(t, rows, snap)
	checkHeadActions(t, hp, map[string]headWant{
		"dh:" + old.ID.String(): {KindDeactivateHead, PhaseB, DecisionApply, nil, ""},
	})
}

func TestPlanHeads_Deterministic(t *testing.T) {
	old := namedHead(1, "OLD", "MGT X TR-5-D-1-C", "Z5T", true)
	nw := namedHead(2, "NEW", "MGT X TR-5 N-D-2-B", "Z5T", true)
	gone := namedHead(3, "GONE", "MGT GONE", "Q1", true)
	snap := &Snapshot{
		Heads: []HeadSnap{old, nw, gone},
		Spins: []SpinSnap{
			spin(1, old, 380, 144, "TBL", "Z5T", 1, named),
			spin(2, old, 160, 36, "TBL", "Z5T", 1, named),
			spin(3, nw, 380, 144, "TBL", "Z5T", 1, named),
			spin(4, nw, 75, 36, "TBL", "Z5T", 1, named),
			spin(5, gone, 75, 36, "TBL", "Q1", 1, named),
		},
		POYs: []POYSnap{poy(10, "P1", "Z5T"), poy(11, "P2", "Z5T")},
	}
	rows := []FileRow{
		frow(1, "NEW", "P1", spec380, "Z5T", 1),
		frow(2, "NEW", "P2", "POY 160/36/TBL", "Z5T", 1),
	}
	keys := func(rs []FileRow) []string {
		_, hp := planAll(t, rs, snap)
		var out []string
		for _, a := range hp.Actions {
			out = append(out, a.ItemKey()+"|"+string(a.Phase)+"|"+a.NewSpinRef())
		}
		return out
	}
	want := keys(rows)
	if len(want) != 4 { // 2 SUPERSEDE + DEACTIVATE_HEAD + DEACTIVATE_SPIN (75/36 on NEW)
		t.Fatalf("actions = %v", want)
	}
	r := rand.New(rand.NewSource(1)) //nolint:gosec // test shuffle
	for i := 0; i < 5; i++ {
		rs := append([]FileRow(nil), rows...)
		r.Shuffle(len(rs), func(a, b int) { rs[a], rs[b] = rs[b], rs[a] })
		if got := keys(rs); !reflect.DeepEqual(got, want) {
			t.Fatalf("run %d: %v != %v", i, got, want)
		}
	}
}

// Reversed pair (Q2: new head inactive, reactivated by its row) whose old head has
// no live spin: head-only SUPERSEDE_HEAD (Q15) in phase D after the REACTIVATE row,
// reason new_head_inactive, no new_spin_ref. Counts splits pairs spin-level/head-only.
func TestPlanHeads_ReversedPairWithoutLiveSpinIsHeadOnlySupersede(t *testing.T) {
	oldH := namedHead(1, "R-OLD", "MGT MILIM GR 7001-D-00100-C", "7001-01", true)
	newH := namedHead(2, "R-NEW", "MGT MILIM GR 7001-ND-D-00200-B", "7001-01", false)
	plain := namedHead(3, "D1", "MGT PLAIN 3", "A1", true)

	sOld := spin(1, oldH, 250, 48, "TBL", "7001-01", 1, named, inactive, withRefs(4, 0)) // not live: refs ignored
	sNew := spin(2, newH, 250, 48, "TBL", "7001-01", 1, named, inactive)

	snap := &Snapshot{
		Heads: []HeadSnap{oldH, newH, plain},
		Spins: []SpinSnap{sOld, sNew},
		POYs:  []POYSnap{poy(10, "P-R", "7001-01")},
	}
	rows := []FileRow{frow(1, "R-NEW", "P-R", "POY 250/48/TBL/DBR", "7001-01", 1)}
	got, hp := planAll(t, rows, snap)
	if got[1].Action != ActionReactivate {
		t.Fatalf("row 1 action = %s, want REACTIVATE", got[1].Action)
	}
	checkHeadActions(t, hp, map[string]headWant{
		"suph:" + oldH.ID.String(): {KindSupersedeHead, PhaseD, DecisionApply, []string{ReasonNewHeadInactive}, ""},
		"dh:" + plain.ID.String():  {KindDeactivateHead, PhaseB, DecisionApply, nil, ""},
	})
	a := actionsByKey(hp)["suph:"+oldH.ID.String()]
	if a.NewHead == nil || a.NewHead.ID != newH.ID || a.OldSpin != nil || a.ActivatorRow != got[1] {
		t.Errorf("head-only supersede: new head / old spin / activator wrong: %+v", a)
	}
	want := HeadCounts{Pairs: 1, PairsHeadOnly: 1, SupersedeHead: 1, DeactivateHead: 1}
	if c := hp.Counts(); c != want {
		t.Errorf("counts = %+v, want %+v", c, want)
	}

	// The activator row going to REVIEW pulls the head-only supersede to REVIEW too.
	got[1].Decision = DecisionReview
	hp.SyncDecisions()
	if a.Decision != DecisionReview || !a.HasReason(ReasonTargetRowReview) {
		t.Errorf("after activator REVIEW: %s %v", a.Decision, a.Reasons)
	}
	got[1].Decision = DecisionApply
	hp.SyncDecisions()
	if a.Decision != DecisionApply || a.HasReason(ReasonTargetRowReview) {
		t.Errorf("after activator APPLY: %s %v", a.Decision, a.Reasons)
	}
}

// Pair without live old spin whose new head is live (KEEP row): head-only
// SUPERSEDE_HEAD in phase B, no activator row. An inactive new head without any
// actionable row cannot be reactivated → REPORT_HEAD(new_head_inactive,no_activating_row).
func TestPlanHeads_HeadOnlySupersedeLiveNewHeadAndNoActivator(t *testing.T) {
	oldH := namedHead(1, "L-OLD", "MGT LARANJA OR 47-C", "Y47T", true)
	newH := namedHead(2, "L-NEW", "MGT LARANJA OR 47-N-D-01795-C", "Y47T", true)
	old2 := namedHead(3, "T-OLD", "MGT TURMERIC YW 2101-D-00860-C", "Y2101", true)
	new2 := namedHead(4, "T-NEW", "MGT TURMERIC YW 2101-N-D-02227-B", "Y2101", false)

	sNew := spin(1, newH, 380, 144, "TBL", "Y47T", 1.2, named)

	snap := &Snapshot{
		Heads: []HeadSnap{oldH, newH, old2, new2},
		Spins: []SpinSnap{sNew},
		POYs:  []POYSnap{poy(10, "P-L", "Y47T"), poy(11, "P-T", "Y2101")},
	}
	rows := []FileRow{
		frow(1, "L-NEW", "P-L", spec380, "Y47T", 1.2),
		frow(2, "T-NEW", "P-UNKNOWN", spec380, "Y2101", 1), // REPORT_ONLY: not actionable
	}
	got, hp := planAll(t, rows, snap)
	if got[1].Action != ActionKeep || got[2].Action != ActionReportOnly {
		t.Fatalf("row actions = %s %s, want KEEP REPORT_ONLY", got[1].Action, got[2].Action)
	}
	checkHeadActions(t, hp, map[string]headWant{
		"suph:" + oldH.ID.String(): {KindSupersedeHead, PhaseB, DecisionApply, nil, ""},
		"rh:" + old2.ID.String():   {KindReportHead, PhaseNone, DecisionSkip, []string{ReasonNewHeadInactive, ReasonNoActivatingRow}, ""},
	})
	if a := actionsByKey(hp)["suph:"+oldH.ID.String()]; a.ActivatorRow != nil {
		t.Errorf("live new head must not have an activator row")
	}
	want := HeadCounts{Pairs: 2, PairsHeadOnly: 2, SupersedeHead: 1, ReportHead: 1}
	if c := hp.Counts(); c != want {
		t.Errorf("counts = %+v, want %+v", c, want)
	}
}

// P2h-4a bug 4 safety net: an unallocated live clone whose FG is a target FG of
// an actionable row of its head is reported, never deactivated; a clone with a
// foreign FG and a plain root are still deactivated (Q5).
func TestPlanHeads_UnallocatedCloneWithFileFGNotDeactivated(t *testing.T) {
	h := namedHead(1, "S1", "MGT S1", "Y145T", true)
	src := spin(1, h, 380, 144, "TBL", "Y145T", 1.39, named)
	// The row (cs TBL, den 380) cannot allocate these 160/36 clones.
	cloneFile := spin(2, h, 160, 36, "TBL", "Y145T", 1.39, named, child(src.ID), withFG(10))
	cloneOther := spin(3, h, 160, 36, "TBL", "Y145T", 1.39, named, child(src.ID), withFG(77))
	rootFree := spin(4, h, 75, 36, "TBL", "Y145T", 1.39, named, withFG(10))
	snap := &Snapshot{
		Heads: []HeadSnap{h},
		Spins: []SpinSnap{src, cloneFile, cloneOther, rootFree},
		POYs:  []POYSnap{poy(10, "PA", "Y145T")},
	}
	_, hp := planAll(t, []FileRow{frow(1, "S1", "PA", spec380, "Y145T", 1.39)}, snap)
	checkHeadActions(t, hp, map[string]headWant{
		"rs:" + cloneFile.ID.String():  {KindReportSpin, PhaseNone, DecisionSkip, []string{ReasonUnallocated, ReasonCloneFGInFile}, ""},
		"ds:" + cloneOther.ID.String(): {KindDeactivateSpin, PhaseB, DecisionApply, []string{ReasonUnallocated}, ""},
		"ds:" + rootFree.ID.String():   {KindDeactivateSpin, PhaseB, DecisionApply, []string{ReasonUnallocated}, ""},
	})
}
