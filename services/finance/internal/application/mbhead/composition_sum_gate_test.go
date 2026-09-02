// Package mbhead_test — [G.5] composition-sum gate on the submit / validate
// workflow transitions (plan §11 item 78, the remainder of P11).
package mbhead_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbcompositiondomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// stubCompositionRepo is a minimal mbcomposition.Repository returning a fixed row
// set. Only ListByMbhID carries behavior — it is the single method the gate uses.
type stubCompositionRepo struct {
	rows      []*mbcompositiondomain.Entity
	listCalls int
}

func (s *stubCompositionRepo) Create(context.Context, *mbcompositiondomain.Entity) error { return nil }
func (s *stubCompositionRepo) Update(context.Context, *mbcompositiondomain.Entity) error { return nil }
func (s *stubCompositionRepo) Delete(context.Context, string) error                      { return nil }

func (s *stubCompositionRepo) CreateWithSumGuard(
	context.Context, *mbcompositiondomain.Entity, mbcompositiondomain.SumGuard,
) error {
	return nil
}

func (s *stubCompositionRepo) UpdateWithSumGuard(
	context.Context, *mbcompositiondomain.Entity, mbcompositiondomain.SumGuard,
) error {
	return nil
}

func (s *stubCompositionRepo) GetByID(context.Context, string) (*mbcompositiondomain.Entity, error) {
	return nil, mbcompositiondomain.ErrNotFound
}

func (s *stubCompositionRepo) ListByMbhID(context.Context, string) ([]*mbcompositiondomain.Entity, error) {
	s.listCalls++
	return s.rows, nil
}

func (s *stubCompositionRepo) SumPercentageByMbhID(context.Context, string) (string, error) {
	return "0", nil
}

func (s *stubCompositionRepo) ListVersionsByMbhID(
	context.Context, string, int32,
) ([]mbcompositiondomain.VersionRow, error) {
	return nil, nil
}

func (s *stubCompositionRepo) ParentEntryStatus(context.Context, string) (string, error) {
	return "DRAFT", nil
}

func (s *stubCompositionRepo) ListMBRefEdgesForBatch(context.Context, []string) ([]mbcompositiondomain.BatchRefEdge, error) {
	return nil, nil
}

// draftHead builds a DRAFT own-production head — the only state from which
// SubmitMBHead is a legal transition.
func draftHead() *mbheaddomain.Entity {
	return mbheaddomain.Reconstruct(
		uuid.New(), nil, "MB001", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, true, time.Now(), "admin",
		nil, nil, nil, nil,
		mbheaddomain.StatusDraft, false, 1, nil,
		"", "", "", "", "", "",
		0, nil, "",
		nil, nil, nil, nil, nil,
		nil, "34", "S",
		nil,
	)
}

// compositionRows builds live (non-carrier) composition rows with the given percentages.
func compositionRows(pcts ...string) []*mbcompositiondomain.Entity {
	out := make([]*mbcompositiondomain.Entity, 0, len(pcts))
	for i, p := range pcts {
		out = append(out, mbcompositiondomain.Reconstruct(
			"row", "mbh", int32(i+1), "", p,
			mbcompositiondomain.SourceTypeMB, "", false, "", "", "tester", "", "", "", "",
		))
	}
	return out
}

// --- Submit ---------------------------------------------------------------------

