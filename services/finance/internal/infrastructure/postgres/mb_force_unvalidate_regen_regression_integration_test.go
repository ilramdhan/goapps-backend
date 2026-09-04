// Package postgres — regression coverage for the Bulk MB Head Regenerate bug: before the
// fix in mb_force_unvalidate.go, ForceUnvalidateTransition NULLed out
// mbh_cost_product_id/mbh_cost_generated_at/mbh_cost_generated_by, which forced the very
// next Validate to always take TransitionWithAutoGen's FULL autoGenCostProduct path
// (mb_autogen_repository.go) instead of the lighter regenerateCostProductRMs path — even
// though the head's cost product already existed. That duplicated cost_product_master and
// mst_mb_spin rows on every single "Regenerate Selected" click, and even re-generated MB
// Spin rows a human had already locked as actual (mbs_ldr_is_actual = TRUE).
//
// This suite proves the full ForceUnvalidateTransition -> TransitionWithAutoGen(Validate)
// round-trip no longer duplicates cost_product_master or mst_mb_spin rows, and that a
// locked-as-actual spin row survives untouched.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB: postgres://finance:finance123@localhost:5434/finance_db).
//
// SAFETY: every fixture row is prefixed with ITEST-MBFUR- and hard-deleted before and
// after each test. Nothing here touches real mst_mb_head/mst_mb_spin/cost_product_master
// rows.
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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// mbForceUnvalRegenFixturePrefix is kept to 12 chars: cost_product_master.cpm_product_code
// is VARCHAR(20) and the product code below appends a 6-char uuid suffix (mirroring
// mb_autogen_regen_integration_test.go's identical constraint), so prefix+suffix must fit
// in 20 chars exactly.
const mbForceUnvalRegenFixturePrefix = "ITEST-MBFUR-"

// MBForceUnvalidateRegenRegressionSuite proves that ForceUnvalidateTransition followed by
// TransitionWithAutoGen(toState=StatusValidated) — the exact sequence the Bulk MB Head
// Regenerate feature runs per head (ForceUnvalidate -> Submit -> Validate) — regenerates
// the existing cost product in place instead of creating a new one, and never touches a
// spin row already locked as actual.
type MBForceUnvalidateRegenRegressionSuite struct {
	suite.Suite
	db       *DB
	compRepo *MBCompositionRepository
	headRepo *MBHeadRepository
	ctx      context.Context
	typeID   int32
}

func TestMBForceUnvalidateRegenRegressionSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBForceUnvalidateRegenRegressionSuite))
}

func mbForceUnvalRegenEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbForceUnvalRegenWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBForceUnvalidateRegenRegressionSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbForceUnvalRegenEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbForceUnvalRegenEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbForceUnvalRegenEnvOrDefault("TEST_DB_USER", "finance")
	password := mbForceUnvalRegenEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbForceUnvalRegenEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbForceUnvalRegenWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
	s.compRepo = NewMBCompositionRepository(s.db)
	s.headRepo = NewMBHeadRepository(s.db, s.compRepo)

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).
			Scan(&s.typeID),
		"integration DB needs the MB cost_product_type seed (migration 000450)")
}

func (s *MBForceUnvalidateRegenRegressionSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBForceUnvalidateRegenRegressionSuite) SetupTest() { s.cleanupFixtures() }

