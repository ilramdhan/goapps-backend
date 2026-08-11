// Package e2e provides end-to-end tests for the finance service gRPC API.
//
// SAFETY: mst_spin_fixed_cost holds a load-bearing production anchor row (period
// 202604) that the calc engine reads for every POY product. These tests never
// create, update, or delete that period. Every fixture uses a throwaway far-past
// period in the 1800xx range, which is:
//   - impossible to collide with real data,
//   - EARLIER than every real row, which is what makes the anchor-guard
//     "earliest live+active row" branch reachable without touching real rows.
//
// The suite hard-deletes the 1800xx range before and after the run.
package e2e

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	_ "github.com/jackc/pgx/v5/stdlib"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// sfcCleanupSQL removes only the throwaway fixture range.
const sfcCleanupSQL = `DELETE FROM mst_spin_fixed_cost WHERE msfc_period LIKE '1800%'`

// SpinFixedCostE2ETestSuite is the end-to-end test suite for SpinFixedCostService.
type SpinFixedCostE2ETestSuite struct {
	suite.Suite
	conn   *grpc.ClientConn
	client financev1.SpinFixedCostServiceClient
	db     *sql.DB
	ctx    context.Context // authenticated context with JWT
}

func TestSpinFixedCostE2ESuite(t *testing.T) {
	if os.Getenv("E2E_TEST") != "true" {
		t.Skip("Skipping E2E test. Set E2E_TEST=true to run.")
	}
	suite.Run(t, new(SpinFixedCostE2ETestSuite))
}

func (s *SpinFixedCostE2ETestSuite) SetupSuite() {
	addr := getEnv("GRPC_ADDR", "localhost:50051")

	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(s.T(), err)
	s.conn = conn
	s.client = financev1.NewSpinFixedCostServiceClient(conn)

	token := s.generateTestToken()
	md := metadata.Pairs("authorization", "Bearer "+token)
	s.ctx = metadata.NewOutgoingContext(context.Background(), md)

	dsn := getEnv("DATABASE_URL", "postgres://finance:finance123@localhost:5434/finance_db?sslmode=disable")
	db, err := sql.Open("pgx", dsn)
	require.NoError(s.T(), err)
	s.db = db

	s.waitForServer()
	s.cleanupFixtures()
}

func (s *SpinFixedCostE2ETestSuite) generateTestToken() string {
	secret := getEnv("JWT_ACCESS_SECRET", "dev-access-secret-change-in-production")

	now := time.Now()
	claims := jwt.MapClaims{
		"token_type": "access",
		"user_id":    "e2e-test-user-id",
		"username":   "e2e_tester",
		"email":      "e2e@test.local",
		"roles":      []string{"SUPER_ADMIN"},
		"iss":        "goapps-iam",
		"sub":        "e2e-test-user-id",
		"iat":        now.Unix(),
		"exp":        now.Add(1 * time.Hour).Unix(),
		"jti":        "e2e-test-token-id",
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString([]byte(secret))
	require.NoError(s.T(), err)
	return signed
}

func (s *SpinFixedCostE2ETestSuite) TearDownSuite() {
	s.cleanupFixtures()
	if s.db != nil {
		require.NoError(s.T(), s.db.Close())
	}
	if s.conn != nil {
		require.NoError(s.T(), s.conn.Close())
	}
}

func (s *SpinFixedCostE2ETestSuite) SetupTest() {
	s.cleanupFixtures()
}

func (s *SpinFixedCostE2ETestSuite) TearDownTest() {
	s.cleanupFixtures()
}

func (s *SpinFixedCostE2ETestSuite) cleanupFixtures() {
	_, err := s.db.ExecContext(context.Background(), sfcCleanupSQL)
	require.NoError(s.T(), err)
}

func (s *SpinFixedCostE2ETestSuite) waitForServer() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			s.T().Fatal("Server not ready within timeout")
		default:
			_, err := s.client.ListSpinFixedCosts(ctx, &financev1.ListSpinFixedCostsRequest{Page: 1, PageSize: 1})
			if err == nil {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
	}
}

// create is a fixture helper that creates a record and asserts success.
func (s *SpinFixedCostE2ETestSuite) create(period string) *financev1.SpinFixedCost {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateSpinFixedCost(ctx, &financev1.CreateSpinFixedCostRequest{
		Period:             period,
		CommonPoyDenier:    329.712,
		PoyProduction:      3027153,
		SpinPowerMonth:     198634,
		SpinManpowerMonth:  275561,
		SpinOverheadsMonth: 46600,
		SpinConssprsMonth:  54100,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), resp.Base.IsSuccess, "create %s failed: %s", period, resp.Base.Message)
	return resp.Data
}

