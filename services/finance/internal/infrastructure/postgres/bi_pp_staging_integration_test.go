package postgres_test

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/require"

	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// Gated by INTEGRATION_TEST=true. Applies migration 000600 (idempotent) and verifies ETL§C
// replace-in-window semantics: two identical runs give identical staging, rows that vanished from
// the source vanish, rows outside the window are kept, NULLs and decimals round-trip exactly.
func TestBiPPStagingRepository_ReplacePackProdDaily(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		getEnvOrDefault("TEST_DB_HOST", "localhost"), getEnvOrDefault("TEST_DB_PORT", "5434"),
		getEnvOrDefault("TEST_DB_USER", "finance"), getEnvOrDefault("TEST_DB_PASSWORD", "finance123"),
		getEnvOrDefault("TEST_DB_NAME", "finance_db"))
	raw, err := sql.Open("postgres", dsn)
	require.NoError(t, err)
	require.NoError(t, waitForDB(raw, 30*time.Second))
	defer func() { _ = raw.Close() }()

	ddl, err := os.ReadFile("../../../migrations/postgres/000600_create_bi_pp_stg_pack_prod_daily.up.sql")
	require.NoError(t, err)
	_, err = raw.ExecContext(ctx, string(ddl))
	require.NoError(t, err)

	// Fixture dates far in the past (view history starts 2003) so real data is never touched.
	cleanup := func() { cleanupPPFixtures(ctx, t, raw) }
	cleanup()
	defer cleanup()

	d := func(s string) time.Time { v, _ := time.Parse(time.DateOnly, s); return v }
	str := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	row := func(date, mc, ax string) oracleinfra.PackProdDailyRow {
		axQ := decimal.RequireFromString(ax)
		fq := decimal.RequireFromString("0.01")
		return oracleinfra.PackProdDailyRow{
			PackDate: d(date), WeekStart: d("1999-01-04"), MonthStart: d("1999-01-01"),
			Period: "199901", WeekPeriod: "199901", DeptID: "TWT", DeptName: "TWISTING", ProdGroup: "TWT",
			TransactionType: str("PACKING"), GroupTypeOrder: sql.NullInt64{Int64: 2, Valid: true},
			McNo: str(mc), MergeNo: str("M1"), GradeEligible: "Y",
			AxQty: axQ, FqQty: fq, TotalQty: axQ.Add(fq),
		}
	}
	repo := postgres.NewBiPPStagingRepository(&postgres.DB{DB: raw})
	srcID := hrisOracleSourceID(ctx, t, raw)

	type snap struct {
		Date, Mc, Ax, Fq, Dg, Total string
		Pallet                      sql.NullString
		Order                       sql.NullInt64
	}
	read := func() []snap {
		rows, qErr := raw.QueryContext(ctx, `SELECT pack_date::text, mc_no, ax_qty::text, fq_qty::text, dg_qty::text,
			total_qty::text, pallet_code, group_type_order
			FROM bi_pp_stg_pack_prod_daily WHERE pack_date < '2000-01-01' ORDER BY pack_date, mc_no`)
		require.NoError(t, qErr)
		defer func() { _ = rows.Close() }()
		var out []snap
		for rows.Next() {
			var s snap
			require.NoError(t, rows.Scan(&s.Date, &s.Mc, &s.Ax, &s.Fq, &s.Dg, &s.Total, &s.Pallet, &s.Order))
			out = append(out, s)
		}
		require.NoError(t, rows.Err())
		return out
	}

	// Seed a row outside the later window; it must survive.
	n, err := repo.ReplacePackProdDaily(ctx, d("1999-01-01"), d("1999-01-01"), []oracleinfra.PackProdDailyRow{row("1999-01-01", "MC0", "1")}, srcID)
	require.NoError(t, err)
	require.Equal(t, 1, n)

	src := []oracleinfra.PackProdDailyRow{
		row("1999-01-02", "MC1", "1234.56"),
		row("1999-01-02", "MC2", "0"),
		row("1999-01-03", "MC1", "0.5"),
	}
	n, err = repo.ReplacePackProdDaily(ctx, d("1999-01-02"), d("1999-01-05"), src, srcID)
	require.NoError(t, err)
	require.Equal(t, 3, n)
	first := read()
	require.Len(t, first, 4)
	require.Equal(t, "1234.560000", first[1].Ax)
	require.Equal(t, "1234.570000", first[1].Total)
	require.Equal(t, "0.000000", first[1].Dg)
	require.False(t, first[1].Pallet.Valid, "NULL pallet_code must stay NULL")
	require.Equal(t, int64(2), first[1].Order.Int64)

	// Second identical run: identical staging, no duplicates.
	_, err = repo.ReplacePackProdDaily(ctx, d("1999-01-02"), d("1999-01-05"), src, srcID)
	require.NoError(t, err)
	require.Equal(t, first, read())

	// A combination dropped by the source disappears; the row outside the window stays.
	_, err = repo.ReplacePackProdDaily(ctx, d("1999-01-02"), d("1999-01-05"), src[:1], srcID)
	require.NoError(t, err)
	got := read()
	require.Len(t, got, 2)
	require.Equal(t, "1999-01-01", got[0].Date)
	require.Equal(t, "MC1", got[1].Mc)

	// Batching: more rows than one INSERT holds.
	many := make([]oracleinfra.PackProdDailyRow, 0, 1203)
	for i := range 1203 {
		many = append(many, row("1999-01-04", fmt.Sprintf("B%04d", i), "1"))
	}
	n, err = repo.ReplacePackProdDaily(ctx, d("1999-01-02"), d("1999-01-05"), many, srcID)
	require.NoError(t, err)
	require.Equal(t, 1203, n)
	require.Len(t, read(), 1204)

	// CountPackProdDaily (fail-closed guard, PP-13): inclusive window bounds, empty window = 0.
	c, err := repo.CountPackProdDaily(ctx, d("1999-01-02"), d("1999-01-05"))
	require.NoError(t, err)
	require.Equal(t, 1203, c)
	c, err = repo.CountPackProdDaily(ctx, d("1999-01-01"), d("1999-01-01"))
	require.NoError(t, err)
	require.Equal(t, 1, c, "bounds are inclusive; row outside the later window still counted")
	c, err = repo.CountPackProdDaily(ctx, d("1999-02-01"), d("1999-02-28"))
	require.NoError(t, err)
	require.Equal(t, 0, c)
}
