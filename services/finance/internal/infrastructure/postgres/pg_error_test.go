package postgres

import (
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/lib/pq"
)

// stubSQLStateErr exercises the driver-agnostic interface{ SQLState() string }
// fallback path of pgErrorInfo.
type stubSQLStateErr struct{ state string }

func (e stubSQLStateErr) Error() string    { return "stub sqlstate error " + e.state }
func (e stubSQLStateErr) SQLState() string { return e.state }

func TestIsPGUniqueViolation(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"plain error", errors.New("boom"), false},
		{"sql.ErrNoRows", sql.ErrNoRows, false},
		// Production path: the pool is opened with sql.Open("pgx", ...) so pgx
		// returns *pgconn.PgError. Matching only *pq.Error here was a silent no-op.
		{"pgconn unique violation", &pgconn.PgError{Code: "23505"}, true},
		{"pgconn wrapped", fmt.Errorf("insert uom: %w", &pgconn.PgError{Code: "23505"}), true},
		{"pgconn other code", &pgconn.PgError{Code: "23503"}, false},
		// Seeder + lib/pq-based tests.
		{"pq unique violation", &pq.Error{Code: "23505"}, true},
		{"pq wrapped", fmt.Errorf("insert uom: %w", &pq.Error{Code: "23505"}), true},
		{"pq other code", &pq.Error{Code: "23503"}, false},
		// Driver-agnostic fallback.
		{"SQLState fallback unique violation", stubSQLStateErr{state: "23505"}, true},
		{"SQLState fallback other code", stubSQLStateErr{state: "42P01"}, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := isPGUniqueViolation(tt.err); got != tt.want {
				t.Fatalf("isPGUniqueViolation(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestPGErrorInfo(t *testing.T) {
	t.Parallel()

	t.Run("pgconn exposes constraint name", func(t *testing.T) {
		t.Parallel()
		state, constraint, ok := pgErrorInfo(&pgconn.PgError{Code: "23505", ConstraintName: "jobs_active_unique"})
		if !ok || state != "23505" || constraint != "jobs_active_unique" {
			t.Fatalf("pgErrorInfo = (%q, %q, %v)", state, constraint, ok)
		}
	})

	t.Run("pq exposes constraint name", func(t *testing.T) {
		t.Parallel()
		state, constraint, ok := pgErrorInfo(&pq.Error{Code: "23505", Constraint: "jobs_active_unique"})
		if !ok || state != "23505" || constraint != "jobs_active_unique" {
			t.Fatalf("pgErrorInfo = (%q, %q, %v)", state, constraint, ok)
		}
	})

	t.Run("non-postgres error is not ok", func(t *testing.T) {
		t.Parallel()
		if _, _, ok := pgErrorInfo(errors.New("boom")); ok {
			t.Fatal("pgErrorInfo reported ok for a non-PostgreSQL error")
		}
	})
}

// TestRepositoryUniqueViolationHelpersUsePgconn guards the actual regression:
// every per-repository helper must recognise the production driver's error type.
func TestRepositoryUniqueViolationHelpersUsePgconn(t *testing.T) {
	t.Parallel()

	prodErr := fmt.Errorf("insert row: %w", &pgconn.PgError{Code: "23505"})
	other := errors.New("boom")

	helpers := map[string]func(error) bool{
		"isMBHeadUniqueViolation":        isMBHeadUniqueViolation,
		"isProductGradeUniqueViolation":  isProductGradeUniqueViolation,
		"isCprUniqueViolation":           isCprUniqueViolation,
		"isMBSpinUniqueViolation":        isMBSpinUniqueViolation,
		"isInterminglingUniqueViolation": isInterminglingUniqueViolation,
		"isFormulaUniqueViolation":       isFormulaUniqueViolation,
		"isUniqueViolation":              isUniqueViolation,
		"isRMCategoryUniqueViolation":    isRMCategoryUniqueViolation,
		"isBoxBobbinCostUniqueViolation": isBoxBobbinCostUniqueViolation,
		"isRmTypeUniqueViolation":        isRmTypeUniqueViolation,
		"isUOMCategoryUniqueViolation":   isUOMCategoryUniqueViolation,
		"isProductMasterUniqueViolation": isProductMasterUniqueViolation,
		"isMachineUniqueViolation":       isMachineUniqueViolation,
		"isProductTypeUniqueViolation":   isProductTypeUniqueViolation,
		"isParameterUniqueViolation":     isParameterUniqueViolation,
		"isRouteUniqueViolation":         isRouteUniqueViolation,
	}

	for name, fn := range helpers {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if !fn(prodErr) {
				t.Errorf("%s did not detect *pgconn.PgError 23505 (production driver)", name)
			}
			if fn(other) {
				t.Errorf("%s reported a unique violation for a plain error", name)
			}
			if fn(nil) {
				t.Errorf("%s reported a unique violation for nil", name)
			}
		})
	}
}

// isDuplicateActiveJob additionally requires the constraint name, so it gets its
// own case rather than joining the table above.
func TestIsDuplicateActiveJob(t *testing.T) {
	t.Parallel()

	if !isDuplicateActiveJob(&pgconn.PgError{Code: "23505", ConstraintName: "idx_jobs_active_unique"}) {
		t.Error("did not detect pgconn duplicate active job")
	}
	if !isDuplicateActiveJob(&pq.Error{Code: "23505", Constraint: "idx_jobs_active_unique"}) {
		t.Error("did not detect pq duplicate active job")
	}
	if isDuplicateActiveJob(&pgconn.PgError{Code: "23505", ConstraintName: "uom_code_key"}) {
		t.Error("matched a unique violation on an unrelated constraint")
	}
	if isDuplicateActiveJob(errors.New("boom")) {
		t.Error("matched a plain error")
	}
	if isDuplicateActiveJob(nil) {
		t.Error("matched nil")
	}
}
