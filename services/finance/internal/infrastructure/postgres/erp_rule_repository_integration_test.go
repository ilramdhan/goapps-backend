// Integration test for the ERP rule master repositories (plan-03 P2-T3).
// LOCAL PostgreSQL only, via internal/testutil/pgcontainer (a throwaway
// postgres:16-alpine container, or a LOCAL TEST_DATABASE_URL). Migrations
// 000001..latest are applied to a fresh database.
//
// Skipped unless INTEGRATION_TEST=true.
package postgres_test

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/internal/testutil/pgcontainer"
)

func TestErpRuleRepositoriesIntegration(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	ctx := context.Background()
	srv := pgcontainer.Start(ctx, t)
	raw := srv.CreateDatabase(t, "erp_rule_it", "")
	mig, err := pgcontainer.NewMigrator(ctx, raw, "../../../migrations/postgres")
	require.NoError(t, err)
	require.NoError(t, mig.Up(ctx))
	db := postgres.NewDBFromSQL(raw)

	t.Run("LoadRuleSet on fresh seed", func(t *testing.T) { testLoadRuleSetFreshSeed(ctx, t, db, raw) })
	t.Run("valloss rule CRUD", func(t *testing.T) { testVallossRuleCRUD(ctx, t, db) })
	t.Run("sell price", func(t *testing.T) { testSellPrice(ctx, t, db) })
	t.Run("grade group", func(t *testing.T) { testGradeGroup(ctx, t, db, raw) })
	t.Run("rule hash usage", func(t *testing.T) { testRuleHashUsage(ctx, t, db, raw) })
	t.Run("LoadRuleSet snapshot", func(t *testing.T) { testLoadRuleSetSnapshot(ctx, t, db, raw) })
}

// testLoadRuleSetFreshSeed: 115 rules / 3 prices from 000548. cost_erp_grade
// is empty after migrations (as in prod, F-2), so 0 grades are grouped until
// the replica has rows and the P0-T15b seed apply runs; then 22.
func testLoadRuleSetFreshSeed(ctx context.Context, t *testing.T, db *postgres.DB, raw *sql.DB) {
	loader := postgres.NewErpRuleSetLoader(db)
	rs, err := loader.LoadRuleSet(ctx)
	require.NoError(t, err)
	assert.Equal(t, 115, rs.RuleCount())
	assert.Equal(t, 3, rs.PriceCount())
	assert.Equal(t, 0, rs.GradeCount(), "empty replica: no grade groups yet")
	assert.Empty(t, rs.AmbiguousKeys())
	assert.Empty(t, rs.RulesMissingPrice())

	// Replica rows for every seed code, then the P0-T15b apply.
	_, err = raw.ExecContext(ctx, `
		INSERT INTO cost_erp_grade (ceg_grade_code, ceg_grade_name)
		SELECT cggs_grade_code, 'grade ' || cggs_grade_code FROM cst_erp_grade_group_seed`)
	require.NoError(t, err)
	rep, err := postgres.NewErpMasterRepository(db).ApplyGradeGroupSeed(ctx)
	require.NoError(t, err)
	assert.Equal(t, int64(22), rep.AppliedNow)

	rs, err = loader.LoadRuleSet(ctx)
	require.NoError(t, err)
	assert.Equal(t, 115, rs.RuleCount())
	assert.Equal(t, 3, rs.PriceCount())
	assert.Equal(t, 22, rs.GradeCount())

	// Spot checks against the 000548 seed.
	basis, loss, err := rs.Loss("Type 1", erprule.ProdTypePOY, erprule.GradeGroupBC)
	require.NoError(t, err)
	assert.Equal(t, erprule.BasisSPPTY, basis)
	assert.True(t, loss.Equal(decimal.RequireFromString("0.5")), loss.String())
	price, err := rs.SellPrice(erprule.BasisSPITY)
	require.NoError(t, err)
	assert.True(t, price.Equal(decimal.RequireFromString("1.5")), price.String())
	g, ok := rs.GradeGroupOf("AXa")
	assert.True(t, ok)
	assert.Equal(t, erprule.GradeGroupPOYA, g)
	g, ok = rs.GradeGroupOf("AX")
	assert.True(t, ok)
	assert.Equal(t, erprule.GradeGroupAX, g)
	_, _, err = rs.Loss("Type 99", erprule.ProdTypePOY, erprule.GradeGroupBC)
	assert.ErrorIs(t, err, erprule.ErrRuleMissing)
}

