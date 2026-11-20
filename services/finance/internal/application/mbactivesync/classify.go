package mbactivesync

import (
	"bytes"
	"fmt"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// Action is the primary action of one file row (design §3.5).
type Action string

// Row actions.
const (
	ActionKeep          Action = "KEEP"
	ActionUpdateLDR     Action = "UPDATE_LDR"
	ActionUpdateShade   Action = "UPDATE_SHADE"
	ActionReactivate    Action = "REACTIVATE"
	ActionDuplicateSpin Action = "DUPLICATE_SPIN"
	ActionGenerateSpin  Action = "GENERATE_SPIN"
	ActionNewRecipe     Action = "NEW_RECIPE"
	ActionReportOnly    Action = "REPORT_ONLY"
)

// Role tells GENERATE_SPIN rows apart: the first row of a SYSID creates the root.
type Role string

// Generate roles.
const (
	RoleNone Role = ""
	RoleRoot Role = "ROOT"
	RoleDup  Role = "DUP"
)

// Phase is the apply phase of an item.
type Phase string

// Apply phases.
const (
	PhaseNone Phase = ""
	PhaseA    Phase = "A"
	PhaseB    Phase = "B"
	PhaseC    Phase = "C"
	PhaseD    Phase = "D"
)

// Decision is the reviewable verdict of a plan item.
type Decision string

// Decisions.
const (
	DecisionApply  Decision = "APPLY"
	DecisionReview Decision = "REVIEW"
	DecisionSkip   Decision = "SKIP"
)

// Sub-actions (column sub_actions, emitted in this canonical order).
const (
	SubSetFG        = "SET_FG"
	SubSetLDR       = "SET_LDR"
	SubSetShade     = "SET_SHADE"
	SubActSpin      = "ACT_SPIN"
	SubActHead      = "ACT_HEAD"
	SubSetHeadShade = "SET_HEAD_SHADE"
	// SubKeepHeadShade: root spin of a head whose Excel rows carry several shades and
	// the head shade is one of them; the spin shade is written, the head is left alone.
	SubKeepHeadShade = "KEEP_HEAD_SHADE"
	// SubForceHeadShade: Excel wins over a head shade that differs beyond format
	// (single Excel shade, or the lowest-ln root row when the head shade is in none of
	// several Excel shades); always paired with SET_HEAD_SHADE, still without cascade.
	SubForceHeadShade = "FORCE_HEAD_SHADE"
)

var subActionOrder = []string{SubSetFG, SubSetLDR, SubSetShade, SubActSpin, SubActHead, SubSetHeadShade, SubKeepHeadShade, SubForceHeadShade}

// Reasons. Blocking reasons turn an APPLY row into REVIEW.
const (
	ReasonPOYNotFound      = "poy_not_found"
	ReasonSpecUnparsed     = "spec_unparsed"
	ReasonDupRow           = "dup_row"
	ReasonSpec             = "spec"
	ReasonSpinTaken        = "spin_taken"
	ReasonNoRoot           = "no_root"
	ReasonPOYAmbiguous     = "poy_ambiguous"
	ReasonShadeFormat      = "shade_format"
	ReasonHeadShadeDiffers = "head_shade_differs"
	ReasonFGConflict       = "fg_conflict"
	ReasonFGDiffers        = "fg_differs"
	ReasonSpinSuperseded   = "spin_superseded"
	ReasonHeadSuperseded   = "head_superseded"
	ReasonHeadDeleted      = "head_deleted"
	ReasonDozMissing       = "doz_missing"
)

var blockingReasons = map[string]bool{
	ReasonHeadShadeDiffers: true,
	ReasonFGConflict:       true,
	ReasonFGDiffers:        true,
	ReasonSpinSuperseded:   true,
	ReasonHeadSuperseded:   true,
	ReasonHeadDeleted:      true,
	ReasonDozMissing:       true,
}

// RowPlan is the classification of one file row.
type RowPlan struct {
	Row   FileRow
	Head  *HeadSnap    // nil → NEW_RECIPE
	POY   *POYSnap     // nil → REPORT_ONLY(poy_not_found)
	Alloc *AllocResult // best candidate (also for spin_taken rows); nil when none
	Spin  *SpinSnap    // existing target spin (KEEP/UPDATE_*/REACTIVATE)

	Action     Action
	Role       Role
	Phase      Phase
	Decision   Decision
	Reasons    []string
	SubActions []string

	SourceSpin *SpinSnap // DUPLICATE_SPIN source (root spin of the head)
	rootRow    bool      // the row's spin is (or becomes) a root: rootHeadShade ran
	// HeadShadeCode is the exact head shade a FORCE_HEAD_SHADE row writes ("" = the
	// row's TargetShadeCode).
	HeadShadeCode string
	headForced    string // exact head shade the plan forces on this row's head ("" = none)
	SourceLn      int    // GENERATE_SPIN role=DUP: ln of the ROOT row

	TargetFG        *int64
	TargetShadeCode string // exact POY shade code
	TargetShadeName string // file shade name
	TargetDen       *float64
	TargetFil       *int
	TargetCS        string
	TargetLDR       *float64
}

// SourceRef is the source_spin_id CSV value: a uuid, "ln:<n>" or "".
func (p *RowPlan) SourceRef() string {
	switch {
	case p.SourceSpin != nil:
		return p.SourceSpin.ID.String()
	case p.SourceLn > 0:
		return "ln:" + strconv.Itoa(p.SourceLn)
	}
	return ""
}

// HasReason reports whether r is among the row's reasons.
func (p *RowPlan) HasReason(r string) bool { return contains(p.Reasons, r) }

// HasSub reports whether s is among the row's sub-actions.
func (p *RowPlan) HasSub(s string) bool { return contains(p.SubActions, s) }

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func (p *RowPlan) addReason(r string) {
	if !p.HasReason(r) {
		p.Reasons = append(p.Reasons, r)
	}
}

func (p *RowPlan) addSub(s string) {
	if p.HasSub(s) {
		return
	}
	p.SubActions = append(p.SubActions, s)
	sort.SliceStable(p.SubActions, func(i, j int) bool {
		return subIndex(p.SubActions[i]) < subIndex(p.SubActions[j])
	})
}

func (p *RowPlan) removeSub(s string) {
	out := p.SubActions[:0]
	for _, x := range p.SubActions {
		if x != s {
			out = append(out, x)
		}
	}
	p.SubActions = out
}

func subIndex(s string) int {
	for i, x := range subActionOrder {
		if x == s {
			return i
		}
	}
	return len(subActionOrder)
}

// actionable reports whether the row would write to the database.
func (p *RowPlan) actionable() bool {
	switch p.Action {
	case ActionKeep, ActionUpdateLDR, ActionUpdateShade, ActionReactivate, ActionDuplicateSpin, ActionGenerateSpin:
		return true
	case ActionNewRecipe, ActionReportOnly:
		return false
	}
	return false
}

// decide sets Decision from the action and the blocking reasons.
func (p *RowPlan) decide() {
	if !p.actionable() {
		p.Decision = DecisionSkip
		return
	}
	for _, r := range p.Reasons {
		if blockingReasons[r] {
			p.Decision = DecisionReview
			return
		}
	}
	p.Decision = DecisionApply
}

// headLiveL14 is hd.live of L-00 (deleted_at IS NULL AND is_active); a superseded
// head is flagged separately (head_superseded) so the counts stay comparable to L-14.
func headLiveL14(h *HeadSnap) bool { return !h.Deleted && h.IsActive }

type classifier struct {
	headsBySys  map[string]*HeadSnap
	spinsByHead map[uuid.UUID][]*SpinSnap
	poyByCode   map[string][]*POYSnap
}

func newClassifier(snap *Snapshot) *classifier {
	c := &classifier{headsBySys: map[string]*HeadSnap{}, poyByCode: map[string][]*POYSnap{}}
	for sys, hs := range snap.HeadsBySysID() {
		c.headsBySys[sys] = pickHead(hs)
	}
	c.spinsByHead = snap.SpinsByHead()
	for i := range snap.POYs {
		p := &snap.POYs[i]
		c.poyByCode[p.ERPItemCode] = append(c.poyByCode[p.ERPItemCode], p)
	}
	return c
}

// pickHead chooses one head per SYSID (the column is UNIQUE; defensive):
// non-deleted first, then created_at, then id.
func pickHead(hs []*HeadSnap) *HeadSnap {
	best := hs[0]
	for _, h := range hs[1:] {
		if h.Deleted != best.Deleted {
			if !h.Deleted {
				best = h
			}
			continue
		}
		if h.CreatedAt.Before(best.CreatedAt) || (h.CreatedAt.Equal(best.CreatedAt) && bytes.Compare(h.ID[:], best.ID[:]) < 0) {
			best = h
		}
	}
	return best
}

// matchPOY ports L-01 fpoy: active POY with the same ERP item code and the same
// loose shade; min(sys_id) when several (reason poy_ambiguous).
func (c *classifier) matchPOY(r FileRow) (*POYSnap, bool) {
	fs := NormShadeLoose(r.Shade)
	if fs == "" || r.POY == "" {
		return nil, false
	}
	var best *POYSnap
	n := 0
	for _, p := range c.poyByCode[r.POY] {
		if !p.IsActive || NormShadeLoose(p.ShadeCode) != fs {
			continue
		}
		n++
		if best == nil || p.SysID < best.SysID {
			best = p
		}
	}
	return best, n > 1
}

// ClassifyRows applies rules 1–10 of design §3.5 to every file row, including
// the L-14 allocation and the duplicate-source selection. The FG-conflict guard
// is a separate step (GuardFGConflicts) because it needs the supersede pairs.
// Output is sorted by ln and independent of the input order.
func ClassifyRows(rows []FileRow, snap *Snapshot) ([]*RowPlan, error) {
	sorted := append([]FileRow(nil), rows...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Ln < sorted[j].Ln })
	for i := 1; i < len(sorted); i++ {
		if sorted[i].Ln == sorted[i-1].Ln {
			return nil, fmt.Errorf("duplicate ln %d in file rows", sorted[i].Ln)
		}
	}
	c := newClassifier(snap)
	plans := make([]*RowPlan, 0, len(sorted))
	var allocIn []AllocInput
	var pending []*RowPlan
	seen := map[string]bool{}
	for _, r := range sorted {
		p := c.prepare(r, seen)
		plans = append(plans, p)
		if p.Action == "" {
			allocIn = append(allocIn, AllocInput{Row: r, HeadID: p.Head.ID, TargetFG: p.TargetFG})
			pending = append(pending, p)
		}
	}
	allocs := Allocate(allocIn, c.spinsByHead)
	for _, p := range pending {
		c.classifyAllocated(p, allocs[p.Row.Ln])
	}
	c.assignGenerateRoles(plans)
	finalizeHeadShade(plans)
	resolveHeadShadeDiffers(plans)
	reshadeDuplicatesOfReshadedSources(plans)
	for _, p := range plans {
		p.decide()
	}
	return plans, nil
}

