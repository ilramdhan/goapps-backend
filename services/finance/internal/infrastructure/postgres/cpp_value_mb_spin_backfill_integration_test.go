// Package postgres_test — proves the safety properties required of migration
// 000495_backfill_cpp_value_mb_spin_id.up.sql (PARTIAL backfill of
// cost_product_parameter.cpp_value_mb_spin_id for legacy MB_SPIN rows).
//
// ⛔ NOT RUN BY THIS AGENT. Written per task instructions to prove the migration's
// safety properties, but gated by INTEGRATION_TEST=true like every other suite in
// this package — it requires a reachable PostgreSQL with migrations 000001..000494
// already applied (cpp_value_mb_spin_id column must exist) and at least one
// non-deleted mst_parameter row with param_code = 'MB_SP_CODE' and at least one
// non-deleted mst_mb_head row. Whoever runs this (CI, or a developer with
// INTEGRATION_TEST=true) is the one actually exercising the migration logic
// against a database — this file only ever WRITES to fixture rows it creates
// itself and hard-deletes in setup/teardown, exactly like the sibling suite in
// mb_spin_repository_integration_test.go.
//
// IMPORTANT — why this test inlines the migration's UPDATE statement instead of
// executing 000495_backfill_cpp_value_mb_spin_id.up.sql directly: golang-migrate
// sends that file to Postgres as a single multi-statement block (see the
// "CREATE INDEX CONCURRENTLY" note in 000490's up migration for why — the DSN
// this repo's migrate runner uses has no x-multi-statement option, so the whole
// file becomes one implicit transaction). This suite's plain database/sql + pgx
// stdlib connection does not run in that mode, so replaying the raw file text
// through it is not reliable. Instead, runBackfillUpdate() below reproduces the
// UP migration's single UPDATE verbatim (same JOIN target, same WHERE clause
// shape, same uniqueness-gating subquery). If 000495_*.up.sql's UPDATE ever
// changes, this copy MUST be updated to match, or this test silently stops
// proving anything about the real migration.
//
// SAFETY: every fixture row this suite creates (mst_mb_spin, cost_product_master,
// cost_product_parameter) is tagged with the ITEST-000495- prefix, which is not a
// shape any real production code, seed, or ORION item code takes. The whole
// prefix range is hard-deleted before and after each test. This suite never
// writes to mst_parameter (it only reads the existing production MB_SP_CODE row,
// exactly like mb_spin_repository_integration_test.go borrows an existing
// mst_mb_head row) and never writes to mst_mb_head.
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
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// cppBackfillFixturePrefix marks every row this suite creates. It must never
// match real production data.
const cppBackfillFixturePrefix = "ITEST-000495-"

// CPPValueMBSpinBackfillSuite exercises the safety properties of the 000495
// backfill against a real DB, without ever touching real production rows.
type CPPValueMBSpinBackfillSuite struct {
	suite.Suite
	db          *postgres.DB
	ctx         context.Context
	headID      uuid.UUID // borrowed mst_mb_head row, never written to
	mbSpinParam uuid.UUID // borrowed mst_parameter row (param_code = MB_SP_CODE), never written to
	typeID      int       // borrowed cost_product_type row, never written to
	nextProduct int       // monotonically increasing suffix for fixture product codes
}

func TestCPPValueMBSpinBackfillSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(CPPValueMBSpinBackfillSuite))
}

func (s *CPPValueMBSpinBackfillSuite) SetupSuite() {
	s.ctx = context.Background()

	host := getEnvOrDefault("TEST_DB_HOST", "localhost")
	port := getEnvOrDefault("TEST_DB_PORT", "5434")
	user := getEnvOrDefault("TEST_DB_USER", "finance")
	password := getEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := getEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitForDB(raw, 10*time.Second))

	s.db = postgres.NewDBFromSQL(raw)

	// mst_mb_head and mst_parameter(MB_SP_CODE) are read-only borrows, exactly
	// like mb_spin_repository_integration_test.go borrows a head. This suite
	// must never INSERT/UPDATE/DELETE either table.
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`,
		).Scan(&s.headID),
		"integration DB needs at least one mst_mb_head row")

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT id FROM mst_parameter WHERE param_code = 'MB_SP_CODE' AND deleted_at IS NULL LIMIT 1`,
		).Scan(&s.mbSpinParam),
		"integration DB needs the production MB_SP_CODE mst_parameter row (schema fact asserted by the orchestrator: exactly 1 non-deleted row)")

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT cpt_type_id FROM cost_product_type LIMIT 1`,
		).Scan(&s.typeID),
		"integration DB needs at least one cost_product_type row")
}

func (s *CPPValueMBSpinBackfillSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *CPPValueMBSpinBackfillSuite) SetupTest()    { s.cleanupFixtures() }
func (s *CPPValueMBSpinBackfillSuite) TearDownTest() { s.cleanupFixtures() }

// cleanupFixtures hard-deletes only this suite's fixture rows, in FK-safe
// order (cost_product_parameter -> cost_product_master, then mst_mb_spin).
// mst_mb_head and mst_parameter are never touched.
func (s *CPPValueMBSpinBackfillSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, `
		DELETE FROM cost_product_parameter
		 WHERE cpp_product_sys_id IN (
		     SELECT cpm_product_sys_id FROM cost_product_master
		      WHERE cpm_product_code LIKE 'ITEST-000495-%'
		 )`)
	require.NoError(s.T(), err)

	_, err = s.db.ExecContext(s.ctx,
		`DELETE FROM cost_product_master WHERE cpm_product_code LIKE 'ITEST-000495-%'`)
	require.NoError(s.T(), err)

	_, err = s.db.ExecContext(s.ctx,
		`DELETE FROM mst_mb_spin WHERE mbs_orion_item_code LIKE 'ITEST-000495-%' OR mbs_mgt_name LIKE 'ITEST-000495-%'`)
	require.NoError(s.T(), err)
}

// insertMBSpin writes one fixture mst_mb_spin row.
func (s *CPPValueMBSpinBackfillSuite) insertMBSpin(id uuid.UUID, orionCode string, deleted bool) {
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_orion_item_code, mbs_is_active,
			created_at, created_by
		) VALUES ($1, $2, $3, $4, TRUE, NOW(), 'itest')`,
		id, s.headID, cppBackfillFixturePrefix+"mgt", orionCode)
	require.NoError(s.T(), err)

	if deleted {
		_, err = s.db.ExecContext(s.ctx,
			`UPDATE mst_mb_spin SET deleted_at = NOW(), deleted_by = 'itest' WHERE mbs_id = $1`, id)
		require.NoError(s.T(), err)
	}
}

