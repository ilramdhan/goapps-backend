package mbactivesync

import (
	"sort"
	"strconv"

	"github.com/google/uuid"
)

// HeadKind is the kind of a head/spin-level plan item (plan-heads.csv, design §3.6).
type HeadKind string

// Head/spin action kinds.
const (
	KindSupersede HeadKind = "SUPERSEDE"
	// KindSupersedeHead is the head-only supersede (Q15): a family+shade pair whose
	// old head has no live spin. Apply sets mbh_superseded_by_head_id old→new and
	// deactivates the old head; no spin is repointed. item_key suph:<old_head>.
	KindSupersedeHead  HeadKind = "SUPERSEDE_HEAD"
	KindDeactivateHead HeadKind = "DEACTIVATE_HEAD"
	KindDeactivateSpin HeadKind = "DEACTIVATE_SPIN"
	KindReportSpin     HeadKind = "REPORT_SPIN"
	KindReportHead     HeadKind = "REPORT_HEAD"
)

var headKindOrder = map[HeadKind]int{
	KindSupersede: 0, KindSupersedeHead: 1, KindDeactivateHead: 2, KindDeactivateSpin: 3, KindReportHead: 4, KindReportSpin: 5,
}

// Head/spin action reasons.
const (
	ReasonAmbiguousFamily  = "ambiguous_family"   // >1 file head of the same family shares a shade with the old head
	ReasonNoTargetSpin     = "no_target_spin"     // old live spin has no file row (same shade, den/fil) on the new head
	ReasonAmbiguousTarget  = "ambiguous_target"   // >1 file row of the new head matches the old spin (shade+den+fil)
	ReasonPendingNewRecipe = "pending_new_recipe" // family+shade successor is a SYSID without head (Q7, MERLION)
	ReasonHasRefs          = "has_refs"           // products still reference the spin(s) (MB_SP_CODE pointer/text)
	ReasonUnallocated      = "unallocated"        // live spin of a file head no row was allocated to (Q5)
	ReasonDeadHead         = "dead_head"          // live spin under a non-live head whose SYSID is not in the file (Q6)
	ReasonNewHeadInactive  = "new_head_inactive"  // reversed case (Q2): new head is reactivated by its row first
	ReasonTargetRowReview  = "target_row_review"  // the new spin's row is not APPLY → the pair is reviewed too
	ReasonNoActivatingRow  = "no_activating_row"  // head-only supersede: new head inactive and no actionable row reactivates it
	// ReasonCloneFGInFile (P2h-4a safety net): an unallocated live clone whose FG is
	// the target FG of an actionable row of its head is reported, never deactivated.
	ReasonCloneFGInFile = "clone_fg_in_file"
)

// SupersedePair is one old→new head pair found by the family+shade rule.
type SupersedePair struct {
	Old *HeadSnap
	New *HeadSnap
	// HeadOnly: the old head has no live spin, so the pair is one SUPERSEDE_HEAD
	// item (Q15) instead of one SUPERSEDE item per live old spin.
	HeadOnly bool
}

// HeadAction is one row of plan-heads.csv.
type HeadAction struct {
	Kind    HeadKind
	OldHead *HeadSnap // SUPERSEDE/DEACTIVATE_HEAD/REPORT_HEAD; head of the spin for *_SPIN
	OldSpin *SpinSnap // SUPERSEDE (old live spin), DEACTIVATE_SPIN, REPORT_SPIN
	NewHead *HeadSnap // SUPERSEDE / SUPERSEDE_HEAD
	NewRow  *RowPlan  // SUPERSEDE: the file row whose (existing or created) spin replaces OldSpin

	// ActivatorRow (SUPERSEDE_HEAD, new head inactive): the new head's row whose
	// ACT_HEAD reactivates it first (earliest phase, then ln). It drives Phase and
	// Decision only; new_spin_ref stays empty (no spin is repointed).
	ActivatorRow *RowPlan

	// NewSysID is the successor SYSID without head (REPORT_HEAD pending_new_recipe).
	NewSysID string

	RefPointer int // products referencing via cpp_value_mb_spin_id (summed over live spins for head items)
	RefText    int // pointer-less text references (same rule as the supersede repoint)

	Phase    Phase
	Reasons  []string
	Decision Decision
}

