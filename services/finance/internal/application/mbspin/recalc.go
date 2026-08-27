// Package mbspin provides application layer handlers for MB Spin operations.
//
// ⛔ ISOLATION (hard boundary, user instruction): this file must never import or
// call the calc-engine v2 package `internal/application/rmcost`, in either
// direction. Recalculation STOPS AT THE CHILD SPIN (decision D24): ⛔ ZERO yarn
// products are recalculated here. The impact_* numbers this file produces are a
// READ-ONLY PREVIEW of which products WOULD be affected — they are counted with
// a SELECT, never produced by running a costing calculation.
package mbspin

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// DefaultImpactLimit caps the impact preview rows a recalc/duplicate response
// carries. Mirrors the PreviewDozingImpact default so both surfaces truncate
// identically.
const DefaultImpactLimit = 20

// RecalcEventDozingChanged is the mbwl_meta "event" discriminator, fixed by user
// decision K-18 / gate G20-META.
const RecalcEventDozingChanged = "DOZING_CHANGED"

// SkippedChild is one direct child spin the pass deliberately did NOT touch (A7).
type SkippedChild struct {
	// SpinID / MgtName identify the child for display without a refetch.
	SpinID  uuid.UUID
	MgtName string
	// Status is the child's mbs_status verbatim; nil when it has none.
	Status *string
	// Reason is mbspin.SkipReasonStatusNotRnD or mbspin.SkipReasonStatusAbsent.
	Reason string
}

// RecalcResult is the outcome of one classification (+ optional write) pass.
type RecalcResult struct {
	// Recalculated lists the children whose dozing was rewritten. Always empty
	// for Preview.
	Recalculated []mbspin.ChildRecalcUpdate
	// Skipped lists the non-candidate children (A7) with their reason.
	Skipped []SkippedChild
	// Incomplete lists candidate children that COULD NOT be scaled because an
	// operand was absent (a nil denier/filament/dozing on either side, or a
	// non-positive filament). They were left untouched.
	//
	// ⚠ They are NOT reported in Skipped: the skip vocabulary is exactly two
	// values (user decision K-46(a)) and inventing a third would change an
	// agreed response contract. Callers surface the count in the response
	// message instead of silently pretending the pass was complete.
	Incomplete []uuid.UUID
	// ImpactRows / ImpactTotals / ImpactTruncated are the D24 PREVIEW: which
	// cost products are bound to this spin. ⛔ Not a recalculation result.
	ImpactRows      []mbdozing.ImpactRow
	ImpactTotals    mbdozing.Totals
	ImpactTruncated bool
}

// RecalcService classifies a parent spin's direct children and, on Apply,
// rewrites the dozing of the eligible ones.
//
// It owns THREE collaborators and no others — deliberately: the spin reads, the
// spin recalc writes, and the READ-ONLY product-impact SELECT. ⛔ There is no
// costing engine here.
type RecalcService struct {
	repo       mbspin.Repository
	recalcRepo mbspin.RecalcRepository
	impactRepo mbdozing.ImpactRepository
}

// NewRecalcService constructs a RecalcService. impactRepo may be nil, in which
// case the impact preview is simply absent (the recalc itself still runs).
func NewRecalcService(repo mbspin.Repository, recalcRepo mbspin.RecalcRepository, impactRepo mbdozing.ImpactRepository) *RecalcService {
	return &RecalcService{repo: repo, recalcRepo: recalcRepo, impactRepo: impactRepo}
}

// Preview classifies the direct children of parent and gathers the product
// impact WITHOUT writing anything.
//
// Used by the duplicate path: cloning a spin copies its dozing verbatim, so no
// number changes and there is nothing to recalculate — but the user still needs
// to see which children are actuals that would be left alone, and which products
// hang off this spin.
func (s *RecalcService) Preview(ctx context.Context, parent *mbspin.Entity) (*RecalcResult, error) {
	candidates, skipped, err := s.classify(ctx, parent.ID())
	if err != nil {
		return nil, err
	}
	if err := mbspin.AssertRecalcFanOut(len(candidates)); err != nil {
		return nil, err
	}
	res := &RecalcResult{Skipped: skipped}
	if err := s.attachImpact(ctx, parent, res); err != nil {
		return nil, err
	}
	return res, nil
}

