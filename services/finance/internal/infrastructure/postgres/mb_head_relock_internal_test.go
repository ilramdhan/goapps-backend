package postgres

import (
	"strings"
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// ⚠ HONEST LIMIT, stated up front. This package has no SQL mock and no test database, so
// the tests below are STRUCTURAL: they prove the predicate is present and shaped
// correctly, ⛔ NOT that PostgreSQL evaluates it the way we intend. The behavioral claim
// — "a head saved after the grant is not a candidate; a head untouched since the grant
// still is" — is ⛔ NOT covered by any automated test in this repository. The aggregate
// SELECT written for the user in the hand-off report is the substitute proof.
//
// These structural tests are still worth having: deleting the predicate would reintroduce
// the work-stealing bug SILENTLY. Nothing would fail, no constraint would fire, and the
// only symptom would be a user's in-progress edit thrown back to APPROVED by the cron.

// TestListExpiredUnlocksQuery_HasUntouchedGuard pins the D7-safety condition onto the
// sweep query: a DRAFT head is only a relock candidate when nothing was saved since the
// unlock was granted.
func TestListExpiredUnlocksQuery_HasUntouchedGuard(t *testing.T) {
	q := listExpiredUnlocksQuery

	// The grant instant must be SELECTed, or the candidate cannot carry it to the
	// transaction-time re-check in relockHeadTx.
	if !strings.Contains(q, "last.mbhl_actor_at AS granted_at") {
		t.Fatalf("sweep query must select the UNLOCK_GRANT instant as granted_at: %q", q)
	}
	// Head-level edits.
	if !strings.Contains(q, "COALESCE(h.updated_at, h.created_at) <=") {
		t.Fatalf("sweep query lost its head updated_at guard — an edited head would be relocked: %q", q)
	}
	// Child-table edits. The head's updated_at does NOT move when only composition rows
	// change, so dropping either of these reopens the bug for the most likely edit.
	for _, table := range []string{"mst_mb_composition", "mst_mb_head_shade"} {
		if !strings.Contains(q, table) {
			t.Fatalf("sweep query must also watch %s for edits since the grant: %q", table, q)
		}
	}
	// ⛔ NEVER `mbh_is_locked = FALSE`: 4190 production rows hold NULL, which means NOT
	// locked and must still match.
	if !strings.Contains(q, "COALESCE(h.mbh_is_locked, FALSE) = FALSE") {
		t.Fatalf("sweep query must COALESCE mbh_is_locked, never compare it directly: %q", q)
	}
	if strings.Contains(q, "h.mbh_is_locked = FALSE") {
		t.Fatalf("sweep query compares mbh_is_locked directly — NULL rows would be skipped: %q", q)
	}
}

// TestUntouchedSinceGrant_ShapeIsReusable pins that the predicate renders against any head
// alias and any grant expression, which is what lets the sweep query use a column
// reference and relockHeadTx use a bind placeholder for the SAME rule. Two divergent
// copies of this condition would be the bug all over again.
func TestUntouchedSinceGrant_ShapeIsReusable(t *testing.T) {
	sweep := untouchedSinceGrant("h", "last.mbhl_actor_at")
	recheck := untouchedSinceGrant("mst_mb_head", "$7")

	if !strings.Contains(sweep, "h.mbh_id") || strings.Contains(sweep, "mst_mb_head.mbh_id") {
		t.Fatalf("alias not applied to the head reference: %q", sweep)
	}
	if !strings.Contains(recheck, "mst_mb_head.mbh_id") {
		t.Fatalf("un-aliased form must reference the table by name: %q", recheck)
	}
	if strings.Count(recheck, "$7") != 3 {
		t.Fatalf("the grant expression must be compared against all three sources "+
			"(head, composition, shade), got: %q", recheck)
	}
	// A delete is an edit: mst_mb_composition's delete path stamps deleted_at only.
	if !strings.Contains(recheck, "cmp.deleted_at") {
		t.Fatalf("composition deletes must count as edits: %q", recheck)
	}
}

// ---------------------------------------------------------------------------
// K-55 — snapshotOnTransition
// ---------------------------------------------------------------------------

// TestSnapshotOnTransition pins the K-55 fix and, just as importantly, its BLAST RADIUS:
// only the reject-unlock re-entry is excluded. A fix that quietly disabled legitimate
// snapshots would be worse than the bug it replaced.
func TestSnapshotOnTransition(t *testing.T) {
	tests := []struct {
		name      string
		fromState string
		toState   string
		want      bool
	}{
		{
			name:      "normal validation from DRAFT still snapshots",
			fromState: mbhead.StatusDraft, toState: mbhead.StatusValidated, want: true,
		},
		{
			name:      "reject-unlock back to VALIDATED must NOT snapshot (K-55)",
			fromState: mbhead.StatusUnlockRequested, toState: mbhead.StatusValidated, want: false,
		},
		{
			name:      "reject-unlock back to APPROVED never snapshotted anyway",
			fromState: mbhead.StatusUnlockRequested, toState: mbhead.StatusApproved, want: false,
		},
		{
			name:      "validation from APPROVED still snapshots",
			fromState: mbhead.StatusApproved, toState: mbhead.StatusValidated, want: true,
		},
		{
			name:      "validation from REJECTED still snapshots",
			fromState: mbhead.StatusRejected, toState: mbhead.StatusValidated, want: true,
		},
		{
			name:      "any non-VALIDATED target never snapshots",
			fromState: mbhead.StatusDraft, toState: mbhead.StatusApproved, want: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := snapshotOnTransition(tc.fromState, tc.toState); got != tc.want {
				t.Fatalf("snapshotOnTransition(%q, %q) = %v, want %v",
					tc.fromState, tc.toState, got, tc.want)
			}
		})
	}
}
