// Package postgres — integration coverage for P2-T7: proves that MBSpinRepository.Create
// (createSpinOn) persists mbs_cost_product_id to the real mst_mb_spin row when the in-memory
// entity has it set via HydrateLineage.
//
// Before this fix, createSpinOn's INSERT column list (mb_spin_repository.go) omitted
// mbs_cost_product_id entirely, even though:
//   - the column exists on mst_mb_spin (migrations 000484 / 000490),
//   - mbspin.Entity exposes CostProductID() (internal/domain/mbspin/entity.go),
//   - and both the manual Create() path (this test) and the auto-gen path
//     (mb_autogen_spin_integration_test.go, mbBuildAutoGenSpin) set it in memory via
//     HydrateLineage(mbspin.Lineage{CostProductID: ...}) before saving.
//
// The value was silently dropped on save — the column was very likely always NULL in
// production. This test is the regression guard for the manual Create() path specifically;
// mb_autogen_spin_integration_test.go covers the auto-gen path.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// SAFETY: every fixture row is prefixed with ITEST-MBSPINCPID- and hard-deleted before and
// after each test. Nothing here touches real mst_mb_head/mst_mb_spin rows.
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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

const mbSpinCostProductIDFixturePrefix = "ITEST-MBSPINCPID-"

// MBSpinCostProductIDSuite proves MBSpinRepository.Create round-trips mbs_cost_product_id
// through a real INSERT + SELECT.
type MBSpinCostProductIDSuite struct {
	suite.Suite
	db    *DB
	repo  *MBSpinRepository
	ctx   context.Context
	mbhID uuid.UUID
}

func TestMBSpinCostProductIDSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSpinCostProductIDSuite))
}

func mbSpinCostProductIDEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbSpinCostProductIDWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBSpinCostProductIDSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbSpinCostProductIDEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbSpinCostProductIDEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbSpinCostProductIDEnvOrDefault("TEST_DB_USER", "finance")
	password := mbSpinCostProductIDEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbSpinCostProductIDEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbSpinCostProductIDWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
	s.repo = NewMBSpinRepository(s.db)
}

func (s *MBSpinCostProductIDSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSpinCostProductIDSuite) SetupTest() {
	s.cleanupFixtures()
	s.seedHead()
}

func (s *MBSpinCostProductIDSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBSpinCostProductIDSuite) cleanupFixtures() {
	prefix := mbSpinCostProductIDFixturePrefix + "%"
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_spin WHERE mbs_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
}

// seedHead inserts the minimal mst_mb_head row mbs_mbh_id's FK needs to exist.
func (s *MBSpinCostProductIDSuite) seedHead() {
	s.mbhID = uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1, $2, 1, 'VALIDATED', 'itest')`,
		s.mbhID, mbSpinCostProductIDFixturePrefix+uuid.NewString()[:8])
	require.NoError(s.T(), err)
}

// TestCreate_WithCostProductIDSet_PersistsToDB proves the manual Create() path: an entity built
// via mbspin.New and then given a CostProductID via HydrateLineage (the same call shape
// mbBuildAutoGenSpin uses) must land mbs_cost_product_id in the database, not drop it.
func (s *MBSpinCostProductIDSuite) TestCreate_WithCostProductIDSet_PersistsToDB() {
	mbCosting := mbSpinCostProductIDFixturePrefix + "MBC-" + uuid.NewString()[:8]
	entity, err := mbspin.New(s.mbhID, "Test Spin CostProductID", nil, nil, nil, nil, nil, &mbCosting, nil, nil, nil, nil, nil, nil, nil, nil, "itest")
	require.NoError(s.T(), err)

	productSysID := int64(999111)
	entity.HydrateLineage(mbspin.Lineage{CostProductID: &productSysID})

	require.NoError(s.T(), s.repo.Create(s.ctx, entity))

	var costProductID sql.NullInt64
	err = s.db.QueryRowContext(s.ctx,
		`SELECT mbs_cost_product_id FROM mst_mb_spin WHERE mbs_id = $1`, entity.ID(),
	).Scan(&costProductID)
	require.NoError(s.T(), err)

	require.True(s.T(), costProductID.Valid, "mbs_cost_product_id must not be NULL after Create with CostProductID set")
	require.Equal(s.T(), productSysID, costProductID.Int64)

	// Round trip through GetByID too, to prove the read side (already correct pre-fix) still
	// agrees with the write side now that both are wired to the same column.
	fetched, err := s.repo.GetByID(s.ctx, entity.ID())
	require.NoError(s.T(), err)
	require.NotNil(s.T(), fetched.CostProductID())
	require.Equal(s.T(), productSysID, *fetched.CostProductID())
}

// TestCreate_WithoutCostProductID_StaysNull proves the DRAFT-head case (D23: a spin under a
// DRAFT head has no cost product yet) is still a legitimate NULL after this fix, i.e. the fix
// only stops a set value from being dropped — it does not force a value where none was given.
func (s *MBSpinCostProductIDSuite) TestCreate_WithoutCostProductID_StaysNull() {
	mbCosting := mbSpinCostProductIDFixturePrefix + "MBC-" + uuid.NewString()[:8]
	entity, err := mbspin.New(s.mbhID, "Test Spin No CostProductID", nil, nil, nil, nil, nil, &mbCosting, nil, nil, nil, nil, nil, nil, nil, nil, "itest")
	require.NoError(s.T(), err)

	require.NoError(s.T(), s.repo.Create(s.ctx, entity))

	var costProductID sql.NullInt64
	err = s.db.QueryRowContext(s.ctx,
		`SELECT mbs_cost_product_id FROM mst_mb_spin WHERE mbs_id = $1`, entity.ID(),
	).Scan(&costProductID)
	require.NoError(s.T(), err)
	require.False(s.T(), costProductID.Valid)
}
