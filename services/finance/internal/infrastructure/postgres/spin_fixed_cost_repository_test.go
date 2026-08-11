// Package postgres_test provides integration tests for SpinFixedCostRepository
// against a real PostgreSQL instance.
//
// SAFETY: the live table holds a load-bearing production anchor row (period 202604)
// that the calc engine reads for every POY product. These tests never touch it. All
// fixtures use throwaway periods outside any plausible business range:
//
//	2990xx — far future, used for CRUD/list fixtures
//	1900xx — far past, used for anchor-stats fixtures where the candidate must be
//	         EARLIER than every real row for the "earliest" branch to be reachable
//
// Both ranges are hard-deleted before and after every test.
package postgres_test

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// sfcCleanupSQL removes only the throwaway fixture ranges. It must never match a
// real period (real data is 20xxxx).
const sfcCleanupSQL = `DELETE FROM mst_spin_fixed_cost WHERE msfc_period LIKE '2990%' OR msfc_period LIKE '1900%'`

// SpinFixedCostRepoSuite exercises SpinFixedCostRepository against a real DB.
type SpinFixedCostRepoSuite struct {
	suite.Suite
	db   *postgres.DB
	repo *postgres.SpinFixedCostRepository
	ctx  context.Context
}

func TestSpinFixedCostRepoSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(SpinFixedCostRepoSuite))
}

func (s *SpinFixedCostRepoSuite) SetupSuite() {
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
	s.repo = postgres.NewSpinFixedCostRepository(s.db)
}

func (s *SpinFixedCostRepoSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	require.NoError(s.T(), s.db.Close())
}

func (s *SpinFixedCostRepoSuite) SetupTest() {
	s.cleanupFixtures()
}

func (s *SpinFixedCostRepoSuite) TearDownTest() {
	s.cleanupFixtures()
}

func (s *SpinFixedCostRepoSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, sfcCleanupSQL)
	require.NoError(s.T(), err)
}

// newFixture builds and persists a record for the given period.
func (s *SpinFixedCostRepoSuite) newFixture(period string) *spinfixedcost.Entity {
	e, err := spinfixedcost.New(spinfixedcost.NewInput{
		Period:             period,
		CommonPoyDenier:    100,
		PoyProduction:      1000,
		SpinPowerMonth:     10,
		SpinManpowerMonth:  20,
		SpinOverheadsMonth: 30,
		SpinConssprsMonth:  40,
		CreatedBy:          "integration_test",
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, e))
	return e
}

// =============================================================================
// Create / Get
// =============================================================================

// TestCreateGetByIDRoundTrip proves all six NUMERIC(20,6) columns survive the
// round trip with full 6-decimal precision, along with is_active and audit fields.
func (s *SpinFixedCostRepoSuite) TestCreateGetByIDRoundTrip() {
	const (
		denier     = 329.712345
		production = 3027153.123456
		power      = 198634.654321
		manpower   = 275561.111111
		overheads  = 46600.999999
		conssprs   = 54100.000001
	)

	entity, err := spinfixedcost.New(spinfixedcost.NewInput{
		Period:             "299001",
		CommonPoyDenier:    denier,
		PoyProduction:      production,
		SpinPowerMonth:     power,
		SpinManpowerMonth:  manpower,
		SpinOverheadsMonth: overheads,
		SpinConssprsMonth:  conssprs,
		CreatedBy:          "integration_test",
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, entity))

	got, err := s.repo.GetByID(s.ctx, entity.ID())
	require.NoError(s.T(), err)

	assert.Equal(s.T(), entity.ID(), got.ID())
	assert.Equal(s.T(), "299001", got.Period())
	// Exact equality: NUMERIC(20,6) must not truncate or round these away.
	assert.Equal(s.T(), denier, got.CommonPoyDenier())
	assert.Equal(s.T(), production, got.PoyProduction())
	assert.Equal(s.T(), power, got.SpinPowerMonth())
	assert.Equal(s.T(), manpower, got.SpinManpowerMonth())
	assert.Equal(s.T(), overheads, got.SpinOverheadsMonth())
	assert.Equal(s.T(), conssprs, got.SpinConssprsMonth())
	assert.True(s.T(), got.IsActive())
	assert.Equal(s.T(), "integration_test", got.CreatedBy())
	assert.False(s.T(), got.CreatedAt().IsZero())
	assert.Nil(s.T(), got.UpdatedAt())
	assert.Nil(s.T(), got.UpdatedBy())
	assert.Nil(s.T(), got.DeletedAt())
	assert.False(s.T(), got.IsDeleted())
}

