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
	bad    string        // non-empty: parse problem, item is reported FAILED(bad_plan_item)
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
	w Writer
}

// NewApplier returns an Applier over w.
func NewApplier(w Writer) *Applier { return &Applier{w: w} }

// LoadApplyItems extracts decision=APPLY items from the reviewed plan CSVs (rows first,
// then heads; heads may be nil). Rows are ordered by sysid then ln.
func LoadApplyItems(rows, heads *PlanCSV) ([]*ApplyItem, error) {
	if rows == nil {
		return nil, errors.New("plan-rows csv missing")
	}
	var items []*ApplyItem
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

// SelectItems applies the phase / only-sysid filters and keeps the dispatch order
// (phase A: rows by sysid, ln). The limit is applied by Run (it counts processed items).
func SelectItems(items []*ApplyItem, opts ApplyOptions) []*ApplyItem {
	only := map[string]bool{}
	for _, s := range opts.OnlySysID {
		if s = strings.TrimSpace(s); s != "" {
			only[s] = true
		}
	}
	var out []*ApplyItem
	for _, it := range items {
		if it.Phase != opts.Phase {
			continue
		}
		if len(only) > 0 && !only[it.SysID] {
			continue
		}
		out = append(out, it)
	}
	return out
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
		stop, err := a.runBatch(ctx, sel[lo:hi], opts, now, sink, sum, &c)
		logf("batch %d/%d: applied %d skipped %d failed %d (elapsed %s)", b+1, nBatches, c.applied, c.skipped, c.failed, now().Sub(start).Round(time.Millisecond))
		if stop {
			return sum, err
		}
	}
	return sum, nil
}

type batchCounts struct{ applied, skipped, failed int }

// runBatch processes one batch; stop=true ends the whole run (err may be nil for the limit).
func (a *Applier) runBatch(ctx context.Context, items []*ApplyItem, opts ApplyOptions, now func() time.Time, sink ResultSink, sum *ApplySummary, c *batchCounts) (stop bool, err error) {
	for _, it := range items {
		if cerr := ctx.Err(); cerr != nil {
			sum.Stopped = "cancelled"
			return true, fmt.Errorf("apply stopped: %w", cerr)
		}
		if opts.Limit > 0 && sum.Processed >= opts.Limit {
			sum.Stopped = "limit"
			return true, nil
		}
		out := a.applyOne(ctx, it, opts.Actor, now)
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
			sum.Stopped = "sink_error"
			return true, fmt.Errorf("record result for %s: %w", it.Key, serr)
		}
	}
	return false, nil
}

func (a *Applier) applyOne(ctx context.Context, it *ApplyItem, actor string, now func() time.Time) ApplyOutcome {
	out := a.dispatch(ctx, it, actor)
	out.AppliedAt = now()
	return out
}

func (a *Applier) dispatch(ctx context.Context, it *ApplyItem, actor string) ApplyOutcome {
	out := ApplyOutcome{Item: it, Actor: actor}
	if it.bad != "" {
		out.Result = ItemResult{Result: ResultFailed, Reason: ReasonBadPlanItem, Message: it.bad}
		out.Err = it.bad
		return out
	}
	if it.Kind != applyKindRow || !isPhaseARowAction(it.Action) {
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
	out.Result = res
	return out
}

func isPhaseARowAction(a string) bool {
	act := Action(a)
	return act == ActionKeep || act == ActionUpdateLDR || act == ActionUpdateShade
}