// =============================================================================
// Happy paths
// =============================================================================

// TestCRUDFlow walks create -> get -> list -> update -> delete through the full
// handler / application / repository / real-DB path.
func (s *SpinFixedCostE2ETestSuite) TestCRUDFlow() {
	ctx, cancel := context.WithTimeout(s.ctx, 30*time.Second)
	defer cancel()

	// 1. Create. A second, later row exists so the anchor guard permits the
	//    later delete of the earliest row's successor.
	created := s.create("180001")
	assert.NotEmpty(s.T(), created.Id)
	assert.Equal(s.T(), "180001", created.Period)
	assert.InDelta(s.T(), 329.712, created.CommonPoyDenier, 1e-9)
	assert.True(s.T(), created.IsActive)
	require.NotNil(s.T(), created.Audit)
	assert.Equal(s.T(), "e2e_tester", created.Audit.CreatedBy)

	// 2. Get
	getResp, err := s.client.GetSpinFixedCost(ctx, &financev1.GetSpinFixedCostRequest{Id: created.Id})
	require.NoError(s.T(), err)
	require.True(s.T(), getResp.Base.IsSuccess, getResp.Base.Message)
	assert.Equal(s.T(), "200", getResp.Base.StatusCode)
	assert.Equal(s.T(), "180001", getResp.Data.Period)

	// 3. List
	listResp, err := s.client.ListSpinFixedCosts(ctx, &financev1.ListSpinFixedCostsRequest{
		Page: 1, PageSize: 10, Search: "1800", SortBy: "period", SortOrder: "asc",
	})
	require.NoError(s.T(), err)
	require.True(s.T(), listResp.Base.IsSuccess, listResp.Base.Message)
	assert.Equal(s.T(), "200", listResp.Base.StatusCode)
	require.NotNil(s.T(), listResp.Pagination)
	assert.Equal(s.T(), int32(1), listResp.Pagination.CurrentPage)
	assert.Equal(s.T(), int32(10), listResp.Pagination.PageSize)
	assert.Equal(s.T(), int64(1), listResp.Pagination.TotalItems)
	assert.Equal(s.T(), int32(1), listResp.Pagination.TotalPages)
	require.Len(s.T(), listResp.Data, 1)
	assert.Equal(s.T(), "180001", listResp.Data[0].Period)

	// 4. Update (values only; is_active stays true so the anchor guard is not hit)
	newPower := 12345.678901
	updateResp, err := s.client.UpdateSpinFixedCost(ctx, &financev1.UpdateSpinFixedCostRequest{
		Id:             created.Id,
		SpinPowerMonth: &newPower,
	})
	require.NoError(s.T(), err)
	require.True(s.T(), updateResp.Base.IsSuccess, updateResp.Base.Message)
	assert.Equal(s.T(), "200", updateResp.Base.StatusCode)
	assert.InDelta(s.T(), newPower, updateResp.Data.SpinPowerMonth, 1e-9)
	assert.Equal(s.T(), "180001", updateResp.Data.Period, "period must be immutable")
	assert.Equal(s.T(), "e2e_tester", updateResp.Data.Audit.UpdatedBy)

	// 5. Delete. 180001 is the earliest live+active row overall (real data is 20xxxx),
	//    so it must first be joined by an earlier sibling for the guard to allow it.
	//    Simpler: add a row before it, making 180001 no longer earliest.
	s.create("180000")

	deleteResp, err := s.client.DeleteSpinFixedCost(ctx, &financev1.DeleteSpinFixedCostRequest{Id: created.Id})
	require.NoError(s.T(), err)
	require.True(s.T(), deleteResp.Base.IsSuccess, deleteResp.Base.Message)
	assert.Equal(s.T(), "200", deleteResp.Base.StatusCode)

	// 6. Verify gone
	goneResp, err := s.client.GetSpinFixedCost(ctx, &financev1.GetSpinFixedCostRequest{Id: created.Id})
	require.NoError(s.T(), err)
	assert.False(s.T(), goneResp.Base.IsSuccess)
	assert.Equal(s.T(), "404", goneResp.Base.StatusCode)
}

// TestGetNotFound asserts an unknown but well-formed UUID yields 404.
func (s *SpinFixedCostE2ETestSuite) TestGetNotFound() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.GetSpinFixedCost(ctx, &financev1.GetSpinFixedCostRequest{
		Id: "00000000-0000-0000-0000-000000000000",
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "404", resp.Base.StatusCode)
}

// =============================================================================
// Validation
// =============================================================================