// TestCreateDuplicateLivePeriod exercises the real uq_msfc_period_live partial
// unique index and the pq 23505 -> ErrDuplicatePeriod mapping in the repository.
func (s *SpinFixedCostRepoSuite) TestCreateDuplicateLivePeriod() {
	s.newFixture("299002")

	dup, err := spinfixedcost.New(spinfixedcost.NewInput{
		Period:             "299002",
		CommonPoyDenier:    1,
		PoyProduction:      1,
		SpinPowerMonth:     1,
		SpinManpowerMonth:  1,
		SpinOverheadsMonth: 1,
		SpinConssprsMonth:  1,
		CreatedBy:          "integration_test",
	})
	require.NoError(s.T(), err)

	err = s.repo.Create(s.ctx, dup)
	require.Error(s.T(), err)
	assert.True(s.T(), errors.Is(err, spinfixedcost.ErrDuplicatePeriod),
		"expected ErrDuplicatePeriod, got %v", err)
}

// TestCreateAfterSoftDeleteSamePeriod verifies the partial unique index is scoped
// WHERE deleted_at IS NULL, so soft-deleting a period frees the slot for reuse.
func (s *SpinFixedCostRepoSuite) TestCreateAfterSoftDeleteSamePeriod() {
	first := s.newFixture("299003")
	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, first.ID(), "integration_test"))

	second, err := spinfixedcost.New(spinfixedcost.NewInput{
		Period:             "299003",
		CommonPoyDenier:    2,
		PoyProduction:      2,
		SpinPowerMonth:     2,
		SpinManpowerMonth:  2,
		SpinOverheadsMonth: 2,
		SpinConssprsMonth:  2,
		CreatedBy:          "integration_test",
	})
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.Create(s.ctx, second), "soft-deleted period must free the unique slot")

	live, err := s.repo.GetByPeriod(s.ctx, "299003")
	require.NoError(s.T(), err)
	assert.Equal(s.T(), second.ID(), live.ID(), "GetByPeriod must return the live row, not the deleted one")
}

// TestGetByIDSoftDeletedNotFound asserts soft-deleted rows are invisible to GetByID.
func (s *SpinFixedCostRepoSuite) TestGetByIDSoftDeletedNotFound() {
	e := s.newFixture("299004")
	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, e.ID(), "integration_test"))

	got, err := s.repo.GetByID(s.ctx, e.ID())
	assert.Nil(s.T(), got)
	assert.True(s.T(), errors.Is(err, spinfixedcost.ErrNotFound), "expected ErrNotFound, got %v", err)
}

// TestGetByIDUnknownNotFound asserts a random UUID yields ErrNotFound.
func (s *SpinFixedCostRepoSuite) TestGetByIDUnknownNotFound() {
	got, err := s.repo.GetByID(s.ctx, uuid.New())
	assert.Nil(s.T(), got)
	assert.True(s.T(), errors.Is(err, spinfixedcost.ErrNotFound))
}

// TestExistsByPeriodAndID covers both existence probes including the soft-delete case.
func (s *SpinFixedCostRepoSuite) TestExistsByPeriodAndID() {
	e := s.newFixture("299005")

	exists, err := s.repo.ExistsByPeriod(s.ctx, "299005")
	require.NoError(s.T(), err)
	assert.True(s.T(), exists)

	exists, err = s.repo.ExistsByPeriod(s.ctx, "299099")
	require.NoError(s.T(), err)
	assert.False(s.T(), exists)

	exists, err = s.repo.ExistsByID(s.ctx, e.ID())
	require.NoError(s.T(), err)
	assert.True(s.T(), exists)

	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, e.ID(), "integration_test"))

	exists, err = s.repo.ExistsByPeriod(s.ctx, "299005")
	require.NoError(s.T(), err)
	assert.False(s.T(), exists, "soft-deleted period must not count as existing")

	exists, err = s.repo.ExistsByID(s.ctx, e.ID())
	require.NoError(s.T(), err)
	assert.False(s.T(), exists, "soft-deleted id must not count as existing")
}

