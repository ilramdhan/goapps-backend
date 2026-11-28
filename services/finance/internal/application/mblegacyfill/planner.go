package mblegacyfill

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"strings"
)

// Decision is the plan outcome of one proposal row.
type Decision string

// Decisions.
const (
	DecisionApply   Decision = "APPLY"
	DecisionChanged Decision = "CHANGED"
	DecisionAlready Decision = "ALREADY"
	DecisionReject  Decision = "REJECT"
)

// ReasonNameFollowsCosting rejects head mgt_name rows: the name is derived from MB Costing.
const ReasonNameFollowsCosting = "mb_name_follows_costing"

// Snapshot is the DB state read for a proposal inside one read-only tx.
type Snapshot interface {
	// Current returns the stored text value of a column. found=false means the
	// entity row is missing or soft-deleted.
	Current(entity Entity, id, column string) (value string, found bool)
	// Holders returns the ids of live heads/spins whose column (a uniqueness
	// column, head or spin orion) currently equals value (trimmed compare).
	Holders(column, value string) []string
}

// Reader loads a Snapshot for the rows of a proposal (port).
type Reader interface {
	Load(ctx context.Context, rows []Row) (Snapshot, error)
}

// PlanRow is a proposal row with its plan decision.
type PlanRow struct {
	Row      Row
	Class    Class
	Decision Decision
	Reason   string
}

// Plan is the result of planning a proposal.
type Plan struct {
	Rows    []PlanRow
	Skipped map[string]int
	Total   int
}

// Spin orion (mbs_orion_item_code) is also unique among live spins: the lookup sync
// MB_SP_* resolves by that code, so no other live spin may carry it.
// uniqueFields are head fields that must stay unique among live heads (the same set
// the UI UpdateHandler checks). mb_costing is NOT unique since migration 000619: the
// MB name follows the Finance Excel, and two live heads may share it. mbh_code is a shade code shared by many heads, so it is
// not unique; its guard is the FG shade check in the writer.
var uniqueFields = map[string]bool{"orion": true, "vs": true, "dev": true}

func needsUniqueCheck(r Row) bool {
	if r.Entity == EntitySpin {
		return r.Field == "orion"
	}
	return r.Entity == EntityHead && uniqueFields[r.Field]
}

// uniqueExempt reports values that are legitimately shared (empty, vs '0').
func uniqueExempt(r Row, v string) bool {
	return v == "" || (r.Field == "vs" && v == "0")
}

// BuildPlan loads the snapshot and decides every row of the proposal.
func BuildPlan(ctx context.Context, rd Reader, p *Proposal) (*Plan, error) {
	snap, err := rd.Load(ctx, p.Rows)
	if err != nil {
		return nil, fmt.Errorf("load snapshot: %w", err)
	}
	plan := PlanFromSnapshot(snap, p)
	return plan, nil
}

// PlanFromSnapshot is the pure planner (no I/O).
func PlanFromSnapshot(snap Snapshot, p *Proposal) *Plan {
	plan := &Plan{Skipped: p.Skipped, Total: p.Total}
	for _, r := range p.Rows {
		plan.Rows = append(plan.Rows, decideRow(snap, r))
	}
	markDuplicateTargets(plan.Rows)
	for _, rj := range p.Rejected {
		plan.Rows = append(plan.Rows, PlanRow{Row: rj.Row, Class: ClassX, Decision: DecisionReject, Reason: rj.Reason})
	}
	return plan
}

func decideRow(snap Snapshot, r Row) PlanRow {
	pr := PlanRow{Row: r, Class: r.Spec.Class}
	reject := func(reason string) PlanRow {
		pr.Decision, pr.Reason = DecisionReject, reason
		return pr
	}
	cur, found := snap.Current(r.Entity, r.ID, r.Spec.Column)
	switch {
	case !found:
		return reject("entity not found or deleted")
	case r.Entity == EntityHead && r.Spec.Column == "mbh_mgt_name":
		// MB Name always equals MB Costing (mb-comparison-p2 item 3); the writer guard is a backstop.
		return reject(ReasonNameFollowsCosting)
	case ValidateApplyRow(r) != nil:
		return reject(ValidateApplyRow(r).Error())
	case Equal(r.Spec.Kind, cur, r.LegacyValue):
		pr.Decision, pr.Reason = DecisionAlready, "current value already equals legacy"
		return pr
	case !Equal(r.Spec.Kind, cur, r.GoappsValue):
		pr.Decision = DecisionChanged
		pr.Reason = fmt.Sprintf("value changed since proposal: current=%q proposal=%q", Normalize(cur), Normalize(r.GoappsValue))
		return pr
	}
	if needsUniqueCheck(r) {
		if why := uniqueClash(snap, r); why != "" {
			return reject(why)
		}
	}
	pr.Decision, pr.Reason = DecisionApply, "guard ok"
	return pr
}

// ValidateApplyRow runs the plan-time legacy-value validators (empty value, fractional
// value for an integer column). Shared by the planner and the plan-file reader.
func ValidateApplyRow(r Row) error {
	if IsEmpty(r.LegacyValue) {
		return errors.New("empty legacy value")
	}
	if fractionalInt(r.Spec.Kind, r.LegacyValue) {
		return fmt.Errorf("fractional legacy value %q for integer column %s", Normalize(r.LegacyValue), r.Spec.Column)
	}
	return nil
}

// fractionalInt reports a legacy value that parses as a number with a
// fractional part for an integer column (must be rejected, never rounded).
func fractionalInt(kind Kind, v string) bool {
	if kind != KindInt {
		return false
	}
	x, ok := new(big.Rat).SetString(Normalize(v))
	return ok && !x.IsInt()
}

func uniqueClash(snap Snapshot, r Row) string {
	target := Normalize(r.LegacyValue)
	if uniqueExempt(r, target) {
		return ""
	}
	var others []string
	for _, id := range snap.Holders(r.Spec.Column, target) {
		if !strings.EqualFold(id, r.ID) {
			others = append(others, id)
		}
	}
	if len(others) == 0 {
		return ""
	}
	return fmt.Sprintf("uniqueness: %s=%q already held by live %s %s", r.Spec.Column, target, r.Entity, others[0])
}

// markDuplicateTargets rejects every APPLY row that targets the same unique
// value as an APPLY row of a different head (both sides rejected).
func markDuplicateTargets(rows []PlanRow) {
	groups := map[string][]int{}
	for i, pr := range rows {
		if pr.Decision != DecisionApply || !needsUniqueCheck(pr.Row) {
			continue
		}
		t := Normalize(pr.Row.LegacyValue)
		if uniqueExempt(pr.Row, t) {
			continue
		}
		k := pr.Row.Spec.Column + "\x00" + t
		groups[k] = append(groups[k], i)
	}
	for _, idx := range groups {
		if !distinctHeads(rows, idx) {
			continue
		}
		for _, i := range idx {
			rows[i].Decision = DecisionReject
			rows[i].Reason = fmt.Sprintf("uniqueness: %s=%q targeted by more than one %s in this proposal",
				rows[i].Row.Spec.Column, Normalize(rows[i].Row.LegacyValue), rows[i].Row.Entity)
		}
	}
}

func distinctHeads(rows []PlanRow, idx []int) bool {
	first := strings.ToLower(rows[idx[0]].Row.ID)
	for _, i := range idx[1:] {
		if strings.ToLower(rows[i].Row.ID) != first {
			return true
		}
	}
	return false
}