// TestCreateValidation_BadPeriod asserts a malformed period is a 400, not a 500.
func (s *SpinFixedCostE2ETestSuite) TestCreateValidation_BadPeriod() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateSpinFixedCost(ctx, &financev1.CreateSpinFixedCostRequest{
		Period:          "20260",
		CommonPoyDenier: 1, PoyProduction: 1,
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)
	assert.NotEmpty(s.T(), resp.Base.ValidationErrors)
	s.T().Logf("bad period message: %q; errors: %v", resp.Base.Message, resp.Base.ValidationErrors)
}

// TestCreateValidation_NegativeNumerics asserts negative amounts are rejected as 400.
func (s *SpinFixedCostE2ETestSuite) TestCreateValidation_NegativeNumerics() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateSpinFixedCost(ctx, &financev1.CreateSpinFixedCostRequest{
		Period:             "180010",
		CommonPoyDenier:    -1,
		PoyProduction:      -1,
		SpinPowerMonth:     -1,
		SpinManpowerMonth:  -1,
		SpinOverheadsMonth: -1,
		SpinConssprsMonth:  -1,
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)

	fields := make(map[string]bool)
	for _, ve := range resp.Base.ValidationErrors {
		fields[ve.Field] = true
	}
	for _, f := range []string{
		"common_poy_denier", "poy_production", "spin_power_month",
		"spin_manpower_month", "spin_overheads_month", "spin_conssprs_month",
	} {
		assert.True(s.T(), fields[f], "expected a validation error for %s, got %v", f, fields)
	}
}

// TestCreateValidation_ZeroDivisorsRejected is the important asymmetry: zero on the
// two divisor fields must be refused (a zero divisor silently zeroes POY fixed cost
// instead of raising), while zero on the four monthly amounts is legitimate.
func (s *SpinFixedCostE2ETestSuite) TestCreateValidation_ZeroDivisorsRejected() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateSpinFixedCost(ctx, &financev1.CreateSpinFixedCostRequest{
		Period:          "180011",
		CommonPoyDenier: 0,
		PoyProduction:   0,
		// The four monthly amounts stay at zero deliberately: they must NOT be flagged.
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)

	fields := make(map[string]bool)
	for _, ve := range resp.Base.ValidationErrors {
		fields[ve.Field] = true
	}
	assert.True(s.T(), fields["common_poy_denier"], "zero common_poy_denier must be rejected (divisor)")
	assert.True(s.T(), fields["poy_production"], "zero poy_production must be rejected (divisor)")
	assert.False(s.T(), fields["spin_power_month"], "zero spin_power_month is legitimate")
	assert.False(s.T(), fields["spin_manpower_month"], "zero spin_manpower_month is legitimate")
	assert.False(s.T(), fields["spin_overheads_month"], "zero spin_overheads_month is legitimate")
	assert.False(s.T(), fields["spin_conssprs_month"], "zero spin_conssprs_month is legitimate")
}

// TestCreateZeroAmountsAllowed proves the four non-divisor numerics accept zero.
func (s *SpinFixedCostE2ETestSuite) TestCreateZeroAmountsAllowed() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	resp, err := s.client.CreateSpinFixedCost(ctx, &financev1.CreateSpinFixedCostRequest{
		Period:          "180012",
		CommonPoyDenier: 1,
		PoyProduction:   1,
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess, "zero monthly amounts must be accepted: %s", resp.Base.Message)
}

// TestUpdateValidation_ZeroDivisor asserts the update path rejects zero divisors too.
func (s *SpinFixedCostE2ETestSuite) TestUpdateValidation_ZeroDivisor() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	created := s.create("180013")

	zero := 0.0
	resp, err := s.client.UpdateSpinFixedCost(ctx, &financev1.UpdateSpinFixedCostRequest{
		Id:              created.Id,
		CommonPoyDenier: &zero,
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)
}

// TestDuplicatePeriod captures the exact conflict message. The frontend BFF pattern
// matches on this string to rewrite it into "edit the existing row instead", so the
// wording is a contract, not an incidental detail.
func (s *SpinFixedCostE2ETestSuite) TestDuplicatePeriod() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	s.create("180020")

	resp, err := s.client.CreateSpinFixedCost(ctx, &financev1.CreateSpinFixedCostRequest{
		Period:          "180020",
		CommonPoyDenier: 1,
		PoyProduction:   1,
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "409", resp.Base.StatusCode)
	assert.Contains(s.T(), resp.Base.Message, "already exists",
		"the BFF duplicate heuristic keys off the 'already exists' substring")
	s.T().Logf("DUPLICATE PERIOD MESSAGE: %q (statusCode=%s)", resp.Base.Message, resp.Base.StatusCode)
}

