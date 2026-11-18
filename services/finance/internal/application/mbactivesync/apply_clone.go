package mbactivesync

import (
	"context"
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
	prev    map[int]uuid.UUID // ln -> clone from --prev-results
	done    map[int]uuid.UUID // ln -> clone/spin resolved in this run
	claimed map[uuid.UUID]int // clone -> ln owning it
}

func newRunState(prev map[int]uuid.UUID) *runState {
	st := &runState{prev: map[int]uuid.UUID{}, done: map[int]uuid.UUID{}, claimed: map[uuid.UUID]int{}}
	for ln, id := range prev {
		if id == uuid.Nil {
			continue
		}
		st.prev[ln] = id
		st.claimed[id] = ln
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
		if id, ok := st.done[n]; ok {
			return id, true, nil
		}
		if id, ok := st.prev[n]; ok {
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
