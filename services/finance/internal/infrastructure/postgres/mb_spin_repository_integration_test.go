// Package postgres_test — IT-4: determinism of the MB Spin lookup getters (D20 / M2a).
//
// Gated by INTEGRATION_TEST=true; requires a reachable PostgreSQL (defaults match the
// docker-compose finance DB).
//
// SAFETY: mst_mb_spin holds real Oracle-seeded production rows. These tests never touch
// them. Every fixture key is prefixed with ITEST-MBSPIN- (a shape no real mb_costing or
// ORION item code takes), and the whole prefix range is hard-deleted before and after
// each test.
//
// What is locked in here (design §8.2 IT-4):
//
//	(a) duplicate key with many spins  -> winner is stable across repeated calls
//	(b) key whose spins are all inactive -> still returns a row, NOT ErrNotFound
//	    (this is what proves no mbs_is_active filter was added)
//	(c) tied created_at                -> mbs_id ASC breaks the tie
//	(d) soft-deleted source spin       -> winner does not move to a NULL-orion clone
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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// mbSpinFixturePrefix marks every row this suite creates. It must never match real data.
const mbSpinFixturePrefix = "ITEST-MBSPIN-"

// mbSpinCleanupSQL hard-deletes only this suite's fixture rows.
const mbSpinCleanupSQL = `DELETE FROM mst_mb_spin
	WHERE mbs_mb_costing LIKE 'ITEST-MBSPIN-%'
	   OR mbs_orion_item_code LIKE 'ITEST-MBSPIN-%'
	   OR mbs_mgt_name LIKE 'ITEST-MBSPIN-%'`

// MBSpinLookupDeterminismSuite exercises the two lookup getters against a real DB.
type MBSpinLookupDeterminismSuite struct {
	suite.Suite
	db     *postgres.DB
	repo   *postgres.MBSpinRepository
	ctx    context.Context
	headID uuid.UUID
}

func TestMBSpinLookupDeterminismSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(MBSpinLookupDeterminismSuite))
}

func (s *MBSpinLookupDeterminismSuite) SetupSuite() {
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
	s.repo = postgres.NewMBSpinRepository(s.db)

	// mbs_mbh_id is NOT NULL and FK-constrained, so borrow any existing head. The tests
	// never write to mst_mb_head.
	require.NoError(s.T(),
		s.db.QueryRowContext(s.ctx,
			`SELECT mbh_id FROM mst_mb_head WHERE deleted_at IS NULL LIMIT 1`,
		).Scan(&s.headID),
		"integration DB needs at least one mst_mb_head row")
}

func (s *MBSpinLookupDeterminismSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *MBSpinLookupDeterminismSuite) SetupTest()    { s.cleanupFixtures() }
func (s *MBSpinLookupDeterminismSuite) TearDownTest() { s.cleanupFixtures() }

func (s *MBSpinLookupDeterminismSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, mbSpinCleanupSQL)
	require.NoError(s.T(), err)
}