// ItemKey is the stable key of the item: sup:<old_head>:<old_spin>,
// suph:<old_head> (head-only supersede, Q15), dh:<head>, ds:<spin>, rs:<spin>, rh:<head>.
func (a *HeadAction) ItemKey() string {
	switch a.Kind {
	case KindSupersede:
		return "sup:" + a.OldHead.ID.String() + ":" + a.OldSpin.ID.String()
	case KindSupersedeHead:
		return "suph:" + a.OldHead.ID.String()
	case KindDeactivateHead:
		return "dh:" + a.OldHead.ID.String()
	case KindDeactivateSpin:
		return "ds:" + a.OldSpin.ID.String()
	case KindReportSpin:
		return "rs:" + a.OldSpin.ID.String()
	case KindReportHead:
		return "rh:" + a.OldHead.ID.String()
	}
	return ""
}

// NewSpinRef is new_spin_ref: "spin:<uuid>" for a row on an existing spin
// (KEEP/UPDATE_*/REACTIVATE), "ln:<n>" for a spin the row creates (GENERATE/DUPLICATE).
func (a *HeadAction) NewSpinRef() string {
	if a.NewRow == nil {
		return ""
	}
	if a.NewRow.Spin != nil {
		return "spin:" + a.NewRow.Spin.ID.String()
	}
	return "ln:" + strconv.Itoa(a.NewRow.Row.Ln)
}

// HasReason reports whether r is among the action's reasons.
func (a *HeadAction) HasReason(r string) bool { return contains(a.Reasons, r) }

func (a *HeadAction) addReason(r string) {
	if !a.HasReason(r) {
		a.Reasons = append(a.Reasons, r)
	}
}

// HeadPlan is the head/spin part of the plan.
type HeadPlan struct {
	Actions []*HeadAction   // sorted by kind, then item key
	Pairs   []SupersedePair // family+shade pairs (R2: 13), spin-level and head-only

	// exempt[ln][oldSpinID]: old spins resolved by the SUPERSEDE whose new spin is row ln.
	exempt map[int]map[uuid.UUID]bool
}

// HeadCounts reconciles the head plan with R2 §7.
type HeadCounts struct {
	Pairs          int // family+shade pairs (R2 §7 SUPERSEDE expectation: 13) = PairsSpinLevel + PairsHeadOnly
	PairsSpinLevel int // pairs whose old head has live spins (→ SUPERSEDE / REPORT_SPIN items)
	PairsHeadOnly  int // pairs whose old head has no live spin (→ SUPERSEDE_HEAD, or REPORT_HEAD)
	Supersede      int // SUPERSEDE items (one per live old spin)
	SupersedeHead  int // SUPERSEDE_HEAD items (head-only, Q15)
	DeactivateHead int // R2 §7: 170
	DeactivateSpin int
	ReportHead     int
	ReportSpin     int
}

// Counts returns the per-kind totals and the spin-level / head-only pair split.
func (hp *HeadPlan) Counts() HeadCounts {
	c := HeadCounts{Pairs: len(hp.Pairs)}
	for _, p := range hp.Pairs {
		if p.HeadOnly {
			c.PairsHeadOnly++
		} else {
			c.PairsSpinLevel++
		}
	}
	for _, a := range hp.Actions {
		switch a.Kind {
		case KindSupersede:
			c.Supersede++
		case KindSupersedeHead:
			c.SupersedeHead++
		case KindDeactivateHead:
			c.DeactivateHead++
		case KindDeactivateSpin:
			c.DeactivateSpin++
		case KindReportHead:
			c.ReportHead++
		case KindReportSpin:
			c.ReportSpin++
		}
	}
	return c
}

// FGExempt returns the GuardFGConflicts exemption: a live spin holding a row's
// (shade, FG) pair is not a conflict when it is the old spin of that row's
// SUPERSEDE pair (it is deactivated by the supersede, design §3.5).
func (hp *HeadPlan) FGExempt() FGHolderExempt {
	return func(p *RowPlan, holder *SpinSnap) bool {
		return hp.exempt[p.Row.Ln][holder.ID]
	}
}

// SyncDecisions re-derives SUPERSEDE / SUPERSEDE_HEAD decisions from the current
// decision of the target (activator) rows. Call it again after GuardFGConflicts
// (which may move rows to REVIEW).
func (hp *HeadPlan) SyncDecisions() {
	for _, a := range hp.Actions {
		switch a.Kind {
		case KindSupersede:
			a.decideSupersede(a.NewRow)
		case KindSupersedeHead:
			a.decideSupersede(a.ActivatorRow) // nil when the new head is already live
		default:
			// Other kinds have no gating row.
		}
	}
}

