package mbactivesync

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ReportVia classifies how a product's MB_SP_CODE is affected by the plan (design §4).
type ReportVia string

// Report via values.
const (
	ViaSupersedeRepoint ReportVia = "SUPERSEDE_REPOINT" // repointed automatically by the supersede apply
	ViaRelinkNeeded     ReportVia = "RELINK_NEEDED"     // FG-aware resolution changes; operator relinks selectively
	ViaAutofillEmpty    ReportVia = "AUTOFILL_EMPTY"    // product had no MB_SP_CODE; auto-fill will now resolve a spin
	ViaNoChange         ReportVia = "NO_CHANGE"
)

// ReportProduct is one product the report simulates (read-only DB view).
type ReportProduct struct {
	SysID        int64
	Code         string
	Name         string
	TypeCode     string
	Shade        string // raw cpm_shade_code
	Locked       bool
	HasSPCode    bool       // a cost_product_parameter MB_SP_CODE row exists
	SPText       string     // cpp_value_text
	SPSpinID     *uuid.UUID // cpp_value_mb_spin_id
	Dozing       *float64   // stored MB_SP_DOZING numeric, nil when absent
	FGCandidates []int64    // ordered POY candidates (product itself when POY, else upstream route POYs)
}

// ReportReader loads the read-only product population of the report. Implemented in
// internal/infrastructure/postgres (one READ ONLY transaction).
type ReportReader interface {
	LoadReportProducts(ctx context.Context) ([]ReportProduct, error)
}

// ReportRow is one line of report.csv.
type ReportRow struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
	Type         string
	Shade        string
	POYUpstream  int64 // matched (simulated) FG candidate, 0 when none
	CurSPCode    string
	CurSpinID    string
	SimSPCode    string
	SimSpinID    string
	CurDozing    *float64
	SimDozing    *float64
	DozingDelta  *float64
	Via          ReportVia
	Locked       bool
	Note         string
}

// ReportStats are counters of the simulated overlay.
type ReportStats struct {
	RowsApplied      int // plan-rows APPLY items simulated
	HeadItemsApplied int // plan-heads APPLY items simulated
	NewSpins         int // spins created by DUPLICATE/GENERATE rows
	Repoints         int // SUPERSEDE old->new spin mappings
	DuplicateKeys    int // (shade,FG) keys held by >1 live spin after the plan (first wins)
	MissingSpins     int // plan references an unknown spin (stale plan)
}

// ReportResult is the output of BuildReport.
type ReportResult struct {
	Rows           []ReportRow
	Considered     int
	ByVia          map[ReportVia]int // over ALL considered products (incl. NO_CHANGE)
	DozingChanged  int               // reported products whose Fix4 dozing differs from the stored one
	LockedAffected []string          // product codes, reported rows with Locked
	Stats          ReportStats
}

// ReportPlanRow is the subset of a reviewed plan-rows record the simulation needs.
type ReportPlanRow struct {
	Ln              int
	Action          Action
	HeadID          uuid.UUID
	SpinID          string // existing spin (KEEP/UPDATE_*/REACTIVATE), "" otherwise
	TargetFG        *int64
	TargetShadeCode string
	TargetLDR       *float64
}

// ReportPlanHead is the subset of a reviewed plan-heads record the simulation needs.
type ReportPlanHead struct {
	Kind      HeadKind
	OldHeadID uuid.UUID
	OldSpinID string
	NewSpin   string // spin:<uuid> | ln:<n>
}

// ReportPlan holds the decision=APPLY items (all phases) of the reviewed plan CSVs.
type ReportPlan struct {
	Rows  []ReportPlanRow
	Heads []ReportPlanHead
}

// LoadReportPlan extracts the APPLY items from parsed reviewed plan CSVs.
func LoadReportPlan(rows, heads *PlanCSV) (*ReportPlan, error) {
	if rows == nil || heads == nil {
		return nil, errors.New("plan csv missing")
	}
	rp := &ReportPlan{}
	for i := range rows.Records {
		if Decision(strings.TrimSpace(rows.Get(i, "decision"))) != DecisionApply {
			continue
		}
		r, err := parseReportRow(rows, i)
		if err != nil {
			return nil, err
		}
		rp.Rows = append(rp.Rows, r)
	}
	for i := range heads.Records {
		if Decision(strings.TrimSpace(heads.Get(i, "decision"))) != DecisionApply {
			continue
		}
		h, err := parseReportHead(heads, i)
		if err != nil {
			return nil, err
		}
		rp.Heads = append(rp.Heads, h)
	}
	return rp, nil
}

