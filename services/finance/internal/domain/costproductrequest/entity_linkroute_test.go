package costproductrequest_test

import (
	"errors"
	"testing"
	"time"

	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
)

func reqAt(status string, linked int64) *cpr.Request {
	return cpr.Reconstruct(cpr.ReconstructInput{
		RequestID:             1,
		RequestNo:             "CR-2026-0001",
		RequestTypeID:         1,
		Title:                 "t",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		Status:                status,
		RequesterUserID:       "user-1",
		LinkedRouteHeadID:     linked,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	})
}

func TestRequest_LinkRoute_AllowedStates(t *testing.T) {
	t.Parallel()
	for _, st := range []string{
		cpr.StatusDraft,
		cpr.StatusSubmitted,
		cpr.StatusUnderReview,
		cpr.StatusRoutingDefined,
		cpr.StatusParameterPending,
		cpr.StatusParameterComplete,
	} {
		r := reqAt(st, 0)
		if err := r.LinkRoute(99); err != nil {
			t.Fatalf("status=%s: want ok, got %v", st, err)
		}
		if r.LinkedRouteHeadID() != 99 {
			t.Fatalf("status=%s: want headID 99, got %d", st, r.LinkedRouteHeadID())
		}
	}
}

func TestRequest_LinkRoute_RejectsZeroOrNegative(t *testing.T) {
	t.Parallel()
	r := reqAt(cpr.StatusDraft, 0)
	if err := r.LinkRoute(0); err == nil {
		t.Fatal("want error for zero head id, got nil")
	}
	if err := r.LinkRoute(-1); err == nil {
		t.Fatal("want error for negative head id, got nil")
	}
}

func TestRequest_LinkRoute_RejectsTerminal(t *testing.T) {
	t.Parallel()
	for _, st := range []string{
		cpr.StatusCostingDone,
		cpr.StatusRejected,
		cpr.StatusClosed,
	} {
		r := reqAt(st, 0)
		if err := r.LinkRoute(99); !errors.Is(err, cpr.ErrInvalidTransition) {
			t.Fatalf("status=%s: want ErrInvalidTransition, got %v", st, err)
		}
	}
}

func TestRequest_UnlinkRoute_ClearsField(t *testing.T) {
	t.Parallel()
	r := reqAt(cpr.StatusUnderReview, 42)
	if err := r.UnlinkRoute(); err != nil {
		t.Fatalf("ok expected, got %v", err)
	}
	if r.LinkedRouteHeadID() != 0 {
		t.Fatalf("expected 0 after unlink, got %d", r.LinkedRouteHeadID())
	}
}

func TestRequest_UnlinkRoute_RejectsTerminal(t *testing.T) {
	t.Parallel()
	for _, st := range []string{
		cpr.StatusCostingDone,
		cpr.StatusRejected,
		cpr.StatusClosed,
	} {
		r := reqAt(st, 42)
		if err := r.UnlinkRoute(); !errors.Is(err, cpr.ErrInvalidTransition) {
			t.Fatalf("status=%s: want ErrInvalidTransition, got %v", st, err)
		}
	}
}