// decideSupersede: APPLY when the gating row is APPLY (or there is none),
// otherwise REVIEW(target_row_review).
func (a *HeadAction) decideSupersede(gate *RowPlan) {
	if gate == nil || gate.Decision == DecisionApply {
		out := a.Reasons[:0]
		for _, r := range a.Reasons {
			if r != ReasonTargetRowReview {
				out = append(out, r)
			}
		}
		a.Reasons = out
		a.Decision = DecisionApply
		return
	}
	a.addReason(ReasonTargetRowReview)
	a.Decision = DecisionReview
}

// supersedePhase: B when the new spin is live after phase A, C when it is created
// in phase C, D when it is reactivated in phase D.
func supersedePhase(p *RowPlan) Phase {
	switch p.Action {
	case ActionKeep, ActionUpdateLDR, ActionUpdateShade:
		return PhaseB
	case ActionGenerateSpin, ActionDuplicateSpin:
		return PhaseC
	case ActionReactivate:
		return PhaseD
	case ActionNewRecipe, ActionReportOnly:
		return PhaseNone
	}
	return PhaseNone
}

type headPlanner struct {
	snap        *Snapshot
	plans       []*RowPlan
	spinsByHead map[uuid.UUID][]*SpinSnap
	fileSys     map[string]bool            // SYSIDs present in the file
	fileShades  map[string]map[string]bool // SYSID → Loose file shades
	rowsByHead  map[uuid.UUID][]*RowPlan   // actionable rows with a head, ln order
	allocated   map[uuid.UUID]bool         // spins owned by a row (KEEP/UPDATE_*/REACTIVATE)
	headIdx     map[uuid.UUID]*HeadSnap
	hp          *HeadPlan
}

// PlanHeads ports L-10/L-11/L-12 (design §3.6). plans are the ClassifyRows output
// (sorted by ln). Recommended order: ClassifyRows → PlanHeads →
// GuardFGConflicts(plans, snap, hp.FGExempt()) → hp.SyncDecisions().
func PlanHeads(plans []*RowPlan, snap *Snapshot) *HeadPlan {
	h := &headPlanner{
		snap: snap, plans: plans, spinsByHead: snap.SpinsByHead(),
		fileSys: map[string]bool{}, fileShades: map[string]map[string]bool{},
		rowsByHead: map[uuid.UUID][]*RowPlan{}, allocated: map[uuid.UUID]bool{},
		hp: &HeadPlan{exempt: map[int]map[uuid.UUID]bool{}},
	}
	h.index()
	h.planOldHeads()
	h.planUnallocatedSpins()
	h.planDeadHeadSpins()
	sort.SliceStable(h.hp.Actions, func(i, j int) bool {
		a, b := h.hp.Actions[i], h.hp.Actions[j]
		if a.Kind != b.Kind {
			return headKindOrder[a.Kind] < headKindOrder[b.Kind]
		}
		return a.ItemKey() < b.ItemKey()
	})
	h.hp.SyncDecisions()
	return h.hp
}

func (h *headPlanner) index() {
	for _, p := range h.plans {
		sys := p.Row.SysID
		h.fileSys[sys] = true
		if s := NormShadeLoose(p.Row.Shade); s != "" {
			if h.fileShades[sys] == nil {
				h.fileShades[sys] = map[string]bool{}
			}
			h.fileShades[sys][s] = true
		}
		if p.Head != nil && p.actionable() {
			h.rowsByHead[p.Head.ID] = append(h.rowsByHead[p.Head.ID], p)
		}
		if p.Spin != nil && p.actionable() {
			h.allocated[p.Spin.ID] = true
		}
	}
}

func (h *headPlanner) inFile(hd *HeadSnap) bool { return hd.SysID != "" && h.fileSys[hd.SysID] }

// headShadeSet is L-10 hshade: Loose(head shade, head code, spin shade, spin cc).
func (h *headPlanner) headShadeSet(hd *HeadSnap) map[string]bool {
	set := map[string]bool{}
	add := func(v string) {
		if s := NormShadeLoose(v); s != "" {
			set[s] = true
		}
	}
	add(hd.ShadeCode)
	add(hd.Code)
	for _, sp := range h.spinsByHead[hd.ID] {
		add(sp.ShadeCode)
		add(sp.CC)
	}
	return set
}