func testVallossRuleCRUD(ctx context.Context, t *testing.T, db *postgres.DB) {
	repo := postgres.NewErpVallossRuleRepository(db)
	now := time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)
	key, err := erprule.NewRuleKey("Type 77", "PTY", "BB")
	require.NoError(t, err)

	rule, err := erprule.NewVallossRule(key, erprule.BasisSPBSD, decimal.RequireFromString("12.345678"), "alice", now)
	require.NoError(t, err)
	created, err := repo.Create(ctx, rule)
	require.NoError(t, err)
	require.Positive(t, created.ID())
	assert.Equal(t, key, created.Key())
	assert.Equal(t, erprule.BasisSPBSD, created.Basis())
	assert.True(t, created.ValLoss().Equal(decimal.RequireFromString("12.345678")), "6 dp round trip: %s", created.ValLoss())
	assert.True(t, created.IsActive())
	assert.Equal(t, "alice", created.CreatedBy())
	assert.Nil(t, created.UpdatedAt())

	// A second active rule on the same key -> ErrDuplicateRule.
	dup, err := erprule.NewVallossRule(key, erprule.BasisCost, decimal.Zero, "bob", now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, dup)
	assert.ErrorIs(t, err, erprule.ErrDuplicateRule)

	// A seeded key clashes too.
	seedKey, err := erprule.NewRuleKey("Type 1", "POY", "BC")
	require.NoError(t, err)
	seedDup, err := erprule.NewVallossRule(seedKey, erprule.BasisCost, decimal.Zero, "bob", now)
	require.NoError(t, err)
	_, err = repo.Create(ctx, seedDup)
	assert.ErrorIs(t, err, erprule.ErrDuplicateRule)

	got, err := repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	assert.Equal(t, created.Key(), got.Key())
	_, err = repo.GetByID(ctx, 99_999_999)
	assert.ErrorIs(t, err, erprule.ErrRuleNotFound)

	// Update.
	later := now.Add(time.Hour)
	require.NoError(t, got.Update(erprule.BasisCost, decimal.RequireFromString("0.05"), "carol", later))
	require.NoError(t, repo.Update(ctx, got))
	got, err = repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	assert.Equal(t, erprule.BasisCost, got.Basis())
	assert.True(t, got.ValLoss().Equal(decimal.RequireFromString("0.05")))
	require.NotNil(t, got.UpdatedAt())
	assert.True(t, got.UpdatedAt().Equal(later))
	assert.Equal(t, "carol", got.UpdatedBy())

	// List filters.
	list, total, err := repo.List(ctx, erprule.VallossRuleFilter{FgType: "Type 77"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, list, 1)
	_, total, err = repo.List(ctx, erprule.VallossRuleFilter{})
	require.NoError(t, err)
	assert.Equal(t, int64(116), total)
	page, total, err := repo.List(ctx, erprule.VallossRuleFilter{ProdType: erprule.ProdTypePOY, Basis: erprule.BasisSPPTY, Page: 2, PageSize: 5})
	require.NoError(t, err)
	assert.Positive(t, total)
	assert.LessOrEqual(t, len(page), 5)
	for _, r := range page {
		assert.Equal(t, erprule.ProdTypePOY, r.Key().ProdType)
		assert.Equal(t, erprule.BasisSPPTY, r.Basis())
	}
	first, _, err := repo.List(ctx, erprule.VallossRuleFilter{PageSize: 1})
	require.NoError(t, err)
	require.Len(t, first, 1)
	assert.Equal(t, "Type 1", first[0].Key().FgType.String(), "ordered by fg/prod/group")

	// Soft delete.
	require.NoError(t, got.Deactivate("dave", later.Add(time.Hour)))
	require.NoError(t, repo.Update(ctx, got))
	got, err = repo.GetByID(ctx, created.ID())
	require.NoError(t, err)
	assert.False(t, got.IsActive())
	_, total, err = repo.List(ctx, erprule.VallossRuleFilter{FgType: "Type 77"})
	require.NoError(t, err)
	assert.Equal(t, int64(0), total, "inactive rules hidden by default")
	_, total, err = repo.List(ctx, erprule.VallossRuleFilter{FgType: "Type 77", IncludeInactive: true})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)

	// The key is free again; the inactive row stays as history.
	again, err := erprule.NewVallossRule(key, erprule.BasisSPPTY, decimal.RequireFromString("0.4"), "erin", now)
	require.NoError(t, err)
	recreated, err := repo.Create(ctx, again)
	require.NoError(t, err)
	assert.NotEqual(t, created.ID(), recreated.ID())

	// Re-activating the old row would clash with the new active rule: the
	// stale (active) domain value is refused as inactive, not resurrected.
	stale := erprule.ReconstructVallossRule(created.ID(), key, erprule.BasisCost, decimal.Zero, true,
		now, "alice", nil, "")
	assert.ErrorIs(t, repo.Update(ctx, stale), erprule.ErrRuleInactive)

	// Update of an unknown id.
	ghost := erprule.ReconstructVallossRule(99_999_999, key, erprule.BasisCost, decimal.Zero, true, now, "x", nil, "")
	assert.ErrorIs(t, repo.Update(ctx, ghost), erprule.ErrRuleNotFound)
}

