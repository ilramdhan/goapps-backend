package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib" // production driver (array binding)
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

const erpMasterTestPrefix = "ZZTEM"

// TestErpMasterRepository_Integration covers AC-11 on a LOCAL database: the
// replica upsert never touches ceg_grade_group, the seed apply fills NULLs
// only, and a re-run is a no-op. It needs Lane A's grade-group migration
// (ceg_grade_group + cst_erp_grade_group_seed) and skips when absent.
func TestErpMasterRepository_Integration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("pgx", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 10*time.Second))
	db := postgres.NewDBFromSQL(raw)
	t.Cleanup(func() { cleanupErpMaster(ctx, t, db) })
	requireGradeGroupSchema(ctx, t, db)
	cleanupErpMaster(ctx, t, db)

	repo := postgres.NewErpMasterRepository(db)
	g1, g2, g3 := erpMasterTestPrefix+"1", erpMasterTestPrefix+"2", erpMasterTestPrefix+"3"

	// Costing already grouped g1 as G9; g2 is new.
	_, err = db.ExecContext(ctx, `INSERT INTO cost_erp_grade (ceg_grade_code, ceg_grade_name, ceg_grade_group) VALUES ($1, 'old', 'G9')`, g1)
	require.NoError(t, err)
	_, err = db.ExecContext(ctx, `INSERT INTO cst_erp_grade_group_seed (cggs_grade_code, cggs_grade_group) VALUES ($1, 'G1'), ($2, 'G2'), ($3, 'G3')`, g1, g2, g3)
	require.NoError(t, err)

	counts, err := repo.UpsertGrades(ctx, []erpintegration.MasterGrade{
		{Code: g1, Name: "renamed", Active: true},
		{Code: g2, Name: "new grade", Active: true},
	})
	require.NoError(t, err)
	require.Equal(t, int64(1), counts.Inserted)
	require.Equal(t, int64(1), counts.Updated)
	require.Equal(t, "G9", gradeGroup(ctx, t, db, g1), "sync must never change ceg_grade_group")
	require.Empty(t, gradeGroup(ctx, t, db, g2), "a new grade arrives with a NULL group")

	rep, err := repo.ApplyGradeGroupSeed(ctx)
	require.NoError(t, err)
	require.GreaterOrEqual(t, rep.AppliedNow, int64(1))
	require.Contains(t, rep.MissingCodes, g3)
	require.Equal(t, "G9", gradeGroup(ctx, t, db, g1), "apply must not overwrite a Costing group")
	require.Equal(t, "G2", gradeGroup(ctx, t, db, g2))

	again, err := repo.UpsertGrades(ctx, []erpintegration.MasterGrade{
		{Code: g1, Name: "renamed", Active: true},
		{Code: g2, Name: "new grade", Active: true},
	})
	require.NoError(t, err)
	require.Equal(t, int64(2), again.Unchanged, "unchanged re-sync is a no-op")
	rerun, err := repo.ApplyGradeGroupSeed(ctx)
	require.NoError(t, err)
	require.Equal(t, int64(0), rerun.AppliedNow, "re-apply is a no-op")
	require.Equal(t, "G9", gradeGroup(ctx, t, db, g1))

	itemCounts, err := repo.UpsertItems(ctx, []erpintegration.MasterItem{{Code: erpMasterTestPrefix + "ITM", Name: "x", Active: true}})
	require.NoError(t, err)
	require.Equal(t, int64(1), itemCounts.Inserted)
}

func requireGradeGroupSchema(ctx context.Context, t *testing.T, db *postgres.DB) {
	t.Helper()
	var n int
	err := db.QueryRowContext(ctx, `
SELECT COUNT(*) FROM information_schema.columns
 WHERE (table_name = 'cost_erp_grade' AND column_name = 'ceg_grade_group')
    OR (table_name = 'cst_erp_grade_group_seed' AND column_name IN ('cggs_grade_code', 'cggs_grade_group'))`).Scan(&n)
	require.NoError(t, err)
	if n < 3 {
		t.Skip("grade-group schema (Lane A migration) not applied on this database")
	}
}

func gradeGroup(ctx context.Context, t *testing.T, db *postgres.DB, code string) string {
	t.Helper()
	var g sql.NullString
	require.NoError(t, db.QueryRowContext(ctx, `SELECT ceg_grade_group FROM cost_erp_grade WHERE ceg_grade_code = $1`, code).Scan(&g))
	return g.String
}

func cleanupErpMaster(ctx context.Context, t *testing.T, db *postgres.DB) {
	t.Helper()
	like := erpMasterTestPrefix + "%"
	for _, q := range []string{
		`DELETE FROM cst_erp_grade_group_seed WHERE cggs_grade_code LIKE $1`,
		`DELETE FROM cost_erp_grade WHERE ceg_grade_code LIKE $1`,
		`DELETE FROM cost_erp_item WHERE cei_item_code LIKE $1`,
	} {
		if _, err := db.ExecContext(ctx, q, like); err != nil {
			t.Logf("cleanup (%s): %v", q, err)
		}
	}
}