// TestSoftDeleteTwice asserts the second soft delete reports ErrNotFound.
func (s *SpinFixedCostRepoSuite) TestSoftDeleteTwice() {
	e := s.newFixture("299006")
	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, e.ID(), "integration_test"))

	err := s.repo.SoftDelete(s.ctx, e.ID(), "integration_test")
	assert.True(s.T(), errors.Is(err, spinfixedcost.ErrNotFound), "expected ErrNotFound, got %v", err)
}

// =============================================================================
// Update
// =============================================================================

// TestUpdatePersistsAndLeavesPeriodUntouched verifies the UPDATE writes every
// mutable column, sets audit fields, and never rewrites msfc_period.
func (s *SpinFixedCostRepoSuite) TestUpdatePersistsAndLeavesPeriodUntouched() {
	e := s.newFixture("299007")

	newDenier := 555.123456
	newProduction := 6666.654321
	newPower := 7.7
	inactive := false
	require.NoError(s.T(), e.Update(spinfixedcost.UpdateInput{
		CommonPoyDenier: &newDenier,
		PoyProduction:   &newProduction,
		SpinPowerMonth:  &newPower,
		IsActive:        &inactive,
	}, "updater_user"))
	require.NoError(s.T(), s.repo.Update(s.ctx, e))

	got, err := s.repo.GetByID(s.ctx, e.ID())
	require.NoError(s.T(), err)
	assert.Equal(s.T(), "299007", got.Period(), "period must be immutable across updates")
	assert.Equal(s.T(), newDenier, got.CommonPoyDenier())
	assert.Equal(s.T(), newProduction, got.PoyProduction())
	assert.Equal(s.T(), newPower, got.SpinPowerMonth())
	assert.InDelta(s.T(), 20.0, got.SpinManpowerMonth(), 1e-9, "untouched column must be preserved")
	assert.False(s.T(), got.IsActive())
	require.NotNil(s.T(), got.UpdatedAt())
	require.NotNil(s.T(), got.UpdatedBy())
	assert.Equal(s.T(), "updater_user", *got.UpdatedBy())
}

// TestUpdateSoftDeletedRowNotFound asserts the WHERE deleted_at IS NULL guard.
func (s *SpinFixedCostRepoSuite) TestUpdateSoftDeletedRowNotFound() {
	e := s.newFixture("299008")
	// Snapshot the entity before the delete so it still looks live in memory.
	live, err := s.repo.GetByID(s.ctx, e.ID())
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, e.ID(), "integration_test"))

	power := 99.0
	require.NoError(s.T(), live.Update(spinfixedcost.UpdateInput{SpinPowerMonth: &power}, "updater"))
	err = s.repo.Update(s.ctx, live)
	assert.True(s.T(), errors.Is(err, spinfixedcost.ErrNotFound), "expected ErrNotFound, got %v", err)
}

// =============================================================================
// List
// =============================================================================

// TestListPagination checks page/pageSize slicing and the total count.
func (s *SpinFixedCostRepoSuite) TestListPagination() {
	periods := []string{"299011", "299012", "299013", "299014", "299015"}
	for _, p := range periods {
		s.newFixture(p)
	}

	page1, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "2990", Page: 1, PageSize: 2, SortBy: "period", SortOrder: "asc",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(len(periods)), total)
	require.Len(s.T(), page1, 2)
	assert.Equal(s.T(), "299011", page1[0].Period())
	assert.Equal(s.T(), "299012", page1[1].Period())

	page3, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "2990", Page: 3, PageSize: 2, SortBy: "period", SortOrder: "asc",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(len(periods)), total)
	require.Len(s.T(), page3, 1)
	assert.Equal(s.T(), "299015", page3[0].Period())
}