func testSellPrice(ctx context.Context, t *testing.T, db *postgres.DB) {
	repo := postgres.NewErpSellPriceRepository(db)
	now := time.Date(2026, 9, 29, 11, 0, 0, 0, time.UTC)

	all, err := repo.List(ctx, false)
	require.NoError(t, err)
	require.Len(t, all, 3)
	assert.Equal(t, []erprule.Basis{erprule.BasisSPBSD, erprule.BasisSPITY, erprule.BasisSPPTY},
		[]erprule.Basis{all[0].Basis(), all[1].Basis(), all[2].Basis()})

	p, err := repo.Get(ctx, erprule.BasisSPPTY)
	require.NoError(t, err)
	assert.True(t, p.Price().Equal(decimal.RequireFromString("1.3")))
	assert.Equal(t, "migration:000548", p.CreatedBy())
	_, err = repo.Get(ctx, erprule.BasisCost)
	assert.ErrorIs(t, err, erprule.ErrSellPriceNotFound)

	// Update an existing basis: created_* kept, updated_* set.
	require.NoError(t, p.UpdatePrice(decimal.RequireFromString("1.234567"), "alice", now))
	require.NoError(t, repo.Upsert(ctx, p))
	p, err = repo.Get(ctx, erprule.BasisSPPTY)
	require.NoError(t, err)
	assert.True(t, p.Price().Equal(decimal.RequireFromString("1.234567")), p.Price().String())
	assert.Equal(t, "migration:000548", p.CreatedBy())
	assert.Equal(t, "alice", p.UpdatedBy())
	require.NotNil(t, p.UpdatedAt())
	assert.True(t, p.UpdatedAt().Equal(now))

	// Deactivate directly (no domain path yet), list hides it, upsert re-activates.
	_, err = db.ExecContext(ctx, `UPDATE cst_erp_sell_price SET cesp_is_active = FALSE WHERE cesp_basis = 'SPBSD'`)
	require.NoError(t, err)
	active, err := repo.List(ctx, false)
	require.NoError(t, err)
	assert.Len(t, active, 2)
	withInactive, err := repo.List(ctx, true)
	require.NoError(t, err)
	assert.Len(t, withInactive, 3)
	bsd, err := repo.Get(ctx, erprule.BasisSPBSD)
	require.NoError(t, err)
	assert.False(t, bsd.IsActive())
	require.NoError(t, bsd.UpdatePrice(decimal.RequireFromString("1.4"), "bob", now))
	require.NoError(t, repo.Upsert(ctx, bsd))
	bsd, err = repo.Get(ctx, erprule.BasisSPBSD)
	require.NoError(t, err)
	assert.True(t, bsd.IsActive())

	// A NewSellPrice upserted over an existing basis updates it and stamps
	// the acting user as updater.
	fresh, err := erprule.NewSellPrice(erprule.BasisSPITY, decimal.RequireFromString("1.55"), "carol", now)
	require.NoError(t, err)
	require.NoError(t, repo.Upsert(ctx, fresh))
	ity, err := repo.Get(ctx, erprule.BasisSPITY)
	require.NoError(t, err)
	assert.True(t, ity.Price().Equal(decimal.RequireFromString("1.55")))
	assert.Equal(t, "migration:000548", ity.CreatedBy())
	assert.Equal(t, "carol", ity.UpdatedBy())
	assert.NotNil(t, ity.UpdatedAt())

	// Restore seed values for later subtests.
	_, err = db.ExecContext(ctx, `UPDATE cst_erp_sell_price SET cesp_price = CASE cesp_basis
		WHEN 'SPPTY' THEN 1.3 WHEN 'SPITY' THEN 1.5 ELSE 1.4 END`)
	require.NoError(t, err)
}

