package mbcomposition

import (
	"context"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// statusDraft is the ONLY parent mbh_entry_status under which composition rows may
// be written. Declared here rather than imported from the mbhead packages because
// application/mbhead already imports this package (submit/validate call
// EnforceHeadSum), so the reverse import would be a cycle.
const statusDraft = "DRAFT"

// ensureParentDraft is the DRAFT gate ([K-33]) shared by the create, update and
// delete handlers — the three and only three write paths into mst_mb_composition.
//
// ⛔ It is ALWAYS ON. Unlike the composition-sum rule it has no feature flag and no
// transition period: production shows the hole was never used through the
// application (zero post-VALIDATE inserts or deletes, zero content divergence), so
// closing it breaks nobody's workflow and an escape hatch would only keep the hole
// open.
//
// The comparison is a WHITELIST — status must EQUAL DRAFT — never a blacklist of
// VALIDATED/REVOKED/etc. That matches the frontend's own gate
// (mb-composition-tab.tsx: `entryStatus === "DRAFT"`) and means any workflow status
// added in the future is refused by default instead of silently editable.
//
// A missing or soft-deleted parent surfaces as mbcomposition.ErrParentHeadNotFound,
// straight from the repository.
func ensureParentDraft(ctx context.Context, repo mbcomposition.Repository, mbhID string) error {
	status, err := repo.ParentEntryStatus(ctx, mbhID)
	if err != nil {
		return err
	}
	if status != statusDraft {
		return mbcomposition.ErrParentNotDraft
	}
	return nil
}