// insertCPPRow writes one fixture cost_product_master + cost_product_parameter
// pair, with cpp_value_text = orionCode and cpp_value_mb_spin_id as given
// (nil => NULL, mirroring a legacy unresolved row; non-nil => an already-resolved
// row, mirroring a live-save-path or previous-backfill result).
func (s *CPPValueMBSpinBackfillSuite) insertCPPRow(orionCode string, existing *uuid.UUID) (productID int64, cppRowID int64) {
	s.nextProduct++
	code := fmt.Sprintf("%sP%04d", cppBackfillFixturePrefix, s.nextProduct)

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `
			INSERT INTO cost_product_master (
				cpm_product_code, cpm_product_type_id, cpm_product_name,
				cpm_is_active, cpm_created_at, cpm_created_by, cpm_updated_at, cpm_updated_by
			) VALUES ($1, $2, $3, TRUE, NOW(), 'itest', NOW(), 'itest')
			RETURNING cpm_product_sys_id`,
			code, s.typeID, cppBackfillFixturePrefix+"product",
		).Scan(&productID))

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `
			INSERT INTO cost_product_parameter (
				cpp_product_sys_id, cpp_param_id, cpp_value_text, cpp_value_mb_spin_id,
				cpp_filled_at, cpp_filled_by, cpp_created_at, cpp_created_by
			) VALUES ($1, $2, $3, $4, NOW(), 'itest', NOW(), 'itest')
			RETURNING cpp_value_id`,
			productID, s.mbSpinParam, orionCode, existing,
		).Scan(&cppRowID))

	return productID, cppRowID
}

// getResolvedSpin reads back cpp_value_mb_spin_id for one cost_product_parameter row.
func (s *CPPValueMBSpinBackfillSuite) getResolvedSpin(cppRowID int64) *uuid.UUID {
	var resolved uuid.NullUUID
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT cpp_value_mb_spin_id FROM cost_product_parameter WHERE cpp_value_id = $1`,
			cppRowID,
		).Scan(&resolved))
	if !resolved.Valid {
		return nil
	}
	id := resolved.UUID
	return &id
}

// getValueText reads back cpp_value_text for one cost_product_parameter row, to
// prove the backfill never touches it.
func (s *CPPValueMBSpinBackfillSuite) getValueText(cppRowID int64) string {
	var text string
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT cpp_value_text FROM cost_product_parameter WHERE cpp_value_id = $1`,
			cppRowID,
		).Scan(&text))
	return text
}

// runBackfillUpdate reproduces migration 000495_backfill_cpp_value_mb_spin_id
// .up.sql's UPDATE statement verbatim. See the package doc comment above for
// why this is an inline copy rather than executing the .up.sql file directly,
// and the obligation to keep it in sync with that file.
func (s *CPPValueMBSpinBackfillSuite) runBackfillUpdate() {
	_, err := s.db.ExecContext(s.ctx, `
		UPDATE cost_product_parameter cpp
		   SET cpp_value_mb_spin_id = (
		        SELECT mbs.mbs_id
		          FROM mst_mb_spin mbs
		         WHERE mbs.mbs_orion_item_code = cpp.cpp_value_text
		           AND mbs.deleted_at IS NULL
		       )
		  FROM mst_parameter mp
		 WHERE cpp.cpp_param_id = mp.id
		   AND mp.param_code = 'MB_SP_CODE'
		   AND mp.deleted_at IS NULL
		   AND cpp.cpp_value_mb_spin_id IS NULL
		   AND cpp.cpp_value_text IS NOT NULL
		   AND (
		        SELECT COUNT(*)
		          FROM mst_mb_spin mbs
		         WHERE mbs.mbs_orion_item_code = cpp.cpp_value_text
		           AND mbs.deleted_at IS NULL
		       ) = 1`)
	require.NoError(s.T(), err)
}

