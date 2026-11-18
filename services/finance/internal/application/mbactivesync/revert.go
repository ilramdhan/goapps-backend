package mbactivesync

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
)

// RevertMessagePrefix marks the result rows written by --revert. LoadRevertItems skips
// them so a revert result file fed back as --input never reverts the revert itself.
const RevertMessagePrefix = "revert: "

// RevertItem is one APPLIED result.csv record selected for --revert (design part 2 §8).
type RevertItem struct {
	Key         string
	Ln          int
	SysID       string
	Action      string
	Phase       Phase
	SpinID      uuid.UUID
	CloneSpinID uuid.UUID
	HeadID      uuid.UUID
	BeforeJSON  string
	AfterJSON   string
	Header      []string
	Record      []string
	bad         string
}

// RevertRequiredColumns are the result.csv columns --revert needs.
var RevertRequiredColumns = []string{"item_key", "action", "phase", "result", "spin_id", "clone_spin_id", "head_id", "before_json", "after_json"}

// LoadRevertItems reads result CSVs (oldest first) and returns the APPLIED items in
// REVERSE application order (last applied item first): the before-image of a later item
// is the after-image of an earlier one on the same head/spin, so the order matters.
// PARTIAL/SKIPPED/FAILED rows and the rows of an earlier revert run are ignored.
func LoadRevertItems(readers ...io.Reader) ([]*RevertItem, error) {
	var items []*RevertItem
	for fi, r := range readers {
		c, err := ReadPlanCSV(r, RevertRequiredColumns)
		if err != nil {
			return nil, fmt.Errorf("--input #%d: %w", fi+1, err)
		}
		hdr := orderedHeader(c)
		for i := range c.Records {
			if it := parseRevertRecord(c, i, hdr); it != nil {
				items = append(items, it)
			}
		}
	}
	for l, r := 0, len(items)-1; l < r; l, r = l+1, r-1 {
		items[l], items[r] = items[r], items[l]
	}
	return items, nil
}

// parseRevertRecord builds the RevertItem of record i, or nil when it is not an APPLIED
// result row of the original run.
func parseRevertRecord(c *PlanCSV, i int, hdr []string) *RevertItem {
	g := func(col string) string { return strings.TrimSpace(c.Get(i, col)) }
	if g("result") != ResultApplied || strings.HasPrefix(c.Get(i, "message"), RevertMessagePrefix) {
		return nil
	}
	it := &RevertItem{
		Key: g("item_key"), SysID: g("sysid"), Action: g("action"), Phase: Phase(g("phase")),
		BeforeJSON: c.Get(i, "before_json"), AfterJSON: c.Get(i, "after_json"),
		Header: hdr, Record: c.Records[i],
	}
	if v := g("ln"); v != "" {
		ln, perr := strconv.Atoi(v)
		if perr != nil {
			it.bad = fmt.Sprintf("bad ln %q", v)
		}
		it.Ln = ln
	}
	for _, f := range []struct {
		col string
		dst *uuid.UUID
	}{{"spin_id", &it.SpinID}, {"clone_spin_id", &it.CloneSpinID}, {"head_id", &it.HeadID}} {
		if v := g(f.col); v != "" {
			id, perr := uuid.Parse(v)
			if perr != nil {
				it.bad = fmt.Sprintf("bad %s %q", f.col, v)
				continue
			}
			*f.dst = id
		}
	}
	return it
}

// RevertOptions are the --revert run filters.
type RevertOptions struct {
	Actor     string
	OnlySysID []string
	Limit     int
	Now       func() time.Time
	Logf      func(format string, args ...any)
}