// prepare runs rules 1–4 and fills targets; Action stays "" for rows that go to allocation.
func (c *classifier) prepare(r FileRow, seen map[string]bool) *RowPlan {
	p := &RowPlan{Row: r}
	p.Head = c.headsBySys[r.SysID]
	if p.Head == nil { // rule 1
		p.Action, p.Phase = ActionNewRecipe, PhaseD
		return p
	}
	poy, ambiguous := c.matchPOY(r)
	p.POY = poy
	if ambiguous {
		p.addReason(ReasonPOYAmbiguous)
	}
	if poy == nil || r.Den == nil { // rule 2
		p.Action = ActionReportOnly
		if poy == nil {
			p.addReason(ReasonPOYNotFound)
		}
		if r.Den == nil {
			p.addReason(ReasonSpecUnparsed)
		}
		return p
	}
	c.fillTargets(p)
	key := r.SysID + "\x00" + NormShadeLoose(r.Shade) + "\x00" + r.POY
	if seen[key] { // rule 3
		p.Action = ActionReportOnly
		p.addReason(ReasonDupRow)
		return p
	}
	seen[key] = true
	if len(c.spinsByHead[p.Head.ID]) == 0 { // rule 4 (role assigned later)
		p.Action, p.Phase = ActionGenerateSpin, PhaseC
	}
	return p
}

