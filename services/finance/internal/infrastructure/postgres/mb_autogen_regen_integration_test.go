// Package postgres — integration coverage for regenerating an MB's cost_route_rm recipe copy
// when the recipe is re-validated after an edit (previously: generated once, then frozen forever
// — see mb_autogen_repository.go's TransitionWithAutoGen). Lives in the internal (non-_test)
// package because regenerateCostProductRMs is unexported.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// SAFETY: every fixture row is prefixed with ITEST-MBREGEN- and hard-deleted before and after
// each test. Nothing here touches real cost_product_master/cost_route_*/mst_mb_head rows.
package postgres

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
)

const mbRegenFixturePrefix = "ITEST-MBREGEN-"

// MBAutoGenRegenSuite proves that regenerating an MB cost product's recipe rows on re-validate:
//
//	(i)   updates cost_route_rm to the latest composition version,
//	(ii)  never changes the linked cost_product_sys_id,
//	(iii) is idempotent — running it twice does not duplicate rows,
//	and that the mbh_cost_generated_at/_by trail is bumped so the change is never silent.
//
// (iv) — the first-ever-validate creation path (mbh_cost_product_id == 0) — is unchanged code
// (autoGenCostProduct) and is not touched by this suite; it is covered by inspection: the new
// branch in TransitionWithAutoGen only fires when entity.CostProductID() != 0.
type MBAutoGenRegenSuite struct {
	suite.Suite
	db           *DB
	compRepo     *MBCompositionRepository
	headRepo     *MBHeadRepository
	ctx          context.Context
	mbhID        uuid.UUID
	productSysID int64
	groupHeadID  uuid.UUID
	typeID       int32
}

func TestMBAutoGenRegenSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBAutoGenRegenSuite))
}

func mbRegenEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbRegenWaitForDB(db *sql.DB, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var err error
	for time.Now().Before(deadline) {
		if err = db.Ping(); err == nil {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return fmt.Errorf("db not reachable within %s: %w", timeout, err)
}

func (s *MBAutoGenRegenSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbRegenEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbRegenEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbRegenEnvOrDefault("TEST_DB_USER", "finance")
	password := mbRegenEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbRegenEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbRegenWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
	s.compRepo = NewMBCompositionRepository(s.db)
	s.headRepo = NewMBHeadRepository(s.db, s.compRepo)

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).
			Scan(&s.typeID),
		"integration DB needs the MB cost_product_type seed (migration 000450)")
}

func (s *MBAutoGenRegenSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBAutoGenRegenSuite) SetupTest() {
	s.cleanupFixtures()
	s.seedFixtures()
}

func (s *MBAutoGenRegenSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBAutoGenRegenSuite) cleanupFixtures() {
	prefix := mbRegenFixturePrefix + "%"
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_composition_version WHERE mbcv_mbh_id IN (SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_rm WHERE crm_seq_id IN (SELECT crs.crs_seq_id FROM cost_route_seq crs JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id WHERE cpm.cpm_product_code LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_seq WHERE crs_head_id IN (SELECT crh.crh_head_id FROM cost_route_head crh JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id WHERE cpm.cpm_product_code LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_product_sys_id IN (SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cst_rm_group_head WHERE group_code LIKE $1`, prefix)
	require.NoError(s.T(), err)
}

// seedFixtures builds the "already validated once" state that regenerateCostProductRMs assumes:
// a linked cost_product_master + one cost_route_head/seq (the shape autoGenCostProduct always
// creates), plus an mst_mb_head row whose mbh_cost_product_id already points at it.
func (s *MBAutoGenRegenSuite) seedFixtures() {
	s.groupHeadID = uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cst_rm_group_head (group_head_id, group_code, group_name, created_by, updated_by)
		VALUES ($1, $2, 'itest group', 'itest', 'itest')`,
		s.groupHeadID, mbRegenFixturePrefix+"GRP")
	require.NoError(s.T(), err)

	// cpm_product_code is VARCHAR(20); mbRegenFixturePrefix (14 chars) + a 6-char
	// suffix fits exactly, unlike the 8-char suffix used for wider columns below.
	code := mbRegenFixturePrefix + uuid.NewString()[:6]
	err = s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_source, cpm_is_locked, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'itest MB product', 'MB_RECIPE', TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`,
		code, s.typeID).Scan(&s.productSysID)
	require.NoError(s.T(), err)

	var headID int64
	err = s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'COMPLETE', 1, 'itest', 'itest') RETURNING crh_head_id`,
		s.productSysID).Scan(&headID)
	require.NoError(s.T(), err)

	var seqID int64
	err = s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, 'itest', 'itest') RETURNING crs_seq_id`,
		headID, s.productSysID).Scan(&seqID)
	require.NoError(s.T(), err)

	// Simulate the FIRST validate's frozen recipe: one RM line at 60%.
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_group_code, crm_rm_type, crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'GROUP', 0.60, 'itest', 'itest')`,
		seqID, s.productSysID, mbRegenFixturePrefix+"GRP")
	require.NoError(s.T(), err)

	s.mbhID = uuid.New()
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_cost_product_id, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1, $2, $3, 1, 'VALIDATED', 'itest')`,
		s.mbhID, mbRegenFixturePrefix+uuid.NewString()[:8], s.productSysID)
	require.NoError(s.T(), err)

	// Version 1 snapshot (matches the cost_route_rm row above) and version 2 (the edited recipe:
	// same group at a DIFFERENT ratio, proving a re-validate must pick up the edit).
	s.insertVersion(1, "60")
	s.insertVersion(2, "75")
}

func (s *MBAutoGenRegenSuite) insertVersion(version int32, pct string) {
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_composition_version
			(mbcv_mbh_id, mbcv_version, mbcv_validated_at, mbcv_validated_by,
			 mbcv_seq_no, mbcv_group_head_id, mbcv_composition_pct, mbcv_source_type, mbcv_is_carrier)
		VALUES ($1, $2, NOW(), 'itest', 1, $3, $4, 'GROUP', FALSE)`,
		s.mbhID, version, s.groupHeadID, pct)
	require.NoError(s.T(), err)
}