// TestListSortByPeriod covers asc and desc ordering.
func (s *SpinFixedCostRepoSuite) TestListSortByPeriod() {
	for _, p := range []string{"299021", "299023", "299022"} {
		s.newFixture(p)
	}

	asc, _, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29902", Page: 1, PageSize: 50, SortBy: "period", SortOrder: "asc",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), []string{"299021", "299022", "299023"}, sfcPeriods(asc))

	desc, _, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29902", Page: 1, PageSize: 50, SortBy: "period", SortOrder: "desc",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), []string{"299023", "299022", "299021"}, sfcPeriods(desc))
}

// TestListLexicographicIsChronological guards the assumption the calc engine and
// the anchor guard both rely on: zero-padded YYYYMM sorts chronologically as text.
func (s *SpinFixedCostRepoSuite) TestListLexicographicIsChronological() {
	for _, p := range []string{"299012", "299010", "299001", "299009"} {
		s.newFixture(p)
	}

	asc, _, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "2990", Page: 1, PageSize: 50, SortBy: "period", SortOrder: "asc",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), []string{"299001", "299009", "299010", "299012"}, sfcPeriods(asc),
		"month 09 must sort before month 10, not after")
}

// TestListPeriodFilter checks the exact-match period filter.
func (s *SpinFixedCostRepoSuite) TestListPeriodFilter() {
	s.newFixture("299031")
	s.newFixture("299032")

	items, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Period: "299031", Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	require.Len(s.T(), items, 1)
	assert.Equal(s.T(), "299031", items[0].Period())
}

// TestListSearchFilter checks the ILIKE search over the period column.
func (s *SpinFixedCostRepoSuite) TestListSearchFilter() {
	s.newFixture("299041")
	s.newFixture("299042")
	s.newFixture("190041")

	items, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29904", Page: 1, PageSize: 50, SortBy: "period", SortOrder: "asc",
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Equal(s.T(), []string{"299041", "299042"}, sfcPeriods(items))

	none, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "zzzz", Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(0), total)
	assert.Empty(s.T(), none)
}

// TestListActiveFilter covers active-only, inactive-only, and unfiltered.
func (s *SpinFixedCostRepoSuite) TestListActiveFilter() {
	active := s.newFixture("299051")
	inactive := s.newFixture("299052")

	off := false
	require.NoError(s.T(), inactive.Update(spinfixedcost.UpdateInput{IsActive: &off}, "integration_test"))
	require.NoError(s.T(), s.repo.Update(s.ctx, inactive))

	yes, no := true, false

	onlyActive, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29905", IsActive: &yes, Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	require.Len(s.T(), onlyActive, 1)
	assert.Equal(s.T(), active.Period(), onlyActive[0].Period())

	onlyInactive, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29905", IsActive: &no, Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	require.Len(s.T(), onlyInactive, 1)
	assert.Equal(s.T(), inactive.Period(), onlyInactive[0].Period())

	all, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29905", Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(2), total)
	assert.Len(s.T(), all, 2)
}

// TestListExcludesSoftDeleted asserts deleted rows never appear in List.
func (s *SpinFixedCostRepoSuite) TestListExcludesSoftDeleted() {
	keep := s.newFixture("299061")
	drop := s.newFixture("299062")
	require.NoError(s.T(), s.repo.SoftDelete(s.ctx, drop.ID(), "integration_test"))

	items, total, err := s.repo.List(s.ctx, spinfixedcost.ListFilter{
		Search: "29906", Page: 1, PageSize: 50,
	})
	require.NoError(s.T(), err)
	assert.Equal(s.T(), int64(1), total)
	require.Len(s.T(), items, 1)
	assert.Equal(s.T(), keep.Period(), items[0].Period())
}

// =============================================================================
// Anchor stats
// =============================================================================

