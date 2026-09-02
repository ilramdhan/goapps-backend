package mbcomposition_test

import (
	"context"
	"errors"
	"testing"

	appmbcomposition "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// fakeRepo is an in-memory mbcomposition.Repository for the [G.5] handler tests.
type fakeRepo struct {
	sum      string
	existing *mbcomposition.Entity
	created  int
	updated  int
	deleted  int
	guarded  int
	guardNil int

	// parentStatus is the parent head's mbh_entry_status for the [K-33] DRAFT gate;
	// empty means DRAFT. parentErr, when set, is returned instead.
	parentStatus string
	parentErr    error
}

func (f *fakeRepo) Create(_ context.Context, _ *mbcomposition.Entity) error {
	f.created++
	return nil
}

func (f *fakeRepo) Update(_ context.Context, _ *mbcomposition.Entity) error {
	f.updated++
	return nil
}

// CreateWithSumGuard mimics the real repository's contract (G24): the guard is
// consulted with the stored sum BEFORE the write, and a guard error aborts it. It
// also records whether a guard was supplied at all, so tests can assert that the
// flag being off means no guard — and therefore no lock and no sum read.
func (f *fakeRepo) CreateWithSumGuard(ctx context.Context, e *mbcomposition.Entity, guard mbcomposition.SumGuard) error {
	if guard == nil {
		f.guardNil++
		return f.Create(ctx, e)
	}
	f.guarded++
	if err := guard(f.sum); err != nil {
		return err
	}
	return f.Create(ctx, e)
}

// UpdateWithSumGuard is the update-path twin of CreateWithSumGuard.
func (f *fakeRepo) UpdateWithSumGuard(ctx context.Context, e *mbcomposition.Entity, guard mbcomposition.SumGuard) error {
	if guard == nil {
		f.guardNil++
		return f.Update(ctx, e)
	}
	f.guarded++
	if err := guard(f.sum); err != nil {
		return err
	}
	return f.Update(ctx, e)
}

func (f *fakeRepo) Delete(_ context.Context, _ string) error {
	f.deleted++
	return nil
}

func (f *fakeRepo) GetByID(_ context.Context, _ string) (*mbcomposition.Entity, error) {
	if f.existing == nil {
		return nil, mbcomposition.ErrNotFound
	}
	return f.existing, nil
}

func (f *fakeRepo) ListByMbhID(_ context.Context, _ string) ([]*mbcomposition.Entity, error) {
	return nil, nil
}

func (f *fakeRepo) SumPercentageByMbhID(_ context.Context, _ string) (string, error) {
	return f.sum, nil
}

func (f *fakeRepo) ListVersionsByMbhID(_ context.Context, _ string, _ int32) ([]mbcomposition.VersionRow, error) {
	return nil, nil
}

// ParentEntryStatus feeds the [K-33] DRAFT gate. The zero value is deliberately
// "DRAFT" via parentStatusOrDraft, so every pre-existing [G.5] test keeps exercising
// the sum rule rather than tripping the new gate first.
func (f *fakeRepo) ParentEntryStatus(_ context.Context, _ string) (string, error) {
	if f.parentErr != nil {
		return "", f.parentErr
	}
	if f.parentStatus == "" {
		return "DRAFT", nil
	}
	return f.parentStatus, nil
}

func (f *fakeRepo) ListMBRefEdgesForBatch(_ context.Context, _ []string) ([]mbcomposition.BatchRefEdge, error) {
	return nil, nil
}

const testMbhID = "11111111-1111-1111-1111-111111111111"

func createCmd(pct string) appmbcomposition.CreateCommand {
	return appmbcomposition.CreateCommand{
		MbhID:          testMbhID,
		CompositionPct: pct,
		SourceType:     mbcomposition.SourceTypeMB,
		SeqNo:          1,
		CreatedBy:      "tester",
	}
}

// TestCreateFlagOffAllowsBadSum pins the plan criterion "flag false → writing 75% passes".
func TestCreateFlagOffAllowsBadSum(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "false")

	repo := &fakeRepo{sum: "0"}
	if _, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("75")); err != nil {
		t.Fatalf("create with flag off = %v, want nil", err)
	}
	if repo.created != 1 {
		t.Fatalf("created = %d, want 1", repo.created)
	}
}

// TestCreateFlagUnsetAllowsBadSum pins the documented default: OFF.
func TestCreateFlagUnsetAllowsBadSum(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "")

	repo := &fakeRepo{sum: "0"}
	if _, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("75")); err != nil {
		t.Fatalf("create with flag unset = %v, want nil", err)
	}
}