// TestSubmitHandler_SumGateBlocksBadComposition proves the gate is wired into
// submit: with the flag ON, a head whose composition totals 75 cannot reach
// SUBMITTED, and no transition is written.
func TestSubmitHandler_SumGateBlocksBadComposition(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := draftHead()
	repo := new(MockRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	comp := &stubCompositionRepo{rows: compositionRows("50", "25")}

	got, err := mbhead.NewSubmitHandlerWithComposition(repo, comp).Handle(ctx, mbhead.SubmitCommand{
		MbhID: entity.ID(), ActorUserID: "admin",
	})

	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbcompositiondomain.ErrCompositionSumInvalid)
	repo.AssertNotCalled(t, "Transition", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestSubmitHandler_SumGateEmptyComposition pins R17 on the submit path: no rows
// reports ErrCompositionEmpty, not a misleading "does not total 100%".
func TestSubmitHandler_SumGateEmptyComposition(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := draftHead()
	repo := new(MockRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)

	_, err := mbhead.NewSubmitHandlerWithComposition(repo, &stubCompositionRepo{}).Handle(
		ctx, mbhead.SubmitCommand{MbhID: entity.ID(), ActorUserID: "admin"})

	assert.ErrorIs(t, err, mbcompositiondomain.ErrCompositionEmpty)
	assert.NotErrorIs(t, err, mbcompositiondomain.ErrCompositionSumInvalid)
}

// TestSubmitHandler_SumGateInertWhenFlagOff is the legacy-safety half: the 4
// production recipes with broken totals must stay submittable while the flag is off.
func TestSubmitHandler_SumGateInertWhenFlagOff(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "false")
	ctx := context.Background()

	entity := draftHead()
	repo := new(MockRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	repo.On("Transition", ctx, entity.ID(), mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).Return(nil)
	comp := &stubCompositionRepo{rows: compositionRows("50", "25")}

	_, err := mbhead.NewSubmitHandlerWithComposition(repo, comp).Handle(ctx, mbhead.SubmitCommand{
		MbhID: entity.ID(), ActorUserID: "admin",
	})

	require.NoError(t, err)
	assert.Zero(t, comp.listCalls, "flag off must not even read the composition")
}

// TestSubmitHandler_NilCompositionRepoSkipsGate pins the optional-dependency
// contract: the legacy constructor keeps working and never consults a composition.
func TestSubmitHandler_NilCompositionRepoSkipsGate(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := draftHead()
	repo := new(MockRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	repo.On("Transition", ctx, entity.ID(), mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).Return(nil)

	_, err := mbhead.NewSubmitHandler(repo).Handle(ctx, mbhead.SubmitCommand{
		MbhID: entity.ID(), ActorUserID: "admin",
	})

	require.NoError(t, err)
}

// TestSubmitHandler_SumGateAllowsGoodComposition is the positive control: a
// composition that totals 100 passes the gate and the transition proceeds.
func TestSubmitHandler_SumGateAllowsGoodComposition(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := draftHead()
	repo := new(MockRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	repo.On("Transition", ctx, entity.ID(), mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).Return(nil)
	comp := &stubCompositionRepo{rows: compositionRows("60", "40")}

	_, err := mbhead.NewSubmitHandlerWithComposition(repo, comp).Handle(ctx, mbhead.SubmitCommand{
		MbhID: entity.ID(), ActorUserID: "admin",
	})

	require.NoError(t, err)
	assert.Equal(t, 1, comp.listCalls)
}

// --- Validate -------------------------------------------------------------------

// TestValidateHandler_SumGateBlocksBadComposition proves the gate is wired into
// validate. This is the more consequential of the two: VALIDATED freezes the
// composition into mst_mb_composition_version and triggers cost auto-gen, so a bad
// total accepted here becomes a permanent snapshot that is then costed.
func TestValidateHandler_SumGateBlocksBadComposition(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := approvedHeadWithParams("34", "S", nil)
	repo := new(MockRepository)
	params := new(MockParamRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	comp := &stubCompositionRepo{rows: compositionRows("40.42")}

	got, err := mbhead.NewValidateHandlerWithComposition(repo, params, comp).Handle(
		ctx, mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "admin"})

	assert.Nil(t, got)
	assert.ErrorIs(t, err, mbcompositiondomain.ErrCompositionSumInvalid)
	// The gate must run BEFORE params are resolved and frozen.
	params.AssertNotCalled(t, "ListActive", mock.Anything)
	repo.AssertNotCalled(t, "TransitionWithAutoGen", mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestValidateHandler_SumGateInertWhenFlagOff mirrors the submit case.
func TestValidateHandler_SumGateInertWhenFlagOff(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "false")
	ctx := context.Background()

	entity := approvedHeadWithParams("34", "S", nil)
	repo := new(MockRepository)
	params := new(MockParamRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	params.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	repo.On("TransitionWithAutoGen", ctx, entity.ID(), mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	comp := &stubCompositionRepo{rows: compositionRows("40.42")}

	_, err := mbhead.NewValidateHandlerWithComposition(repo, params, comp).Handle(
		ctx, mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "admin"})

	require.NoError(t, err)
	assert.Zero(t, comp.listCalls, "flag off must not even read the composition")
}

// TestValidateHandler_SumGateAllowsGoodComposition is the positive control.
func TestValidateHandler_SumGateAllowsGoodComposition(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := approvedHeadWithParams("34", "S", nil)
	repo := new(MockRepository)
	params := new(MockParamRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	params.On("ListActive", ctx).Return(activeParamMasters(t), nil)
	repo.On("TransitionWithAutoGen", ctx, entity.ID(), mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	comp := &stubCompositionRepo{rows: compositionRows("33.34", "33.33", "33.33")}

	_, err := mbhead.NewValidateHandlerWithComposition(repo, params, comp).Handle(
		ctx, mbhead.ValidateCommand{MbhID: entity.ID(), ActorUserID: "admin"})

	require.NoError(t, err)
	assert.Equal(t, 1, comp.listCalls)
}

// TestSumGateExcludesCarrierRows pins that carrier rows do not count toward the
// total on the workflow path either — consistent with the CRUD path, where the sum
// query filters mbcm_is_carrier = FALSE. A 100% composition plus a carrier must pass.
func TestSumGateExcludesCarrierRows(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")
	ctx := context.Background()

	entity := draftHead()
	repo := new(MockRepository)
	repo.On("GetByID", ctx, entity.ID()).Return(entity, nil)
	repo.On("Transition", ctx, entity.ID(), mock.Anything, mock.Anything, mock.Anything,
		mock.Anything, mock.Anything, mock.Anything).Return(nil)

	rows := compositionRows("60", "40")
	rows = append(rows, mbcompositiondomain.Reconstruct(
		"carrier", "mbh", 3, "", "30",
		mbcompositiondomain.SourceTypeCarrier, "", true, "", "", "tester", "", "", "", "",
	))

	_, err := mbhead.NewSubmitHandlerWithComposition(repo, &stubCompositionRepo{rows: rows}).Handle(
		ctx, mbhead.SubmitCommand{MbhID: entity.ID(), ActorUserID: "admin"})

	require.NoError(t, err, "a carrier row must not push a valid 100%% composition over tolerance")
}
