// Package postgres — integration coverage proving the end-to-end auto-gen MB Spin write
// (mbAutoGenSpin / mbBuildAutoGenSpin, mb_autogen_repository.go, P2-T5) actually lands the
// derived fields on the real mst_mb_spin row: mbs_cc, mbs_shade_code, mbs_shade_name,
// mbs_cross_section, mbs_lusture_code, mbs_final_product, mbs_ldr_calculated_pct and (P2-T7)
// mbs_cost_product_id.
// mb_autogen_spin_fields_test.go already proves the pure construction step (mbBuildAutoGenSpin)
// without a database; this suite proves the DB-writing half (mbAutoGenSpin -> createSpinOn) round
// trips the same values through a real INSERT + SELECT, the way TransitionWithAutoGen's Validate
// flow actually exercises it. Lives in the internal (non-_test) package because mbAutoGenSpin is
// unexported.
//
// P2-T7: prior to this task, createSpinOn's INSERT column list omitted mbs_cost_product_id
// entirely, so mbBuildAutoGenSpin's spin.HydrateLineage(mbspin.Lineage{CostProductID:
// &productSysID}) call (mb_autogen_repository.go:233) was silently dropped on save — the
// in-memory entity carried the value, but it never reached the database. This test's
// mbs_cost_product_id assertion below is the regression guard for that fix.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// SAFETY: every fixture row is prefixed with ITEST-MBSPIN- and hard-deleted before and after each
// test. Nothing here touches real mst_mb_head/mst_mb_spin rows.
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

const mbSpinAutoGenFixturePrefix = "ITEST-MBSPIN-"

// MBAutoGenSpinSuite proves that mbAutoGenSpin persists the P2-T5 field-derivation rules onto a
// real mst_mb_spin row: mbs_cc, mbs_shade_code, mbs_shade_name, mbs_cross_section,
// mbs_lusture_code, mbs_final_product and mbs_ldr_calculated_pct are all copied down from the
// source MB Head, matching what mb_autogen_spin_fields_test.go already proves in isolation for
// mbBuildAutoGenSpin's pure construction step.
type MBAutoGenSpinSuite struct {
	suite.Suite
	db    *DB
	ctx   context.Context
	mbhID uuid.UUID
}

func TestMBAutoGenSpinSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBAutoGenSpinSuite))
}

func mbSpinAutoGenEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbSpinAutoGenWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBAutoGenSpinSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbSpinAutoGenEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbSpinAutoGenEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbSpinAutoGenEnvOrDefault("TEST_DB_USER", "finance")
	password := mbSpinAutoGenEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbSpinAutoGenEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbSpinAutoGenWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
}

func (s *MBAutoGenSpinSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBAutoGenSpinSuite) SetupTest() {
	s.cleanupFixtures()
	s.seedHead()
}

func (s *MBAutoGenSpinSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBAutoGenSpinSuite) cleanupFixtures() {
	prefix := mbSpinAutoGenFixturePrefix + "%"
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_spin WHERE mbs_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
	_, err = s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, prefix)
	require.NoError(s.T(), err)
}

// seedHead inserts the minimal mst_mb_head row mbAutoGenSpin's FK (mbs_mbh_id) needs to exist.
// The auto-gen field derivation itself reads from the in-memory *mbhead.Entity built by
// mbhead.New below, not from this row — this row only needs to exist for referential integrity.
func (s *MBAutoGenSpinSuite) seedHead() {
	s.mbhID = uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_current_version, mbh_entry_status, created_by)
		VALUES ($1, $2, 1, 'VALIDATED', 'itest')`,
		s.mbhID, mbSpinAutoGenFixturePrefix+uuid.NewString()[:8])
	require.NoError(s.T(), err)
}

// End-to-end: a fully-populated MB Head, run through the same mbAutoGenSpin call
// autoGenCostProduct makes at Validate time, must produce an mst_mb_spin row whose mbs_cc,
// mbs_shade_code, mbs_shade_name, mbs_cross_section, mbs_lusture_code, mbs_final_product and
// mbs_ldr_calculated_pct all carry the source MB Head's values — not left NULL.
func (s *MBAutoGenSpinSuite) TestAutoGenSpin_FullyPopulatedHead_PersistsDerivedFields() {
	mbCosting := mbSpinAutoGenFixturePrefix + "MBC-" + uuid.NewString()[:8]
	entity, err := mbhead.New(mbhead.NewParams{
		MBCosting:       mbCosting,
		CreatedBy:       "itest",
		ShadeCode:       "SH-900",
		ShadeName:       "ROYAL PURPLE",
		CrossSection:    "TRILOBAL",
		LustureCode:     "LU-900",
		MBHFinalProduct: stringPtr("FP-900"),
		MBHRunLdrPct:    floatPtr(21.5),
	})
	require.NoError(s.T(), err)

	const productSysID int64 = 424242
	err = s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		return mbAutoGenSpin(s.ctx, tx, s.mbhID, entity, productSysID, "itest")
	})
	require.NoError(s.T(), err)

	var (
		cc, shadeCode, shadeName, crossSection, lustureCode, finalProduct sql.NullString
		ldrCalculatedPct                                                  sql.NullFloat64
		costProductID                                                     sql.NullInt64
	)
	err = s.db.QueryRowContext(s.ctx, `
		SELECT mbs_cc, mbs_shade_code, mbs_shade_name, mbs_cross_section,
		       mbs_lusture_code, mbs_final_product, mbs_ldr_calculated_pct, mbs_cost_product_id
		FROM mst_mb_spin WHERE mbs_mbh_id = $1`, s.mbhID).
		Scan(&cc, &shadeCode, &shadeName, &crossSection, &lustureCode, &finalProduct, &ldrCalculatedPct, &costProductID)
	require.NoError(s.T(), err)

	require.True(s.T(), cc.Valid)
	require.Equal(s.T(), "SH-900", cc.String)

	require.True(s.T(), shadeCode.Valid)
	require.Equal(s.T(), "SH-900", shadeCode.String)

	require.True(s.T(), shadeName.Valid)
	require.Equal(s.T(), "ROYAL PURPLE", shadeName.String)

	require.True(s.T(), crossSection.Valid)
	require.Equal(s.T(), "TRILOBAL", crossSection.String)

	require.True(s.T(), lustureCode.Valid)
	require.Equal(s.T(), "LU-900", lustureCode.String)

	require.True(s.T(), finalProduct.Valid)
	require.Equal(s.T(), "FP-900", finalProduct.String)

	require.True(s.T(), ldrCalculatedPct.Valid)
	require.InDelta(s.T(), 21.5, ldrCalculatedPct.Float64, 0.0001)

	// P2-T7 regression guard: mbs_cost_product_id must round-trip, not silently NULL.
	require.True(s.T(), costProductID.Valid, "mbs_cost_product_id must not be NULL after auto-gen createSpinOn")
	require.Equal(s.T(), productSysID, costProductID.Int64)
}