// TestCreateFlagOnRejectsBadSum pins "flag true → rejected".
func TestCreateFlagOnRejectsBadSum(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	repo := &fakeRepo{sum: "0"}
	_, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("75"))
	if !errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
		t.Fatalf("create 75%% with flag on = %v, want ErrCompositionSumInvalid", err)
	}
	if repo.created != 0 {
		t.Fatalf("rejected create still wrote: created = %d, want 0", repo.created)
	}
}

// TestCreateFlagOnAcceptsCompletingRow verifies a row that brings the total to 100 is allowed.
func TestCreateFlagOnAcceptsCompletingRow(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	repo := &fakeRepo{sum: "60"}
	if _, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("40")); err != nil {
		t.Fatalf("create completing row = %v, want nil", err)
	}
	if repo.created != 1 {
		t.Fatalf("created = %d, want 1", repo.created)
	}
}

// TestCreateFlagOnCarrierRowExcluded verifies carrier rows do not count toward the sum.
func TestCreateFlagOnCarrierRowExcluded(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	repo := &fakeRepo{sum: "100"}
	cmd := createCmd("30")
	cmd.SourceType = mbcomposition.SourceTypeCarrier
	cmd.IsCarrier = true
	if _, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), cmd); err != nil {
		t.Fatalf("create carrier row = %v, want nil", err)
	}
}

// TestUpdateFlagOnUsesDelta verifies update validates (new - old), since the stored
// sum already contains the row being edited.
func TestUpdateFlagOnUsesDelta(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	// Stored total 90 includes this row at 30; changing it to 40 yields 100.
	existing := mbcomposition.Reconstruct("row-1", testMbhID, 1, "", "30",
		mbcomposition.SourceTypeMB, "", false, "", "", "tester", "", "", "", "")
	repo := &fakeRepo{sum: "90", existing: existing}

	okCmd := appmbcomposition.UpdateCommand{
		ID: "row-1", CompositionPct: "40",
		SourceType: mbcomposition.SourceTypeMB, UpdatedBy: "tester",
	}
	if _, err := appmbcomposition.NewUpdateHandler(repo).Handle(context.Background(), okCmd); err != nil {
		t.Fatalf("update 30->40 on total 90 = %v, want nil", err)
	}

	badCmd := okCmd
	badCmd.CompositionPct = "50"
	if _, err := appmbcomposition.NewUpdateHandler(repo).Handle(context.Background(), badCmd); !errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
		t.Fatalf("update 30->50 on total 90 = %v, want ErrCompositionSumInvalid", err)
	}
}

// TestUpdateFlagOffAllowsBadSum pins "flag false → passes" on the update path too.
func TestUpdateFlagOffAllowsBadSum(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "false")

	existing := mbcomposition.Reconstruct("row-1", testMbhID, 1, "", "30",
		mbcomposition.SourceTypeMB, "", false, "", "", "tester", "", "", "", "")
	repo := &fakeRepo{sum: "90", existing: existing}

	cmd := appmbcomposition.UpdateCommand{
		ID: "row-1", CompositionPct: "50",
		SourceType: mbcomposition.SourceTypeMB, UpdatedBy: "tester",
	}
	if _, err := appmbcomposition.NewUpdateHandler(repo).Handle(context.Background(), cmd); err != nil {
		t.Fatalf("update with flag off = %v, want nil", err)
	}
}

// TestDeleteAlwaysPasses pins the plan criterion "deleting a row ALWAYS passes" —
// removing rows to fix a broken composition must stay possible even with the flag on.
func TestDeleteAlwaysPasses(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	// existing must be populated: since [K-33] the delete path reads the row first to
	// learn its parent mbh_id for the DRAFT gate. fakeRepo's parent defaults to DRAFT,
	// so this test still measures only the sum rule, which is its point.
	repo := &fakeRepo{sum: "100", existing: mbcomposition.Reconstruct(
		"row-1", testMbhID, 1, "", "30",
		mbcomposition.SourceTypeMB, "", false, "", "", "tester", "", "", "", "",
	)}
	if err := appmbcomposition.NewDeleteHandler(repo).Handle(context.Background(), appmbcomposition.DeleteCommand{ID: "row-1"}); err != nil {
		t.Fatalf("delete with flag on = %v, want nil", err)
	}
	if repo.deleted != 1 {
		t.Fatalf("deleted = %d, want 1", repo.deleted)
	}
}

