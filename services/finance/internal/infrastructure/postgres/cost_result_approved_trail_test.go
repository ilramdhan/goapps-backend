// Package postgres_test — approved-trail (cpc_approved_at/by, migration 000552)
// integration tests for CostResultRepository (P0-T8, AC-10).
//
// Gated by INTEGRATION_TEST=true. Requires a LOCAL PostgreSQL with all finance
// migrations applied up to at least 000552 (same TEST_DB_* defaults as
// cost_calc_repos_test.go). Never point this at a shared/staging/prod DB.
package postgres_test

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// These are methods on CostCalcReposSuite (cost_calc_repos_test.go) so they
// reuse its seeded product/route and its INTEGRATION_TEST gate.

// seedCalculated inserts a fresh CALCULATED ACTUAL row for a unique period.
func (s *CostCalcReposSuite) seedCalculatedForTrail(period string) int64 {
	job := s.createJob(period)
	res := costcalc.NewResult(
		s.productSysID, period, costcalc.CalcTypeActual, s.routeHeadID, 1,
		10.0, 6.0, 4.0, 10.0, 0, "USD",
		nil, nil, nil, nil, "hash-"+period, job.ID(), "integ-test",
		0, 0, 0, 0, 0, 0, 0,
	)
	id, _, _, _, err := s.resultRepo.UpsertWithSupersede(s.ctx, res)
	require.NoError(s.T(), err)
	return id
}

func (s *CostCalcReposSuite) approvalTrail(id int64) (verifiedBy sql.NullString, approvedAt sql.NullTime, approvedBy sql.NullString) {
	require.NoError(s.T(), s.db.QueryRowContext(s.ctx, `
		SELECT cpc_verified_by, cpc_approved_at, cpc_approved_by
		  FROM cst_product_cost WHERE cpc_cost_id = $1`, id,
	).Scan(&verifiedBy, &approvedAt, &approvedBy))
	return verifiedBy, approvedAt, approvedBy
}

func trailTestPeriod(offset int) string {
	// 9999xx periods are filtered out of the UI dropdown (ListDistinctPeriods).
	return fmt.Sprintf("9999%02d", (time.Now().Nanosecond()/1000+offset)%100)
}

func (s *CostCalcReposSuite) TestResult_VerifyOnly_LeavesApprovedNull() {
	id := s.seedCalculatedForTrail(trailTestPeriod(1))
	require.NoError(s.T(), s.resultRepo.MarkVerified(s.ctx, id, "verifier-1"))

	vBy, aAt, aBy := s.approvalTrail(id)
	require.Equal(s.T(), "verifier-1", vBy.String)
	require.False(s.T(), aAt.Valid, "verify must not stamp cpc_approved_at")
	require.False(s.T(), aBy.Valid, "verify must not stamp cpc_approved_by")

	got, err := s.resultRepo.GetByID(s.ctx, id)
	require.NoError(s.T(), err)
	require.Nil(s.T(), got.ApprovedAt())
	require.Empty(s.T(), got.ApprovedBy())
}

func (s *CostCalcReposSuite) TestResult_VerifyThenApprove_SetsBothTrails() {
	id := s.seedCalculatedForTrail(trailTestPeriod(2))
	require.NoError(s.T(), s.resultRepo.MarkVerified(s.ctx, id, "verifier-1"))
	require.NoError(s.T(), s.resultRepo.MarkApproved(s.ctx, id, "approver-1"))

	vBy, aAt, aBy := s.approvalTrail(id)
	require.Equal(s.T(), "approver-1", vBy.String, "legacy verified_* keeps last-actor semantics")
	require.True(s.T(), aAt.Valid)
	require.Equal(s.T(), "approver-1", aBy.String)

	got, err := s.resultRepo.GetByID(s.ctx, id)
	require.NoError(s.T(), err)
	require.Equal(s.T(), costcalc.ResultStatusApproved, got.Status())
	require.NotNil(s.T(), got.ApprovedAt())
	require.Equal(s.T(), "approver-1", got.ApprovedBy())
}

func (s *CostCalcReposSuite) TestResult_ApproveFromCalculatedTx_SetsBothTrails() {
	id := s.seedCalculatedForTrail(trailTestPeriod(3))
	tx, err := s.db.BeginTx(s.ctx, nil)
	require.NoError(s.T(), err)
	require.NoError(s.T(), s.resultRepo.MarkApprovedFromCalculatedTx(s.ctx, tx, id, "mb-pusher"))
	require.NoError(s.T(), tx.Commit())

	vBy, aAt, aBy := s.approvalTrail(id)
	require.Equal(s.T(), "mb-pusher", vBy.String)
	require.True(s.T(), aAt.Valid)
	require.Equal(s.T(), "mb-pusher", aBy.String)
}

func (s *CostCalcReposSuite) TestResult_Approve_WrongStatus_NoTrail() {
	id := s.seedCalculatedForTrail(trailTestPeriod(4))
	err := s.resultRepo.MarkApproved(s.ctx, id, "approver-1") // CALCULATED, not VERIFIED
	require.ErrorIs(s.T(), err, costcalc.ErrCostInvalidStatus)
	_, aAt, aBy := s.approvalTrail(id)
	require.False(s.T(), aAt.Valid)
	require.False(s.T(), aBy.Valid)
}
