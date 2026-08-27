// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"errors"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// mbHeadLockErrToBase maps the P10 lock/unlock domain errors onto client-meaningful
// BaseResponse status codes, following the requestErrToBase / mbCompositionErrToBase
// pattern already used across this package.
//
// 🔴 These three errors must NEVER surface as a generic 500. They are all
// caller-correctable conditions, and a client that cannot tell them apart cannot tell
// the user what to do next:
//   - ErrHeadLocked          → 409: the recipe is locked; request an unlock first.
//   - ErrUnlockNotRequested  → 409: nothing is pending to grant or refuse.
//   - ErrUnlockOriginUnknown → 422: the request is well-formed but the state to return
//     to cannot be established, so the domain refuses rather than guessing (K-52). It is
//     a data condition, not a bad request — 422 matches app.ErrRouteNotLocked's use in
//     requestErrToBase.
//
// Everything not listed here falls through to domainErrorToBaseResponse so that
// ErrNotFound / ErrInvalidTransition / ErrReasonRequired keep the mapping they already
// have across the rest of MBHeadService. ⛔ No new pattern is introduced.
func mbHeadLockErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, mbhead.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, mbhead.ErrHeadLocked),
		errors.Is(err, mbhead.ErrUnlockNotRequested):
		return ConflictResponse(err.Error())
	case errors.Is(err, mbhead.ErrUnlockOriginUnknown):
		return ErrorResponse("422", err.Error())
	case errors.Is(err, mbhead.ErrReasonRequired),
		errors.Is(err, mbhead.ErrInvalidTransition):
		return ErrorResponse("400", err.Error())
	default:
		return domainErrorToBaseResponse(err)
	}
}

// authUserUUIDFromContext returns the authenticated caller's IAM user UUID, or "" when
// the caller has none (service tokens, unauthenticated paths).
//
// ⛔ Deliberately NOT getUserFromContext: that helper prefers the human-readable username
// and falls back to the "system" sentinel, and both are values IAM's BY_USER_ID resolver
// cannot parse. Nothing here substitutes a fallback — "" means "no addressable user", and
// the application layer turns that into "send no rule at all".
func authUserUUIDFromContext(ctx context.Context) string {
	userID, _ := GetUserIDFromCtx(ctx)
	if _, err := uuid.Parse(userID); err != nil {
		return ""
	}
	return userID
}

// RequestUnlockMBHead parks a locked MB Head in UNLOCK_REQUESTED so an approver can
// decide (P10). The reason is MANDATORY — the domain rejects an empty or whitespace-only
// one with ErrReasonRequired, and the proto carries min_len = 1 to match.
//
// 🔴 K-53: there is deliberately ⛔ NO check that the head is already locked. Legacy rows
// hold mbh_is_locked = NULL (which reads as "not locked") while sitting in VALIDATED;
// refusing those would make the feature useless on the very data it exists for.
func (h *MBHeadHandler) RequestUnlockMBHead(ctx context.Context, req *financev1.RequestUnlockMBHeadRequest) (*financev1.RequestUnlockMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("request_unlock", false)
		return &financev1.RequestUnlockMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("request_unlock", false)
		return &financev1.RequestUnlockMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.requestUnlockHandler.Handle(ctx, appmbhead.RequestUnlockCommand{
		MbhID: id, Reason: req.Reason,
		ActorUserID: getUserFromContext(ctx),
		// 🔴 A SECOND, SEPARATE identity — ⛔ never getUserFromContext, which returns the
		// USERNAME first and would land a non-UUID where IAM does uuid.Parse. This one is
		// read straight from AuthUserIDKey, which the auth interceptor fills with the JWT
		// claims' user UUID. Empty for a system/unauthenticated caller, which simply means
		// the later grant/reject notifies nobody.
		ActorUserUUID: authUserUUIDFromContext(ctx),
	})
	if err != nil {
		RecordMBHeadOperation("request_unlock", false)
		return &financev1.RequestUnlockMBHeadResponse{Base: mbHeadLockErrToBase(err)}, nil
	}

	RecordMBHeadOperation("request_unlock", true)
	return &financev1.RequestUnlockMBHeadResponse{
		Base: successResponse("MB head unlock requested successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// GrantUnlockMBHead approves a pending unlock request: the MB Head is unlocked and
// returns to DRAFT for editing (P10).
//
// ⛔ The request carries no reason on purpose — granting is an assent, not a refusal, and
// the ORIGINAL request reason stays on record (it is never cleared).
func (h *MBHeadHandler) GrantUnlockMBHead(ctx context.Context, req *financev1.GrantUnlockMBHeadRequest) (*financev1.GrantUnlockMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("grant_unlock", false)
		return &financev1.GrantUnlockMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("grant_unlock", false)
		return &financev1.GrantUnlockMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.grantUnlockHandler.Handle(ctx, appmbhead.GrantUnlockCommand{
		MbhID: id, ActorUserID: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBHeadOperation("grant_unlock", false)
		return &financev1.GrantUnlockMBHeadResponse{Base: mbHeadLockErrToBase(err)}, nil
	}

	RecordMBHeadOperation("grant_unlock", true)
	return &financev1.GrantUnlockMBHeadResponse{
		Base: successResponse("MB head unlock granted successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// RejectUnlockMBHead refuses a pending unlock request: the MB Head stays locked and
// returns to the state it was parked from (P10, K-52). The reason is MANDATORY.
//
// 🔴 When the parked-from state cannot be established the domain returns
// ErrUnlockOriginUnknown and this surfaces as 422 — ⛔ it never guesses between APPROVED
// and VALIDATED, which would silently rewrite the recipe's costing standing.
func (h *MBHeadHandler) RejectUnlockMBHead(ctx context.Context, req *financev1.RejectUnlockMBHeadRequest) (*financev1.RejectUnlockMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("reject_unlock", false)
		return &financev1.RejectUnlockMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("reject_unlock", false)
		return &financev1.RejectUnlockMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.rejectUnlockHandler.Handle(ctx, appmbhead.RejectUnlockCommand{
		MbhID: id, Reason: req.Reason, ActorUserID: getUserFromContext(ctx),
	})
	if err != nil {
		RecordMBHeadOperation("reject_unlock", false)
		return &financev1.RejectUnlockMBHeadResponse{Base: mbHeadLockErrToBase(err)}, nil
	}

	RecordMBHeadOperation("reject_unlock", true)
	return &financev1.RejectUnlockMBHeadResponse{
		Base: successResponse("MB head unlock rejected successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}