func (s *MBAutoGenRegenSuite) runRegen(version int32) {
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		return s.headRepo.regenerateCostProductRMs(s.ctx, tx, s.mbhID, version, "itest2", s.productSysID)
	})
	require.NoError(s.T(), err)
}

func (s *MBAutoGenRegenSuite) routeRMRatios() []string {
	rows, err := s.db.QueryContext(s.ctx, `
		SELECT crm.crm_route_rm_ratio::text FROM cost_route_rm crm
		JOIN cost_route_seq crs ON crs.crs_seq_id = crm.crm_seq_id
		JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
		WHERE crh.crh_product_sys_id = $1`, s.productSysID)
	require.NoError(s.T(), err)
	defer func() { _ = rows.Close() }()

	var out []string
	for rows.Next() {
		var ratio string
		require.NoError(s.T(), rows.Scan(&ratio))
		out = append(out, ratio)
	}
	require.NoError(s.T(), rows.Err())
	return out
}

// (i) + (iii): re-validating with version 2 replaces the stale 60% row with 75%, and running the
// same regeneration again does not duplicate it.
func (s *MBAutoGenRegenSuite) TestRegenerate_UpdatesRecipe_AndIsIdempotent() {
	s.runRegen(2)

	ratios := s.routeRMRatios()
	require.Len(s.T(), ratios, 1, "must replace, not append, the recipe row")
	// crm_route_rm_ratio is NUMERIC(10,6); Postgres always renders a numeric's
	// ::text cast with the column's full declared scale, so 0.75 reads back as
	// "0.750000" (verified directly: SELECT (0.75::numeric(10,6))::text ->
	// '0.750000'), never the trimmed "0.75".
	require.Equal(s.T(), "0.750000", ratios[0])

	// Run again with the same version — must not duplicate.
	s.runRegen(2)
	ratios = s.routeRMRatios()
	require.Len(s.T(), ratios, 1, "re-running the regeneration must not duplicate rows")
	require.Equal(s.T(), "0.750000", ratios[0])
}

// (ii): cpm_product_sys_id (and therefore mbh_cost_product_id) must never change across a regen.
func (s *MBAutoGenRegenSuite) TestRegenerate_NeverChangesCostProductID() {
	s.runRegen(2)

	var linkedID int64
	err := s.db.QueryRowContext(s.ctx, `SELECT mbh_cost_product_id FROM mst_mb_head WHERE mbh_id = $1`, s.mbhID).Scan(&linkedID)
	require.NoError(s.T(), err)
	require.Equal(s.T(), s.productSysID, linkedID, "regeneration must reuse the same cost product, never create/relink a new one")

	var productCount int
	err = s.db.QueryRowContext(s.ctx, `SELECT COUNT(*) FROM cost_product_master WHERE cpm_product_sys_id = $1`, s.productSysID).Scan(&productCount)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, productCount)
}

// The "regenerated at/by" trail (mbh_cost_generated_at/_by, migration 000445) is bumped on every
// regen — this is the (a) warning-trail half of the fix: an edit never silently moves the numbers.
func (s *MBAutoGenRegenSuite) TestRegenerate_RefreshesGeneratedAtByTrail() {
	var beforeAt sql.NullTime
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_cost_generated_at FROM mst_mb_head WHERE mbh_id = $1`, s.mbhID).
		Scan(&beforeAt))
	require.False(s.T(), beforeAt.Valid, "fixture starts with no generated trail yet")

	s.runRegen(2)

	var afterAt sql.NullTime
	var afterBy sql.NullString
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_cost_generated_at, mbh_cost_generated_by FROM mst_mb_head WHERE mbh_id = $1`, s.mbhID).
		Scan(&afterAt, &afterBy))
	require.True(s.T(), afterAt.Valid, "regeneration must stamp mbh_cost_generated_at so an edit leaves a visible trail")
	require.Equal(s.T(), "itest2", afterBy.String)
}
