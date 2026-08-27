// Package postgres_test — regression coverage for ListMasterOptions (U-mbspin-lookup-detail,
// putaran ke-3, Bagian 1).
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// Bug being covered: mbspin.CreateCommand has no OrionItemCode field, so every MB Spin
// created through the application has mbs_orion_item_code = NULL. The old query filtered
// "codeField IS NOT NULL", which silently excluded those rows from the MB_SPIN lookup
// dropdown. The fix makes MB_SPIN fall back to mbs_id (its permanent primary key) when
// mbs_orion_item_code is NULL, and drops the NOT NULL filter for that one table only.
//
// SAFETY: mst_mb_spin and mst_mb_head hold real Oracle-seeded production rows. These
// tests never touch them (except a read-only borrow of one mst_mb_head row via FK
// requirement). Every fixture key is prefixed with ITEST-LMOPT- (a shape no real
// mb_costing or ORION item code takes), and the whole prefix range is hard-deleted
// before and after each test.
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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// lmOptFixturePrefix marks every row this suite creates. It must never match real data.
const lmOptFixturePrefix = "ITEST-LMOPT-"

// lmOptCleanupSQL hard-deletes only this suite's fixture rows.
const lmOptCleanupSQL = `DELETE FROM mst_mb_spin
	WHERE mbs_mb_costing LIKE 'ITEST-LMOPT-%'
	   OR mbs_orion_item_code LIKE 'ITEST-LMOPT-%'
	   OR mbs_mgt_name LIKE 'ITEST-LMOPT-%'`

// ListMasterOptionsSuite exercises ListMasterOptions against a real DB.
type ListMasterOptionsSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.LookupMasterRepository
	ctx    context.Context
	headID uuid.UUID
}

func TestListMasterOptionsSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(ListMasterOptionsSuite))
}

func (s *ListMasterOptionsSuite) SetupSuite() {
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
	s.repo = postgres.NewLookupMasterRepository(s.db)

	// mbs_mbh_id is NOT NULL and FK-constrained, so borrow any existing head. The tests
	// never write to mst_mb_head.
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`,
		).Scan(&s.headID),
		"integration DB needs at least one mst_mb_head row")
}

func (s *ListMasterOptionsSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *ListMasterOptionsSuite) SetupTest()    { s.cleanupFixtures() }
func (s *ListMasterOptionsSuite) TearDownTest() { s.cleanupFixtures() }

func (s *ListMasterOptionsSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, lmOptCleanupSQL)
	require.NoError(s.T(), err)
}

// insertSpin writes one mst_mb_spin fixture row. orionCode nil => NULL (the app-created
// spin scenario from the bug report).
func (s *ListMasterOptionsSuite) insertSpin(id uuid.UUID, mgtName string, orionCode *string) {
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code,
			mbs_is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, TRUE, NOW(), 'itest')`,
		id, s.headID, mgtName, lmOptFixturePrefix+"mbc", orionCode)
	require.NoError(s.T(), err)
}

// findOptionByLabel locates the option whose Label matches label.
func findOptionByLabel(opts []lookupmaster.MasterOption, label string) (lookupmaster.MasterOption, bool) {
	for _, o := range opts {
		if o.Label == label {
			return o, true
		}
	}
	return lookupmaster.MasterOption{}, false
}

// (i) Regression: a row that DOES have an ORION item code must keep using it as the
// dropdown value — unchanged from before the fix.
func (s *ListMasterOptionsSuite) TestListMasterOptions_MBSpin_WithOrionCode_UsesOrionCodeAsValue() {
	id := uuid.New()
	label := lmOptFixturePrefix + "with-orion"
	orion := lmOptFixturePrefix + "ORION-1"
	s.insertSpin(id, label, &orion)

	opts, err := s.repo.ListMasterOptions(s.ctx, "MB_SPIN", "", 0)
	require.NoError(s.T(), err)

	got, found := findOptionByLabel(opts, label)
	require.True(s.T(), found, "expected fixture row to appear in options")
	require.Equal(s.T(), orion, got.Value)
}

// (ii) The bug fix: a row with NULL ORION item code (every app-created MB Spin, since
// mbspin.CreateCommand has no OrionItemCode field) must still appear, falling back to
// mbs_id as the value instead of being silently dropped by the old "IS NOT NULL" filter.
func (s *ListMasterOptionsSuite) TestListMasterOptions_MBSpin_WithoutOrionCode_FallsBackToMBSID() {
	id := uuid.New()
	label := lmOptFixturePrefix + "without-orion"
	s.insertSpin(id, label, nil)

	opts, err := s.repo.ListMasterOptions(s.ctx, "MB_SPIN", "", 0)
	require.NoError(s.T(), err)

	got, found := findOptionByLabel(opts, label)
	require.True(s.T(), found, "row with NULL orion code must still appear in the dropdown")
	require.Equal(s.T(), id.String(), got.Value, "value must fall back to mbs_id")
}

// (iii) Other lookup masters must not change behavior: MB_HEAD's code field
// (mbh_mb_costing) is NOT NULL in the schema, so every row already satisfies the
// pre-existing filter. This asserts the non-MB_SPIN branch still selects the raw code
// field verbatim as the value (never an mbs_id-style fallback, never dropping the
// NOT NULL clause).
func (s *ListMasterOptionsSuite) TestListMasterOptions_MBHead_UnaffectedByMBSpinBranch() {
	opts, err := s.repo.ListMasterOptions(s.ctx, "MB_HEAD", "", 0)
	require.NoError(s.T(), err)
	require.NotEmpty(s.T(), opts, "integration DB needs at least one mst_mb_head row")

	for _, o := range opts {
		require.NotEmpty(s.T(), o.Value, "MB_HEAD value must be mbh_mb_costing, never blank")
	}
}