// (a) Ambiguous rows (code matches >1 active mst_mb_spin row) MUST be skipped,
// left NULL. This is the single most important guarantee of 000495: a wrong
// guess here would silently lock a product to the wrong MB Spin and feed a
// wrong number into cost calculations.
func (s *CPPValueMBSpinBackfillSuite) TestBackfill_AmbiguousCode_LeftNull() {
	code := cppBackfillFixturePrefix + "DUP-ORION"
	s.insertMBSpin(uuid.New(), code, false)
	s.insertMBSpin(uuid.New(), code, false) // second active spin sharing the same code -> ambiguous

	_, cppRowID := s.insertCPPRow(code, nil)

	s.runBackfillUpdate()

	require.Nil(s.T(), s.getResolvedSpin(cppRowID),
		"ambiguous code must be left NULL, never guessed via LIMIT 1/ORDER BY/DISTINCT ON/MIN()")
}

// (b) cpp_value_text must never change, for both the row the backfill fills and
// the row it skips.
func (s *CPPValueMBSpinBackfillSuite) TestBackfill_NeverTouchesValueText() {
	uniqueCode := cppBackfillFixturePrefix + "UNIQUE-ORION"
	dupCode := cppBackfillFixturePrefix + "DUP2-ORION"

	spinID := uuid.New()
	s.insertMBSpin(spinID, uniqueCode, false)
	s.insertMBSpin(uuid.New(), dupCode, false)
	s.insertMBSpin(uuid.New(), dupCode, false)

	_, resolvableRow := s.insertCPPRow(uniqueCode, nil)
	_, ambiguousRow := s.insertCPPRow(dupCode, nil)

	s.runBackfillUpdate()

	require.Equal(s.T(), uniqueCode, s.getValueText(resolvableRow), "cpp_value_text must be untouched on the resolved row")
	require.Equal(s.T(), dupCode, s.getValueText(ambiguousRow), "cpp_value_text must be untouched on the skipped row")

	resolved := s.getResolvedSpin(resolvableRow)
	require.NotNil(s.T(), resolved)
	require.Equal(s.T(), spinID, *resolved)
	require.Nil(s.T(), s.getResolvedSpin(ambiguousRow))
}

// (c) Running the backfill twice must be safe: the second run must not change
// anything, because cpp_value_mb_spin_id IS NULL already excludes every row the
// first run touched.
func (s *CPPValueMBSpinBackfillSuite) TestBackfill_RunTwice_SecondRunIsNoop() {
	code := cppBackfillFixturePrefix + "IDEMPOTENT-ORION"
	spinID := uuid.New()
	s.insertMBSpin(spinID, code, false)

	_, cppRowID := s.insertCPPRow(code, nil)

	s.runBackfillUpdate()
	firstResult := s.getResolvedSpin(cppRowID)
	require.NotNil(s.T(), firstResult)
	require.Equal(s.T(), spinID, *firstResult)

	s.runBackfillUpdate() // second run: must be a 0-row no-op for this row
	secondResult := s.getResolvedSpin(cppRowID)
	require.NotNil(s.T(), secondResult)
	require.Equal(s.T(), spinID, *secondResult, "second run must not change an already-resolved row")
}

// (d) A row already filled (by the live application save path, or a prior
// backfill run) must never be overwritten by this migration, even if a
// *different* spin now also matches its code uniquely. cpp_value_mb_spin_id
// IS NULL is the guard that makes this true — this test proves it holds even
// when the WHERE clause's other conditions would otherwise match.
func (s *CPPValueMBSpinBackfillSuite) TestBackfill_AlreadyResolvedRow_NeverOverwritten() {
	code := cppBackfillFixturePrefix + "ALREADY-RESOLVED-ORION"
	realSpin := uuid.New()
	decoySpin := uuid.New()
	s.insertMBSpin(realSpin, code, false)
	// decoySpin must be a real mst_mb_spin row too (fk_cpp_value_mb_spin
	// requires cpp_value_mb_spin_id to reference an existing mst_mb_spin.mbs_id)
	// even though it deliberately carries a DIFFERENT orion code — the point is
	// solely to prove the migration does not touch a non-NULL cell, not that
	// the pre-existing value is a plausible resolution for this row's code.
	s.insertMBSpin(decoySpin, cppBackfillFixturePrefix+"ALREADY-RESOLVED-DECOY", false)

	// Row already resolved to decoySpin (simulating a prior save-path/backfill
	// write) even though decoySpin does not actually share this orion code —
	// the point is solely to prove the migration does not touch a non-NULL cell.
	_, cppRowID := s.insertCPPRow(code, &decoySpin)

	s.runBackfillUpdate()

	got := s.getResolvedSpin(cppRowID)
	require.NotNil(s.T(), got)
	require.Equal(s.T(), decoySpin, *got, "pre-existing non-NULL cpp_value_mb_spin_id must never be overwritten")
}