func (c *classifier) fillTargets(p *RowPlan) {
	fg := p.POY.SysID
	p.TargetFG = &fg
	p.TargetShadeCode = p.POY.ShadeCode
	p.TargetShadeName = p.Row.ShadeName
	if p.Row.DozOK {
		d := p.Row.Doz
		p.TargetLDR = &d
	}
}

// classifyAllocated runs rules 5–10 for a row that went through allocation.
func (c *classifier) classifyAllocated(p *RowPlan, a *AllocResult) {
	p.Alloc = a
	switch {
	case a == nil: // rule 5
		c.toDuplicate(p, ReasonSpec)
	case a.Rank > 1: // rule 6
		c.toDuplicate(p, ReasonSpinTaken)
	default:
		c.toExisting(p, a)
	}
}

func (c *classifier) toDuplicate(p *RowPlan, reason string) {
	p.addReason(reason)
	src := SelectDuplicateSource(c.spinsByHead[p.Head.ID], p.Row)
	if src == nil {
		p.Action = ActionReportOnly
		p.addReason(ReasonNoRoot)
		return
	}
	p.Action, p.Phase, p.SourceSpin = ActionDuplicateSpin, PhaseC, src
	p.setSpecTargets()
	p.newSpinSubs(src.ShadeCode, src.ShadeName)
}

