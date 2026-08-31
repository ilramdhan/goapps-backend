// Package postgres — integration coverage proving mbInsertCostProductMaster's FULL column set
// (mb_autogen_repository.go) survives a real round trip: written by the MB Recipe auto-gen INSERT,
// then read back through CostProductMasterRepository.GetBySysID — i.e. through cpmFromRow
// (cost_product_master_repository.go), not raw SQL — so both halves of the P1-T2/P1-T3 fix are
// exercised together the way production actually reads this row.
//
// mb_autogen_shade_integration_test.go already covers cpm_shade_code/cpm_shade_name in isolation
// via a raw SELECT; this suite intentionally does NOT repeat that — it instead widens coverage to
// every other column mbInsertCostProductMaster touches (product_code format, product_name, source,
// is_locked, grade_code, and the untouched legacy flex_01/02/03 fields), and swaps the raw SELECT
// for the repository read path so cpmFromRow's MB-aware grade fallback is verified end-to-end too.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// SAFETY: every fixture row is prefixed with ITEST-MBFULL- and hard-deleted before and after
// each test. Nothing here touches real cost_product_master rows.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"regexp"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

const mbFullRowFixturePrefix = "ITEST-MBFULL-"

// cstMBCodePattern matches generate_cost_product_code's output for the MB cost_product_type:
// "CST" + type_code("MB") + YYMM(4 digits) + zero-padded sequence (6 digits) = "CSTMB" + 10 digits.
var cstMBCodePattern = regexp.MustCompile(`^CSTMB\d{10}$`)

// MBAutoGenCPMFullRowSuite proves that mbInsertCostProductMaster writes a complete, correctly
// shaped cost_product_master row for an MB Head, read back through the repository (GetBySysID ->
// cpmFromRow), not raw SQL:
//
//	cpm_product_code  -- generated, matches generate_cost_product_code's CSTMB+YYMM+seq6 format
//	cpm_product_name  -- equals entity.MBCosting()
//	cpm_source        -- "MB_RECIPE" (mbCostProductSource)
//	cpm_is_locked     -- TRUE
//	cpm_shade_code/_name -- copied from the MB Head
//	cpm_grade_code    -- empty (NOT "AX") once read back via cpmFromRow, because cpm_source is MB_RECIPE
//	cpm_flex_01/02/03 -- never written by this INSERT column list, so they read back empty
type MBAutoGenCPMFullRowSuite struct {
	suite.Suite
	db     *DB
	repo   *CostProductMasterRepository
	ctx    context.Context
	typeID int32
}

func TestMBAutoGenCPMFullRowSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBAutoGenCPMFullRowSuite))
}

func mbFullRowEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbFullRowWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBAutoGenCPMFullRowSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbFullRowEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbFullRowEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbFullRowEnvOrDefault("TEST_DB_USER", "finance")
	password := mbFullRowEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbFullRowEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbFullRowWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
	s.repo = NewCostProductMasterRepository(s.db)

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).
			Scan(&s.typeID),
		"integration DB needs the MB cost_product_type seed (migration 000450)")
}

func (s *MBAutoGenCPMFullRowSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBAutoGenCPMFullRowSuite) SetupTest()    { s.cleanupFixtures() }
func (s *MBAutoGenCPMFullRowSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBAutoGenCPMFullRowSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_name LIKE $1`, mbFullRowFixturePrefix+"%")
	require.NoError(s.T(), err)
}

func (s *MBAutoGenCPMFullRowSuite) newEntity(mbCosting, shadeCode, shadeName string) *mbhead.Entity {
	entity, err := mbhead.New(mbhead.NewParams{
		MBCosting: mbCosting,
		CreatedBy: "itest",
		ShadeCode: shadeCode,
		ShadeName: shadeName,
	})
	require.NoError(s.T(), err)
	return entity
}

// Full MB Head, all columns verified through the repository read path (GetBySysID -> cpmFromRow),
// never raw SQL, so cpmFromRow's MB-aware grade handling is exercised, not just the write side.
func (s *MBAutoGenCPMFullRowSuite) TestInsertAndReadBack_AllColumnsCorrect() {
	entity := s.newEntity(mbFullRowFixturePrefix+"MB COSTING A", "SH-777", "MIDNIGHT BLUE")

	var productSysID int64
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		var insertErr error
		productSysID, insertErr = mbInsertCostProductMaster(s.ctx, tx, s.typeID, entity, "itest")
		return insertErr
	})
	require.NoError(s.T(), err)

	got, err := s.repo.GetBySysID(s.ctx, productSysID)
	require.NoError(s.T(), err)

	s.Run("product_code is generated and matches CSTMB+YYMM+seq6", func() {
		require.NotEmpty(s.T(), got.ProductCode())
		require.Regexp(s.T(), cstMBCodePattern, got.ProductCode())
	})

	s.Run("product_name equals entity.MBCosting()", func() {
		require.Equal(s.T(), entity.MBCosting(), got.ProductName())
	})

	s.Run("source is MB_RECIPE", func() {
		require.Equal(s.T(), mbCostProductSource, got.Source())
	})

	s.Run("is_locked is TRUE", func() {
		require.True(s.T(), got.IsLocked())
	})

	s.Run("shade code/name copied from MB Head", func() {
		require.Equal(s.T(), "SH-777", got.ShadeCode())
		require.Equal(s.T(), "MIDNIGHT BLUE", got.ShadeName())
	})

	s.Run("grade_code is empty, not AX, once read back via cpmFromRow", func() {
		require.Empty(s.T(), got.GradeCode(), "MB Recipe rows must never fall back to the AX default")
	})

	s.Run("legacy flex fields are empty: mbInsertCostProductMaster's column list never writes them", func() {
		require.Empty(s.T(), got.Flex01())
		require.Empty(s.T(), got.Flex02())
		require.Empty(s.T(), got.Flex03())
	})
}

// A second MB Head with no shade set proves the read-back path also carries an empty
// shade/blank grade correctly when the head itself has nothing to copy (complements
// mb_autogen_shade_integration_test.go's own NULL-column assertion, this time via the repo).
func (s *MBAutoGenCPMFullRowSuite) TestInsertAndReadBack_NoShade_ReadsBackEmpty() {
	entity := s.newEntity(mbFullRowFixturePrefix+"MB COSTING B", "", "")

	var productSysID int64
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		var insertErr error
		productSysID, insertErr = mbInsertCostProductMaster(s.ctx, tx, s.typeID, entity, "itest")
		return insertErr
	})
	require.NoError(s.T(), err)

	got, err := s.repo.GetBySysID(s.ctx, productSysID)
	require.NoError(s.T(), err)

	require.Empty(s.T(), got.ShadeCode())
	require.Empty(s.T(), got.ShadeName())
	require.Empty(s.T(), got.GradeCode(), "still must not fall back to AX for an MB_RECIPE row")
	require.True(s.T(), got.IsLocked())
	require.Equal(s.T(), mbCostProductSource, got.Source())
}