// ApplyInput is the payload of a real (writing) recalc pass.
type ApplyInput struct {
	// Parent is the spin AFTER its own update was applied — its denier/filament/
	// dozing are the reference operands of formula C-1.
	Parent *mbspin.Entity
	// OldDozing is the parent's dozing BEFORE the change, for the mbwl_meta
	// "old" field. nil when it had none.
	OldDozing *float64
	// Actor lands in mbs_last_recalc_by and mbwl_actor_user_id.
	Actor string
}

// Apply recalculates the dozing of every eligible direct child of Parent and
// persists the pass as ONE operation.
//
// ⛔ ONE LEVEL DEEP (R13). There is no recursion anywhere in this function: it
// calls ListAllChildren exactly once, for exactly one parent, and never calls
// itself or Apply/Preview on a child. A grandchild therefore cannot be reached.
//
// ⛔ ZERO yarn-product recalculation (D24): the only downstream contact with
// products is attachImpact, which COUNTS them with a SELECT.
func (s *RecalcService) Apply(ctx context.Context, in ApplyInput) (*RecalcResult, error) {
	if in.Parent == nil {
		return nil, mbspin.ErrNotFound
	}
	if in.Actor == "" {
		return nil, mbspin.ErrEmptyCreatedBy
	}

	candidates, skipped, err := s.classify(ctx, in.Parent.ID())
	if err != nil {
		return nil, err
	}
	if err := mbspin.AssertRecalcFanOut(len(candidates)); err != nil {
		return nil, err
	}

	res := &RecalcResult{Skipped: skipped}
	for _, child := range candidates {
		newDozing, ok := scaleChildDozing(in.Parent, child)
		if !ok {
			res.Incomplete = append(res.Incomplete, child.ID())
			continue
		}
		res.Recalculated = append(res.Recalculated, mbspin.ChildRecalcUpdate{
			SpinID: child.ID(), NewDozing: newDozing,
		})
	}

	if err := s.attachImpact(ctx, in.Parent, res); err != nil {
		return nil, err
	}

	meta, err := buildRecalcMeta(in.OldDozing, in.Parent.Dozing(), res)
	if err != nil {
		return nil, err
	}

	// ONE operation => ONE mst_mb_workflow_log row (plan §P8 "Jejak"),
	// ⛔ never one per child.
	applyErr := s.recalcRepo.ApplyChildRecalc(ctx, mbspin.RecalcApplyInput{
		ParentSpinID: in.Parent.ID(),
		HeadID:       in.Parent.HeadID(),
		Actor:        in.Actor,
		At:           time.Now(),
		Updates:      res.Recalculated,
		LogReason: fmt.Sprintf(
			"child spin recalc from parent %s: %d recalculated, %d skipped, %d incomplete",
			in.Parent.ID(), len(res.Recalculated), len(res.Skipped), len(res.Incomplete)),
		LogMeta: meta,
	})
	if applyErr != nil {
		return nil, applyErr
	}
	return res, nil
}

// classify splits the parent's DIRECT children into recalc candidates (A6) and
// skipped non-candidates (A7).
//
// ⚠ The A7 enforcement is HERE, in the `if !child.IsRnD()` branch below: a child
// whose status is anything other than "R and D" — Spinning, Boughtout, or absent
// — is appended to skipped and `continue`s, so it never reaches the candidate
// slice and therefore never reaches an UPDATE. A child that is already an ACTUAL
// is never recalculated. The repository re-asserts the same predicate in its
// WHERE clause as a second, concurrency-safe line of defense.
func (s *RecalcService) classify(ctx context.Context, parentID uuid.UUID) ([]*mbspin.Entity, []SkippedChild, error) {
	// ONE flat read of the DIRECT children. ⛔ No recursion, no recursive CTE:
	// grandchildren are unreachable by construction (R13).
	children, err := s.recalcRepo.ListAllChildren(ctx, parentID)
	if err != nil {
		return nil, nil, err
	}

	candidates := make([]*mbspin.Entity, 0, len(children))
	skipped := make([]SkippedChild, 0)
	for _, child := range children {
		if !child.IsRnD() {
			// ⛔ A7 ENFORCED HERE: a child in Spinning / Boughtout / no status
			// holds an ACTUAL value. It is recorded and skipped — it never
			// reaches candidates, so no UPDATE can ever touch it.
			skipped = append(skipped, SkippedChild{
				SpinID:  child.ID(),
				MgtName: child.MgtName(),
				Status:  child.MBSStatus(),
				Reason:  mbspin.SkipReasonFor(child.MBSStatus()),
			})
			continue
		}
		candidates = append(candidates, child)
	}
	return candidates, skipped, nil
}

