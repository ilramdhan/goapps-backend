package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

// =============================================================================
// Integration tests for LoadSpinFixedCost period resolution.
//
// SAFETY: mst_spin_fixed_cost carries a load-bearing anchor row (period 202604)
// that the calc engine reads for every POY product. This suite never inserts,
// updates or deletes that period. Every fixture uses a throwaway range:
//
//	1800xx / 1900xx — far past. Needed because the anchor is live: with a real row
//	                  at 202604, a FUTURE request can never be the "no row at or
//	                  before the period" case. Only a request earlier than every
//	                  live row reaches that branch.
//	2990xx          — far future, for exact-match and fallback cases where the
//	                  resolved row must be a fixture rather than the anchor.
//
// Cleanup is scoped to those three prefixes and runs before and after every test,
// and the suite asserts on teardown that the table is back to exactly the anchor.
//
// The suite locks BOTH halves of the resolution contract on purpose. Tightening
// the loader's `msfc_period <= $1` to `= $1` would still satisfy a test set that
// only proved the guard blocks an absent pool, while breaking every local and dev
// database whose only row is the 202604 anchor. TestFallback... is the test that
// fails in that scenario.
// =============================================================================

// spinAnchorPeriod is the seeded production anchor (migration 000474). Never touched.
const spinAnchorPeriod = "202604"

// spinFixtureCleanupSQL removes only the throwaway fixture ranges. It must never
// match a real period (real data is 20xxxx).
const spinFixtureCleanupSQL = `
	DELETE FROM mst_spin_fixed_cost
	WHERE msfc_period LIKE '1800%'
	   OR msfc_period LIKE '1900%'
	   OR msfc_period LIKE '2990%'`

// spinFixtureValues is one row's worth of pool values. Each column gets a
// distinct value so a transposed SELECT list cannot pass.
type spinFixtureValues struct {
	denier     float64
	production float64
	power      float64
	manpower   float64
	overheads  float64
	conssprs   float64
}

// spinValuesFor derives a per-period value set from a seed, keeping every column
// distinct from every other column AND from the same column in another fixture.
func spinValuesFor(seed float64) spinFixtureValues {
	return spinFixtureValues{
		denier:     seed + 1.111111,
		production: seed + 2.222222,
		power:      seed + 3.333333,
		manpower:   seed + 4.444444,
		overheads:  seed + 5.555555,
		conssprs:   seed + 6.666666,
	}
}

type SpinFixedCostLoaderSuite struct {
	suite.Suite
	ctx    context.Context
	db     *sql.DB
	loader ProductLoader
}

func TestSpinFixedCostLoaderSuite(t *testing.T) {
	if os.Getenv("INTEGRATION_TEST") != "true" {
		t.Skip("Skipping integration test. Set INTEGRATION_TEST=true to run.")
	}
	suite.Run(t, new(SpinFixedCostLoaderSuite))
}

func (s *SpinFixedCostLoaderSuite) SetupSuite() {
	s.ctx = context.Background()

	host := envOr("TEST_DB_HOST", "localhost")
	port := envOr("TEST_DB_PORT", "5434")
	user := envOr("TEST_DB_USER", "finance")
	password := envOr("TEST_DB_PASSWORD", "finance123")
	dbname := envOr("TEST_DB_NAME", "finance_db")
	dsn := fmt.Sprintf("host=%s port=%s user=%s password=%s dbname=%s sslmode=disable",
		host, port, user, password, dbname)

	raw, err := sql.Open("postgres", dsn)
	require.NoError(s.T(), err)
	require.NoError(s.T(), waitDB(raw, 10*time.Second))
	s.db = raw
	s.loader = NewProductLoader(raw)

	// Refuse to run against a table that does not look like the one these tests
	// reason about. Asserting "resolves to the anchor" is meaningless if the
	// anchor is absent, and would pass vacuously.
	s.requireAnchorIntact()
}