func (s *MBForceUnvalidateRegenRegressionSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBForceUnvalidateRegenRegressionSuite) cleanupFixtures() {
	prefix := mbForceUnvalRegenFixturePrefix + "%"
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_workflow_log WHERE mbwl_mbh_id IN (
		SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_spin WHERE mbs_mbh_id IN (
		SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_composition_version WHERE mbcv_mbh_id IN (
		SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_composition WHERE mbcm_mbh_id IN (
		SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_rm WHERE crm_seq_id IN (
		SELECT crs.crs_seq_id FROM cost_route_seq crs
		JOIN cost_route_head crh ON crh.crh_head_id = crs.crs_head_id
		JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
		WHERE cpm.cpm_product_code LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_seq WHERE crs_head_id IN (
		SELECT crh.crh_head_id FROM cost_route_head crh
		JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = crh.crh_product_sys_id
		WHERE cpm.cpm_product_code LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_route_head WHERE crh_product_sys_id IN (
		SELECT cpm_product_sys_id FROM cost_product_master WHERE cpm_product_code LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cst_rm_group_head WHERE group_code LIKE $1`, prefix)
	require.NoError(s.T(), err)
}

// mbFURegenFixture bundles the IDs seeded for one test run.
type mbFURegenFixture struct {
	mbhID        uuid.UUID
	productSysID int64
	groupHeadID  uuid.UUID
}

// seedValidatedHeadWithSpin builds the "already validated once, with a locked-actual spin"
// state the Bulk MB Head Regenerate feature operates on: a linked cost_product_master with
// one cost_route_head/seq/rm (the shape autoGenCostProduct always creates), one mst_mb_spin
// row already locked as actual, an mst_mb_head row whose mbh_cost_product_id already points
// at the product, and one mst_mb_composition row so SnapshotVersion has something to freeze
// on the subsequent re-validate.
func (s *MBForceUnvalidateRegenRegressionSuite) seedValidatedHeadWithSpin() mbFURegenFixture {
	f := mbFURegenFixture{mbhID: uuid.New(), groupHeadID: uuid.New()}

	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO cst_rm_group_head (group_head_id, group_code, group_name, created_by, updated_by)
		VALUES ($1, $2, 'itest group', 'itest', 'itest')`,
		f.groupHeadID, mbForceUnvalRegenFixturePrefix+"GRP")
	require.NoError(s.T(), err)

	code := mbForceUnvalRegenFixturePrefix + uuid.NewString()[:6]
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_source, cpm_is_locked, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, 'itest MB product', 'MB_RECIPE', TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`,
		code, s.typeID).Scan(&f.productSysID))

	var headID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_head (crh_product_sys_id, crh_routing_status, crh_version, crh_created_by, crh_updated_by)
		VALUES ($1, 'COMPLETE', 1, 'itest', 'itest') RETURNING crh_head_id`,
		f.productSysID).Scan(&headID))

	var seqID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_route_seq (crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq, crs_created_by, crs_updated_by)
		VALUES ($1, $2, 1, 1, 'itest', 'itest') RETURNING crs_seq_id`,
		headID, f.productSysID).Scan(&seqID))

	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO cost_route_rm (crm_seq_id, crm_parent_product_sys_id, crm_rm_group_code, crm_rm_type, crm_route_rm_ratio, crm_created_by, crm_updated_by)
		VALUES ($1, $2, $3, 'GROUP', 0.60, 'itest', 'itest')`,
		seqID, f.productSysID, mbForceUnvalRegenFixturePrefix+"GRP")
	require.NoError(s.T(), err)

	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_head (
			mbh_id, mbh_mb_costing, mbh_cost_product_id, mbh_cost_generated_at, mbh_cost_generated_by,
			mbh_current_version, mbh_entry_status, mbh_is_locked, created_by
		)
		VALUES ($1, $2, $3, NOW(), 'itest', 1, 'VALIDATED', TRUE, 'itest')`,
		f.mbhID, mbForceUnvalRegenFixturePrefix+uuid.NewString()[:8], f.productSysID)
	require.NoError(s.T(), err)

	// One live composition row so SnapshotVersion has something to freeze into
	// mst_mb_composition_version at the re-validate below.
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_composition
			(mbcm_mbh_id, mbcm_seq_no, mbcm_group_head_id, mbcm_composition_pct,
			 mbcm_source_type, mbcm_is_carrier, mbcm_created_by)
		VALUES ($1, 1, $2, 100, 'GROUP', FALSE, 'itest')`,
		f.mbhID, f.groupHeadID)
	require.NoError(s.T(), err)

	// The spin row already locked as actual by a human — must survive the regenerate
	// completely untouched, and no second spin row must ever be created for this head.
	_, err = s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (mbs_mbh_id, mbs_mgt_name, mbs_ldr_is_actual, mbs_cost_product_id, created_by)
		VALUES ($1, $2, TRUE, $3, 'itest')`,
		f.mbhID, mbForceUnvalRegenFixturePrefix+"spin", f.productSysID)
	require.NoError(s.T(), err)

	return f
}

// regenEntity builds a *mbhead.Entity whose only meaningful field for
// TransitionWithAutoGen's regen branch is CostProductID — regenerateCostProductRMs only
// ever receives entity.CostProductID() as a plain int64, never the entity pointer itself,
// so nothing else needs to be valid (mbhead.Reconstruct performs no validation by design).
func regenEntity(mbhID uuid.UUID, productSysID int64, version int32) *mbhead.Entity {
	return mbhead.Reconstruct(
		mbhID, nil, "ITEST-MBFUR-COSTING", nil, nil,
		nil, nil, nil, nil, nil,
		nil, nil, nil, true, time.Now(), "itest",
		nil, nil, nil, nil,
		mbhead.StatusDraft, false, version, nil,
		"", "", "", "", "", "",
		productSysID, nil, "",
		nil, nil, nil, nil, nil, nil, "", "",
		nil,
	)
}

// TestBulkRegenerate_ForceUnvalidateThenValidate_DoesNotDuplicateProductOrSpin_AndProtectsLockedActualSpin
// reproduces the exact sequence the Bulk MB Head Regenerate feature runs per head
// (ForceUnvalidate -> Submit -> Validate, modeled here as ForceUnvalidateTransition ->
// TransitionWithAutoGen(toState=Validated)) and proves the fix: the existing cost product
// and its single spin row are reused/regenerated in place, never duplicated, and the spin
// row already locked as actual is left completely untouched.
func (s *MBForceUnvalidateRegenRegressionSuite) TestBulkRegenerate_ForceUnvalidateThenValidate_DoesNotDuplicateProductOrSpin_AndProtectsLockedActualSpin() {
	f := s.seedValidatedHeadWithSpin()

	require.NoError(s.T(), s.headRepo.ForceUnvalidateTransition(s.ctx, f.mbhID, 1, "bulk regenerate", "super-admin-1"))

	entity := regenEntity(f.mbhID, f.productSysID, 1)
	err := s.headRepo.TransitionWithAutoGen(
		s.ctx, f.mbhID, mbhead.StatusDraft, mbhead.StatusValidated, 1, "", "super-admin-1", nil, entity,
	)
	require.NoError(s.T(), err)

	var productCount int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM cost_product_master WHERE cpm_product_sys_id = $1`, f.productSysID,
	).Scan(&productCount))
	require.Equal(s.T(), 1, productCount, "regenerate must never create a second cost_product_master row for this product")

	var linkedProductID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbh_cost_product_id FROM mst_mb_head WHERE mbh_id = $1`, f.mbhID,
	).Scan(&linkedProductID))
	require.Equal(s.T(), f.productSysID, linkedProductID, "the head must keep pointing at the same cost product, never a new one")

	var spinCount int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM mst_mb_spin WHERE mbs_mbh_id = $1`, f.mbhID,
	).Scan(&spinCount))
	require.Equal(s.T(), 1, spinCount, "regenerate must never create a second mst_mb_spin row for this head")

	var spinStillActual bool
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT mbs_ldr_is_actual FROM mst_mb_spin WHERE mbs_mbh_id = $1`, f.mbhID,
	).Scan(&spinStillActual))
	require.True(s.T(), spinStillActual, "the spin row already locked as actual must remain locked, untouched by regenerate")
}