func (p *RowPlan) setSpecTargets() {
	p.TargetDen, p.TargetFil, p.TargetCS = p.Row.Den, p.Row.Fil, p.Row.CS
}

// newSpinSubs computes sub-actions of a spin created by the tool (clone or
// generated root) whose initial shade is fromShade/fromName.
func (p *RowPlan) newSpinSubs(fromShade, fromName string) {
	p.addSub(SubSetFG)
	p.ldrSub(nil)
	if shadeDiffers(fromShade, fromName, p.TargetShadeCode, p.TargetShadeName) {
		p.addSub(SubSetShade)
	}
	p.headSubs()
}

func (c *classifier) toExisting(p *RowPlan, a *AllocResult) {
	sp, h := a.Spin, p.Head
	p.Spin = sp
	live := headLiveL14(h) && sp.Live()
	shEq := a.ShEq != nil && *a.ShEq
	switch {
	case live && a.LDRD != nil && *a.LDRD <= keepLDRDiff4 && shEq: // rule 7
		p.Action, p.Phase = ActionKeep, PhaseA
	case live && !shEq: // rule 8
		p.Action, p.Phase = ActionUpdateShade, PhaseA
	case live: // rule 9
		p.Action, p.Phase = ActionUpdateLDR, PhaseA
	default: // rule 10
		p.Action, p.Phase = ActionReactivate, PhaseD
	}
	p.existingSubs(sp)
	if p.Action == ActionKeep && NormShadeExact(sp.ShadeCode) != NormShadeExact(p.TargetShadeCode) {
		// Same shade loosely (Y-145-T vs Y145T) but not exactly: FG validation and
		// uix_mbs_shade_fg_live compare UPPER(TRIM), so the shade must be rewritten (Q11).
		p.Action = ActionUpdateShade
		p.addReason(ReasonShadeFormat)
	}
	if sp.ParentID == nil {
		p.rootHeadShade()
	}
}

