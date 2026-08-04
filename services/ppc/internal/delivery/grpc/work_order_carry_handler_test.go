package grpc

import (
	"strings"
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// The carry request's lot_no bound must match work_order.wo_lot_no VARCHAR(30),
// like every other lot_no field in work_order.proto. It was 50, so a 31..50-char
// lot cleared validation and then failed at INSERT as a raw driver error — which
// surfaces as a 500 rather than a complaint about the field the planner typed.
func TestProcessWorkOrderCarryForwardRequest_LotBoundMatchesColumn(t *testing.T) {
	validator, err := protovalidate.New()
	require.NoError(t, err)

	req := func(lot string) *ppcv1.ProcessWorkOrderCarryForwardRequest {
		return &ppcv1.ProcessWorkOrderCarryForwardRequest{
			SourceWoId: 1, TargetMonth: "2026-09", LotNo: lot,
		}
	}

	// Blank still reaches the handler: it selects the lot generator.
	require.NoError(t, validator.Validate(req("")))
	// A generated lot is 10 chars; the boundary itself is accepted.
	require.NoError(t, validator.Validate(req("TXT0009-26")))
	require.NoError(t, validator.Validate(req(strings.Repeat("X", 30))))
	// One over the column width is a client error, not an INSERT failure.
	assert.Error(t, validator.Validate(req(strings.Repeat("X", 31))))
}

// Every carry refusal is the planner's to resolve: pick another month, or a
// smaller quantity. None of them is a server fault.
//
// domainErrorToBaseResponse classifies by substring, and none of these messages
// contains "invalid", "not found" or "must be" — so before carryErrorToBaseResponse
// existed they all fell through to its 500 default. A 500 tells the planner the
// system is broken and to retry, when the answer will not change until they
// change their input; it also drags a routine refusal into the error-rate
// alerting. This pins the code so a reworded message cannot quietly restore the
// 500.
func TestCarryErrors_MapToClientRefusals(t *testing.T) {
	tests := []struct {
		name string
		err  error
	}{
		{name: "nothing left to carry", err: workorderdomain.ErrNothingToCarry},
		{name: "already carried into this month", err: workorderdomain.ErrAlreadyCarriedIntoMonth},
		{name: "requested more than remains", err: workorderdomain.ErrCarryQtyExceedsRemaining},
		{name: "status makes it ineligible", err: workorderdomain.ErrWONotEligibleForCarry},
		{name: "target month is not later", err: workorderdomain.ErrCarryTargetNotLater},
		{name: "target month is malformed", err: workorderdomain.ErrInvalidTargetMonth},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base := carryErrorToBaseResponse(tt.err)
			require.False(t, base.GetIsSuccess())
			assert.Equal(t, "400", base.GetStatusCode(),
				"a planner-resolvable refusal must not be reported as a server error")
			assert.NotEmpty(t, base.GetMessage())
			// Standing project rule: no message may expose a raw id.
			assert.NotRegexp(t, `\bid\s+\d+`, base.GetMessage())
		})
	}
}

// A genuine fault must keep reaching the shared mapper, so the refusal list does
// not become a catch-all that hides real breakage behind a 400.
func TestCarryErrorToBaseResponse_DefersUnknownErrors(t *testing.T) {
	base := carryErrorToBaseResponse(workorderdomain.ErrNotFound)
	assert.Equal(t, "404", base.GetStatusCode())

	base = carryErrorToBaseResponse(assertAnError{})
	assert.Equal(t, "500", base.GetStatusCode(),
		"an unrecognized error is still a server error")
}

// assertAnError is an error that matches none of the mapper's substrings.
type assertAnError struct{}

func (assertAnError) Error() string { return "pool exhausted" }