func parseReportRow(c *PlanCSV, i int) (ReportPlanRow, error) {
	get := func(col string) string { return strings.TrimSpace(c.Get(i, col)) }
	ln, err := strconv.Atoi(get("ln"))
	if err != nil {
		return ReportPlanRow{}, fmt.Errorf("plan-rows record %d: bad ln %q", i+1, c.Get(i, "ln"))
	}
	r := ReportPlanRow{Ln: ln, Action: Action(get("action")), SpinID: get("spin_id"), TargetShadeCode: get("target_shade_code")}
	if h := get("head_id"); h != "" {
		if r.HeadID, err = uuid.Parse(h); err != nil {
			return r, fmt.Errorf("plan-rows ln %d: bad head_id: %w", ln, err)
		}
	}
	if v := get("target_fg"); v != "" {
		fg, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			return r, fmt.Errorf("plan-rows ln %d: bad target_fg %q", ln, v)
		}
		r.TargetFG = &fg
	}
	if v := get("target_ldr"); v != "" {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			return r, fmt.Errorf("plan-rows ln %d: bad target_ldr %q", ln, v)
		}
		r.TargetLDR = &f
	}
	return r, nil
}

func parseReportHead(c *PlanCSV, i int) (ReportPlanHead, error) {
	get := func(col string) string { return strings.TrimSpace(c.Get(i, col)) }
	h := ReportPlanHead{Kind: HeadKind(get("kind")), OldSpinID: get("old_spin_id"), NewSpin: get("new_spin_ref")}
	if v := get("old_head_id"); v != "" {
		id, err := uuid.Parse(v)
		if err != nil {
			return h, fmt.Errorf("plan-heads %s: bad old_head_id: %w", get("item_key"), err)
		}
		h.OldHeadID = id
	}
	return h, nil
}

// Reporter runs the read-only report.
type Reporter struct {
	snap     SnapshotReader
	products ReportReader
}

// NewReporter returns a Reporter.
func NewReporter(snap SnapshotReader, products ReportReader) *Reporter {
	return &Reporter{snap: snap, products: products}
}

// Run loads the snapshot and the product population (read-only) and simulates plan.
func (r *Reporter) Run(ctx context.Context, plan *ReportPlan) (*ReportResult, error) {
	if r == nil || r.snap == nil || r.products == nil {
		return nil, errors.New("reporter has no readers")
	}
	snap, err := r.snap.Load(ctx)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	prods, err := r.products.LoadReportProducts(ctx)
	if err != nil {
		return nil, fmt.Errorf("load report products: %w", err)
	}
	return BuildReport(plan, snap, prods)
}

type simKey struct {
	Shade string
	FG    int64
}

type simSpin struct {
	ref   string // uuid string, or "ln:<n>" for a spin the plan creates
	id    uuid.UUID
	head  uuid.UUID
	code  string // SpinCode semantics: orion, else MB costing, else id; "" for new spins
	text  string // SpinTextKey (existing spins only)
	shade string // NormShadeExact
	fg    *int64
	live  bool
	ldr   *float64
}

type simState struct {
	spins []*simSpin
	byRef map[string]*simSpin
}

func newSimState(snap *Snapshot) *simState {
	st := &simState{byRef: make(map[string]*simSpin, len(snap.Spins))}
	for i := range snap.Spins {
		sp := &snap.Spins[i]
		code := strings.TrimSpace(sp.OrionItemCode)
		if code == "" {
			code = strings.TrimSpace(sp.MBCosting)
		}
		if code == "" {
			code = sp.ID.String()
		}
		s := &simSpin{ref: sp.ID.String(), id: sp.ID, head: sp.HeadID, code: code, text: sp.SpinTextKey(),
			shade: NormShadeExact(sp.ShadeCode), live: sp.Live(), ldr: sp.RunLDRPct}
		if sp.FGProductID != nil {
			v := *sp.FGProductID
			s.fg = &v
		}
		st.spins = append(st.spins, s)
		st.byRef[s.ref] = s
	}
	return st
}

// index maps (shade, FG) to the live spin holding it. First holder wins (the DB unique
// index forbids more than one; a second one means the plan is internally inconsistent).
func (st *simState) index() (map[simKey]*simSpin, int) {
	idx := make(map[simKey]*simSpin)
	dup := 0
	for _, s := range st.spins {
		if !s.live || s.fg == nil || s.shade == "" {
			continue
		}
		k := simKey{s.shade, *s.fg}
		if _, ok := idx[k]; ok {
			dup++
			continue
		}
		idx[k] = s
	}
	return idx, dup
}

