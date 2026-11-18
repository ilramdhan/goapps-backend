package mbactivesync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
)

// ReasonSourceNotReady marks a GENERATE_SPIN role=DUP row whose source is the spin of
// another row (ln:<n>) that has not been created yet — rerun after that row.
const ReasonSourceNotReady = "source_not_ready"

// runState carries the clone claims of one apply run (design part 2 §5.4): clones
// recorded by earlier runs (--prev-results) and clones resolved in this run, so two
// rows sharing (source, den, fil) never claim the same clone.
type runState struct {
	prev    map[int]uuid.UUID  // ln -> clone from --prev-results
	done    map[int]uuid.UUID  // ln -> clone/spin resolved in this run
	claimed map[uuid.UUID]int  // clone -> ln owning it
	roots   map[int]uuid.UUID  // ln -> ROOT spin from --prev-results
	static  map[int]uuid.UUID  // ln -> spin_id column of plan rows
	confirm map[uuid.UUID]bool // head -> validate with confirmSupersede
}

func newRunState(prev map[int]uuid.UUID, roots map[int]uuid.UUID) *runState {
	st := &runState{prev: map[int]uuid.UUID{}, done: map[int]uuid.UUID{}, roots: map[int]uuid.UUID{}, claimed: map[uuid.UUID]int{}, static: map[int]uuid.UUID{}, confirm: map[uuid.UUID]bool{}}
	for ln, id := range prev {
		if id == uuid.Nil {
			continue
		}
		st.prev[ln] = id
		st.claimed[id] = ln
	}
	for ln, id := range roots { // root spins are resolvable but never claimed as clones
		if _, ok := st.prev[ln]; !ok && id != uuid.Nil {
			st.roots[ln] = id
		}
	}
	return st
}

// excludeFor returns the clones claimed by rows other than ln, sorted for determinism.
func (st *runState) excludeFor(ln int) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(st.claimed))
	for id, owner := range st.claimed {
		if owner != ln {
			out = append(out, id)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].String() < out[j].String() })
	return out
}

func (st *runState) record(ln int, id uuid.UUID) {
	if id == uuid.Nil {
		return
	}
	st.done[ln] = id
	st.claimed[id] = ln
}

// isCloneRow reports a phase C row created as a clone of a root spin:
// DUPLICATE_SPIN, or GENERATE_SPIN with role DUP.
func isCloneRow(it *ApplyItem) bool {
	if it.Kind != applyKindRow || it.Phase != PhaseC {
		return false
	}
	switch Action(it.Action) {
	case ActionDuplicateSpin:
		return true
	case ActionGenerateSpin:
		// role column is optional in older reviewed plans: a DUP row's source is "ln:<root>".
		return Role(it.Role) == RoleDup || (it.Role == "" && strings.HasPrefix(it.Source, "ln:"))
	case ActionKeep, ActionUpdateLDR, ActionUpdateShade, ActionReactivate, ActionNewRecipe, ActionReportOnly:
		return false
	}
	return false
}

// resolveSource parses source_spin_id: "<uuid>" or "ln:<n>" (spin created by row n in
// this run or recorded by --prev-results). ready=false: that row has no spin yet.
func (st *runState) resolveSource(ref string) (id uuid.UUID, ready bool, err error) {
	if v, ok := strings.CutPrefix(ref, "ln:"); ok {
		n, perr := strconv.Atoi(strings.TrimSpace(v))
		if perr != nil || n <= 0 {
			return uuid.Nil, false, fmt.Errorf("bad source_spin_id %q", ref)
		}
		if id, ok := st.spinOfLn(n); ok {
			return id, true, nil
		}
		return uuid.Nil, false, nil
	}
	id, err = uuid.Parse(strings.TrimSpace(ref))
	if err != nil || id == uuid.Nil {
		return uuid.Nil, false, fmt.Errorf("bad source_spin_id %q", ref)
	}
	return id, true, nil
}

