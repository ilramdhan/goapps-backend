package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"
)

// =============================================================================
// Unit tests for the MB guard on the manual verify / approve RPCs.
//
// MB cost rows go CALCULATED -> APPROVED exclusively inside MB Push-to-Head's
// transaction (CostResultRepository.MarkApprovedFromCalculatedTx), which requires the
// source row to be exactly CALCULATED. A user hand-verifying an MB row through the
// generic Cost Results screen flips it to VERIFIED and makes the next push silently
// SKIP that MB. These tests lock the rejection, and equally lock that a non-MB row is
// untouched.
//
// Service is left empty on the reject paths on purpose: reaching resultRepo would
// nil-panic, which is the assertion that no status was written.
// =============================================================================

// stubCostRowChecker answers IsMBCostRow from fixed values and records the ask.
type stubCostRowChecker struct {
	isMB       bool
	err        error
	askedCost  int64
	callsCount int
}

func (s *stubCostRowChecker) IsMBCostRow(_ context.Context, costID int64) (bool, error) {
	s.callsCount++
	s.askedCost = costID
	return s.isMB, s.err
}

func TestVerifyCost_MBRow_Rejected(t *testing.T) {
	t.Parallel()
	guard := &stubCostRowChecker{isMB: true}
	h := NewVerifyCostHandler(&Service{}, WithVerifyMBGuard(guard))

	err := h.Handle(context.Background(), VerifyCostCommand{CostID: 9001, Actor: "test"})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMBCostNotManuallyTransitionable))
	require.Equal(t, int64(9001), guard.askedCost)
}

func TestApproveCost_MBRow_Rejected(t *testing.T) {
	t.Parallel()
	guard := &stubCostRowChecker{isMB: true}
	h := NewApproveCostHandler(&Service{}, WithApproveMBGuard(guard))

	err := h.Handle(context.Background(), ApproveCostCommand{CostID: 9002, Actor: "test"})

	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMBCostNotManuallyTransitionable))
	require.Equal(t, int64(9002), guard.askedCost)
}

// A non-MB row must pass the guard untouched. The handler then proceeds and panics on
// the nil resultRepo — proving the guard did NOT short-circuit a legitimate yarn/POY/ACY
// verification, which is the constraint that matters most here.
func TestVerifyCost_NonMBRow_PassesGuard(t *testing.T) {
	t.Parallel()
	guard := &stubCostRowChecker{isMB: false}
	h := NewVerifyCostHandler(&Service{}, WithVerifyMBGuard(guard))

	require.Panics(t, func() {
		_ = h.Handle(context.Background(), VerifyCostCommand{CostID: 55, Actor: "test"})
	})
	require.Equal(t, 1, guard.callsCount)
}

func TestApproveCost_NonMBRow_PassesGuard(t *testing.T) {
	t.Parallel()
	guard := &stubCostRowChecker{isMB: false}
	h := NewApproveCostHandler(&Service{}, WithApproveMBGuard(guard))

	require.Panics(t, func() {
		_ = h.Handle(context.Background(), ApproveCostCommand{CostID: 56, Actor: "test"})
	})
	require.Equal(t, 1, guard.callsCount)
}

// Without the option the guard is disabled entirely and existing call sites keep
// working unchanged.
func TestVerifyApprove_NilGuard_NotConsulted(t *testing.T) {
	t.Parallel()
	vh := NewVerifyCostHandler(&Service{})
	ah := NewApproveCostHandler(&Service{})

	require.Panics(t, func() {
		_ = vh.Handle(context.Background(), VerifyCostCommand{CostID: 1, Actor: "a"})
	})
	require.Panics(t, func() {
		_ = ah.Handle(context.Background(), ApproveCostCommand{CostID: 1, Actor: "a"})
	})
}

// Input validation still runs before the guard: a bad command must not spend a query.
func TestVerifyCost_InvalidInput_GuardNotConsulted(t *testing.T) {
	t.Parallel()
	guard := &stubCostRowChecker{isMB: true}
	h := NewVerifyCostHandler(&Service{}, WithVerifyMBGuard(guard))

	require.Error(t, h.Handle(context.Background(), VerifyCostCommand{CostID: 0, Actor: "test"}))
	require.Error(t, h.Handle(context.Background(), VerifyCostCommand{CostID: 5, Actor: ""}))
	require.Zero(t, guard.callsCount)
}

// A checker failure must not degrade into "not MB": an MB row would otherwise slip
// through on a transient database error.
func TestVerifyCost_GuardError_Propagates(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db down")
	h := NewVerifyCostHandler(&Service{}, WithVerifyMBGuard(&stubCostRowChecker{err: sentinel}))

	err := h.Handle(context.Background(), VerifyCostCommand{CostID: 7, Actor: "test"})

	require.Error(t, err)
	require.True(t, errors.Is(err, sentinel))
	require.False(t, errors.Is(err, ErrMBCostNotManuallyTransitionable))
}

func TestApproveCost_GuardError_Propagates(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db down")
	h := NewApproveCostHandler(&Service{}, WithApproveMBGuard(&stubCostRowChecker{err: sentinel}))

	err := h.Handle(context.Background(), ApproveCostCommand{CostID: 7, Actor: "test"})

	require.Error(t, err)
	require.True(t, errors.Is(err, sentinel))
}
