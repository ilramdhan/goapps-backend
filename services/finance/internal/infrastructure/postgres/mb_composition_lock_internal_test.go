package postgres

import (
	"strings"
	"testing"
)

// TestMBCompositionParentLockQuery pins the row lock that makes the composition-sum
// rule (G24) safe under concurrency.
//
// This is a structural guard, not a behavioral one: there is no SQL mock or test
// database in this package, so the lock cannot be exercised here. It is still worth
// pinning, because dropping "FOR UPDATE" would reintroduce the read-then-write race
// SILENTLY — no test would fail, no constraint would fire, and the only symptom
// would be an out-of-tolerance total committed in production.
//
// ⚠ HONEST LIMIT: passing this test does NOT prove two concurrent transactions are
// actually serialized. That needs an integration test against a real PostgreSQL,
// which this package has no harness for.
func TestMBCompositionParentLockQuery(t *testing.T) {
	q := mbCompositionParentLockQuery

	if !strings.Contains(q, "FOR UPDATE") {
		t.Fatalf("parent lock query lost its FOR UPDATE clause, reintroducing the G24 race: %q", q)
	}
	// The lock must be on the parent head, not on the composition rows: a concurrent
	// INSERT adds a row that no pre-existing composition-row lock could cover.
	if !strings.Contains(q, "mst_mb_head") {
		t.Fatalf("parent lock query must lock mst_mb_head, got: %q", q)
	}
	if strings.Contains(q, "mst_mb_composition") {
		t.Fatalf("parent lock query must not lock composition rows (insufficient for INSERTs), got: %q", q)
	}
	// Soft-deleted heads must not be lockable targets.
	if !strings.Contains(q, "deleted_at IS NULL") {
		t.Fatalf("parent lock query must exclude soft-deleted heads, got: %q", q)
	}
}
