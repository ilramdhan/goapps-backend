package mbactivesync

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Extra result reasons produced by the applier itself (not by a writer).
const (
	ReasonNotWired    = "not_wired"
	ReasonBadPlanItem = "bad_plan_item"
	ReasonCancelled   = "cancelled"
	applyKindRow      = "row"
	applyKindHead     = "head"
)

// ApplyItem is one reviewed plan record selected for apply. Header/Record keep the
// original CSV line so a FAILED item can be written to errors.csv as a re-runnable plan.
type ApplyItem struct {
	Kind   string // "row" | "head"
	Key    string // item_key
	Ln     int    // rows only
	SysID  string // rows: sysid; heads: old head sysid when known
	Action string // rows: action; heads: kind
	Phase  Phase
	Header []string
	Record []string
	Spin   SyncSpinInput // rows only: filled from the plan columns (Actor set at apply time)
	// Phase C clone rows (DUPLICATE_SPIN, GENERATE_SPIN role=DUP): source_spin_id
	// ("<uuid>" or "ln:<n>"), role and the target den/fil.
	Source    string
	Role      string
	TargetDen *float64
	TargetFil *int
	HeadID    uuid.UUID // rows: head_id column (phase C ROOT rows; uuid.Nil when empty)
	bad       string    // non-empty: parse problem, item is reported FAILED(bad_plan_item)
}

// ApplyOptions are the apply-run filters (design §2).
type ApplyOptions struct {
	Phase     Phase
	Actor     string
	OnlySysID []string // empty = all
	Limit     int      // 0 = unlimited; counts processed items
	Batch     int      // 0 = one batch
	Now       func() time.Time
	Logf      func(format string, args ...any)
	// PrevClones is ln -> clone_spin_id from earlier result CSVs (--prev-results, §5.4).
	PrevClones map[int]uuid.UUID
	// PrevRoots is ln -> ROOT spin from earlier result CSVs (LoadPrevRoots).
	PrevRoots map[int]uuid.UUID
}

// ApplyOutcome is one processed item with its result.
type ApplyOutcome struct {
	Item      *ApplyItem
	Result    ItemResult
	Actor     string
	AppliedAt time.Time
	Err       string // set when Result.Result == FAILED
}

// ResultSink receives each outcome immediately (flush per item). A sink error stops the run.
type ResultSink interface {
	Record(o ApplyOutcome) error
}

// StoppedSinkError is ApplySummary.Stopped when the result sink failed.
const StoppedSinkError = "sink_error"

// ApplySummary counts outcomes by result and reason.
type ApplySummary struct {
	Selected  int
	Processed int
	ByResult  map[string]int
	ByReason  map[string]int
	Stopped   string // "" | "limit" | "cancelled" | "sink_error"
}

// Applier runs reviewed plan items through a Writer.
type Applier struct {
	w  Writer
	hw HeadWorkflow
}

// WithHeadWorkflow wires the Submit→Validate workflow used by phase C ROOT rows whose
// head never got its cost product (design part 2 §5.5). Without it such rows are
// SKIPPED not_wired.
func (a *Applier) WithHeadWorkflow(hw HeadWorkflow) *Applier {
	a.hw = hw
	return a
}

// NewApplier returns an Applier over w.
func NewApplier(w Writer) *Applier { return &Applier{w: w} }

// LoadApplyItems extracts decision=APPLY items from the reviewed plan CSVs (rows first,
// then heads; heads may be nil). Rows are ordered by sysid then ln.
func LoadApplyItems(rows, heads *PlanCSV) ([]*ApplyItem, error) {
	if rows == nil {
		return nil, errors.New("plan-rows csv missing")
	}
	items := make([]*ApplyItem, 0, len(rows.Records))
	rowHdr := orderedHeader(rows)
	for i := range rows.Records {
		if Decision(strings.TrimSpace(rows.Get(i, "decision"))) != DecisionApply {
			continue
		}
		items = append(items, parseApplyRow(rows, rowHdr, i))
	}
	sort.SliceStable(items, func(a, b int) bool {
		if items[a].SysID != items[b].SysID {
			return items[a].SysID < items[b].SysID
		}
		return items[a].Ln < items[b].Ln
	})
	if heads != nil {
		hdr := orderedHeader(heads)
		for i := range heads.Records {
			if Decision(strings.TrimSpace(heads.Get(i, "decision"))) != DecisionApply {
				continue
			}
			g := func(c string) string { return strings.TrimSpace(heads.Get(i, c)) }
			items = append(items, &ApplyItem{
				Kind: applyKindHead, Key: g("item_key"), Action: g("kind"), Phase: Phase(g("phase")),
				SysID: g("old_sysid"), Header: hdr, Record: heads.Records[i],
			})
		}
	}
	return items, nil
}

