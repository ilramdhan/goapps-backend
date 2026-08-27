package mbcomposition_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appmbcomposition "github.com/mutugading/goapps-backend/services/finance/internal/application/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// nonDraftStatuses is every mbh_entry_status a head can hold EXCEPT DRAFT, taken
// from internal/domain/mbhead/state_machine.go and the CHECK constraint in
// migration 000488. All of them must be refused — the gate is a whitelist, so this
// list exists to prove no status was accidentally left editable, not to define the
// rule.
var nonDraftStatuses = []string{
	"SUBMITTED",
	"APPROVED",
	"VALIDATED",
	"UN_APPROVED",
	"REVOKED",
	"REJECTED",
}

// existingRow is the stored composition row the update and delete paths read back.
func existingRow() *mbcomposition.Entity {
	return mbcomposition.Reconstruct(
		"row-1", testMbhID, 1, "", "30",
		mbcomposition.SourceTypeMB, "", false, "", "", "tester", "", "", "", "",
	)
}

func updateCmd() appmbcomposition.UpdateCommand {
	return appmbcomposition.UpdateCommand{
		ID: "row-1", CompositionPct: "30",
		SourceType: mbcomposition.SourceTypeMB, UpdatedBy: "tester",
	}
}

// runWrite drives one of the three composition write paths against repo and returns
// its error, so every gate case can be expressed as one table row per path.
func runWrite(path string, repo *fakeRepo) error {
	switch path {
	case "create":
		_, err := appmbcomposition.NewCreateHandler(repo).Handle(context.Background(), createCmd("30"))
		return err
	case "update":
		_, err := appmbcomposition.NewUpdateHandler(repo).Handle(context.Background(), updateCmd())
		return err
	default:
		return appmbcomposition.NewDeleteHandler(repo).Handle(
			context.Background(), appmbcomposition.DeleteCommand{ID: "row-1"})
	}
}

// writes reports how many rows the fake actually persisted, across all three paths.
func writes(repo *fakeRepo) int {
	return repo.created + repo.updated + repo.deleted
}

var writePaths = []string{"create", "update", "delete"}

// TestDraftGateRejectsEveryNonDraftStatus is the core of [K-33]: composition rows
// may be written ONLY while the parent head is DRAFT. Every non-DRAFT status is
// exercised on every write path, and the write must be refused BEFORE anything is
// persisted.
func TestDraftGateRejectsEveryNonDraftStatus(t *testing.T) {
	for _, path := range writePaths {
		for _, status := range nonDraftStatuses {
			t.Run(path+"/"+status, func(t *testing.T) {
				repo := &fakeRepo{sum: "0", existing: existingRow(), parentStatus: status}

				err := runWrite(path, repo)

				assert.ErrorIs(t, err, mbcomposition.ErrParentNotDraft)
				assert.Zero(t, writes(repo), "a rejected write must persist nothing")
			})
		}
	}
}

// TestDraftGateAllowsDraft pins the other half of the whitelist: DRAFT still works,
// on all three paths, with no feature flag set.
func TestDraftGateAllowsDraft(t *testing.T) {
	for _, path := range writePaths {
		t.Run(path, func(t *testing.T) {
			repo := &fakeRepo{sum: "0", existing: existingRow(), parentStatus: "DRAFT"}

			require.NoError(t, runWrite(path, repo))
			assert.Equal(t, 1, writes(repo), "a DRAFT parent must let the write through")
		})
	}
}

// TestDraftGateReportsMissingParent covers the third outcome of the status read: a
// parent head that does not exist (or is soft-deleted) surfaces as
// ErrParentHeadNotFound, distinct from "exists but is locked".
func TestDraftGateReportsMissingParent(t *testing.T) {
	for _, path := range writePaths {
		t.Run(path, func(t *testing.T) {
			repo := &fakeRepo{
				sum: "0", existing: existingRow(),
				parentErr: mbcomposition.ErrParentHeadNotFound,
			}

			err := runWrite(path, repo)

			assert.ErrorIs(t, err, mbcomposition.ErrParentHeadNotFound)
			assert.NotErrorIs(t, err, mbcomposition.ErrParentNotDraft)
			assert.Zero(t, writes(repo))
		})
	}
}

// TestDraftGateIsUnconditional guards against the gate ever acquiring an escape
// hatch: it must fire with the composition-sum flag explicitly OFF, since that flag
// governs a different rule entirely.
func TestDraftGateIsUnconditional(t *testing.T) {
	t.Setenv("MB_COMPOSITION_SUM_ENFORCED", "false")

	repo := &fakeRepo{sum: "0", existing: existingRow(), parentStatus: "VALIDATED"}

	assert.ErrorIs(t, runWrite("create", repo), mbcomposition.ErrParentNotDraft)
}

// TestDeleteMissingRowStillReportsNotFound pins that adding the pre-read to the
// delete path did not change how a missing row is reported: ErrNotFound from
// GetByID must propagate unchanged, never be masked by a gate error.
func TestDeleteMissingRowStillReportsNotFound(t *testing.T) {
	repo := &fakeRepo{sum: "0", parentStatus: "VALIDATED"} // existing == nil → ErrNotFound

	err := appmbcomposition.NewDeleteHandler(repo).Handle(
		context.Background(), appmbcomposition.DeleteCommand{ID: "missing"})

	assert.ErrorIs(t, err, mbcomposition.ErrNotFound)
	assert.NotErrorIs(t, err, mbcomposition.ErrParentNotDraft)
	assert.Zero(t, repo.deleted)
}
