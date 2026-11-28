package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"

	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Gated by INTEGRATION_TEST=true. Fixtures prefixed ITEST-CMBVS- and hard-deleted after.
func TestRecipeFullExport_CMBAndVSFallback(t *testing.T) {
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
	defer func() { _ = db.Close() }()

	const prefix = "ITEST-CMBVS-"
	exec := func(q string, a ...any) {
		_, e := db.ExecContext(ctx, q, a...)
		require.NoError(t, e)
	}
	clean := func() {
		exec(`DELETE FROM mst_mb_spin WHERE mbs_mgt_name LIKE $1`, prefix+"%")
		exec(`DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix+"%")
	}
	clean()
	defer clean()

	head := func(code string, orion, vs *string) uuid.UUID {
		id := uuid.New()
		exec(`INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_orion_item_code, mbh_vs_number, mbh_current_version, mbh_entry_status, created_by)
			VALUES ($1,$2,$3,$4,1,'VALIDATED','itest')`, id, prefix+code, orion, vs)
		return id
	}
	// age orders spins: larger = older.
	spin := func(h uuid.UUID, tag string, orion, vs *string, active bool, deleted bool, ageH int) {
		exec(`INSERT INTO mst_mb_spin (mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code, mbs_vs_number,
				mbs_is_active, deleted_at, created_at, created_by)
			VALUES ($1,$2,$3,$3,$4,$5,$6, CASE WHEN $7 THEN NOW() END, NOW() - make_interval(hours => $8),'itest')`,
			uuid.New(), h, prefix+tag, orion, vs, active, deleted, ageH)
	}
	p := func(s string) *string { return &s }

	hA := head("A", p("HEADCMB"), p("HEADVS")) // head wins
	spin(hA, "A1", p("SPINCMB"), p("SPINVS"), true, false, 1)
	hB := head("B", p(" "), p("0"))                              // blank / '0' -> oldest active spin
	spin(hB, "B-del", p("DELCMB"), p("DELVS"), true, true, 100)  // soft-deleted: skipped
	spin(hB, "B-off", p("OFFCMB"), p("OFFVS"), false, false, 90) // inactive: skipped
	spin(hB, "B-old", p("OLDCMB"), p("OLDVS"), true, false, 10)  // oldest active: wins
	spin(hB, "B-new", p("NEWCMB"), p("NEWVS"), true, false, 1)   // newer: ignored
	head("C", nil, p("0"))                                       // no spin -> ''

	rows, err := postgres.NewMBRecipeFullExportRepository(db).ListRecipeFullRows(ctx,
		appmbhead.RecipeFullFilter{CostType: "ACTUAL", IncludeRejected: true})
	require.NoError(t, err)
	got := map[string][2]string{}
	for _, r := range rows {
		if len(r.MBCosting) > len(prefix) && r.MBCosting[:len(prefix)] == prefix {
			c, v := "", ""
			if r.CMBCode != nil {
				c = *r.CMBCode
			}
			if r.VSNumber != nil {
				v = *r.VSNumber
			}
			got[r.MBCosting[len(prefix):]] = [2]string{c, v}
		}
	}
	require.Equal(t, [2]string{"HEADCMB", "HEADVS"}, got["A"])
	require.Equal(t, [2]string{"OLDCMB", "OLDVS"}, got["B"])
	require.Equal(t, [2]string{"", ""}, got["C"])
}
