package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// ---------------------------------------------------------------------------
// USER DECISION 2026-08-26 — Revoke and Un-approve were REMOVED from the MB Recipe
// workflow (DRAFT → SUBMITTED → APPROVED, then Request Unlock only).
//
// 🔴 The RPCs stay in the proto contract — deleting one is a breaking change — so the
// refusal lives HERE, at the surface. This file pins three things a future edit must
// not quietly undo:
//
//  1. both RPCs refuse EVERY call, including a perfectly valid request;
//  2. they refuse through the BaseResponse envelope with a nil transport error,
//     ⛔ never a raw gRPC error, so REST clients through the gateway see the same
//     shape as every other MB Head response;
//  3. the message NAMES the replacement, so an operator learns the new rule.
//
// The fake repository from the frozen-check-status suite is reused: it counts calls,
// and every counter must stay at zero. Refusing before any I/O is the point — a stray
// caller must not be able to write an audit trail for an action that no longer exists.
// ---------------------------------------------------------------------------

func TestUnApproveMBHead_IsRemoved_RefusesWithBaseResponse(t *testing.T) {
	h, repo := newFrozenSealHandler(t)

	// A well-formed request with a valid UUID and a real reason: the refusal must not
	// depend on the request being bad in any way.
	resp, err := h.UnApproveMBHead(context.Background(), &financev1.UnApproveMBHeadRequest{
		MbhId:  "11111111-1111-1111-1111-111111111111",
		Reason: "wrong recipe",
	})

	require.NoError(t, err, "⛔ BaseResponse pattern: the error travels in the body, not the transport")
	require.NotNil(t, resp)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "410", resp.Base.StatusCode, "410 Gone: the capability is gone, the request is not malformed")
	assert.Contains(t, resp.Base.Message, "un-approve has been removed")
	assert.Contains(t, resp.Base.Message, "Request Unlock", "the message must name the replacement")
	assert.Nil(t, resp.Data, "a refused transition must not return an entity")

	assert.Zero(t, repo.updateCall, "the removed RPC must not reach persistence")
	assert.Zero(t, repo.createCall, "the removed RPC must not reach persistence")
}

func TestRevokeMBHead_IsRemoved_RefusesWithBaseResponse(t *testing.T) {
	h, repo := newFrozenSealHandler(t)

	resp, err := h.RevokeMBHead(context.Background(), &financev1.RevokeMBHeadRequest{
		MbhId:  "11111111-1111-1111-1111-111111111111",
		Reason: "no longer needed",
	})

	require.NoError(t, err, "⛔ BaseResponse pattern: the error travels in the body, not the transport")
	require.NotNil(t, resp)
	require.NotNil(t, resp.Base)
	assert.False(t, resp.Base.IsSuccess)
	assert.Equal(t, "410", resp.Base.StatusCode)
	assert.Contains(t, resp.Base.Message, "revoke has been removed")
	assert.Contains(t, resp.Base.Message, "active flag", "the message must name the replacement")
	assert.Nil(t, resp.Data, "a refused transition must not return an entity")

	assert.Zero(t, repo.updateCall, "the removed RPC must not reach persistence")
	assert.Zero(t, repo.createCall, "the removed RPC must not reach persistence")
}

// TestRemovedTransitions_RefuseEvenOnGarbageInput — the refusal must come FIRST, ahead
// of UUID parsing and proto validation. An unparseable id previously produced a 400
// "invalid mbh_id"; now it must produce the same 410 as everything else, because the
// endpoint no longer inspects the request at all. This keeps the two failure modes from
// being confused in logs.
func TestRemovedTransitions_RefuseEvenOnGarbageInput(t *testing.T) {
	h, _ := newFrozenSealHandler(t)

	unApprove, err := h.UnApproveMBHead(context.Background(), &financev1.UnApproveMBHeadRequest{MbhId: "not-a-uuid"})
	require.NoError(t, err)
	assert.Equal(t, "410", unApprove.Base.StatusCode)

	revoke, err := h.RevokeMBHead(context.Background(), &financev1.RevokeMBHeadRequest{MbhId: "not-a-uuid"})
	require.NoError(t, err)
	assert.Equal(t, "410", revoke.Base.StatusCode)
}

// TestRemovedTransitions_StillPermissionMapped guards a specific foot-gun. The
// interceptor treats an RPC MISSING from getRequiredPermission's map as "any
// authenticated user may call this". Deleting the two now-dead mappings as cleanup
// would therefore turn two gated RPCs into open ones and widen the hole the K-34 (c)
// coverage ratchet exists to freeze. The mappings must stay.
func TestRemovedTransitions_StillPermissionMapped(t *testing.T) {
	assert.Equal(t, "finance.mb.head.unapprove",
		getRequiredPermission("/finance.v1.MBHeadService/UnApproveMBHead"),
		"⛔ do not drop the mapping: an unmapped RPC becomes open to any authenticated user")
	assert.Equal(t, "finance.mb.head.revoke",
		getRequiredPermission("/finance.v1.MBHeadService/RevokeMBHead"),
		"⛔ do not drop the mapping: an unmapped RPC becomes open to any authenticated user")
}