func (s *SpinFixedCostLoaderSuite) TearDownSuite() {
	if s.db == nil {
		return
	}
	s.cleanupFixtures()
	s.requireAnchorIntact()

	// The whole table must be back to exactly the anchor row: nothing added,
	// nothing left inactive or soft-deleted by a fixture.
	var total int
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx,
		`SELECT COUNT(*) FROM mst_spin_fixed_cost`).Scan(&total))
	require.Equal(s.T(), 1, total,
		"mst_spin_fixed_cost must end the run holding only the %s anchor row", spinAnchorPeriod)

	require.NoError(s.T(), s.db.Close())
}

func (s *SpinFixedCostLoaderSuite) SetupTest()    { s.cleanupFixtures() }
func (s *SpinFixedCostLoaderSuite) TearDownTest() { s.cleanupFixtures() }

func (s *SpinFixedCostLoaderSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(s.ctx, spinFixtureCleanupSQL)
	require.NoError(s.T(), err)
}

// requireAnchorIntact asserts the production anchor is present, active and undeleted.
func (s *SpinFixedCostLoaderSuite) requireAnchorIntact() {
	var isActive bool
	var deletedAt sql.NullTime
	err := s.db.QueryRowContext(s.ctx, `
		SELECT msfc_is_active, deleted_at
		FROM mst_spin_fixed_cost
		WHERE msfc_period = $1`, spinAnchorPeriod).Scan(&isActive, &deletedAt)
	require.NoError(s.T(), err,
		"the %s anchor row must exist; run migration 000474", spinAnchorPeriod)
	require.True(s.T(), isActive, "the %s anchor row must stay active", spinAnchorPeriod)
	require.False(s.T(), deletedAt.Valid, "the %s anchor row must stay undeleted", spinAnchorPeriod)
}