// TestLoadAnchorStats verifies the three aggregates the anchor guard depends on,
// for each candidate in turn. Fixtures use far-PAST periods so the candidates can
// legitimately be the earliest live+active row without disturbing the real anchor
// (which stays the earliest of the real data and is never a candidate here).
func (s *SpinFixedCostRepoSuite) TestLoadAnchorStats() {
	baselineActive, baselineEarliest := s.liveActiveBaseline()
	require.NotEmpty(s.T(), baselineEarliest,
		"expected the production anchor row to be present; refusing to assert against an empty table")

	a := s.newFixture("190001")
	b := s.newFixture("190002")
	c := s.newFixture("190003")

	// Excluding the earliest fixture: the next fixture becomes the earliest remaining.
	stats, err := s.repo.LoadAnchorStats(s.ctx, a.ID())
	require.NoError(s.T(), err)
	assert.Equal(s.T(), baselineActive+2, stats.RemainingActiveCount)
	assert.Equal(s.T(), "190002", stats.EarliestRemainingActivePeriod)
	assert.True(s.T(), stats.HasLiveRowAfterCandidate)

	// Excluding a middle fixture: earliest is unchanged, later rows still exist.
	stats, err = s.repo.LoadAnchorStats(s.ctx, b.ID())
	require.NoError(s.T(), err)
	assert.Equal(s.T(), baselineActive+2, stats.RemainingActiveCount)
	assert.Equal(s.T(), "190001", stats.EarliestRemainingActivePeriod)
	assert.True(s.T(), stats.HasLiveRowAfterCandidate)

	// Excluding the newest fixture: real rows are still later than 190003.
	stats, err = s.repo.LoadAnchorStats(s.ctx, c.ID())
	require.NoError(s.T(), err)
	assert.Equal(s.T(), baselineActive+2, stats.RemainingActiveCount)
	assert.Equal(s.T(), "190001", stats.EarliestRemainingActivePeriod)
	assert.True(s.T(), stats.HasLiveRowAfterCandidate,
		"the production anchor sits after 190003, so later live rows exist")

	// Deactivating a fixture drops it out of the active aggregates but it remains
	// live for the has-later-rows probe.
	off := false
	require.NoError(s.T(), a.Update(spinfixedcost.UpdateInput{IsActive: &off}, "integration_test"))
	require.NoError(s.T(), s.repo.Update(s.ctx, a))

	stats, err = s.repo.LoadAnchorStats(s.ctx, c.ID())
	require.NoError(s.T(), err)
	assert.Equal(s.T(), baselineActive+1, stats.RemainingActiveCount)
	assert.Equal(s.T(), "190002", stats.EarliestRemainingActivePeriod,
		"an inactive row must not count as the earliest active anchor")
}

// TestAnchorGuardAgainstRealStats wires the real LoadAnchorStats output into the
// pure guard, covering the earliest-row refusal and the safe middle-row case.
func (s *SpinFixedCostRepoSuite) TestAnchorGuardAgainstRealStats() {
	earliest := s.newFixture("190011")
	middle := s.newFixture("190012")

	stats, err := s.repo.LoadAnchorStats(s.ctx, earliest.ID())
	require.NoError(s.T(), err)
	guardErr := spinfixedcost.CheckAnchorGuard(earliest, stats)
	assert.True(s.T(), errors.Is(guardErr, spinfixedcost.ErrAnchorRowEarliest),
		"expected ErrAnchorRowEarliest, got %v", guardErr)

	stats, err = s.repo.LoadAnchorStats(s.ctx, middle.ID())
	require.NoError(s.T(), err)
	assert.NoError(s.T(), spinfixedcost.CheckAnchorGuard(middle, stats),
		"a non-earliest row must be removable")
}

// liveActiveBaseline returns the number of live+active rows and the earliest live+active
// period BEFORE this test's fixtures are inserted.
func (s *SpinFixedCostRepoSuite) liveActiveBaseline() (int64, string) {
	var count int64
	var earliest string
	err := s.db.QueryRowContext(s.ctx, `
		SELECT COUNT(*), COALESCE(MIN(msfc_period), '')
		FROM mst_spin_fixed_cost
		WHERE deleted_at IS NULL AND msfc_is_active`).Scan(&count, &earliest)
	require.NoError(s.T(), err)
	return count, earliest
}

func sfcPeriods(items []*spinfixedcost.Entity) []string {
	out := make([]string, len(items))
	for i, e := range items {
		out[i] = e.Period()
	}
	return out
}