func (p *RowPlan) existingSubs(sp *SpinSnap) {
	if sp.FGProductID == nil || *sp.FGProductID != *p.TargetFG {
		p.addSub(SubSetFG)
		if sp.FGProductID != nil {
			p.addReason(ReasonFGDiffers) // the tool never overwrites another FG
		}
	}
	p.ldrSub(sp)
	if shadeDiffers(sp.ShadeCode, sp.ShadeName, p.TargetShadeCode, p.TargetShadeName) {
		p.addSub(SubSetShade)
	}
	if !sp.IsActive {
		p.addSub(SubActSpin)
	}
	if sp.SupersededBy != nil {
		p.addReason(ReasonSpinSuperseded)
	}
	p.headSubs()
}

// ldrSub adds SET_LDR unless sp already holds doz as a fixed ACTUAL LDR (sp nil = new spin).
func (p *RowPlan) ldrSub(sp *SpinSnap) {
	if p.TargetLDR == nil {
		p.addReason(ReasonDozMissing)
		return
	}
	if sp != nil && sp.RunLDRPct != nil && absInt64(Fix4(*sp.RunLDRPct)-Fix4(*p.TargetLDR)) <= ldrEqual4 &&
		sp.LDRType == "ACTUAL" && sp.LDRIsActual && sp.LDRIsFixed {
		return
	}
	p.addSub(SubSetLDR)
}

func (p *RowPlan) headSubs() {
	h := p.Head
	if h.Deleted {
		p.addReason(ReasonHeadDeleted)
	}
	if h.SupersededBy != nil {
		p.addReason(ReasonHeadSuperseded)
	}
	if !h.IsActive {
		p.addSub(SubActHead)
	}
}

// rootHeadShade is rule 7b / Q12 for a row whose spin is (or becomes) the root:
// re-validate copies the head shade onto the root spin, so the head shade must
// equal the target. Same loosely → SET_HEAD_SHADE (format only, no cascade);
// really different → REVIEW head_shade_differs. Q14: an EMPTY head shade is
// treated like a format-only difference (SET_HEAD_SHADE, stays APPLY).
func (p *RowPlan) rootHeadShade() {
	p.rootRow = true
	h := p.Head
	hl := NormShadeLoose(h.ShadeCode)
	if hl != "" && hl != NormShadeLoose(p.TargetShadeCode) {
		p.addReason(ReasonHeadShadeDiffers)
		return
	}
	if shadeDiffers(h.ShadeCode, h.ShadeName, p.TargetShadeCode, p.TargetShadeName) {
		p.addSub(SubSetHeadShade)
	}
}

// finalizeHeadShade completes Q14 per head: the head shade belongs to the whole
// head, so when several root rows of one head with an EMPTY head shade would each
// write SET_HEAD_SHADE, they must agree (NormShadeLoose) on the target shade.
// Disagreement → every row of that head loses SET_HEAD_SHADE and becomes REVIEW
// head_shade_differs (otherwise apply order decides and later rows get SKIPPED).
func finalizeHeadShade(plans []*RowPlan) {
	groups := map[*HeadSnap][]*RowPlan{}
	for _, p := range plans {
		if p.Head != nil && p.HasSub(SubSetHeadShade) && NormShadeLoose(p.Head.ShadeCode) == "" {
			groups[p.Head] = append(groups[p.Head], p)
		}
	}
	for _, g := range groups {
		agree := true
		for _, p := range g[1:] {
			if NormShadeLoose(p.TargetShadeCode) != NormShadeLoose(g[0].TargetShadeCode) {
				agree = false
				break
			}
		}
		if agree {
			continue
		}
		for _, p := range g {
			p.removeSub(SubSetHeadShade)
			p.addReason(ReasonHeadShadeDiffers)
		}
	}
}