func orderedHeader(c *PlanCSV) []string {
	h := make([]string, len(c.Index))
	for name, i := range c.Index {
		if i < len(h) {
			h[i] = name
		}
	}
	return h
}

func parseApplyRow(c *PlanCSV, hdr []string, i int) *ApplyItem {
	g := func(col string) string { return strings.TrimSpace(c.Get(i, col)) }
	it := &ApplyItem{
		Kind: applyKindRow, Key: g("item_key"), SysID: g("sysid"), Action: g("action"),
		Phase: Phase(g("phase")), Header: hdr, Record: c.Records[i],
	}
	ln, err := strconv.Atoi(g("ln"))
	if err != nil {
		it.bad = fmt.Sprintf("bad ln %q", g("ln"))
		return it
	}
	it.Ln = ln
	if it.Key == "" {
		it.Key = "row:" + strconv.Itoa(ln)
	}
	it.Source, it.Role = g("source_spin_id"), g("role")
	it.HeadID = parseOptionalUUID(g("head_id"))
	if err := parseCloneSpec(it, g("target_den"), g("target_fil")); err != nil {
		it.bad = err.Error()
		return it
	}
	in := SyncSpinInput{TargetShadeCode: g("target_shade_code"), TargetShadeName: g("target_shade_name"), TargetCS: g("target_cs")}
	if s := g("sub_actions"); s != "" {
		for _, p := range strings.Split(s, sep) {
			if p = strings.TrimSpace(p); p != "" {
				in.SubActions = append(in.SubActions, p)
			}
		}
	}
	if sp := g("spin_id"); sp != "" {
		id, perr := uuid.Parse(sp)
		if perr != nil {
			it.bad = fmt.Sprintf("bad spin_id %q", sp)
			return it
		}
		in.SpinID = id
	}
	if v := g("target_fg"); v != "" {
		fg, perr := strconv.ParseInt(v, 10, 64)
		if perr != nil {
			it.bad = fmt.Sprintf("bad target_fg %q", v)
			return it
		}
		in.TargetFG = &fg
	}
	if v := g("target_ldr"); v != "" {
		f, perr := strconv.ParseFloat(v, 64)
		if perr != nil {
			it.bad = fmt.Sprintf("bad target_ldr %q", v)
			return it
		}
		in.TargetLDR = &f
	}
	it.Spin = in
	return it
}

// parseOptionalUUID returns uuid.Nil for an empty or unparsable value.
func parseOptionalUUID(v string) uuid.UUID {
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil
	}
	return id
}

func parseCloneSpec(it *ApplyItem, den, fil string) error {
	if den != "" {
		v, err := strconv.ParseFloat(den, 64)
		if err != nil {
			return fmt.Errorf("bad target_den %q", den)
		}
		it.TargetDen = &v
	}
	if fil != "" {
		v, err := strconv.Atoi(fil)
		if err != nil {
			return fmt.Errorf("bad target_fil %q", fil)
		}
		it.TargetFil = &v
	}
	return nil
}

// SelectItems applies the phase / only-sysid filters and keeps the dispatch order
// (phase A: rows by sysid, ln). The limit is applied by Run (it counts processed items).
func SelectItems(items []*ApplyItem, opts ApplyOptions) []*ApplyItem {
	only := map[string]bool{}
	for _, s := range opts.OnlySysID {
		if s = strings.TrimSpace(s); s != "" {
			only[s] = true
		}
	}
	out := make([]*ApplyItem, 0, len(items))
	for _, it := range items {
		if it.Phase != opts.Phase {
			continue
		}
		if len(only) > 0 && !only[it.SysID] {
			continue
		}
		out = append(out, it)
	}
	switch opts.Phase {
	case PhaseB:
		sort.SliceStable(out, func(a, b int) bool { return phaseBRank(out[a]) < phaseBRank(out[b]) })
	case PhaseC:
		sort.SliceStable(out, func(a, b int) bool { return phaseCRank(out[a]) < phaseCRank(out[b]) })
	case PhaseD:
		sort.SliceStable(out, func(a, b int) bool { return phaseDRank(out[a]) < phaseDRank(out[b]) })
	case PhaseA, PhaseNone:
	}
	return out
}