func (st *simState) clone() *simState {
	c := &simState{byRef: make(map[string]*simSpin, len(st.byRef))}
	for _, s := range st.spins {
		cp := *s
		c.spins = append(c.spins, &cp)
		c.byRef[cp.ref] = &cp
	}
	return c
}

func (st *simState) lookup(ref string) *simSpin {
	return st.byRef[strings.TrimPrefix(ref, "spin:")]
}

// apply simulates the APPLY items of plan on st and returns old->new supersede refs.
func (st *simState) apply(plan *ReportPlan, stats *ReportStats) map[string]string {
	rows := append([]ReportPlanRow(nil), plan.Rows...)
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].Ln < rows[j].Ln })
	for _, r := range rows {
		if st.applyRow(r, stats) {
			stats.RowsApplied++
		}
	}
	repoint := make(map[string]string)
	for _, h := range plan.Heads {
		if st.applyHead(h, repoint, stats) {
			stats.HeadItemsApplied++
		}
	}
	return repoint
}

// applyRow simulates one file-row item; false when it changes nothing (or is stale).
func (st *simState) applyRow(r ReportPlanRow, stats *ReportStats) bool {
	switch r.Action {
	case ActionKeep, ActionUpdateLDR, ActionUpdateShade, ActionReactivate:
		sp := st.lookup(r.SpinID)
		if sp == nil {
			stats.MissingSpins++
			return false
		}
		st.setTarget(sp, r)
	case ActionDuplicateSpin, ActionGenerateSpin:
		sp := &simSpin{ref: "ln:" + strconv.Itoa(r.Ln), head: r.HeadID}
		st.setTarget(sp, r)
		st.spins = append(st.spins, sp)
		st.byRef[sp.ref] = sp
		stats.NewSpins++
	case ActionNewRecipe, ActionReportOnly:
		return false
	}
	return true
}

// applyHead simulates one head/spin item; false when it changes nothing (or is stale).
func (st *simState) applyHead(h ReportPlanHead, repoint map[string]string, stats *ReportStats) bool {
	switch h.Kind {
	case KindSupersede:
		old, nw := st.lookup(h.OldSpinID), st.lookup(h.NewSpin)
		if old == nil || nw == nil {
			stats.MissingSpins++
			return false
		}
		old.live = false
		repoint[old.ref] = nw.ref
		stats.Repoints++
	case KindDeactivateSpin:
		sp := st.lookup(h.OldSpinID)
		if sp == nil {
			stats.MissingSpins++
			return false
		}
		sp.live = false
	case KindDeactivateHead:
		for _, sp := range st.spins {
			if sp.head == h.OldHeadID && sp.id != uuid.Nil {
				sp.live = false
			}
		}
	case KindSupersedeHead, KindReportSpin, KindReportHead:
		// no spin state change (head-only supersede / report only)
	}
	return true
}

func (st *simState) setTarget(sp *simSpin, r ReportPlanRow) {
	sp.live = true
	if r.TargetShadeCode != "" {
		sp.shade = NormShadeExact(r.TargetShadeCode)
	}
	if r.TargetFG != nil {
		v := *r.TargetFG
		sp.fg = &v
	}
	if r.TargetLDR != nil {
		v := *r.TargetLDR
		sp.ldr = &v
	}
}

func resolveFG(idx map[simKey]*simSpin, p *ReportProduct) (*simSpin, int64) {
	shade := NormShadeExact(p.Shade)
	if shade == "" {
		return nil, 0
	}
	for _, fg := range p.FGCandidates {
		if fg <= 0 {
			continue
		}
		if s, ok := idx[simKey{shade, fg}]; ok {
			return s, fg
		}
	}
	return nil, 0
}

// currentSpin is the spin MB_SP_CODE points at (pointer) or, pointer-less, the unique
// spin whose text key equals the stored text (nil when none or ambiguous).
func currentSpin(st *simState, p *ReportProduct) *simSpin {
	if p.SPSpinID != nil {
		return st.byRef[p.SPSpinID.String()]
	}
	key := NormShadeExact(p.SPText)
	if key == "" {
		return nil
	}
	var found *simSpin
	for _, s := range st.spins {
		if s.id != uuid.Nil && s.text == key {
			if found != nil {
				return nil
			}
			found = s
		}
	}
	return found
}