// RunRevert reverts the items in order (one writer call each, continue-on-error) and
// records every outcome through sink with Phase/Action of the original item.
func (a *Applier) RunRevert(ctx context.Context, items []*RevertItem, opts RevertOptions, sink ResultSink) (*ApplySummary, error) {
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
	sel := selectRevertItems(items, opts.OnlySysID)
	sum := &ApplySummary{Selected: len(sel), ByResult: map[string]int{}, ByReason: map[string]int{}}
	for _, it := range sel {
		if cerr := ctx.Err(); cerr != nil {
			sum.Stopped = "cancelled"
			return sum, fmt.Errorf("revert stopped: %w", cerr)
		}
		if opts.Limit > 0 && sum.Processed >= opts.Limit {
			sum.Stopped = "limit"
			return sum, nil
		}
		out := a.revertOne(ctx, it, opts.Actor)
		out.AppliedAt = now()
		sum.Processed++
		sum.ByResult[out.Result.Result]++
		if out.Result.Reason != "" {
			sum.ByReason[out.Result.Reason]++
		}
		if serr := sink.Record(out); serr != nil {
			sum.Stopped = StoppedSinkError
			return sum, fmt.Errorf("record revert result for %s: %w", it.Key, serr)
		}
	}
	logf("revert: processed %d of %d item(s): %v", sum.Processed, sum.Selected, sum.ByResult)
	return sum, nil
}

// selectRevertItems keeps the items whose SysID is in only (all when only is empty).
func selectRevertItems(items []*RevertItem, onlySysID []string) []*RevertItem {
	only := map[string]bool{}
	for _, s := range onlySysID {
		if s = strings.TrimSpace(s); s != "" {
			only[s] = true
		}
	}
	var sel []*RevertItem
	for _, it := range items {
		if len(only) == 0 || only[it.SysID] {
			sel = append(sel, it)
		}
	}
	return sel
}

func (a *Applier) revertOne(ctx context.Context, it *RevertItem, actor string) ApplyOutcome {
	ai := &ApplyItem{Kind: applyKindRow, Key: it.Key, Ln: it.Ln, SysID: it.SysID, Action: it.Action, Phase: it.Phase, Header: it.Header, Record: it.Record}
	if HeadKind(it.Action) != "" && isHeadKind(HeadKind(it.Action)) {
		ai.Kind = applyKindHead
	}
	out := ApplyOutcome{Item: ai, Actor: actor}
	if it.bad != "" {
		return badItem(out, it.bad)
	}
	res, err := a.w.Revert(ctx, RevertInput{
		Key: it.Key, Action: it.Action, Phase: it.Phase, SpinID: it.SpinID, CloneSpinID: it.CloneSpinID, HeadID: it.HeadID,
		BeforeJSON: it.BeforeJSON, AfterJSON: it.AfterJSON, Actor: actor,
	})
	out = finishOutcome(out, res, err)
	if out.Result.Message == "" {
		out.Result.Message = RevertMessagePrefix + strings.ToLower(out.Result.Result)
	} else {
		out.Result.Message = RevertMessagePrefix + out.Result.Message
	}
	return out
}

func isHeadKind(k HeadKind) bool {
	switch k {
	case KindSupersede, KindSupersedeHead, KindDeactivateHead, KindDeactivateSpin, KindReportHead, KindReportSpin:
		return true
	}
	return false
}

// ManualPlanRecords lists the reviewed plan items that never get applied and need an
// operator decision (design §3.5/§4): NEW_RECIPE and REPORT_ONLY rows, REPORT_HEAD and
// REPORT_SPIN head items. Records match ManualItemsHeader.
func ManualPlanRecords(rows, heads *PlanCSV) [][]string {
	var out [][]string
	if rows != nil {
		for i := range rows.Records {
			g := func(c string) string { return strings.TrimSpace(rows.Get(i, c)) }
			act := Action(g("action"))
			if act != ActionNewRecipe && act != ActionReportOnly {
				continue
			}
			key := g("item_key")
			if key == "" {
				key = "row:" + g("ln")
			}
			msg := fmt.Sprintf("file_poy=%s file_shade=%s", g("file_poy"), g("file_shade"))
			if r := g("reason"); r != "" {
				msg += " reasons=" + r
			}
			out = append(out, []string{key, g("ln"), g("sysid"), string(act), g("phase"), strings.ToLower(string(act)), g("spin_id"), "", g("head_id"), msg})
		}
	}
	if heads != nil {
		for i := range heads.Records {
			g := func(c string) string { return strings.TrimSpace(heads.Get(i, c)) }
			k := HeadKind(g("kind"))
			if k != KindReportHead && k != KindReportSpin {
				continue
			}
			out = append(out, []string{g("item_key"), "", g("old_sysid"), string(k), g("phase"), strings.ToLower(string(k)), g("old_spin_id"), "", g("old_head_id"), g("reason")})
		}
	}
	return out
}