// phaseBRank orders phase B items: SUPERSEDE (repoints spins) first, then DEACTIVATE_SPIN,
// SUPERSEDE_HEAD (needs the old head without live spins) and DEACTIVATE_HEAD last (its
// has_refs guard needs every spin already superseded or deactivated). Equal ranks keep
// plan order (sort.SliceStable).
func phaseBRank(it *ApplyItem) int {
	if it.Kind != applyKindHead {
		return phaseBRankOther
	}
	switch HeadKind(it.Action) {
	case KindSupersede:
		return 0
	case KindDeactivateSpin:
		return 1
	case KindSupersedeHead:
		return 2
	case KindDeactivateHead:
		return 3
	default:
		return phaseBRankOther
	}
}

const phaseBRankOther = 4

// Phase D ranks: REACTIVATE rows first (they make the new spin and its head live; ACT_HEAD
// reactivates the head without cascade), then SUPERSEDE (old spin -> reactivated spin) and
// SUPERSEDE_HEAD (Q15, new head must be live). Equal ranks keep plan order.
const (
	phaseDRankRow = iota
	phaseDRankSupersede
	phaseDRankSupersedeHead
	phaseDRankOther
)

func phaseDRank(it *ApplyItem) int {
	if it.Kind != applyKindHead {
		return phaseDRankRow
	}
	switch HeadKind(it.Action) {
	case KindSupersede:
		return phaseDRankSupersede
	case KindSupersedeHead:
		return phaseDRankSupersedeHead
	default:
		return phaseDRankOther
	}
}

// Phase C ranks: ROOT rows create/validate the first spin of a head, DUP rows clone a
// freshly created root (source ln:<root>), DUPLICATE rows clone existing roots, then the
// SUPERSEDE pairs (new_spin_ref ln:<n> needs the spin of an earlier row) and finally
// SUPERSEDE_HEAD. Equal ranks keep plan order (rows by sysid, ln; then heads).
const (
	phaseCRankRoot = iota
	phaseCRankDup
	phaseCRankDuplicate
	phaseCRankSupersede
	phaseCRankSupersedeHead
	phaseCRankOther
)

func phaseCRank(it *ApplyItem) int {
	if it.Kind == applyKindHead {
		switch HeadKind(it.Action) {
		case KindSupersede:
			return phaseCRankSupersede
		case KindSupersedeHead:
			return phaseCRankSupersedeHead
		default:
			return phaseCRankOther
		}
	}
	switch {
	case isRootRow(it):
		return phaseCRankRoot
	case isCloneRow(it) && Action(it.Action) == ActionGenerateSpin:
		return phaseCRankDup
	case isCloneRow(it):
		return phaseCRankDuplicate
	}
	return phaseCRankOther
}

// isRootRow reports a phase C GENERATE_SPIN row that builds the first spin of a head
// (role ROOT, or an older plan without role and without an ln: source).
func isRootRow(it *ApplyItem) bool {
	return it.Kind == applyKindRow && it.Phase == PhaseC && Action(it.Action) == ActionGenerateSpin && !isCloneRow(it)
}