// Only ROOT rows count as head shades: re-validate copies the head shade onto every
// root spin (syncRootSpinShadeFromHead), so child/duplicate rows never conflict.
//
// resolveHeadShadeDiffers turns head_shade_differs on root rows (head shade different
// beyond format, or EMPTY with disagreeing Excel shades, see finalizeHeadShade) into an
// explicit decision, Excel wins:
//   - head has several Excel shades and the head shade is one of them → KEEP_HEAD_SHADE
//     on every such row (spin shade only, head untouched);
//   - several Excel shades, head shade in none → the lowest-ln such row sets the head
//     (SET_HEAD_SHADE+FORCE_HEAD_SHADE), rows with the same loose shade too, others KEEP;
//   - one Excel shade → SET_HEAD_SHADE+FORCE_HEAD_SHADE (no cascade, revertible).
//
// An empty head shade is never one of the Excel shades, so it follows the second case.
// plans is sorted by ln.
func resolveHeadShadeDiffers(plans []*RowPlan) {
	shades := map[*HeadSnap]map[string]bool{}
	differs := map[*HeadSnap][]*RowPlan{}
	for _, p := range plans {
		if p.Head == nil {
			continue
		}
		if shades[p.Head] == nil {
			shades[p.Head] = map[string]bool{}
		}
		if s := NormShadeLoose(p.TargetShadeCode); s != "" && p.rootRow {
			shades[p.Head][s] = true
		}
		if p.HasReason(ReasonHeadShadeDiffers) {
			differs[p.Head] = append(differs[p.Head], p)
		}
	}
	for h, g := range differs {
		forced := ""
		if len(shades[h]) <= 1 || !shades[h][NormShadeLoose(h.ShadeCode)] {
			forced = g[0].TargetShadeCode
		}
		decideHeadShade(g, forced)
	}
}

// decideHeadShade marks one head's rows: those matching forced (loose) FORCE the head to
// forced, exactly (one exact code per head, so two winners never flip the head format;
// each spin keeps its own exact POY shade); the rest KEEP. forced "" = KEEP all.
func decideHeadShade(g []*RowPlan, forced string) {
	winner := NormShadeLoose(forced)
	for _, p := range g {
		p.headForced = forced
		p.Reasons = removeStr(p.Reasons, ReasonHeadShadeDiffers)
		if winner != "" && NormShadeLoose(p.TargetShadeCode) == winner {
			p.addSub(SubSetHeadShade)
			p.addSub(SubForceHeadShade)
			p.HeadShadeCode = forced
			continue
		}
		p.addSub(SubKeepHeadShade)
	}
}

// reshadeDuplicatesOfReshadedSources: phase A re-shades a source spin (another row's
// SET_SHADE) before phase C clones it, so a DUPLICATE_SPIN row planned without
// SET_SHADE (source shade == target at plan time) would get the new shade and fail
// the FG shade check. Such rows always carry SET_SHADE (writer applies it only when
// the live shade differs).
func reshadeDuplicatesOfReshadedSources(plans []*RowPlan) {
	reshaded := map[uuid.UUID]bool{}
	for _, p := range plans {
		if p.Spin != nil && p.HasSub(SubSetShade) {
			reshaded[p.Spin.ID] = true
		}
	}
	for _, p := range plans {
		if p.Action == ActionDuplicateSpin && p.SourceSpin != nil && reshaded[p.SourceSpin.ID] {
			p.addSub(SubSetShade)
		}
	}
}

func removeStr(list []string, v string) []string {
	out := list[:0]
	for _, x := range list {
		if x != v {
			out = append(out, x)
		}
	}
	return out
}

// shadeDiffers compares code exactly (UPPER(TRIM)) and the name trimmed; an empty
// target name is never written, so it never differs.
func shadeDiffers(code, name, tCode, tName string) bool {
	if NormShadeExact(code) != NormShadeExact(tCode) {
		return true
	}
	tName = strings.TrimSpace(tName)
	return tName != "" && strings.TrimSpace(name) != tName
}