// TestListNeverValidates pins "reading/exporting the 4 broken recipes keeps working".
func TestListNeverValidates(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	repo := &fakeRepo{sum: "75"}
	if _, err := appmbcomposition.NewListHandler(repo).Handle(context.Background(), appmbcomposition.ListQuery{MbhID: testMbhID}); err != nil {
		t.Fatalf("list with flag on = %v, want nil", err)
	}
}

// --- G24: the sum check is delegated to the repository, not run ahead of it -----------

// TestCreatePassesGuardWhenEnforced pins the G24 contract on the create path: with
// the flag ON the handler hands the repository a non-nil guard, so the sum is read
// inside the write's own transaction under the parent-head lock rather than on a
// separate connection beforehand.
func TestCreatePassesGuardWhenEnforced(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	repo := &fakeRepo{sum: "60"}
	if _, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("40")); err != nil {
		t.Fatalf("create = %v, want nil", err)
	}
	if repo.guarded != 1 {
		t.Fatalf("guarded writes = %d, want 1 (flag on must supply a guard)", repo.guarded)
	}
	if repo.guardNil != 0 {
		t.Fatalf("unguarded writes = %d, want 0", repo.guardNil)
	}
}

// TestCreatePassesNilGuardWhenNotEnforced is the other half: with the flag OFF the
// guard must be nil, so the repository skips the lock and the sum query entirely and
// the unenforced path costs exactly what it did before G24.
func TestCreatePassesNilGuardWhenNotEnforced(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "false")

	repo := &fakeRepo{sum: "60"}
	if _, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("75")); err != nil {
		t.Fatalf("create = %v, want nil", err)
	}
	if repo.guardNil != 1 {
		t.Fatalf("unguarded writes = %d, want 1 (flag off must supply no guard)", repo.guardNil)
	}
	if repo.guarded != 0 {
		t.Fatalf("guarded writes = %d, want 0", repo.guarded)
	}
}

// TestUpdatePassesGuardWhenEnforced is the update-path twin.
func TestUpdatePassesGuardWhenEnforced(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	existing := mbcomposition.Reconstruct("row-1", testMbhID, 1, "", "30",
		mbcomposition.SourceTypeMB, "", false, "", "", "tester", "", "", "", "")
	repo := &fakeRepo{sum: "90", existing: existing}

	cmd := appmbcomposition.UpdateCommand{
		ID: "row-1", CompositionPct: "40",
		SourceType: mbcomposition.SourceTypeMB, UpdatedBy: "tester",
	}
	if _, err := appmbcomposition.NewUpdateHandler(repo).Handle(context.Background(), cmd); err != nil {
		t.Fatalf("update = %v, want nil", err)
	}
	if repo.guarded != 1 {
		t.Fatalf("guarded writes = %d, want 1", repo.guarded)
	}
}

// TestGuardSeesRepositorySumNotAPreReadSum is the heart of G24. The fake's stored
// sum CHANGES between the moment the handler builds its guard and the moment the
// repository invokes it — exactly what a concurrent committed writer does in
// production. A guard that closed over a pre-read total would still see the stale 60
// and wrongly accept; a guard evaluated against the sum the repository reads under
// its lock sees 90 and correctly rejects.
func TestGuardSeesRepositorySumNotAPreReadSum(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "true")

	repo := &shiftingSumRepo{fakeRepo: fakeRepo{sum: "60"}, sumAtGuardTime: "90"}

	// 40 completes a total of 60, but the total is really 90 by write time.
	_, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("40"))
	if !errors.Is(err, mbcomposition.ErrCompositionSumInvalid) {
		t.Fatalf("create against a shifted total = %v, want ErrCompositionSumInvalid "+
			"(the guard must be evaluated against the sum read at write time, not a pre-read one)", err)
	}
	if repo.created != 0 {
		t.Fatalf("rejected create still wrote: created = %d, want 0", repo.created)
	}
}

// shiftingSumRepo simulates a concurrent writer committing between the handler
// building its guard and the repository evaluating it.
type shiftingSumRepo struct {
	fakeRepo
	sumAtGuardTime string
}

func (f *shiftingSumRepo) CreateWithSumGuard(ctx context.Context, e *mbcomposition.Entity, guard mbcomposition.SumGuard) error {
	if guard == nil {
		f.guardNil++
		return f.Create(ctx, e)
	}
	f.guarded++
	if err := guard(f.sumAtGuardTime); err != nil {
		return err
	}
	return f.Create(ctx, e)
}