// Run applies the selected items in order, one writer call each. Continue-on-error:
// a FAILED item does not stop the run. A cancelled ctx, a sink error or the limit stops
// it (remaining items are not processed).
func (a *Applier) Run(ctx context.Context, items []*ApplyItem, opts ApplyOptions, sink ResultSink) (*ApplySummary, error) {
	if a == nil || a.w == nil {
		return nil, errors.New("applier has no writer")
	}
	if sink == nil {
		return nil, errors.New("applier has no result sink")
	}
	if strings.TrimSpace(opts.Actor) == "" {
		return nil, errors.New("actor must not be empty")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	logf := opts.Logf
	if logf == nil {
		logf = func(string, ...any) {}
	}
	sel := SelectItems(items, opts)
	st := newRunState(opts.PrevClones, opts.PrevRoots)
	st.indexPlan(items)
	sum := &ApplySummary{Selected: len(sel), ByResult: map[string]int{}, ByReason: map[string]int{}}
	batch := opts.Batch
	if batch <= 0 {
		batch = len(sel)
	}
	if batch == 0 {
		return sum, nil
	}
	nBatches := (len(sel) + batch - 1) / batch
	start := now()
	for b := 0; b < nBatches; b++ {
		lo, hi := b*batch, min((b+1)*batch, len(sel))
		var c batchCounts
		stop, err := a.runBatch(ctx, sel[lo:hi], opts, now, sink, sum, &c, st)
		logf("batch %d/%d: applied %d skipped %d failed %d (elapsed %s)", b+1, nBatches, c.applied, c.skipped, c.failed, now().Sub(start).Round(time.Millisecond))
		if stop {
			return sum, err
		}
	}
	return sum, nil
}

type batchCounts struct{ applied, skipped, failed int }

// runBatch processes one batch; stop=true ends the whole run (err may be nil for the limit).
func (a *Applier) runBatch(ctx context.Context, items []*ApplyItem, opts ApplyOptions, now func() time.Time, sink ResultSink, sum *ApplySummary, c *batchCounts, st *runState) (stop bool, err error) {
	for _, it := range items {
		if cerr := ctx.Err(); cerr != nil {
			sum.Stopped = "cancelled"
			return true, fmt.Errorf("apply stopped: %w", cerr)
		}
		if opts.Limit > 0 && sum.Processed >= opts.Limit {
			sum.Stopped = "limit"
			return true, nil
		}
		out, perr := a.applyOne(ctx, it, opts.Actor, now, sink, st)
		if perr != nil {
			sum.Stopped = StoppedSinkError
			return true, fmt.Errorf("record partial result for %s: %w", it.Key, perr)
		}
		sum.Processed++
		sum.ByResult[out.Result.Result]++
		if out.Result.Reason != "" {
			sum.ByReason[out.Result.Reason]++
		}
		switch out.Result.Result {
		case ResultApplied, ResultPartial:
			c.applied++
		case ResultSkipped:
			c.skipped++
		default:
			c.failed++
		}
		if serr := sink.Record(out); serr != nil {
			sum.Stopped = StoppedSinkError
			return true, fmt.Errorf("record result for %s: %w", it.Key, serr)
		}
	}
	return false, nil
}

// applyOne dispatches one item. Clone rows flush their PARTIAL result (tx 1 done)
// through sink before tx 2; a sink error there is returned as perr (run stops).
func (a *Applier) applyOne(ctx context.Context, it *ApplyItem, actor string, now func() time.Time, sink ResultSink, st *runState) (out ApplyOutcome, perr error) {
	if isCloneRow(it) && it.bad == "" {
		onCloned := func(r ItemResult) error {
			perr = sink.Record(ApplyOutcome{Item: it, Result: r, Actor: actor, AppliedAt: now()})
			return perr
		}
		out = a.dispatchClone(ctx, it, actor, st, onCloned)
	} else {
		out = a.dispatch(ctx, it, actor, st)
	}
	out.AppliedAt = now()
	return out, perr
}

func (a *Applier) dispatch(ctx context.Context, it *ApplyItem, actor string, st *runState) ApplyOutcome {
	out := ApplyOutcome{Item: it, Actor: actor}
	if it.bad != "" {
		out.Result = ItemResult{Result: ResultFailed, Reason: ReasonBadPlanItem, Message: it.bad}
		out.Err = it.bad
		return out
	}
	if it.Kind == applyKindHead && (it.Phase == PhaseB || it.Phase == PhaseC || it.Phase == PhaseD) {
		if o, ok := a.dispatchHead(ctx, it, actor, out, st); ok {
			return o
		}
	}
	if isRootRow(it) {
		return a.dispatchRoot(ctx, it, actor, out, st)
	}
	if it.Kind != applyKindRow || !isSyncRowAction(it.Action) {
		out.Result = ItemResult{Result: ResultSkipped, Reason: ReasonNotWired, Message: fmt.Sprintf("%s %q is not wired in this build", it.Kind, it.Action)}
		return out
	}
	if it.Spin.SpinID == uuid.Nil {
		out.Result = ItemResult{Result: ResultFailed, Reason: ReasonBadPlanItem, Message: "row has no spin_id"}
		out.Err = out.Result.Message
		return out
	}
	in := it.Spin
	in.Actor = actor
	res, err := a.w.SyncExistingSpin(ctx, in)
	return finishOutcome(out, res, err)
}

// finishOutcome normalizes a writer result: DB error -> FAILED(db_error), empty result ->
// FAILED, and fills Err for every FAILED outcome.
func finishOutcome(out ApplyOutcome, res ItemResult, err error) ApplyOutcome {
	if err != nil {
		res.Result = ResultFailed
		if res.Reason == "" {
			res.Reason = ReasonDBError
		}
		if res.Message == "" {
			res.Message = err.Error()
		}
		out.Err = err.Error()
	} else if res.Result == "" {
		res.Result = ResultFailed
		res.Reason = ReasonDBError
		res.Message = "writer returned empty result"
	}
	if res.Result == ResultFailed && out.Err == "" {
		out.Err = res.Reason
		if res.Message != "" {
			out.Err += ": " + res.Message
		}
	}
	if res.Result == ResultFailed && res.Reason == ReasonValidateGate && res.ManualReason == "" {
		res.ManualReason = ReasonValidateGate // handled in the UI (manual-items.csv)
	}
	out.Result = res
	return out
}

func badItem(out ApplyOutcome, msg string) ApplyOutcome {
	out.Result = ItemResult{Result: ResultFailed, Reason: ReasonBadPlanItem, Message: msg}
	out.Err = msg
	return out
}

// col returns the trimmed plan value of column name on a head item ("" when absent).
func (it *ApplyItem) col(name string) string {
	for i, h := range it.Header {
		if h == name && i < len(it.Record) {
			return strings.TrimSpace(it.Record[i])
		}
	}
	return ""
}

// uuidCol parses a UUID column; required columns must be non-empty, optional ones may be empty (uuid.Nil).
func (it *ApplyItem) uuidCol(name string, required bool) (uuid.UUID, error) {
	v := it.col(name)
	if v == "" {
		if required {
			return uuid.Nil, fmt.Errorf("%s is empty", name)
		}
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(v)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%s %q is not a uuid", name, v)
	}
	return id, nil
}

// dispatchHead routes a phase B/C head-plan item to its Writer method. ok=false means the
// kind is not a wired action (REPORT_*, unknown): the caller reports SKIPPED not_wired.
func (a *Applier) dispatchHead(ctx context.Context, it *ApplyItem, actor string, out ApplyOutcome, st *runState) (ApplyOutcome, bool) {
	kind := HeadKind(it.Action)
	if (it.Phase == PhaseC || it.Phase == PhaseD) && kind != KindSupersede && kind != KindSupersedeHead {
		return out, false
	}
	switch kind {
	case KindSupersede:
		return a.dispatchSupersede(ctx, it, actor, out, st), true
	case KindSupersedeHead:
		return a.dispatchSupersedeHead(ctx, it, actor, out), true
	case KindDeactivateSpin, KindDeactivateHead:
		return a.dispatchDeactivate(ctx, it, kind, actor, out), true
	case KindReportHead, KindReportSpin:
		return out, false
	}
	return out, false
}

func (a *Applier) dispatchSupersede(ctx context.Context, it *ApplyItem, actor string, out ApplyOutcome, st *runState) ApplyOutcome {
	oldSpin, err := it.uuidCol("old_spin_id", true)
	if err != nil {
		return badItem(out, err.Error())
	}
	oldHead, err := it.uuidCol("old_head_id", false)
	if err != nil {
		return badItem(out, err.Error())
	}
	newSpin, skip, perr := st.resolveNewSpin(it.col("new_spin_ref"))
	if perr != nil {
		return badItem(out, perr.Error())
	}
	if skip {
		out.Result = ItemResult{Result: ResultSkipped, Reason: "new_spin_not_ready", Message: fmt.Sprintf("new_spin_ref %q has no existing spin yet", it.col("new_spin_ref"))}
		return out
	}
	res, werr := a.w.SupersedeExisting(ctx, SupersedeInput{OldHeadID: oldHead, OldSpinID: oldSpin, NewSpinID: newSpin, Actor: actor})
	return finishOutcome(out, res, werr)
}

func (a *Applier) dispatchSupersedeHead(ctx context.Context, it *ApplyItem, actor string, out ApplyOutcome) ApplyOutcome {
	oldHead, err := it.uuidCol("old_head_id", true)
	if err != nil {
		return badItem(out, err.Error())
	}
	newHead, err := it.uuidCol("new_head_id", true)
	if err != nil {
		return badItem(out, err.Error())
	}
	res, werr := a.w.SupersedeHeadOnly(ctx, SupersedeHeadInput{OldHeadID: oldHead, NewHeadID: newHead, Actor: actor})
	return finishOutcome(out, res, werr)
}

func (a *Applier) dispatchDeactivate(ctx context.Context, it *ApplyItem, kind HeadKind, actor string, out ApplyOutcome) ApplyOutcome {
	if kind == KindDeactivateSpin {
		spin, err := it.uuidCol("old_spin_id", true)
		if err != nil {
			return badItem(out, err.Error())
		}
		res, werr := a.w.DeactivateSpin(ctx, DeactivateSpinInput{SpinID: spin, Actor: actor})
		return finishOutcome(out, res, werr)
	}
	head, err := it.uuidCol("old_head_id", true)
	if err != nil {
		return badItem(out, err.Error())
	}
	res, werr := a.w.DeactivateHead(ctx, DeactivateHeadInput{HeadID: head, Actor: actor})
	return finishOutcome(out, res, werr)
}

// isSyncRowAction reports the plan rows moved by Writer.SyncExistingSpin (phase A rows and
// phase D REACTIVATE rows).
func isSyncRowAction(a string) bool {
	act := Action(a)
	return act == ActionKeep || act == ActionUpdateLDR || act == ActionUpdateShade || act == ActionReactivate
}