// assignGenerateRoles: per SYSID the first GENERATE_SPIN row (by ln) is ROOT,
// the others are DUP with source ln:<root>. plans is sorted by ln.
func (c *classifier) assignGenerateRoles(plans []*RowPlan) {
	roots := map[string]*RowPlan{}
	for _, p := range plans {
		if p.Action != ActionGenerateSpin {
			continue
		}
		root, ok := roots[p.Row.SysID]
		if !ok {
			roots[p.Row.SysID] = p
			p.Role = RoleRoot
			// The generated root starts with the HEAD spec (auto-gen copies den/fil from the
			// head, cross-section empty): target the ROW spec so the apply moves it there and a
			// re-plan of the row allocates it (KEEP) instead of duplicating again (P2h-4c).
			p.setSpecTargets()
			p.newSpinSubs(p.Head.ShadeCode, p.Head.ShadeName)
			p.rootHeadShade()
			continue
		}
		p.Role, p.SourceLn = RoleDup, root.Row.Ln
		p.setSpecTargets()
		p.newSpinSubs(root.TargetShadeCode, root.TargetShadeName)
	}
}

// FGHolderExempt reports that a live spin already holding p's (shade, FG) pair is
// resolved by the plan itself (the old spin of p's SUPERSEDE pair, design §3.6).
type FGHolderExempt func(p *RowPlan, holder *SpinSnap) bool

type fgPair struct {
	shade string
	fg    int64
}

// GuardFGConflicts enforces uix_mbs_shade_fg_live at plan time (design §3.5):
// a (UPPER(TRIM) target shade, FG) pair can be owned by one live spin only. Rows
// (ln order) whose pair is already held by a live spin outside the plan, or
// claimed by an earlier row, get reason fg_conflict, lose SET_FG and become
// REVIEW. A holder that is the row's own target spin is not a conflict; a holder
// that is the target spin of another row makes that row the owner. exempt may be nil.
func GuardFGConflicts(plans []*RowPlan, snap *Snapshot, exempt FGHolderExempt) {
	holders := map[fgPair][]*SpinSnap{}
	for i := range snap.Spins {
		s := &snap.Spins[i]
		if s.Live() && s.FGProductID != nil {
			k := fgPair{NormShadeExact(s.ShadeCode), *s.FGProductID}
			holders[k] = append(holders[k], s)
		}
	}
	targetOf := map[fgPair]map[uuid.UUID]bool{}
	for _, p := range plans {
		if p.actionable() && p.TargetFG != nil && p.Spin != nil {
			k := p.pair()
			if targetOf[k] == nil {
				targetOf[k] = map[uuid.UUID]bool{}
			}
			targetOf[k][p.Spin.ID] = true
		}
	}
	claimed := map[fgPair]bool{}
	for _, p := range plans { // sorted by ln
		if !p.actionable() || p.TargetFG == nil {
			continue
		}
		k := p.pair()
		if heldElsewhere(p, holders[k], targetOf[k], exempt) || claimed[k] {
			p.addReason(ReasonFGConflict)
			p.removeSub(SubSetFG)
			p.decide()
			continue
		}
		claimed[k] = true
	}
}

func (p *RowPlan) pair() fgPair { return fgPair{NormShadeExact(p.TargetShadeCode), *p.TargetFG} }

func heldElsewhere(p *RowPlan, hs []*SpinSnap, targets map[uuid.UUID]bool, exempt FGHolderExempt) bool {
	for _, h := range hs {
		if p.Spin != nil && p.Spin.ID == h.ID {
			continue
		}
		if targets[h.ID] {
			return true // another row owns this holder
		}
		if exempt != nil && exempt(p, h) {
			continue
		}
		return true
	}
	return false
}