// =============================================================================
// Anchor guard, end to end
// =============================================================================

// TestAnchorGuard_DeleteEarliestRefused asserts the earliest live+active row cannot
// be deleted while later live rows exist. The 1800xx fixtures are earlier than every
// real row, so 180030 is genuinely the global earliest.
func (s *SpinFixedCostE2ETestSuite) TestAnchorGuard_DeleteEarliestRefused() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	earliest := s.create("180030")
	s.create("180031")

	resp, err := s.client.DeleteSpinFixedCost(ctx, &financev1.DeleteSpinFixedCostRequest{Id: earliest.Id})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)
	s.T().Logf("ANCHOR GUARD (delete earliest) MESSAGE: %q (statusCode=%s)", resp.Base.Message, resp.Base.StatusCode)

	// Still there.
	getResp, err := s.client.GetSpinFixedCost(ctx, &financev1.GetSpinFixedCostRequest{Id: earliest.Id})
	require.NoError(s.T(), err)
	assert.True(s.T(), getResp.Base.IsSuccess, "the refused delete must not have taken effect")
}

// TestAnchorGuard_DeleteMiddleAllowed asserts a non-earliest row is removable.
func (s *SpinFixedCostE2ETestSuite) TestAnchorGuard_DeleteMiddleAllowed() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	s.create("180040")
	middle := s.create("180041")
	s.create("180042")

	resp, err := s.client.DeleteSpinFixedCost(ctx, &financev1.DeleteSpinFixedCostRequest{Id: middle.Id})
	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess, "a middle row must be deletable: %s", resp.Base.Message)
	assert.Equal(s.T(), "200", resp.Base.StatusCode)
}

// TestAnchorGuard_DeactivateEarliestRefused asserts flipping is_active=false on the
// earliest live+active row is refused, since it removes the row from period
// resolution just as surely as a delete.
func (s *SpinFixedCostE2ETestSuite) TestAnchorGuard_DeactivateEarliestRefused() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	earliest := s.create("180050")
	s.create("180051")

	off := false
	resp, err := s.client.UpdateSpinFixedCost(ctx, &financev1.UpdateSpinFixedCostRequest{
		Id:       earliest.Id,
		IsActive: &off,
	})
	require.NoError(s.T(), err)
	assert.False(s.T(), resp.Base.IsSuccess)
	assert.Equal(s.T(), "400", resp.Base.StatusCode)
	s.T().Logf("ANCHOR GUARD (deactivate earliest) MESSAGE: %q (statusCode=%s)", resp.Base.Message, resp.Base.StatusCode)

	getResp, err := s.client.GetSpinFixedCost(ctx, &financev1.GetSpinFixedCostRequest{Id: earliest.Id})
	require.NoError(s.T(), err)
	require.True(s.T(), getResp.Base.IsSuccess)
	assert.True(s.T(), getResp.Data.IsActive, "the refused deactivation must not have taken effect")
}

// TestAnchorGuard_UpdateStayingActiveAllowed asserts an update that leaves is_active
// true on the earliest row passes the guard.
func (s *SpinFixedCostE2ETestSuite) TestAnchorGuard_UpdateStayingActiveAllowed() {
	ctx, cancel := context.WithTimeout(s.ctx, 10*time.Second)
	defer cancel()

	earliest := s.create("180060")
	s.create("180061")

	on := true
	newPower := 4242.424242
	resp, err := s.client.UpdateSpinFixedCost(ctx, &financev1.UpdateSpinFixedCostRequest{
		Id:             earliest.Id,
		IsActive:       &on,
		SpinPowerMonth: &newPower,
	})
	require.NoError(s.T(), err)
	assert.True(s.T(), resp.Base.IsSuccess, "an update keeping is_active=true must pass: %s", resp.Base.Message)
	assert.InDelta(s.T(), newPower, resp.Data.SpinPowerMonth, 1e-9)
	assert.True(s.T(), resp.Data.IsActive)
}

// TestAnchorGuard_DeleteOnlyActiveRowRefused covers the "only live+active row"
// branch. It cannot be reached against the shared live table without deactivating
// the real anchor, so the branch is exercised at the domain level with the same
// AnchorStats shape the repository produces (RemainingActiveCount == 0).
//
// The two reachable-through-gRPC branches are covered by the tests above.
func (s *SpinFixedCostE2ETestSuite) TestAnchorGuard_DeleteOnlyActiveRowRefused() {
	s.T().Skip("cannot be exercised end-to-end without deactivating the production anchor row; " +
		"covered by the domain unit test for CheckAnchorGuard with RemainingActiveCount == 0")
}