// insertFixture persists one fixture row in the given state and returns its values.
func (s *SpinFixedCostLoaderSuite) insertFixture(period string, seed float64, isActive, isDeleted bool) spinFixtureValues {
	require.NotEqual(s.T(), spinAnchorPeriod, period, "fixtures must never use the anchor period")

	v := spinValuesFor(seed)
	var deletedAt sql.NullTime
	var deletedBy sql.NullString
	if isDeleted {
		deletedAt = sql.NullTime{Time: time.Now(), Valid: true}
		deletedBy = sql.NullString{String: "spin-loader-test", Valid: true}
	}

	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_spin_fixed_cost (
			msfc_period, msfc_common_poy_denier, msfc_poy_production,
			msfc_spin_power_month, msfc_spin_manpower_month,
			msfc_spin_overheads_month, msfc_spin_conssprs_month,
			msfc_is_active, msfc_created_by, deleted_at, deleted_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 'spin-loader-test', $9, $10)`,
		period, v.denier, v.production, v.power, v.manpower, v.overheads, v.conssprs,
		isActive, deletedAt, deletedBy)
	require.NoError(s.T(), err)
	return v
}

// requireValues asserts every one of the six scope keys carries the fixture value.
// Checking all six is what catches a transposed SELECT list; checking one would not.
func (s *SpinFixedCostLoaderSuite) requireValues(got map[string]float64, want spinFixtureValues) {
	require.Len(s.T(), got, 6, "the pool must carry exactly the six scope keys")
	assert.InDelta(s.T(), want.denier, got[ScopeKeySpinCommonPOYDenier], 0.000001)
	assert.InDelta(s.T(), want.production, got[ScopeKeySpinPOYProduction], 0.000001)
	assert.InDelta(s.T(), want.power, got[ScopeKeySpinPowerMonth], 0.000001)
	assert.InDelta(s.T(), want.manpower, got[ScopeKeySpinManpowerMonth], 0.000001)
	assert.InDelta(s.T(), want.overheads, got[ScopeKeySpinOverheadsMonth], 0.000001)
	assert.InDelta(s.T(), want.conssprs, got[ScopeKeySpinConsSprsMonth], 0.000001)
}

// =============================================================================
// Exact match
// =============================================================================

// TestExactMatch_ReturnsRequestedPeriodAndValues covers the ordinary path: a row
// exists for the requested period, so no carry-forward happens.
func (s *SpinFixedCostLoaderSuite) TestExactMatch_ReturnsRequestedPeriodAndValues() {
	want := s.insertFixture("299005", 5000, true, false)

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299005")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "299005", pool.Period, "an exact row must resolve to itself, not to an older one")
	s.requireValues(pool.Values, want)
}

// TestExactMatch_PrefersOwnRowOverEarlierOnes proves the ORDER BY DESC is doing
// real work: with earlier rows present the exact row still wins.
func (s *SpinFixedCostLoaderSuite) TestExactMatch_PrefersOwnRowOverEarlierOnes() {
	s.insertFixture("299001", 1000, true, false)
	s.insertFixture("299002", 2000, true, false)
	want := s.insertFixture("299003", 3000, true, false)

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299003")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "299003", pool.Period)
	s.requireValues(pool.Values, want)
}

// =============================================================================
// Carry-forward — the half a "= $1" regression would break
// =============================================================================

// TestFallback_LaterPeriodResolvesToOlderRow is the load-bearing test for the
// carry-forward contract. Legacy MST_PARAM_DATA was current-only, and on most
// databases the 202604 anchor is the only row, so a request for any later period
// MUST resolve to it rather than fail. If someone changes `msfc_period <= $1` to
// `= $1`, this is what turns red.
func (s *SpinFixedCostLoaderSuite) TestFallback_LaterPeriodResolvesToOlderRow() {
	want := s.insertFixture("299010", 10000, true, false)

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299099")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "299010", pool.Period,
		"a period with no exact row must carry the newest earlier row forward, not fail")
	// The carried-forward values must be the older row's, not zeroes.
	s.requireValues(pool.Values, want)
}

// TestFallback_ResolvesToProductionAnchor is the same contract expressed against
// the real seeded row, which is the shape every clean local and dev database has:
// one anchor at 202604 and nothing else.
func (s *SpinFixedCostLoaderSuite) TestFallback_ResolvesToProductionAnchor() {
	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299099")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), spinAnchorPeriod, pool.Period,
		"with only the anchor present, every later period must resolve to it")
	require.Len(s.T(), pool.Values, 6)
	assert.Positive(s.T(), pool.Values[ScopeKeySpinPOYProduction],
		"a zero divisor would make every POY per-kg formula guard to 0")
	assert.Positive(s.T(), pool.Values[ScopeKeySpinCommonPOYDenier])
}

// TestFallback_PicksNewestRowAtOrBeforePeriod pins the direction of the sort. The
// middle request must not pick up the later row (that would let a future pool leak
// into a recompute of an earlier period) nor the earliest one.
func (s *SpinFixedCostLoaderSuite) TestFallback_PicksNewestRowAtOrBeforePeriod() {
	s.insertFixture("299020", 20000, true, false)
	want := s.insertFixture("299021", 21000, true, false)
	s.insertFixture("299023", 23000, true, false)

	// 299022 has no row: newest at-or-before is 299021.
	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299022")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "299021", pool.Period,
		"must take the newest row at or before the period, never the globally newest")
	s.requireValues(pool.Values, want)
}

// TestCutoff_FuturePoolNeverLeaksBackwards states the same rule from the other
// side: costing an earlier period must not read a later month's pool.
func (s *SpinFixedCostLoaderSuite) TestCutoff_FuturePoolNeverLeaksBackwards() {
	want := s.insertFixture("190030", 30000, true, false)
	s.insertFixture("190031", 31000, true, false)

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "190030")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "190030", pool.Period)
	s.requireValues(pool.Values, want)
}

// =============================================================================
// No row at or before the period — the fatal case
// =============================================================================

// TestNoRowAtOrBefore_ReturnsZeroPoolAndNilError proves the loader reports absence
// as a zero-value SpinPool rather than an error: fatality is bulkLoad's call, and
// bulkLoad keys off the empty Period.
//
// The request period is far-past on purpose. With the 202604 anchor live, no
// future period can ever reach this branch.
func (s *SpinFixedCostLoaderSuite) TestNoRowAtOrBefore_ReturnsZeroPoolAndNilError() {
	// Rows exist, but all of them are LATER than the requested period.
	s.insertFixture("190001", 1000, true, false)

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "180001")
	require.NoError(s.T(), err, "absence is not a loader error")

	assert.Empty(s.T(), pool.Period,
		"an empty Period is the signal bulkLoad turns into ErrMissingSpinFixedCost")
	assert.Nil(s.T(), pool.Values,
		"Values must be nil, not an empty map: a map would read as a real pool of zeroes")
	assert.Equal(s.T(), SpinPool{}, pool)
}

// =============================================================================
// Liveness predicates
// =============================================================================

// TestSoftDeletedRowIsNotResolved covers deleted_at IS NULL. The soft-deleted row
// sits at the exact requested period, so a loader that ignored deleted_at would
// return it and this test would catch the exact value.
func (s *SpinFixedCostLoaderSuite) TestSoftDeletedRowIsNotResolved() {
	want := s.insertFixture("299030", 30000, true, false)
	s.insertFixture("299031", 31000, true, true) // soft-deleted, exact match

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299031")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "299030", pool.Period, "a soft-deleted row must be invisible to the loader")
	s.requireValues(pool.Values, want)
}

// TestInactiveRowIsNotResolved covers msfc_is_active = TRUE, same shape.
func (s *SpinFixedCostLoaderSuite) TestInactiveRowIsNotResolved() {
	want := s.insertFixture("299040", 40000, true, false)
	s.insertFixture("299041", 41000, false, false) // inactive, exact match

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "299041")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), "299040", pool.Period, "an inactive row must be invisible to the loader")
	s.requireValues(pool.Values, want)
}

// TestOnlyNonLiveRowsBeforePeriod_ReturnsZeroPool combines both predicates with
// the absence branch: rows exist at or before the period but none is live, which
// must read the same as no rows at all.
func (s *SpinFixedCostLoaderSuite) TestOnlyNonLiveRowsBeforePeriod_ReturnsZeroPool() {
	s.insertFixture("180050", 50000, false, false) // inactive
	s.insertFixture("180051", 51000, true, true)   // soft-deleted

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, "180052")
	require.NoError(s.T(), err)

	assert.Equal(s.T(), SpinPool{}, pool,
		"non-live rows must not rescue a period from the missing-pool guard")
}

// =============================================================================
// Precision
// =============================================================================

// TestValuesSurviveNumericPrecision guards the NUMERIC(20,6) → float64 scan. Six
// decimals is the column's full precision; losing them would shift per-kg costs.
func (s *SpinFixedCostLoaderSuite) TestValuesSurviveNumericPrecision() {
	const period = "299050"
	_, err := s.db.ExecContext(s.ctx, `
		INSERT INTO mst_spin_fixed_cost (
			msfc_period, msfc_common_poy_denier, msfc_poy_production,
			msfc_spin_power_month, msfc_spin_manpower_month,
			msfc_spin_overheads_month, msfc_spin_conssprs_month, msfc_created_by)
		VALUES ($1, 329.712345, 3027153.123456, 198634.111111, 275561.222222,
		        46600.333333, 54100.444444, 'spin-loader-test')`, period)
	require.NoError(s.T(), err)

	pool, err := s.loader.LoadSpinFixedCost(s.ctx, period)
	require.NoError(s.T(), err)
	require.Equal(s.T(), period, pool.Period)

	assert.InDelta(s.T(), 329.712345, pool.Values[ScopeKeySpinCommonPOYDenier], 0.0000001)
	assert.InDelta(s.T(), 3027153.123456, pool.Values[ScopeKeySpinPOYProduction], 0.0000001)
	assert.InDelta(s.T(), 198634.111111, pool.Values[ScopeKeySpinPowerMonth], 0.0000001)
	assert.InDelta(s.T(), 275561.222222, pool.Values[ScopeKeySpinManpowerMonth], 0.0000001)
	assert.InDelta(s.T(), 46600.333333, pool.Values[ScopeKeySpinOverheadsMonth], 0.0000001)
	assert.InDelta(s.T(), 54100.444444, pool.Values[ScopeKeySpinConsSprsMonth], 0.0000001)
}
