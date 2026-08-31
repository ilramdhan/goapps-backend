// Package postgres — integration coverage proving mbInsertCostProductMaster (the first-ever-
// validate branch of the MB Recipe auto-gen flow, mb_autogen_repository.go) carries the MB Head's
// shade code/name onto the generated cost_product_master row instead of leaving cpm_shade_code/
// cpm_shade_name blank forever. Lives in the internal (non-_test) package because
// mbInsertCostProductMaster is unexported.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// SAFETY: every fixture row is prefixed with ITEST-MBSHADE- and hard-deleted before and after
// each test. Nothing here touches real cost_product_master rows.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

const mbShadeFixturePrefix = "ITEST-MBSHADE-"

// MBAutoGenShadeSuite proves that mbInsertCostProductMaster writes the MB Head's shade
// code/name onto the auto-generated cost_product_master row: populated when the MB Head
// carries a shade, and SQL NULL (not empty string) when it does not.
type MBAutoGenShadeSuite struct {
	suite.Suite
	db     *DB
	ctx    context.Context
	typeID int32
}

func TestMBAutoGenShadeSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBAutoGenShadeSuite))
}

func mbShadeEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbShadeWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBAutoGenShadeSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbShadeEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbShadeEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbShadeEnvOrDefault("TEST_DB_USER", "finance")
	password := mbShadeEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbShadeEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbShadeWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)

	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx, `SELECT cpt_type_id FROM cost_product_type WHERE cpt_type_code = 'MB' AND cpt_is_active = TRUE`).
			Scan(&s.typeID),
		"integration DB needs the MB cost_product_type seed (migration 000450)")
}

func (s *MBAutoGenShadeSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBAutoGenShadeSuite) SetupTest()    { s.cleanupFixtures() }
func (s *MBAutoGenShadeSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBAutoGenShadeSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, mbShadeFixturePrefix+"%")
	require.NoError(s.T(), err)
}

func (s *MBAutoGenShadeSuite) newEntity(mbCosting, shadeCode, shadeName string) *mbhead.Entity {
	entity, err := mbhead.New(mbhead.NewParams{
		MBCosting: mbCosting,
		CreatedBy: "itest",
		ShadeCode: shadeCode,
		ShadeName: shadeName,
	})
	require.NoError(s.T(), err)
	return entity
}

func (s *MBAutoGenShadeSuite) insertAndFetch(entity *mbhead.Entity) (shadeCode, shadeName sql.NullString) {
	var productSysID int64
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		var insertErr error
		productSysID, insertErr = mbInsertCostProductMaster(s.ctx, tx, s.typeID, entity, "itest")
		return insertErr
	})
	require.NoError(s.T(), err)

	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT cpm_shade_code, cpm_shade_name FROM cost_product_master WHERE cpm_product_sys_id = $1`, productSysID).
		Scan(&shadeCode, &shadeName))
	return shadeCode, shadeName
}

// A MB Head with shade code + name set must have both columns populated on the generated
// Master Product row, matching what the MB Recipe UI already collects.
func (s *MBAutoGenShadeSuite) TestInsert_WithShade_PopulatesBothColumns() {
	entity := s.newEntity(mbShadeFixturePrefix+"WITH", "SH-001", "JET BLACK")

	shadeCode, shadeName := s.insertAndFetch(entity)

	require.True(s.T(), shadeCode.Valid)
	require.Equal(s.T(), "SH-001", shadeCode.String)
	require.True(s.T(), shadeName.Valid)
	require.Equal(s.T(), "JET BLACK", shadeName.String)
}

// A MB Head with no shade set must leave both columns as SQL NULL — never an empty string —
// so downstream NULL-aware queries/UI checks behave correctly.
func (s *MBAutoGenShadeSuite) TestInsert_WithoutShade_LeavesColumnsNull() {
	entity := s.newEntity(mbShadeFixturePrefix+"WITHOUT", "", "")

	shadeCode, shadeName := s.insertAndFetch(entity)

	require.False(s.T(), shadeCode.Valid, "cpm_shade_code must be NULL, not empty string")
	require.False(s.T(), shadeName.Valid, "cpm_shade_name must be NULL, not empty string")
}