// dispatchClone runs a clone row through Writer.DuplicateToTarget and records the
// clone it ended up with (also a FAILED partial clone: no other row may claim it).
func (a *Applier) dispatchClone(ctx context.Context, it *ApplyItem, actor string, st *runState, onCloned func(ItemResult) error) ApplyOutcome {
	out := ApplyOutcome{Item: it, Actor: actor}
	src, ready, err := st.resolveSource(it.Source)
	if err != nil {
		return badItem(out, err.Error())
	}
	if !ready {
		out.Result = ItemResult{Result: ResultSkipped, Reason: ReasonSourceNotReady,
			Message: fmt.Sprintf("source %q has no spin yet; rerun after it", it.Source)}
		return out
	}
	target := it.Spin
	target.Actor = actor
	target.SpinID = uuid.Nil
	in := DuplicateTargetInput{
		Ln: it.Ln, SourceSpinID: src, CloneSpinID: st.prev[it.Ln], ExcludeClones: st.excludeFor(it.Ln),
		TargetDen: it.TargetDen, TargetFil: it.TargetFil, Target: target, OnCloned: onCloned,
	}
	res, werr := a.w.DuplicateToTarget(ctx, in)
	if id, perr := uuid.Parse(res.CloneSpinID); perr == nil {
		st.record(it.Ln, id)
	}
	return finishOutcome(out, res, werr)
}

// LoadPrevClones reads earlier result CSVs (--prev-results) into ln -> clone_spin_id.
// Rows without ln or clone_spin_id are ignored; a later file/row wins. Two different
// clones for the same ln are an error (the operator must pick one).
func LoadPrevClones(readers ...io.Reader) (map[int]uuid.UUID, error) {
	out := map[int]uuid.UUID{}
	for fi, r := range readers {
		c, err := ReadPlanCSV(r, []string{"ln", "clone_spin_id"})
		if err != nil {
			return nil, fmt.Errorf("prev-results #%d: %w", fi+1, err)
		}
		for i := range c.Records {
			lnS, idS := strings.TrimSpace(c.Get(i, "ln")), strings.TrimSpace(c.Get(i, "clone_spin_id"))
			if lnS == "" || idS == "" {
				continue
			}
			ln, perr := strconv.Atoi(lnS)
			if perr != nil {
				return nil, fmt.Errorf("prev-results #%d record %d: bad ln %q", fi+1, i+1, lnS)
			}
			id, perr := uuid.Parse(idS)
			if perr != nil {
				return nil, fmt.Errorf("prev-results #%d record %d: bad clone_spin_id %q", fi+1, i+1, idS)
			}
			if old, ok := out[ln]; ok && old != id {
				return nil, fmt.Errorf("prev-results: ln %d has two clones (%s, %s)", ln, old, id)
			}
			out[ln] = id
		}
	}
	return out, nil
}

// LoadPrevRoots reads earlier result CSVs into ln -> spin_id for GENERATE_SPIN rows that
// ended APPLIED or SKIPPED already_in_target (the ROOT spins), so a rerun without the
// ROOT row still resolves ln:<root>. Clone claims stay separate (LoadPrevClones).
func LoadPrevRoots(readers ...io.Reader) (map[int]uuid.UUID, error) {
	out := map[int]uuid.UUID{}
	for fi, r := range readers {
		c, err := ReadPlanCSV(r, []string{"ln"})
		if err != nil {
			return nil, fmt.Errorf("prev-results #%d: %w", fi+1, err)
		}
		for i := range c.Records {
			g := func(col string) string { return strings.TrimSpace(c.Get(i, col)) }
			if Action(g("action")) != ActionGenerateSpin || g("spin_id") == "" || g("clone_spin_id") != "" {
				continue
			}
			if res := g("result"); res != ResultApplied && (res != ResultSkipped || g("reason") != ReasonAlreadyInTarget) {
				continue
			}
			ln, perr := strconv.Atoi(g("ln"))
			id, uerr := uuid.Parse(g("spin_id"))
			if perr != nil || uerr != nil {
				return nil, fmt.Errorf("prev-results #%d record %d: bad ln/spin_id", fi+1, i+1)
			}
			out[ln] = id
		}
	}
	return out, nil
}

// indexPlan records, from the whole reviewed plan (not only the selected items), the
// spin of every row that already names one (spin_id) and the heads whose phase C ROOT
// row is paired with a phase C SUPERSEDE (new_spin_ref ln:<n>): those validate with
// confirmSupersede=true (design part 2 §5.5).
func (st *runState) indexPlan(items []*ApplyItem) {
	st.static = map[int]uuid.UUID{}
	st.confirm = map[uuid.UUID]bool{}
	headOfLn := map[int]uuid.UUID{}
	for _, it := range items {
		if it.Kind != applyKindRow || it.bad != "" {
			continue
		}
		if it.Spin.SpinID != uuid.Nil {
			st.static[it.Ln] = it.Spin.SpinID
		}
		if it.HeadID != uuid.Nil {
			headOfLn[it.Ln] = it.HeadID
		}
	}
	for _, it := range items {
		if it.Kind != applyKindHead || it.Phase != PhaseC || HeadKind(it.Action) != KindSupersede {
			continue
		}
		v, ok := strings.CutPrefix(it.col("new_spin_ref"), "ln:")
		if !ok {
			continue
		}
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			if h := headOfLn[n]; h != uuid.Nil {
				st.confirm[h] = true
			}
		}
	}
}