func intersects(a, b map[string]bool) bool {
	for k := range a {
		if b[k] {
			return true
		}
	}
	return false
}

func (h *headPlanner) liveSpins(hd *HeadSnap) []*SpinSnap {
	var out []*SpinSnap
	for _, sp := range h.spinsByHead[hd.ID] {
		if sp.Live() {
			out = append(out, sp)
		}
	}
	return out
}

func spinRefs(sp *SpinSnap) (int, int) { return sp.PointerProducts, sp.TextProducts }

func hasRefs(sp *SpinSnap) bool { return sp.PointerProducts > 0 || sp.TextProducts > 0 }

func (h *headPlanner) add(a *HeadAction) { h.hp.Actions = append(h.hp.Actions, a) }

// oldHead: live (L-14 live, not superseded) head whose SYSID is not in the file.
func (h *headPlanner) isOldHead(hd *HeadSnap) bool {
	return headLiveL14(hd) && hd.SupersededBy == nil && !h.inFile(hd)
}

// planOldHeads handles the live heads missing from the file: SUPERSEDE to a
// family+shade successor in the file, REPORT_HEAD (ambiguous / pending new recipe /
// has_refs) or DEACTIVATE_HEAD.
func (h *headPlanner) planOldHeads() {
	for i := range h.snap.Heads {
		old := &h.snap.Heads[i]
		if !h.isOldHead(old) {
			continue
		}
		fam := FamilyKey(old.MBCosting)
		shades := h.headShadeSet(old)
		news := h.successors(old, fam, shades)
		switch {
		case len(news) > 1:
			h.reportHead(old, ReasonAmbiguousFamily, "")
		case len(news) == 1:
			h.supersede(old, news[0])
		default:
			if sys := h.pendingNewRecipe(fam, shades); sys != "" {
				h.reportHead(old, ReasonPendingNewRecipe, sys)
				continue
			}
			h.deactivateOrReport(old, nil, "")
		}
	}
}

// successors: non-deleted file heads (other than old) of the same non-empty family
// whose file shade set intersects old's shade set (L-12 m_fam AND m_shade).
func (h *headPlanner) successors(old *HeadSnap, fam string, shades map[string]bool) []*HeadSnap {
	if fam == "" {
		return nil
	}
	var out []*HeadSnap
	seen := map[uuid.UUID]bool{}
	for _, p := range h.plans {
		n := p.Head
		if n == nil || n.Deleted || n.ID == old.ID || seen[n.ID] {
			continue
		}
		seen[n.ID] = true
		if FamilyKey(n.MBCosting) == fam && intersects(shades, h.fileShades[n.SysID]) {
			out = append(out, n)
		}
	}
	return out
}

// pendingNewRecipe returns the lowest NEW_RECIPE SYSID whose dye name has the same
// family and whose file shades intersect old's shade set (Q7, MERLION).
func (h *headPlanner) pendingNewRecipe(fam string, shades map[string]bool) string {
	if fam == "" {
		return ""
	}
	best := ""
	for _, p := range h.plans {
		if p.Action != ActionNewRecipe || FamilyKey(p.Row.DyeName) != fam {
			continue
		}
		if intersects(shades, h.fileShades[p.Row.SysID]) && (best == "" || p.Row.SysID < best) {
			best = p.Row.SysID
		}
	}
	return best
}

func (h *headPlanner) reportHead(old *HeadSnap, reason, newSys string) {
	a := &HeadAction{Kind: KindReportHead, OldHead: old, NewSysID: newSys, Decision: DecisionSkip}
	h.sumHeadRefs(a, old)
	a.addReason(reason)
	h.add(a)
}

func (h *headPlanner) sumHeadRefs(a *HeadAction, hd *HeadSnap) {
	for _, sp := range h.liveSpins(hd) {
		p, t := spinRefs(sp)
		a.RefPointer += p
		a.RefText += t
	}
}

// deactivateOrReport: DEACTIVATE_HEAD (phase B) when no live spin is referenced,
// otherwise REPORT_HEAD(has_refs). extra is an additional reason (may be "").
func (h *headPlanner) deactivateOrReport(old, newHead *HeadSnap, extra string) {
	a := &HeadAction{Kind: KindDeactivateHead, OldHead: old, NewHead: newHead, Phase: PhaseB, Decision: DecisionApply}
	h.sumHeadRefs(a, old)
	if extra != "" {
		a.addReason(extra)
	}
	if a.RefPointer > 0 || a.RefText > 0 {
		a.Kind, a.Phase, a.Decision = KindReportHead, PhaseNone, DecisionSkip
		a.addReason(ReasonHasRefs)
	}
	h.add(a)
}