// currentSpinSet returns every existing spin matching the product's current text key / pointer.
func currentSpinSet(st *simState, p *ReportProduct) []*simSpin {
	if p.SPSpinID != nil {
		if s := st.byRef[p.SPSpinID.String()]; s != nil {
			return []*simSpin{s}
		}
		return nil
	}
	key := NormShadeExact(p.SPText)
	if key == "" {
		return nil
	}
	var out []*simSpin
	for _, s := range st.spins {
		if s.id != uuid.Nil && s.text == key {
			out = append(out, s)
		}
	}
	return out
}

func spinCodeOrNew(s *simSpin) string {
	if s == nil {
		return ""
	}
	if s.code == "" {
		return "NEW(" + s.ref + ")"
	}
	return s.code
}

func curSPCode(p *ReportProduct, cur *simSpin) string {
	if p.SPText != "" {
		return p.SPText
	}
	return spinCodeOrNew(cur)
}

// BuildReport simulates plan on the snapshot and compares the FG-aware MB_SP_CODE
// resolution and MB_SP_DOZING of every product before/after (design §4). Pure: no I/O.
func BuildReport(plan *ReportPlan, snap *Snapshot, products []ReportProduct) (*ReportResult, error) {
	if plan == nil || snap == nil {
		return nil, errors.New("nil plan or snapshot")
	}
	base := newSimState(snap)
	sim := base.clone()
	res := &ReportResult{ByVia: map[ReportVia]int{}}
	repoint := sim.apply(plan, &res.Stats)
	baseIdx, _ := base.index()
	simIdx, dup := sim.index()
	res.Stats.DuplicateKeys = dup

	for i := range products {
		p := &products[i]
		row, reported := evalProduct(p, base, sim, baseIdx, simIdx, repoint)
		res.Considered++
		res.ByVia[row.Via]++
		if !reported {
			continue
		}
		if row.DozingDelta != nil && Fix4(*row.DozingDelta) != 0 {
			res.DozingChanged++
		}
		if row.Locked {
			res.LockedAffected = append(res.LockedAffected, row.ProductCode)
		}
		res.Rows = append(res.Rows, row)
	}
	return res, nil
}

func evalProduct(p *ReportProduct, base, sim *simState, baseIdx, simIdx map[simKey]*simSpin, repoint map[string]string) (ReportRow, bool) {
	row := ReportRow{ProductSysID: p.SysID, ProductCode: p.Code, ProductName: p.Name, Type: p.TypeCode, Shade: p.Shade,
		Locked: p.Locked, Via: ViaNoChange, CurDozing: p.Dozing}
	cur := currentSpin(sim, p)
	row.CurSPCode = curSPCode(p, cur)
	if p.SPSpinID != nil {
		row.CurSpinID = p.SPSpinID.String()
	} else if cur != nil {
		row.CurSpinID = cur.ref
	}
	if p.SPSpinID == nil && p.SPText != "" && cur == nil {
		row.Note = "cur_text_unmatched_or_ambiguous"
	}

	baseEff := currentSpin(base, p)
	baseHit, _ := resolveFG(baseIdx, p)
	if baseEff == nil {
		baseEff = baseHit
	}
	simHit, simFG := resolveFG(simIdx, p)
	if simHit != nil {
		row.POYUpstream = simFG
	}
	simEff := classifyVia(&row, p, sim, cur, baseHit, simHit, repoint)
	if simEff == nil {
		simEff = cur
	}
	if row.Via == ViaNoChange && simEff == nil {
		simEff = simHit
	}
	fillSim(&row, simEff, simHit)
	impact := fillDozing(&row, p, baseEff, simEff)
	return row, row.Via != ViaNoChange || impact
}

// classifyVia sets row.Via and returns the effective simulated spin when the plan
// changes the product's link (nil otherwise).
func classifyVia(row *ReportRow, p *ReportProduct, sim *simState, cur, baseHit, simHit *simSpin, repoint map[string]string) *simSpin {
	for _, s := range currentSpinSet(sim, p) {
		if to, ok := repoint[s.ref]; ok {
			row.Via = ViaSupersedeRepoint
			return sim.byRef[to]
		}
	}
	if simHit == nil || (baseHit != nil && baseHit.ref == simHit.ref) {
		return nil
	}
	switch {
	case !p.HasSPCode || (p.SPText == "" && p.SPSpinID == nil):
		row.Via = ViaAutofillEmpty
		return simHit
	case cur == nil || cur.ref != simHit.ref:
		row.Via = ViaRelinkNeeded
		return simHit
	}
	return nil
}