// spinOfLn returns the spin produced/recorded for row ln: this run, --prev-results,
// then the row's own spin_id column.
func (st *runState) spinOfLn(n int) (uuid.UUID, bool) {
	if id, ok := st.done[n]; ok {
		return id, true
	}
	if id, ok := st.prev[n]; ok {
		return id, true
	}
	if id, ok := st.roots[n]; ok {
		return id, true
	}
	if id, ok := st.static[n]; ok {
		return id, true
	}
	return uuid.Nil, false
}

// resolveNewSpin parses new_spin_ref: "spin:<uuid>" -> the spin id; empty or "ln:<n>"
// whose spin does not exist yet -> skip=true; "ln:<n>" with a spin -> that spin;
// anything else is a parse error.
func (st *runState) resolveNewSpin(ref string) (id uuid.UUID, skip bool, err error) {
	if v, found := strings.CutPrefix(ref, "spin:"); found {
		id, err = uuid.Parse(strings.TrimSpace(v))
		if err != nil || id == uuid.Nil {
			return uuid.Nil, false, fmt.Errorf("new_spin_ref %q is not spin:<uuid>", ref)
		}
		return id, false, nil
	}
	if v, found := strings.CutPrefix(ref, "ln:"); found {
		n, perr := strconv.Atoi(strings.TrimSpace(v))
		if perr != nil || n <= 0 {
			return uuid.Nil, false, fmt.Errorf("new_spin_ref %q is not ln:<n>", ref)
		}
		if id, ok := st.spinOfLn(n); ok {
			return id, false, nil
		}
		return uuid.Nil, true, nil
	}
	if ref == "" {
		return uuid.Nil, true, nil
	}
	return uuid.Nil, false, fmt.Errorf("new_spin_ref %q is not spin:<uuid> or ln:<n>", ref)
}

// dispatchRoot runs a phase C GENERATE_SPIN role=ROOT row (design part 2 §5.5):
// EnsureValidated (Submit→Validate: creates cost product + root spin) and, for a head
// that already has its cost product but no spin, Writer.GenerateRootSpin (K7); then
// SyncExistingSpin moves the root spin to the row's target state. The root spin is
// recorded for ln so DUP rows (source ln:<root>) and SUPERSEDE refs resolve it.
func (a *Applier) dispatchRoot(ctx context.Context, it *ApplyItem, actor string, out ApplyOutcome, st *runState) ApplyOutcome {
	if it.HeadID == uuid.Nil {
		return badItem(out, "ROOT row has no valid head_id")
	}
	if a.hw == nil {
		out.Result = ItemResult{Result: ResultSkipped, Reason: ReasonNotWired, Message: "head workflow is not wired in this build"}
		return out
	}
	gen, gerr := a.hw.EnsureValidated(ctx, it.HeadID, actor, st.confirm[it.HeadID])
	if gerr != nil {
		return finishOutcome(out, gen, gerr)
	}
	gen, rootID, done := a.ensureRootSpin(ctx, it, actor, gen, out)
	if done != nil {
		return *done
	}
	in := it.Spin
	in.Actor, in.SpinID = actor, rootID
	in.TargetDen, in.TargetFil = it.TargetDen, it.TargetFil // move the generated root from the head spec to the row spec (P2h-4c)
	res, serr := a.w.SyncExistingSpin(ctx, in)
	res = finishRoot(it, st, rootID, gen, res, serr)
	return finishOutcome(out, res, serr)
}