// scaleChildDozing applies formula C-1 to one child.
//
// Reference operands come from the PARENT (its dozing is the reference LDR),
// target operands from the CHILD — exactly the argument order the plan fixes.
// Returns ok=false when any operand is absent or the arithmetic rejects it; the
// caller records the child as Incomplete rather than writing a wrong number.
func scaleChildDozing(parent, child *mbspin.Entity) (float64, bool) {
	pd, pdn, pf := parent.Dozing(), parent.Denier(), parent.Filament()
	cdn, cf := child.Denier(), child.Filament()
	if pd == nil || pdn == nil || pf == nil || cdn == nil || cf == nil {
		return 0, false
	}
	out, err := mbdozing.ScaleLDR(mbdozing.ScaleInput{
		LDRRef:         *pd,
		DenierRef:      *pdn,
		FilamentRef:    float64(*pf),
		DenierTarget:   *cdn,
		FilamentTarget: float64(*cf),
	})
	if err != nil {
		return 0, false
	}
	return out, true
}

// attachImpact fills the D24 preview fields.
//
// ⛔ This is the ONLY downstream-of-spin contact in the whole recalc path, and it
// is a pure SELECT: it counts the cost products bound to the spin's ORION item
// code. ⛔ It does not compute, write, or refresh a single product cost. A spin
// without an ORION code simply has no linkable products, so the preview stays
// empty rather than guessing another join key.
func (s *RecalcService) attachImpact(ctx context.Context, parent *mbspin.Entity, res *RecalcResult) error {
	if s.impactRepo == nil {
		return nil
	}
	code := parent.OrionItemCode()
	if code == nil || *code == "" {
		return nil
	}
	rows, totals, err := s.impactRepo.ImpactBySpin(ctx, *code, DefaultImpactLimit)
	if err != nil {
		return err
	}
	res.ImpactRows = rows
	res.ImpactTotals = totals
	res.ImpactTruncated = totals.TotalAffected > len(rows)
	return nil
}

// recalcMeta is the mbwl_meta JSONB document, whose exact shape is fixed by user
// decision K-18 / gate G20-META:
//
//	{"event":"DOZING_CHANGED","old":…,"new":…,"affected_products":…,"locked":…}
//
// old/new are the PARENT's dozing before and after the change (null when
// absent). affected_products/locked are the D24 PREVIEW counts — ⛔ they describe
// which products would be affected, they are NOT evidence that any product was
// recalculated.
type recalcMeta struct {
	Event            string   `json:"event"`
	Old              *float64 `json:"old"`
	New              *float64 `json:"new"`
	AffectedProducts int      `json:"affected_products"`
	Locked           int      `json:"locked"`
}

func buildRecalcMeta(oldDozing, newDozing *float64, res *RecalcResult) (string, error) {
	b, err := json.Marshal(recalcMeta{
		Event:            RecalcEventDozingChanged,
		Old:              oldDozing,
		New:              newDozing,
		AffectedProducts: res.ImpactTotals.TotalAffected,
		Locked:           res.ImpactTotals.TotalLocked,
	})
	if err != nil {
		return "", fmt.Errorf("marshal mbwl_meta: %w", err)
	}
	return string(b), nil
}