// supersede pairs each live old spin with one row of the new head: same Loose
// shade, narrowed by den/fil when several rows match. An old head without live
// spin becomes one head-only SUPERSEDE_HEAD item (Q15).
func (h *headPlanner) supersede(old, newHead *HeadSnap) {
	spins := h.liveSpins(old)
	h.hp.Pairs = append(h.hp.Pairs, SupersedePair{Old: old, New: newHead, HeadOnly: len(spins) == 0})
	if len(spins) == 0 {
		h.supersedeHead(old, newHead)
		return
	}
	for _, sp := range spins {
		row, reason := h.targetRow(sp, newHead)
		if row == nil {
			a := &HeadAction{Kind: KindReportSpin, OldHead: old, OldSpin: sp, NewHead: newHead, Decision: DecisionSkip}
			a.RefPointer, a.RefText = spinRefs(sp)
			a.addReason(reason)
			h.add(a)
			continue
		}
		a := &HeadAction{Kind: KindSupersede, OldHead: old, OldSpin: sp, NewHead: newHead, NewRow: row,
			Phase: supersedePhase(row)}
		a.RefPointer, a.RefText = spinRefs(sp)
		if !newHead.IsActive {
			a.addReason(ReasonNewHeadInactive)
		}
		h.add(a)
		if h.hp.exempt[row.Row.Ln] == nil {
			h.hp.exempt[row.Row.Ln] = map[uuid.UUID]bool{}
		}
		h.hp.exempt[row.Row.Ln][sp.ID] = true
	}
}

// supersedeHead emits the head-only supersede (Q15): set superseded_by old→new
// and deactivate the old head, no spin repoint. Phase B when the new head is live
// after phase A; otherwise the new head is reactivated first by the ACT_HEAD of
// its earliest-phase row (A→B, C→C, D→D; reason new_head_inactive). No actionable
// row on an inactive new head → REPORT_HEAD(no_activating_row). Referrers are
// summed over live spins like every head item, so they are 0 here; the has_refs
// guard is kept for consistency with deactivateOrReport.
func (h *headPlanner) supersedeHead(old, newHead *HeadSnap) {
	a := &HeadAction{Kind: KindSupersedeHead, OldHead: old, NewHead: newHead, Phase: PhaseB}
	h.sumHeadRefs(a, old)
	if a.RefPointer > 0 || a.RefText > 0 {
		a.Kind, a.Phase, a.Decision = KindReportHead, PhaseNone, DecisionSkip
		a.addReason(ReasonHasRefs)
		h.add(a)
		return
	}
	if !newHead.IsActive {
		a.addReason(ReasonNewHeadInactive)
		act := h.activatorRow(newHead)
		if act == nil {
			a.Kind, a.Phase, a.Decision = KindReportHead, PhaseNone, DecisionSkip
			a.addReason(ReasonNoActivatingRow)
			h.add(a)
			return
		}
		a.ActivatorRow = act
		a.Phase = afterActivation(act.Phase)
	}
	h.add(a)
}

// activatorRow is the actionable row of hd with the earliest phase (then lowest
// ln): its ACT_HEAD is the first write that makes hd active.
func (h *headPlanner) activatorRow(hd *HeadSnap) *RowPlan {
	var best *RowPlan
	for _, p := range h.rowsByHead[hd.ID] {
		if best == nil || phaseRank(p.Phase) < phaseRank(best.Phase) {
			best = p
		}
	}
	return best
}

func phaseRank(p Phase) int {
	switch p {
	case PhaseA:
		return 0
	case PhaseB:
		return 1
	case PhaseC:
		return 2
	case PhaseD:
		return 3
	case PhaseNone:
		return 4
	}
	return 4
}

// afterActivation: the phase in which a head reactivated by a row of phase p is
// live: A → B, otherwise the row's own phase (items run after rows in a phase).
func afterActivation(p Phase) Phase {
	if p == PhaseA {
		return PhaseB
	}
	return p
}