func testGradeGroup(ctx context.Context, t *testing.T, db *postgres.DB, raw *sql.DB) {
	repo := postgres.NewErpGradeGroupRepository(db)
	_, err := raw.ExecContext(ctx, `
		INSERT INTO cost_erp_grade (ceg_grade_code, ceg_grade_name, ceg_is_active, ceg_synced_at)
		VALUES ('ZZ_NEW', 'New 100%', TRUE, '2026-01-01T00:00:00Z'),
		       ('ZZNEW2', NULL, FALSE, '2026-01-01T00:00:00Z')`)
	require.NoError(t, err)

	g, err := repo.GetGrade(ctx, "ZZ_NEW")
	require.NoError(t, err)
	assert.Equal(t, "New 100%", g.Name())
	assert.True(t, g.IsActive())
	assert.False(t, g.HasGroup())
	_, err = repo.GetGrade(ctx, "NOPE")
	assert.ErrorIs(t, err, erprule.ErrGradeNotFound)

	// V-08 worklist.
	list, total, err := repo.ListGrades(ctx, erprule.GradeFilter{UnassignedOnly: true})
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	require.Len(t, list, 2)
	assert.Equal(t, "ZZNEW2", list[0].Code(), "ordered by code")
	assert.Empty(t, list[0].Name(), "NULL name scans as empty")

	// Search matches LIKE metacharacters literally: "_" does not match "N".
	list, total, err = repo.ListGrades(ctx, erprule.GradeFilter{Search: "zz_"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, list, 1)
	assert.Equal(t, "ZZ_NEW", list[0].Code())
	_, total, err = repo.ListGrades(ctx, erprule.GradeFilter{Search: "100%"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total, "search on name")
	all, total, err := repo.ListGrades(ctx, erprule.GradeFilter{Page: 1, PageSize: 10})
	require.NoError(t, err)
	assert.Equal(t, int64(24), total)
	assert.Len(t, all, 10)

	// Assign, then clear: only ceg_grade_group changes.
	prev, err := g.AssignGroup(erprule.GradeGroupBC, "alice")
	require.NoError(t, err)
	assert.Nil(t, prev)
	require.NoError(t, repo.SetGradeGroup(ctx, g))
	g, err = repo.GetGrade(ctx, "ZZ_NEW")
	require.NoError(t, err)
	require.NotNil(t, g.Group())
	assert.Equal(t, erprule.GradeGroupBC, *g.Group())
	var (
		name   string
		active bool
		synced time.Time
	)
	require.NoError(t, raw.QueryRowContext(ctx,
		`SELECT ceg_grade_name, ceg_is_active, ceg_synced_at FROM cost_erp_grade WHERE ceg_grade_code = 'ZZ_NEW'`).
		Scan(&name, &active, &synced))
	assert.Equal(t, "New 100%", name)
	assert.True(t, active)
	assert.True(t, synced.Equal(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)), "replica columns untouched")

	_, err = g.ClearGroup("bob")
	require.NoError(t, err)
	require.NoError(t, repo.SetGradeGroup(ctx, g))
	g, err = repo.GetGrade(ctx, "ZZ_NEW")
	require.NoError(t, err)
	assert.False(t, g.HasGroup())

	ghost := erprule.ReconstructGrade("NOPE", "", true, nil)
	assert.ErrorIs(t, repo.SetGradeGroup(ctx, ghost), erprule.ErrGradeNotFound)
}

func testRuleHashUsage(ctx context.Context, t *testing.T, db *postgres.DB, raw *sql.DB) {
	loader := postgres.NewErpRuleSetLoader(db)
	hash := func(c byte) string {
		b := make([]byte, 64)
		for i := range b {
			b[i] = c
		}
		return string(b)
	}
	insert := func(period string, seq int, status, h string) {
		_, err := raw.ExecContext(ctx, `
			INSERT INTO cst_erp_int_batch (ceib_period, ceib_seq, ceib_status, ceib_rule_hash, created_by)
			VALUES ($1, $2, $3, $4, 'integ-test')`, period, seq, status, h)
		require.NoError(t, err)
	}
	insert("202601", 1, "DERIVED", hash('a'))
	insert("202602", 1, "VALUATED", hash('b'))
	insert("202603", 1, "SUPERSEDED", hash('c'))
	insert("202604", 1, "LOCKED", hash('d'))
	insert("202605", 1, "DRAFT", hash('e'))

	for c, want := range map[byte]bool{'a': true, 'b': true, 'c': false, 'd': false, 'e': false, 'f': false} {
		used, err := loader.IsRuleHashInUse(ctx, hash(c))
		require.NoError(t, err)
		assert.Equal(t, want, used, "hash %c", c)
	}
	used, err := loader.IsRuleHashInUse(ctx, "")
	require.NoError(t, err)
	assert.False(t, used)
}

// testLoadRuleSetSnapshot: a rule committed by another session while the
// loader's snapshot is open is not seen; and invalid stored data fails the load.
func testLoadRuleSetSnapshot(ctx context.Context, t *testing.T, db *postgres.DB, raw *sql.DB) {
	loader := postgres.NewErpRuleSetLoader(db)
	before, err := loader.LoadRuleSet(ctx)
	require.NoError(t, err)

	// Snapshot semantics of the same statement mix, driven by hand: open a
	// REPEATABLE READ tx, read once, commit a new rule elsewhere, re-read.
	tx, err := raw.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	require.NoError(t, err)
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // read-only tx
	var n1, n2 int
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cst_erp_valloss_rule WHERE cevr_is_active`).Scan(&n1))
	_, err = raw.ExecContext(ctx, `INSERT INTO cst_erp_valloss_rule
		(cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss, created_by)
		VALUES ('Type 88', 'ITY', 'NS', 'COST', 0, 'integ-test')`)
	require.NoError(t, err)
	require.NoError(t, tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM cst_erp_valloss_rule WHERE cevr_is_active`).Scan(&n2))
	assert.Equal(t, n1, n2, "REPEATABLE READ hides concurrent commits")

	after, err := loader.LoadRuleSet(ctx)
	require.NoError(t, err)
	assert.Equal(t, before.RuleCount()+1, after.RuleCount())

	// A group written outside GoApps that the domain does not know fails the load.
	_, err = raw.ExecContext(ctx, `UPDATE cost_erp_grade SET ceg_grade_group = 'BOGUS' WHERE ceg_grade_code = 'ZZNEW2'`)
	require.NoError(t, err)
	_, err = loader.LoadRuleSet(ctx)
	assert.ErrorIs(t, err, erprule.ErrInvalidGradeGroup)
	_, err = raw.ExecContext(ctx, `UPDATE cost_erp_grade SET ceg_grade_group = NULL WHERE ceg_grade_code = 'ZZNEW2'`)
	require.NoError(t, err)
}
