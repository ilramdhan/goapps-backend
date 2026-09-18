// Package postgres — integration coverage for mbResolveRefProductSysID's NULL-safe scan of
// mbh_cost_product_id when resolving a nested MB composition row's referenced MB Head.
//
// Regression target: a bulk regenerate of 99 MB Heads hit
// "mb_autogen: resolve nested MB cost product: sql: Scan error on column index 0, name
// "mbh_cost_product_id": converting NULL to int64 is unsupported" for one head whose nested
// reference had not (yet) had its own cost product generated. The fix scans into sql.NullInt64
// and returns a clear, actionable error instead of the raw driver panic.
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB). Lives in the internal (non-_test) package because
// mbResolveRefProductSysID is unexported.
//
// SAFETY: every fixture row is prefixed with ITEST-MBREFRESOLVE- and hard-deleted before and
// after each test. Nothing here touches real mst_mb_head rows.
package postgres

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const mbRefResolveFixturePrefix = "ITEST-MBREFRESOLVE-"

// mbRefResolveCostProductCodePrefix tags every cost_product_master fixture row this
// suite inserts. cpm_product_code is varchar(20), so keep this short: prefix (11) +
// 6 hex chars is 17, leaving headroom.
const mbRefResolveCostProductCodePrefix = "ITMBRR-CPM-"

type MBResolveRefProductSysIDSuite struct {
	suite.Suite
	db  *DB
	ctx context.Context
}

func TestMBResolveRefProductSysIDSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBResolveRefProductSysIDSuite))
}

func mbRefResolveEnvOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func mbRefResolveWaitForDB(db *sql.DB, timeout time.Duration) error {
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

func (s *MBResolveRefProductSysIDSuite) SetupSuite() {
	s.ctx = context.Background()

	host := mbRefResolveEnvOrDefault("TEST_DB_HOST", "localhost")
	port := mbRefResolveEnvOrDefault("TEST_DB_PORT", "5434")
	user := mbRefResolveEnvOrDefault("TEST_DB_USER", "finance")
	password := mbRefResolveEnvOrDefault("TEST_DB_PASSWORD", "finance123")
	dbname := mbRefResolveEnvOrDefault("TEST_DB_NAME", "finance_db")

	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), mbRefResolveWaitForDB(raw, 10*time.Second))

	s.db = NewDBFromSQL(raw)
}

func (s *MBResolveRefProductSysIDSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBResolveRefProductSysIDSuite) SetupTest() { s.cleanupFixtures() }

func (s *MBResolveRefProductSysIDSuite) TearDownTest() { s.cleanupFixtures() }

// cleanupFixtures removes every fixture row this suite created. Order matters: migration
// 000517 added `fk_mbh_cost_product` (mst_mb_head.mbh_cost_product_id ->
// cost_product_master.cpm_product_sys_id, ON DELETE SET NULL), so deleting
// cost_product_master rows before the mst_mb_head rows that reference them would not
// error — it would silently null out mbh_cost_product_id and leave orphaned mst_mb_head
// fixture rows behind instead of removing them. Delete mst_mb_head first, then the
// cost_product_master rows it referenced.
func (s *MBResolveRefProductSysIDSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, `DELETE FROM mst_mb_head WHERE mbh_mb_costing LIKE $1`, mbRefResolveFixturePrefix+"%")
	require.NoError(s.T(), err)

	_, err = s.db.ExecContext(s.ctx, `DELETE FROM cost_product_master WHERE cpm_product_code LIKE $1`, mbRefResolveCostProductCodePrefix+"%")
	require.NoError(s.T(), err)
}