func (h *headPlanner) targetRow(sp *SpinSnap, newHead *HeadSnap) (*RowPlan, string) {
	sh := NormShadeLoose(sp.ShadeCode)
	if sh == "" {
		sh = NormShadeLoose(sp.CC)
	}
	if sh == "" {
		return nil, ReasonNoTargetSpin
	}
	cands := filterRows(h.rowsByHead[newHead.ID], func(p *RowPlan) bool { return NormShadeLoose(p.Row.Shade) == sh })
	if len(cands) > 1 {
		if cands = filterRows(cands, func(p *RowPlan) bool { return matchLevel(sp, p.Row) != 0 }); len(cands) == 0 {
			return nil, ReasonAmbiguousTarget // several same-shade rows, none with the spin's den/fil
		}
	}
	if len(cands) > 1 {
		if lvl2 := filterRows(cands, func(p *RowPlan) bool { return matchLevel(sp, p.Row) == AllocLvlDenFilCS }); len(lvl2) > 0 {
			cands = lvl2
		}
	}
	switch len(cands) {
	case 0:
		return nil, ReasonNoTargetSpin
	case 1:
		return cands[0], ""
	}
	return nil, ReasonAmbiguousTarget
}

func filterRows(in []*RowPlan, keep func(*RowPlan) bool) []*RowPlan {
	var out []*RowPlan
	for _, p := range in {
		if keep(p) {
			out = append(out, p)
		}
	}
	return out
}

// planUnallocatedSpins (Q5): live spins of file heads that no row owns →
// DEACTIVATE_SPIN (phase B) with 0 referrers, REPORT_SPIN(has_refs) otherwise.
func (h *headPlanner) planUnallocatedSpins() {
	for i := range h.snap.Spins {
		sp := &h.snap.Spins[i]
		hd := h.headOf(sp)
		if hd == nil || !h.inFile(hd) || !sp.Live() || h.allocated[sp.ID] {
			continue
		}
		a := &HeadAction{Kind: KindDeactivateSpin, OldHead: hd, OldSpin: sp, Phase: PhaseB, Decision: DecisionApply}
		a.RefPointer, a.RefText = spinRefs(sp)
		a.addReason(ReasonUnallocated)
		if hasRefs(sp) {
			a.Kind, a.Phase, a.Decision = KindReportSpin, PhaseNone, DecisionSkip
			a.addReason(ReasonHasRefs)
		}
		if h.cloneFGInFile(sp) {
			a.Kind, a.Phase, a.Decision = KindReportSpin, PhaseNone, DecisionSkip
			a.addReason(ReasonCloneFGInFile)
		}
		h.add(a)
	}
}

// cloneFGInFile (P2h-4a safety net, bug 4): sp is a clone (mbs_parent_spin_id set,
// as every spin DUPLICATE_SPIN creates) whose FG equals the target FG of an
// actionable row of the same head. Deactivating it would destroy the spin a row
// needs (the next plan re-creates it), so the planner must never do that.
func (h *headPlanner) cloneFGInFile(sp *SpinSnap) bool {
	if sp.ParentID == nil || sp.FGProductID == nil {
		return false
	}
	for _, p := range h.rowsByHead[sp.HeadID] {
		if p.TargetFG != nil && *p.TargetFG == *sp.FGProductID {
			return true
		}
	}
	return false
}

// planDeadHeadSpins (Q6): live spins under a non-live head whose SYSID is not in
// the file are reported only, never written.
func (h *headPlanner) planDeadHeadSpins() {
	for i := range h.snap.Spins {
		sp := &h.snap.Spins[i]
		hd := h.headOf(sp)
		if hd == nil || h.inFile(hd) || !sp.Live() || (headLiveL14(hd) && hd.SupersededBy == nil) {
			continue
		}
		a := &HeadAction{Kind: KindReportSpin, OldHead: hd, OldSpin: sp, Decision: DecisionSkip}
		a.RefPointer, a.RefText = spinRefs(sp)
		a.addReason(ReasonDeadHead)
		if hasRefs(sp) {
			a.addReason(ReasonHasRefs)
		}
		h.add(a)
	}
}

func (h *headPlanner) headOf(sp *SpinSnap) *HeadSnap {
	if h.headIdx == nil {
		h.headIdx = make(map[uuid.UUID]*HeadSnap, len(h.snap.Heads))
		for i := range h.snap.Heads {
			h.headIdx[h.snap.Heads[i].ID] = &h.snap.Heads[i]
		}
	}
	return h.headIdx[sp.HeadID]
}