func fillSim(row *ReportRow, simEff, simHit *simSpin) {
	switch {
	case row.Via != ViaNoChange && simEff != nil:
		row.SimSpinID, row.SimSPCode = simEff.ref, spinCodeOrNew(simEff)
	case simHit != nil:
		row.SimSpinID, row.SimSPCode = simHit.ref, spinCodeOrNew(simHit)
	default:
		row.SimSpinID, row.SimSPCode = row.CurSpinID, row.CurSPCode
	}
}

// fillDozing sets the simulated dozing / delta (Fix4 compare) and returns true when
// the effective spin's LDR changes and the stored (frozen) value differs from it.
func fillDozing(row *ReportRow, p *ReportProduct, baseEff, simEff *simSpin) bool {
	if simEff == nil || simEff.ldr == nil {
		return false
	}
	v := *simEff.ldr
	row.SimDozing = &v
	if p.Dozing == nil {
		return false
	}
	d := float64(Fix4(v)-Fix4(*p.Dozing)) / fixScale
	row.DozingDelta = &d
	ldrMoved := baseEff == nil || baseEff.ldr == nil || Fix4(*baseEff.ldr) != Fix4(v)
	impact := ldrMoved && Fix4(*p.Dozing) != Fix4(v)
	if impact {
		row.Note = strings.TrimPrefix(row.Note+";dozing_changes", ";")
	}
	return impact
}

// ReportHeader is the fixed report.csv header (design §4).
var ReportHeader = []string{
	"product_sys_id", "product_code", "product_name", "type", "shade", "poy_upstream",
	"cur_sp_code", "cur_spin_id", "sim_sp_code", "sim_spin_id", "cur_dozing", "sim_dozing", "dozing_delta",
	"via", "locked", "note",
}

// ReportRecord renders one ReportRow matching ReportHeader.
func ReportRecord(r *ReportRow) []string {
	poy := ""
	if r.POYUpstream > 0 {
		poy = strconv.FormatInt(r.POYUpstream, 10)
	}
	return []string{
		strconv.FormatInt(r.ProductSysID, 10), r.ProductCode, r.ProductName, r.Type, r.Shade, poy,
		r.CurSPCode, r.CurSpinID, r.SimSPCode, r.SimSpinID, fmtF(r.CurDozing), fmtF(r.SimDozing), fmtDelta(r.DozingDelta),
		string(r.Via), boolStr(r.Locked), r.Note,
	}
}

func fmtDelta(p *float64) string {
	if p == nil {
		return ""
	}
	return strconv.FormatFloat(*p, 'f', 4, 64)
}

// WriteReportCSV writes report.csv.
func WriteReportCSV(w io.Writer, rows []ReportRow) error {
	cw := csv.NewWriter(w)
	if err := cw.Write(ReportHeader); err != nil {
		return fmt.Errorf("write header: %w", err)
	}
	for i := range rows {
		if err := cw.Write(ReportRecord(&rows[i])); err != nil {
			return fmt.Errorf("write report row %d: %w", i+1, err)
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteReportSummary writes the human summary: counts per via, locked products affected.
func WriteReportSummary(w io.Writer, res *ReportResult) error {
	var b strings.Builder
	fmt.Fprintf(&b, "sync-mb-active --report (read-only simulation, decision=APPLY, all phases)\n\n")
	fmt.Fprintf(&b, "plan items simulated: rows=%d head-items=%d | new spins=%d supersede repoints=%d | stale refs=%d | duplicate (shade,FG) keys after plan=%d\n",
		res.Stats.RowsApplied, res.Stats.HeadItemsApplied, res.Stats.NewSpins, res.Stats.Repoints, res.Stats.MissingSpins, res.Stats.DuplicateKeys)
	fmt.Fprintf(&b, "products considered: %d | reported rows: %d | dozing differs (Fix4): %d\n\n", res.Considered, len(res.Rows), res.DozingChanged)
	b.WriteString("per via (all considered products):\n")
	for _, v := range []ReportVia{ViaSupersedeRepoint, ViaRelinkNeeded, ViaAutofillEmpty, ViaNoChange} {
		fmt.Fprintf(&b, "  %-18s %d\n", v, res.ByVia[v])
	}
	locked := append([]string(nil), res.LockedAffected...)
	sort.Strings(locked)
	fmt.Fprintf(&b, "\nlocked products affected: %d\n", len(locked))
	for _, c := range locked {
		fmt.Fprintf(&b, "  %s\n", c)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