// insertCostProduct inserts a real cost_product_master fixture row and returns its
// cpm_product_sys_id, for use as a valid mst_mb_head.mbh_cost_product_id reference.
//
// Before migration 000517, mst_mb_head.mbh_cost_product_id carried no foreign key, so
// TestResolve_ValidCostProductID_ReturnsID could get away with an arbitrary hardcoded
// int64 (424242) that did not exist anywhere. Migration 000517 added
// `fk_mbh_cost_product` (mst_mb_head.mbh_cost_product_id ->
// cost_product_master.cpm_product_sys_id, ON DELETE SET NULL), so an INSERT of
// mst_mb_head referencing an ID that is not a real cost_product_master row is now
// rejected with SQLSTATE 23503 (foreign_key_violation) instead of succeeding. This
// helper creates a real row so the reference is valid, rather than borrowing existing
// data — a freshly migrated database can legitimately have zero rows in
// cost_product_master, and this suite must not depend on imported data being present.
//
// cpm_product_type_id is NOT NULL with its own FK to cost_product_type (ON DELETE
// RESTRICT), so the type id is read via SELECT rather than guessed/hardcoded.
func (s *MBResolveRefProductSysIDSuite) insertCostProduct() int64 {
	var typeID int64
	err := s.db.QueryRowContext(s.ctx,
		`SELECT cpt_type_id FROM cost_product_type ORDER BY cpt_type_id LIMIT 1`).Scan(&typeID)
	require.NoError(s.T(), err, "cost_product_type must have at least one seeded row for this fixture to reference")

	// Uniqueness via crypto/rand, not time.Now().UnixNano()%N: on darwin/arm64 UnixNano
	// only advances in whole microseconds, so its low digits are effectively constant
	// and produce collisions on uk_cost_product_master_code.
	var b [3]byte
	_, err = rand.Read(b[:])
	require.NoError(s.T(), err)
	code := mbRefResolveCostProductCodePrefix + hex.EncodeToString(b[:])

	var sysID int64
	err = s.db.QueryRowContext(s.ctx, `
		INSERT INTO cost_product_master
			(cpm_product_code, cpm_product_type_id, cpm_product_name, cpm_created_by, cpm_updated_by)
		VALUES ($1, $2, $3, 'itest', 'itest')
		RETURNING cpm_product_sys_id`,
		code, typeID, "ITEST MB Ref Resolve Fixture").Scan(&sysID)
	require.NoError(s.T(), err)
	return sysID
}

func (s *MBResolveRefProductSysIDSuite) insertHead(costing string, costProductID *int64) uuid.UUID {
	id := uuid.New()
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_head (mbh_id, mbh_mb_costing, mbh_cost_product_id, created_by)
		VALUES ($1, $2, $3, 'itest')`,
		id, costing, costProductID)
	require.NoError(s.T(), err)
	return id
}

// The regression case: a nested reference whose own cost product has not been generated yet
// (mbh_cost_product_id IS NULL) must fail with a clear, actionable message naming the
// dependency — not a raw "converting NULL to int64" scan error.
func (s *MBResolveRefProductSysIDSuite) TestResolve_NullCostProductID_ReturnsActionableError() {
	costing := mbRefResolveFixturePrefix + uuid.NewString()[:8]
	refID := s.insertHead(costing, nil)

	var resolveErr error
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		_, resolveErr = mbResolveRefProductSysID(s.ctx, tx, refID.String())
		return nil
	})
	require.NoError(s.T(), err)

	require.Error(s.T(), resolveErr)
	require.NotContains(s.T(), resolveErr.Error(), "converting NULL to int64",
		"must never surface the raw driver scan panic")
	require.Contains(s.T(), resolveErr.Error(), "mb_autogen: resolve nested MB cost product:")
	require.Contains(s.T(), resolveErr.Error(), costing,
		"error must name the dependency head so the user knows what to regenerate first")
	require.Contains(s.T(), resolveErr.Error(), "has not generated its cost product yet")
}

// The happy path: a resolved, non-NULL cost product ID is returned as-is.
//
// want must be a real cost_product_master row, not a hardcoded literal: migration
// 000517's `fk_mbh_cost_product` foreign key rejects mst_mb_head inserts whose
// mbh_cost_product_id does not exist in cost_product_master (SQLSTATE 23503). See
// insertCostProduct for why the fixture is created rather than borrowed from existing
// data.
func (s *MBResolveRefProductSysIDSuite) TestResolve_ValidCostProductID_ReturnsID() {
	costing := mbRefResolveFixturePrefix + uuid.NewString()[:8]
	want := s.insertCostProduct()
	refID := s.insertHead(costing, &want)

	var got int64
	var resolveErr error
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		got, resolveErr = mbResolveRefProductSysID(s.ctx, tx, refID.String())
		return nil
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), resolveErr)
	require.Equal(s.T(), want, got)
}

// A reference that does not resolve to any row (deleted or bogus mbh_id) must also fail
// clearly, not with a bare sql.ErrNoRows.
func (s *MBResolveRefProductSysIDSuite) TestResolve_NoSuchHead_ReturnsActionableError() {
	bogus := uuid.New()

	var resolveErr error
	err := s.db.Transaction(s.ctx, func(tx *sql.Tx) error {
		_, resolveErr = mbResolveRefProductSysID(s.ctx, tx, bogus.String())
		return nil
	})
	require.NoError(s.T(), err)

	require.Error(s.T(), resolveErr)
	require.Contains(s.T(), resolveErr.Error(), "mb_autogen: resolve nested MB cost product:")
	require.Contains(s.T(), resolveErr.Error(), "not found")
}