// ensureRootSpin resolves the root spin id after EnsureValidated (K7 fallback: cost
// product exists, no spin yet -> GenerateRootSpin). A non-nil done is the final outcome.
func (a *Applier) ensureRootSpin(ctx context.Context, it *ApplyItem, actor string, gen ItemResult, out ApplyOutcome) (ItemResult, uuid.UUID, *ApplyOutcome) {
	finish := func(o ApplyOutcome) (ItemResult, uuid.UUID, *ApplyOutcome) { return gen, uuid.Nil, &o }
	rootID, perr := parseSpinID(gen.SpinID)
	if perr != nil {
		return finish(finishOutcome(out, ItemResult{Result: ResultFailed, Reason: ReasonDBError, Message: perr.Error()}, nil))
	}
	switch {
	case gen.Result == ResultApplied:
	case gen.Result == ResultSkipped && gen.Reason == ReasonAlreadyInTarget:
		if rootID == uuid.Nil {
			first := gen
			var gerr error
			gen, gerr = a.w.GenerateRootSpin(ctx, GenerateRootInput{HeadID: it.HeadID, Actor: actor})
			if gerr != nil || gen.Result == ResultFailed || gen.Result == "" {
				return finish(finishOutcome(out, gen, gerr))
			}
			gen = mergeGen(first, gen, ItemResult{})
			if rootID, perr = parseSpinID(gen.SpinID); perr != nil {
				return finish(finishOutcome(out, ItemResult{Result: ResultFailed, Reason: ReasonDBError, Message: perr.Error()}, nil))
			}
		}
	default:
		return finish(finishOutcome(out, gen, nil))
	}
	if rootID == uuid.Nil {
		return finish(badItem(out, "root spin id missing after generate"))
	}
	return gen, rootID, nil
}

// parseSpinID parses a writer-returned spin id: empty is uuid.Nil, garbage is an error.
func parseSpinID(s string) (uuid.UUID, error) {
	if s == "" {
		return uuid.Nil, nil
	}
	id, err := uuid.Parse(s)
	if err != nil {
		return uuid.Nil, fmt.Errorf("writer returned bad spin id %q", s)
	}
	return id, nil
}

// finishRoot patches the SyncExistingSpin result of a root row: records the root spin,
// fills spin/head ids, turns an already_in_target sync after a fresh generate into
// APPLIED, and merges the generation image (supersede batch, deactivated heads...).
func finishRoot(it *ApplyItem, st *runState, rootID uuid.UUID, gen, res ItemResult, serr error) ItemResult {
	if res.SpinID == "" {
		res.SpinID = rootID.String()
	}
	if serr == nil && res.Result != ResultFailed && res.Result != "" {
		st.record(it.Ln, rootID)
		if res.HeadID == "" {
			res.HeadID = it.HeadID.String()
		}
		if res.Result == ResultSkipped && res.Reason == ReasonAlreadyInTarget && gen.Result == ResultApplied {
			res.Result, res.Reason = ResultApplied, ""
		}
	}
	return mergeGen(gen, ItemResult{}, res)
}

// mergeGen folds the generation step's images and message into the final result so
// result.csv keeps the supersede batch / deactivated heads (needed for rollback). With
// a zero final the second generation result (K7) is folded into the first instead.
func mergeGen(gen, gen2, final ItemResult) ItemResult {
	if final.Result == "" && gen2.Result != "" {
		gen2.BeforeJSON = combineJSON(gen.BeforeJSON, gen2.BeforeJSON, "ensure", "generate")
		gen2.AfterJSON = combineJSON(gen.AfterJSON, gen2.AfterJSON, "ensure", "generate")
		return gen2
	}
	if final.Result == "" {
		return gen
	}
	final.BeforeJSON = combineJSON(gen.BeforeJSON, final.BeforeJSON, "generate", "sync")
	final.AfterJSON = combineJSON(gen.AfterJSON, final.AfterJSON, "generate", "sync")
	if gen.Message != "" && gen.Message != final.Message {
		if final.Message == "" {
			final.Message = gen.Message
		} else {
			final.Message = gen.Message + "; " + final.Message
		}
	}
	return final
}

// combineJSON nests two JSON images under named keys; a single non-empty side is kept as is.
func combineJSON(a, b, ka, kb string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	raw := func(s string) json.RawMessage {
		if json.Valid([]byte(s)) {
			return json.RawMessage(s)
		}
		q, err := json.Marshal(s)
		if err != nil {
			return json.RawMessage(`null`)
		}
		return q
	}
	out, err := json.Marshal(map[string]json.RawMessage{ka: raw(a), kb: raw(b)})
	if err != nil {
		return b
	}
	return string(out)
}