// insertSpin writes one fixture row with fully controlled mbs_id / created_at so the
// ORDER BY under test is the only thing deciding the winner. orionCode nil => NULL.
func (s *MBSpinLookupDeterminismSuite) insertSpin(
	id uuid.UUID, mbCosting string, orionCode *string, createdAt time.Time, isActive bool,
) {
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_mbh_id, mbs_mgt_name, mbs_mb_costing, mbs_orion_item_code,
			mbs_is_active, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, 'itest')`,
		id, s.headID, mbSpinFixturePrefix+"mgt", mbCosting, orionCode, isActive, createdAt)
	require.NoError(s.T(), err)
}

// softDeleteSpin marks a fixture row deleted the way the repository's soft delete does.
func (s *MBSpinLookupDeterminismSuite) softDeleteSpin(id uuid.UUID) {
	_, err := s.db.ExecContext(s.ctx,
		`UPDATE mst_mb_spin SET deleted_at = NOW(), deleted_by = 'itest' WHERE mbs_id = $1`, id)
	require.NoError(s.T(), err)
}

// baseTime is a fixed instant so fixture ordering never depends on wall-clock timing.
func baseTime() time.Time {
	return time.Date(2020, time.January, 1, 0, 0, 0, 0, time.UTC)
}

// (a) Pattern MBB0000041: many spins on one key, only one active. The winner must be the
// oldest row and must not change across repeated calls.
func (s *MBSpinLookupDeterminismSuite) TestGetByMBCosting_DuplicateKey_WinnerIsStableOldestRow() {
	key := mbSpinFixturePrefix + "DUP-MBC"
	oldest := uuid.MustParse("00000000-0000-0000-0000-0000000000aa")

	// Oldest row is inactive on purpose: with a mbs_is_active filter it would lose.
	s.insertSpin(oldest, key, nil, baseTime(), false)
	for i := 1; i < 16; i++ {
		s.insertSpin(uuid.New(), key, nil, baseTime().Add(time.Duration(i)*time.Hour), i == 7)
	}

	for i := 0; i < 100; i++ {
		got, err := s.repo.GetByMBCosting(s.ctx, key)
		require.NoError(s.T(), err)
		require.Equal(s.T(), oldest, got.ID(), "winner drifted on call %d", i)
	}
}

// Same guarantee for the ORION getter — the two getters must agree in behavior.
func (s *MBSpinLookupDeterminismSuite) TestGetByOrionItemCode_DuplicateKey_WinnerIsStableOldestRow() {
	key := mbSpinFixturePrefix + "DUP-ORION"
	oldest := uuid.MustParse("00000000-0000-0000-0000-0000000000bb")

	s.insertSpin(oldest, mbSpinFixturePrefix+"x0", &key, baseTime(), false)
	for i := 1; i < 16; i++ {
		code := key
		s.insertSpin(uuid.New(), fmt.Sprintf("%sx%d", mbSpinFixturePrefix, i), &code,
			baseTime().Add(time.Duration(i)*time.Hour), i == 7)
	}

	for i := 0; i < 100; i++ {
		got, err := s.repo.GetByOrionItemCode(s.ctx, key)
		require.NoError(s.T(), err)
		require.Equal(s.T(), oldest, got.ID(), "winner drifted on call %d", i)
	}
}

// (b) Pattern MBC0000246: zero active spins on the key. The lookup must STILL return a
// row. If someone adds `AND mbs_is_active = TRUE`, this test fails with ErrNotFound.
func (s *MBSpinLookupDeterminismSuite) TestLookups_KeyWithNoActiveSpins_StillReturnsRow() {
	mbKey := mbSpinFixturePrefix + "NOACTIVE-MBC"
	orionKey := mbSpinFixturePrefix + "NOACTIVE-ORION"
	first := uuid.MustParse("00000000-0000-0000-0000-0000000000cc")

	s.insertSpin(first, mbKey, &orionKey, baseTime(), false)
	s.insertSpin(uuid.New(), mbKey, &orionKey, baseTime().Add(time.Hour), false)

	byMB, err := s.repo.GetByMBCosting(s.ctx, mbKey)
	require.NoError(s.T(), err, "must not be ErrNotFound — no mbs_is_active filter is allowed")
	require.Equal(s.T(), first, byMB.ID())
	require.False(s.T(), byMB.IsActive())

	byOrion, err := s.repo.GetByOrionItemCode(s.ctx, orionKey)
	require.NoError(s.T(), err, "must not be ErrNotFound — no mbs_is_active filter is allowed")
	require.Equal(s.T(), first, byOrion.ID())
}

// (c) Tied created_at — created_at alone cannot decide, so mbs_id ASC must.
func (s *MBSpinLookupDeterminismSuite) TestLookups_TiedCreatedAt_ResolvedByLowestID() {
	mbKey := mbSpinFixturePrefix + "TIE-MBC"
	orionKey := mbSpinFixturePrefix + "TIE-ORION"
	tie := baseTime()

	lowest := uuid.MustParse("00000000-0000-0000-0000-000000000001")
	middle := uuid.MustParse("00000000-0000-0000-0000-000000000002")
	highest := uuid.MustParse("00000000-0000-0000-0000-000000000003")

	// Insert out of order so insertion order cannot be mistaken for the tie-breaker.
	s.insertSpin(highest, mbKey, &orionKey, tie, true)
	s.insertSpin(lowest, mbKey, &orionKey, tie, true)
	s.insertSpin(middle, mbKey, &orionKey, tie, true)

	for i := 0; i < 25; i++ {
		byMB, err := s.repo.GetByMBCosting(s.ctx, mbKey)
		require.NoError(s.T(), err)
		require.Equal(s.T(), lowest, byMB.ID())

		byOrion, err := s.repo.GetByOrionItemCode(s.ctx, orionKey)
		require.NoError(s.T(), err)
		require.Equal(s.T(), lowest, byOrion.ID())
	}
}

// (d) Soft-deleting the source spin must not hand the win to a clone. The clone carries a
// NULL mbs_orion_item_code, so the ORION lookup falls through to ErrNotFound rather than
// silently resolving to a different record.
func (s *MBSpinLookupDeterminismSuite) TestGetByOrionItemCode_SoftDeletedSource_DoesNotFallToClone() {
	orionKey := mbSpinFixturePrefix + "SOFTDEL-ORION"
	mbKey := mbSpinFixturePrefix + "SOFTDEL-MBC"

	source := uuid.MustParse("00000000-0000-0000-0000-0000000000d1")
	clone := uuid.MustParse("00000000-0000-0000-0000-0000000000d2")

	s.insertSpin(source, mbKey, &orionKey, baseTime(), true)
	s.insertSpin(clone, mbKey, nil, baseTime().Add(time.Hour), true) // clone: orion is NULL

	got, err := s.repo.GetByOrionItemCode(s.ctx, orionKey)
	require.NoError(s.T(), err)
	require.Equal(s.T(), source, got.ID())

	s.softDeleteSpin(source)

	_, err = s.repo.GetByOrionItemCode(s.ctx, orionKey)
	require.ErrorIs(s.T(), err, mbspin.ErrNotFound, "must not resolve to the NULL-orion clone")

	// The mb_costing lookup still works and now deterministically picks the clone.
	byMB, err := s.repo.GetByMBCosting(s.ctx, mbKey)
	require.NoError(s.T(), err)
	require.Equal(s.T(), clone, byMB.ID())
}

// ResolveUniqueByOrionItemCode is the SAVE-time resolver for cpp_value_mb_spin_id
// (migration 000494). Unlike the two getters above, it must NEVER pick a winner
// among duplicates — these tests lock in that it reports ok=false rather than an
// arbitrary row whenever the code is shared by more than one spin, matches the
// GetByOrionItemCode getter's exact single match, or matches nothing at all.
func (s *MBSpinLookupDeterminismSuite) TestResolveUniqueByOrionItemCode_SingleMatch_ReturnsIt() {
	orionKey := mbSpinFixturePrefix + "UNIQUE-ORION"
	only := uuid.MustParse("00000000-0000-0000-0000-0000000000e1")

	s.insertSpin(only, mbSpinFixturePrefix+"mbc-e1", &orionKey, baseTime(), true)

	id, ok, err := s.repo.ResolveUniqueByOrionItemCode(s.ctx, orionKey)
	require.NoError(s.T(), err)
	require.True(s.T(), ok)
	require.Equal(s.T(), only, id)
}

func (s *MBSpinLookupDeterminismSuite) TestResolveUniqueByOrionItemCode_MultipleMatches_ReportsAmbiguous() {
	orionKey := mbSpinFixturePrefix + "DUP-RESOLVE-ORION"

	s.insertSpin(uuid.New(), mbSpinFixturePrefix+"mbc-e2a", &orionKey, baseTime(), true)
	s.insertSpin(uuid.New(), mbSpinFixturePrefix+"mbc-e2b", &orionKey, baseTime().Add(time.Hour), true)

	id, ok, err := s.repo.ResolveUniqueByOrionItemCode(s.ctx, orionKey)
	require.NoError(s.T(), err)
	require.False(s.T(), ok, "must refuse to pick a winner among duplicates")
	require.Equal(s.T(), uuid.UUID{}, id)
}

func (s *MBSpinLookupDeterminismSuite) TestResolveUniqueByOrionItemCode_NoMatch_ReportsAmbiguous() {
	id, ok, err := s.repo.ResolveUniqueByOrionItemCode(s.ctx, mbSpinFixturePrefix+"NO-SUCH-CODE")
	require.NoError(s.T(), err)
	require.False(s.T(), ok)
	require.Equal(s.T(), uuid.UUID{}, id)
}

// A soft-deleted spin must not count toward uniqueness in either direction: it
// must not make an otherwise-unique code ambiguous, and it must not itself be
// returned as the resolved winner.
func (s *MBSpinLookupDeterminismSuite) TestResolveUniqueByOrionItemCode_SoftDeletedSpinExcluded() {
	orionKey := mbSpinFixturePrefix + "SOFTDEL-RESOLVE-ORION"
	live := uuid.MustParse("00000000-0000-0000-0000-0000000000e3")
	deleted := uuid.MustParse("00000000-0000-0000-0000-0000000000e4")

	s.insertSpin(live, mbSpinFixturePrefix+"mbc-e3", &orionKey, baseTime(), true)
	s.insertSpin(deleted, mbSpinFixturePrefix+"mbc-e4", &orionKey, baseTime().Add(time.Hour), true)
	s.softDeleteSpin(deleted)

	id, ok, err := s.repo.ResolveUniqueByOrionItemCode(s.ctx, orionKey)
	require.NoError(s.T(), err)
	require.True(s.T(), ok, "soft-deleted duplicate must not make this ambiguous")
	require.Equal(s.T(), live, id)
}
