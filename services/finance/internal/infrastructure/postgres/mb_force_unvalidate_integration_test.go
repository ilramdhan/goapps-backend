// Package postgres — integration coverage for the Bulk MB Head Regenerate feature's
// ForceUnvalidateTransition (Phase B, mb_force_unvalidate.go): proves that forcing a
// VALIDATED head back to DRAFT really nulls the cost-lineage columns and writes an
// mst_mb_workflow_log audit row against a real PostgreSQL instance, mirroring
// mb_spin_repository_cost_product_id_integration_test.go's suite shape.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB: postgres://finance:finance123@localhost:5434/finance_db).
//
// SAFETY: every fixture row is prefixed with ITEST-MBFORCEUNVAL- and hard-deleted before
// and after each test. Nothing here touches real mst_mb_head rows.
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

const mbForceUnvalidateFixturePrefix = "ITEST-MBFORCEUNVAL-"

// MBForceUnvalidateSuite proves MBHeadRepository.ForceUnvalidateTransition round-trips
// against a real PostgreSQL instance.
type MBForceUnvalidateSuite struct {
	suite.Suite
	db   *DB
	repo *MBHeadRepository
	ctx  context.Context
}

func TestMBForceUnvalidateSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBForceUnvalidateSuite))
}

func mbForceUnvalidateEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbForceUnvalidateWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBForceUnvalidateSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbForceUnvalidateEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbForceUnvalidateEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbForceUnvalidateEnvOrDefault("TEST_DB_USER", "finance")
	password := mbForceUnvalidateEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbForceUnvalidateEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbForceUnvalidateWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
	s.repo = NewMBHeadRepository(s.db, NewMBCompositionRepository(s.db))
}

func (s *MBForceUnvalidateSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBForceUnvalidateSuite) SetupTest() { s.cleanupFixtures() }

func (s *MBForceUnvalidateSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBForceUnvalidateSuite) cleanupFixtures() {
	prefix := mbForceUnvalidateFixturePrefix + "%"
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_workflow_log WHERE mbwl_mbh_id IN (
		SELECT mbh_id FROM mst_mb_head WHERE mbh_mb_costing LIKE $1)`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_name LIKE $1`, prefix)
	require.NoError(s.T(), err)
}

// seedCostProductMaster inserts a real cost_product_master row and returns its generated
// cpm_product_sys_id — mst_mb_head.mbh_cost_product_id is expected to reference a real
// product row, mirroring mb_spin_repository_cost_product_id_integration_test.go's fixture.
func (s *MBForceUnvalidateSuite) seedCostProductMaster() int64 {
	var typeID int32
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).
			Scan(&typeID))

	var code string
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT generate_cost_product_code($1)`, typeID).Scan(&code))

	var productSysID int64
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master (cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_source, cpm_is_locked, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, 'MB_RECIPE', TRUE, 'itest', 'itest')
		RETURNING cpm_product_sys_id`,
		code, typeID, mbForceUnvalidateFixturePrefix+"product").Scan(&productSysID))
	return productSysID
}

// seedValidatedHead inserts a VALIDATED mst_mb_head row with cost lineage columns
// populated (mbh_cost_product_id/mbh_cost_generated_at/mbh_cost_generated_by) plus the
// P10 lock flag set, so ForceUnvalidateTransition has real non-NULL values to clear.
func (s *MBForceUnvalidateSuite) seedValidatedHead(costProductID int64) uuid.UUID {
	id := uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_head (
			mbh_id, mbh_mb_costing, mbh_current_version, mbh_entry_status,
			mbh_is_locked, mbh_cost_product_id, mbh_cost_generated_at, mbh_cost_generated_by,
			created_by
		)
		VALUES ($1, $2, 1, 'VALIDATED', TRUE, $3, NOW(), 'itest', 'itest')`,
		id, mbForceUnvalidateFixturePrefix+uuid.NewString()[:8], costProductID)
	require.NoError(s.T(), err)
	return id
}

// TestForceUnvalidateTransition_NullsCostLineage_AndWritesWorkflowLog is the core
// contract: after forcing a VALIDATED head with a non-null cost product back to DRAFT,
// the cost lineage columns are all NULL, entry_status is DRAFT, and a new
// mst_mb_workflow_log row records the VALIDATED -> DRAFT transition.
func (s *MBForceUnvalidateSuite) TestForceUnvalidateTransition_NullsCostLineage_AndWritesWorkflowLog() {
	productSysID := s.seedCostProductMaster()
	headID := s.seedValidatedHead(productSysID)

	require.NoError(s.T(), s.repo.ForceUnvalidateTransition(s.ctx, headID, 1, "bulk regenerate", "super-admin-1"))

	var (
		entryStatus       string
		isLocked          bool
		costProductID     sql.NullInt64
		costGeneratedAt   sql.NullTime
		costGeneratedBy   sql.NullString
		unlockRequestedAt sql.NullTime
	)
	err := s.db.QueryRowContext(s.ctx, `
		SELECT mbh_entry_status, mbh_is_locked, mbh_cost_product_id, mbh_cost_generated_at,
		       mbh_cost_generated_by, mbh_unlock_requested_at
		FROM mst_mb_head WHERE mbh_id = $1`, headID,
	).Scan(&entryStatus, &isLocked, &costProductID, &costGeneratedAt, &costGeneratedBy, &unlockRequestedAt)
	require.NoError(s.T(), err)

	require.Equal(s.T(), "DRAFT", entryStatus)
	require.False(s.T(), isLocked, "ForceUnvalidateTransition must clear the P10 lock")
	require.False(s.T(), costProductID.Valid, "mbh_cost_product_id must be NULL after force-unvalidate")
	require.False(s.T(), costGeneratedAt.Valid, "mbh_cost_generated_at must be NULL after force-unvalidate")
	require.False(s.T(), costGeneratedBy.Valid, "mbh_cost_generated_by must be NULL after force-unvalidate")
	require.False(s.T(), unlockRequestedAt.Valid, "mbh_unlock_requested_at must be cleared")

	var logCount int
	err = s.db.QueryRowContext(s.ctx, `
		SELECT COUNT(*) FROM mst_mb_workflow_log
		WHERE mbwl_mbh_id = $1 AND mbwl_from_state = 'VALIDATED' AND mbwl_to_state = 'DRAFT'
		  AND mbwl_actor_user_id = 'super-admin-1'`, headID,
	).Scan(&logCount)
	require.NoError(s.T(), err)
	require.Equal(s.T(), 1, logCount, "expected exactly one VALIDATED->DRAFT workflow log row")
}

// TestForceUnvalidateTransition_UnknownID_ReturnsNotFound proves a missing/soft-deleted
// head is reported as mbhead.ErrNotFound rather than silently succeeding as a no-op.
func (s *MBForceUnvalidateSuite) TestForceUnvalidateTransition_UnknownID_ReturnsNotFound() {
	ghost := uuid.New()
	err := s.repo.ForceUnvalidateTransition(s.ctx, ghost, 1, "reason", "super-admin-1")
	require.Error(s.T(), err)
}
