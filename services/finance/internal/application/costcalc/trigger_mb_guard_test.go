package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// stubMBChecker records what it was asked and answers from fixed values.
type stubMBChecker struct {
	productIsMB  bool
	typeIsMB     bool
	err          error
	askedProduct int64
	askedType    int32
	productCalls int
	typeCalls    int
}

func (s *stubMBChecker) IsMBProduct(_ context.Context, productSysID int64) (bool, error) {
	s.productCalls++
	s.askedProduct = productSysID
	return s.productIsMB, s.err
}

func (s *stubMBChecker) IsMBProductType(_ context.Context, productTypeID int32) (bool, error) {
	s.typeCalls++
	s.askedType = productTypeID
	return s.typeIsMB, s.err
}

// A deliberate FILTERED-by-MB-type trigger must fail before any cal_job row is written,
// because MB is owned by the MB_BATCH path. Service is empty: reaching any repo would
// nil-panic, which is exactly the assertion that nothing was persisted.
func TestTriggerJob_FilteredByMBType_Rejected(t *testing.T) {
	t.Parallel()
	guard := &stubMBChecker{typeIsMB: true}
	h := NewTriggerJobHandler(&Service{}, WithMBGuard(guard))

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:              "202607",
		CalcType:            costcalcdom.CalcTypeActual,
		Scope:               costcalcdom.ScopeFiltered,
		ProductTypeIDFilter: 81,
		Actor:               "test",
		TriggeredBy:         "TEST",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMBNotCalcJobEligible))
	require.Equal(t, int32(81), guard.askedType)
}

// A non-MB product type is untouched by the guard: the request proceeds and fails only
// later, for the unrelated reason that no orchestrator publisher is configured.
func TestTriggerJob_FilteredByNonMBType_PassesGuard(t *testing.T) {
	t.Parallel()
	guard := &stubMBChecker{typeIsMB: false}
	h := NewTriggerJobHandler(&Service{}, WithMBGuard(guard))

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:              "202607",
		CalcType:            costcalcdom.CalcTypeActual,
		Scope:               costcalcdom.ScopeFiltered,
		ProductTypeIDFilter: 12,
		Actor:               "test",
		TriggeredBy:         "TEST",
	})
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrMBNotCalcJobEligible))
	require.True(t, errors.Is(err, ErrScopeNotYetSupported))
}

func TestTriggerJob_SingleProductMB_Rejected(t *testing.T) {
	t.Parallel()
	guard := &stubMBChecker{productIsMB: true}
	h := NewTriggerJobHandler(&Service{}, WithMBGuard(guard))

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:       "202607",
		CalcType:     costcalcdom.CalcTypeActual,
		Scope:        costcalcdom.ScopeSingleProduct,
		ProductSysID: 40418,
		Actor:        "test",
		TriggeredBy:  "TEST",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, ErrMBNotCalcJobEligible))
	require.Equal(t, int64(40418), guard.askedProduct)
}

// ALL is the "everything the calc engine owns" request. MB simply is not part of that
// population, so the trigger is not rejected — the orchestrator filters MB out of the
// seed set silently. Asserting the guard is never consulted keeps that contract honest.
func TestTriggerJob_ScopeAll_NotRejectedAndGuardNotConsulted(t *testing.T) {
	t.Parallel()
	guard := &stubMBChecker{productIsMB: true, typeIsMB: true}
	h := NewTriggerJobHandler(&Service{}, WithMBGuard(guard))

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:      "202607",
		CalcType:    costcalcdom.CalcTypeActual,
		Scope:       costcalcdom.ScopeAll,
		Actor:       "test",
		TriggeredBy: "TEST",
	})
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrMBNotCalcJobEligible))
	require.Zero(t, guard.productCalls)
	require.Zero(t, guard.typeCalls)
}

// FILTERED with no type chosen has nothing to check; the guard must not be consulted and
// must not invent a rejection.
func TestTriggerJob_FilteredWithoutType_SkipsGuard(t *testing.T) {
	t.Parallel()
	guard := &stubMBChecker{typeIsMB: true}
	h := NewTriggerJobHandler(&Service{}, WithMBGuard(guard))

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:      "202607",
		CalcType:    costcalcdom.CalcTypeActual,
		Scope:       costcalcdom.ScopeFiltered,
		Actor:       "test",
		TriggeredBy: "TEST",
	})
	require.False(t, errors.Is(err, ErrMBNotCalcJobEligible))
	require.Zero(t, guard.typeCalls)
}

// Without WithMBGuard the finance-side check is disabled entirely (the orchestrator still
// enforces the rule). Existing constructor call sites must keep working unchanged.
func TestTriggerJob_NilGuard_NoRejection(t *testing.T) {
	t.Parallel()
	h := NewTriggerJobHandler(&Service{})

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:              "202607",
		CalcType:            costcalcdom.CalcTypeActual,
		Scope:               costcalcdom.ScopeFiltered,
		ProductTypeIDFilter: 81,
		Actor:               "test",
		TriggeredBy:         "TEST",
	})
	require.Error(t, err)
	require.False(t, errors.Is(err, ErrMBNotCalcJobEligible))
}

// A checker failure must not be swallowed into "not MB": a job that should have been
// rejected would otherwise be created.
func TestTriggerJob_GuardError_Propagates(t *testing.T) {
	t.Parallel()
	sentinel := errors.New("db down")
	guard := &stubMBChecker{err: sentinel}
	h := NewTriggerJobHandler(&Service{}, WithMBGuard(guard))

	_, err := h.Handle(context.Background(), TriggerCommand{
		Period:       "202607",
		CalcType:     costcalcdom.CalcTypeActual,
		Scope:        costcalcdom.ScopeSingleProduct,
		ProductSysID: 40418,
		Actor:        "test",
		TriggeredBy:  "TEST",
	})
	require.Error(t, err)
	require.True(t, errors.Is(err, sentinel))
}
