// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// MBHeadHandler implements financev1.MBHeadServiceServer.
type MBHeadHandler struct {
	financev1.UnimplementedMBHeadServiceServer
	createHandler *appmbhead.CreateHandler
	getHandler    *appmbhead.GetHandler
	listHandler   *appmbhead.ListHandler
	updateHandler *appmbhead.UpdateHandler
	deleteHandler *appmbhead.DeleteHandler
	submitHandler *appmbhead.SubmitHandler
	// 🔴 2026-08-26 ("Opsi A") — the approveHandler field was DROPPED. ApproveMBHead now
	// drives validateHandler so that pressing Approve lands the recipe in VALIDATED (see
	// that method for why). ⛔ appmbhead.ApproveHandler itself was NOT deleted: it is
	// still used by cmd/backfill-mb-validate, which walks legacy rows through the
	// intermediate APPROVED state on purpose.
	validateHandler *appmbhead.ValidateHandler
	// 🔴 2026-08-26 — the unApproveHandler / revokeHandler fields were DROPPED. Both
	// features were removed by user decision, so their RPCs refuse before reaching the
	// application layer and holding a handler nobody calls would only mislead readers.
	// ⛔ The application-layer handlers themselves still exist (and also refuse) — only
	// the wiring here is gone.
	rejectHandler        *appmbhead.RejectHandler
	returnToDraftHandler *appmbhead.ReturnToDraftHandler
	// unrevokeHandler drives REVOKED -> DRAFT (2026-08-31), gated by the dedicated
	// finance.mb.head.unrevoke permission granted only to Super Admin.
	unrevokeHandler *appmbhead.UnrevokeHandler
	// P10 lock/unlock workflow handlers. All three go through the SAME
	// repo.Transition path as the other workflow handlers, so the lock columns and
	// the mst_mb_head_lock_log row are written in one transaction.
	requestUnlockHandler *appmbhead.RequestUnlockHandler
	grantUnlockHandler   *appmbhead.GrantUnlockHandler
	rejectUnlockHandler  *appmbhead.RejectUnlockHandler
	refreezeHandler      *appmbhead.RefreezeHandler
	exportHandler        *appmbhead.ExportHandler
	exportFullH          *appmbhead.ExportFullHandler
	importHandler        *appmbhead.ImportHandler
	templateHandler      *appmbhead.TemplateHandler
	validation           *ValidationHelper
}

// NewMBHeadHandler constructs an MBHeadHandler without the [G.5] composition-sum
// gate on submit/validate. Retained for callers that have no composition repository
// to hand; the server wires NewMBHeadHandlerWithComposition instead.
func NewMBHeadHandler(repo mbhead.Repository, paramRepo mbparam.Repository, machineRepo machine.Repository) (*MBHeadHandler, error) {
	return NewMBHeadHandlerWithComposition(repo, paramRepo, machineRepo, nil)
}

// NewMBHeadHandlerWithComposition constructs an MBHeadHandler whose submit and
// validate paths enforce the composition-sum rule (plan §11 item 78). A nil
// compositionRepo disables that gate; the rule also stays inert while
// MB_COMPOSITION_SUM_ENFORCED is off.
//
// machineRepo is needed for B5: every created head is pinned to the "MB" machine.
func NewMBHeadHandlerWithComposition(
	repo mbhead.Repository, paramRepo mbparam.Repository, machineRepo machine.Repository,
	compositionRepo mbcomposition.Repository,
) (*MBHeadHandler, error) {
	return NewMBHeadHandlerWithRecipeFull(repo, paramRepo, machineRepo, compositionRepo, nil)
}

// NewMBHeadHandlerWithRecipeFull is NewMBHeadHandlerWithComposition plus the read-only
// reader that backs the P12 full-recipe export. A nil recipeFullReader leaves
// ExportMBRecipeFull returning a clean "not configured" error rather than panicking —
// callers that never wire the reader (tests, trimmed-down servers) still work.
func NewMBHeadHandlerWithRecipeFull(
	repo mbhead.Repository, paramRepo mbparam.Repository, machineRepo machine.Repository,
	compositionRepo mbcomposition.Repository, recipeFullReader appmbhead.RecipeFullReader,
) (*MBHeadHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &MBHeadHandler{
		createHandler:        appmbhead.NewCreateHandler(repo, machineRepo),
		getHandler:           appmbhead.NewGetHandler(repo),
		listHandler:          appmbhead.NewListHandler(repo),
		updateHandler:        appmbhead.NewUpdateHandler(repo),
		deleteHandler:        appmbhead.NewDeleteHandler(repo),
		submitHandler:        appmbhead.NewSubmitHandlerWithComposition(repo, compositionRepo),
		validateHandler:      appmbhead.NewValidateHandlerWithComposition(repo, paramRepo, compositionRepo),
		rejectHandler:        appmbhead.NewRejectHandler(repo),
		returnToDraftHandler: appmbhead.NewReturnToDraftHandler(repo),
		unrevokeHandler:      appmbhead.NewUnrevokeHandler(repo),
		requestUnlockHandler: appmbhead.NewRequestUnlockHandler(repo),
		grantUnlockHandler:   appmbhead.NewGrantUnlockHandler(repo),
		rejectUnlockHandler:  appmbhead.NewRejectUnlockHandler(repo),
		refreezeHandler:      appmbhead.NewRefreezeHandler(repo, paramRepo),
		exportHandler:        appmbhead.NewExportHandler(repo),
		exportFullH:          newExportFullHandler(recipeFullReader),
		importHandler:        appmbhead.NewImportHandler(repo),
		templateHandler:      appmbhead.NewTemplateHandler(),
		validation:           v,
	}, nil
}

// WithNotifier attaches the MB recipe notifier to every workflow handler that emits
// lifecycle notifications, and returns the handler for chaining.
//
// 🔴 Notifications are best-effort: a nil notifier (or a failing one) leaves every
// transition working exactly as before.
func (h *MBHeadHandler) WithNotifier(n appmbhead.Notifier) *MBHeadHandler {
	h.submitHandler = h.submitHandler.WithNotifier(n)
	h.returnToDraftHandler = h.returnToDraftHandler.WithNotifier(n)
	h.unrevokeHandler = h.unrevokeHandler.WithNotifier(n)
	h.requestUnlockHandler = h.requestUnlockHandler.WithNotifier(n)
	// E4/E5 — the two unlock DECISIONS notify the original requester.
	h.grantUnlockHandler = h.grantUnlockHandler.WithNotifier(n)
	h.rejectUnlockHandler = h.rejectUnlockHandler.WithNotifier(n)
	return h
}

// WithUnlockActors attaches the store that records and recovers the unlock REQUESTER's
// UUID, and returns the handler for chaining.
//
// 🔴 Without it E4/E5 stay silent: there is no other record of who asked for the unlock
// that IAM can address. The username on the audit columns is ⛔ not usable — IAM's
// BY_USER_ID resolver parses its value as a UUID. Passing nil is safe and simply keeps
// the two decision notifications switched off.
func (h *MBHeadHandler) WithUnlockActors(s appmbhead.UnlockActorStore) *MBHeadHandler {
	h.requestUnlockHandler = h.requestUnlockHandler.WithUnlockActors(s)
	h.grantUnlockHandler = h.grantUnlockHandler.WithUnlockActors(s)
	h.rejectUnlockHandler = h.rejectUnlockHandler.WithUnlockActors(s)
	return h
}

// CreateMBHead creates a new MB head record.
func (h *MBHeadHandler) CreateMBHead(ctx context.Context, req *financev1.CreateMBHeadRequest) (*financev1.CreateMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: baseResp}, nil
	}

	// §11 item 106 seal (user decision: option 2 — reject, do NOT ignore silently).
	// ⛔ mbh_check_status is the FROZEN Oracle import trace (K-1) and must not be
	// written from any path. Presence alone is refused; see frozenCheckStatusResponse.
	// This intentionally BREAKS older clients that still send the field.
	if req.MbhCheckStatus != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: frozenCheckStatusResponse()}, nil
	}

	var filament *int
	if req.MbhFilament != nil {
		v := int(*req.MbhFilament)
		filament = &v
	}

	// The parse still runs so a malformed UUID is reported rather than swallowed,
	// but the VALUE is discarded by the application layer: B5 pins every head to the
	// "MB" machine regardless of what the client sends.
	machineID, badResp := parseOptionalMachineID(req.MbhMachineId)
	if badResp != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: badResp}, nil
	}

	entity, err := h.createHandler.Handle(ctx, appmbhead.CreateCommand{
		MBCosting:   req.MbhMbCosting,
		OracleSysID: req.MbhOracleSysId,
		MgtName:     req.MbhMgtName,
		Denier:      req.MbhDenier,
		Filament:    filament,
		Dozing:      req.MbhDozing,
		// ⛔ MBHCheckStatus is deliberately NOT forwarded (§11 item 106). The request
		// can no longer carry it — the guard above rejects any request that does.
		MBHStatus:       req.MbhStatus,
		MBHLdrPrsn:      req.MbhLdrPrsn,
		MBHRunLdrPct:    req.MbhRunLdrPct,
		MBHFinalProduct: req.MbhFinalProduct,
		MBHCode:         req.MbhCode,
		CreatedBy:       getUserFromContext(ctx),
		IsBoughtout:     req.MbhIsBoughtout,
		DevCode:         req.GetMbhDevCode(),
		ShadeCode:       req.GetMbhCode(),
		ShadeName:       req.GetMbhShadeName(),
		CrossSection:    req.GetMbhCrossSection(),
		LustureCode:     req.GetMbhLustureCode(),
		MachineID:       machineID,
		// ⛔ Absent stays absent: nil is persisted as NULL, no default is invented
		// here or downstream (D13).
		VSNumber:         req.MbhVsNumber,
		NoOfProcess:      req.MbhNoOfProcess,
		AdditionalShades: shadeInputsToDomain(req.AdditionalShades),
	})
	if err != nil {
		RecordMBHeadOperation("create", false)
		return &financev1.CreateMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("create", true)
	return &financev1.CreateMBHeadResponse{
		Base: successResponse("MB head created successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// GetMBHead retrieves an MB head record by ID.
func (h *MBHeadHandler) GetMBHead(ctx context.Context, req *financev1.GetMBHeadRequest) (*financev1.GetMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("get", false)
		return &financev1.GetMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("get", false)
		return &financev1.GetMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.getHandler.Handle(ctx, appmbhead.GetQuery{ID: id})
	if err != nil {
		RecordMBHeadOperation("get", false)
		return &financev1.GetMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("get", true)
	return &financev1.GetMBHeadResponse{
		Base: successResponse("MB head retrieved successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// UpdateMBHead updates an existing MB head record.
func (h *MBHeadHandler) UpdateMBHead(ctx context.Context, req *financev1.UpdateMBHeadRequest) (*financev1.UpdateMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	var filament *int
	if req.MbhFilament != nil {
		v := int(*req.MbhFilament)
		filament = &v
	}

	// §11 item 106 seal — same rule as create. ⛔ On update the stake is higher: the
	// row may already carry an Oracle trace, and an accepted write (including an
	// explicit "") would destroy it. Rejecting keeps the stored value INTACT, because
	// the repository UPDATE writes back the value loaded by GetByID untouched.
	if req.MbhCheckStatus != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: frozenCheckStatusResponse()}, nil
	}

	machineID, badResp := parseOptionalMachineID(req.MbhMachineId)
	if badResp != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: badResp}, nil
	}

	entity, err := h.updateHandler.Handle(ctx, appmbhead.UpdateCommand{
		ID:        id,
		MBCosting: req.MbhMbCosting,
		MgtName:   req.MbhMgtName,
		Denier:    req.MbhDenier,
		Filament:  filament,
		Dozing:    req.MbhDozing,
		// ⛔ MBHCheckStatus is deliberately NOT forwarded (§11 item 106). The request
		// can no longer carry it — the guard above rejects any request that does.
		MBHStatus:       req.MbhStatus,
		MBHLdrPrsn:      req.MbhLdrPrsn,
		MBHRunLdrPct:    req.MbhRunLdrPct,
		MBHFinalProduct: req.MbhFinalProduct,
		MBHCode:         req.MbhCode,
		IsActive:        req.MbhIsActive,
		DevCode:         req.MbhDevCode,
		ShadeCode:       req.MbhCode,
		ShadeName:       req.MbhShadeName,
		CrossSection:    req.MbhCrossSection,
		LustureCode:     req.MbhLustureCode,
		MachineID:       machineID,
		UpdatedBy:       getUserFromContext(ctx),
		VSNumber:        req.MbhVsNumber,
		NoOfProcess:     req.MbhNoOfProcess,
		// Shades are only rewritten on an explicit opt-in. A legacy client that never
		// sends replace_additional_shades leaves the stored shade rows intact.
		AdditionalShades:        shadeInputsToDomain(req.AdditionalShades),
		ReplaceAdditionalShades: req.GetReplaceAdditionalShades(),
	})
	if err != nil {
		RecordMBHeadOperation("update", false)
		return &financev1.UpdateMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("update", true)
	return &financev1.UpdateMBHeadResponse{
		Base: successResponse("MB head updated successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// DeleteMBHead soft-deletes an MB head record.
func (h *MBHeadHandler) DeleteMBHead(ctx context.Context, req *financev1.DeleteMBHeadRequest) (*financev1.DeleteMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("delete", false)
		return &financev1.DeleteMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("delete", false)
		return &financev1.DeleteMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	if err := h.deleteHandler.Handle(ctx, appmbhead.DeleteCommand{ID: id, DeletedBy: getUserFromContext(ctx)}); err != nil {
		RecordMBHeadOperation("delete", false)
		return &financev1.DeleteMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("delete", true)
	return &financev1.DeleteMBHeadResponse{Base: successResponse("MB head deleted successfully")}, nil
}

// SubmitMBHead submits an MB Head for approval (DRAFT -> SUBMITTED).
func (h *MBHeadHandler) SubmitMBHead(ctx context.Context, req *financev1.SubmitMBHeadRequest) (*financev1.SubmitMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("submit", false)
		return &financev1.SubmitMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("submit", false)
		return &financev1.SubmitMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.submitHandler.Handle(ctx, appmbhead.SubmitCommand{MbhID: id, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("submit", false)
		return &financev1.SubmitMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("submit", true)
	return &financev1.SubmitMBHeadResponse{
		Base: successResponse("MB head submitted successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// ApproveMBHead approves a submitted MB Head.
//
// 🔴 USER DECISION 2026-08-26 ("Opsi A") — approving now lands the recipe DIRECTLY in
// ~~APPROVED~~ VALIDATED, and this RPC therefore drives the validateHandler, ⛔ not the
// approveHandler. Pressing Approve performs the whole of what Validate used to do:
// freeze the 8 params, bump mbh_current_version, run the composition-sum gate, snapshot
// the composition version, auto-generate the MB cost product, and lock the recipe.
//
// ⚠ Why not simply stop at APPROVED: mb_head_repository.ListValidated() filters
// WHERE mbh_entry_status = 'VALIDATED', and it feeds BOTH MB Push to Head
// (application/mbpush) and Trigger MB Batch (application/mbbatch/dag.go). A workflow that
// ended at APPROVED would leave those two permanently empty and stop MB costing dead.
// Landing on VALIDATED keeps them working with ⛔ zero changes on their side.
//
// ⛔ The logic is NOT copy-pasted into ApproveHandler: ValidateHandler already carries
// the freeze, the gate and TransitionWithAutoGen, and is the tested one. ApproveHandler
// still exists and is still used by cmd/backfill-mb-validate; only this wiring moved.
//
// The response message still says "approved" — that is the button the user pressed, and
// the returned entity carries the real entry status for anyone who looks.
func (h *MBHeadHandler) ApproveMBHead(ctx context.Context, req *financev1.ApproveMBHeadRequest) (*financev1.ApproveMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("approve", false)
		return &financev1.ApproveMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("approve", false)
		return &financev1.ApproveMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.validateHandler.Handle(ctx, appmbhead.ValidateCommand{MbhID: id, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("approve", false)
		return &financev1.ApproveMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("approve", true)
	return &financev1.ApproveMBHeadResponse{
		Base: successResponse("MB head approved successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// ValidateMBHead validates an approved MB Head, freezing its param snapshot and auto-generating
// its product (APPROVED -> VALIDATED, or DRAFT -> VALIDATED for boughtout MBs).
func (h *MBHeadHandler) ValidateMBHead(ctx context.Context, req *financev1.ValidateMBHeadRequest) (*financev1.ValidateMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("validate", false)
		return &financev1.ValidateMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("validate", false)
		return &financev1.ValidateMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.validateHandler.Handle(ctx, appmbhead.ValidateCommand{MbhID: id, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("validate", false)
		return &financev1.ValidateMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("validate", true)
	return &financev1.ValidateMBHeadResponse{
		Base: successResponse("MB head validated successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// UnApproveMBHead ~~reverts an approved MB Head back to a prior state (APPROVED -> UN_APPROVED).~~
//
// 🔴 USER DECISION 2026-08-26 — Un-approve was REMOVED from the MB Recipe workflow. The
// simplified flow is DRAFT (editable) → SUBMITTED (not editable) → APPROVED (locked); a
// locked recipe is reopened through Request Unlock, ⛔ never by un-approving it.
//
// ⛔ The RPC itself is deliberately KEPT — removing it from the proto contract would be a
// breaking change. The SURFACE is what was switched off: this method now refuses every
// call with a plain-language BaseResponse (⛔ not a raw gRPC error), so an old client
// learns why instead of guessing. The application handler refuses too, so nothing depends
// on this check alone.
func (h *MBHeadHandler) UnApproveMBHead(_ context.Context, _ *financev1.UnApproveMBHeadRequest) (*financev1.UnApproveMBHeadResponse, error) {
	RecordMBHeadOperation("unapprove", false)
	return &financev1.UnApproveMBHeadResponse{Base: featureRemovedResponse(unApproveRemovedMessage)}, nil
}

// RevokeMBHead ~~revokes an MB Head, moving it to the terminal REVOKED state.~~
//
// 🔴 USER DECISION 2026-08-26 — Revoke was REMOVED from the MB Recipe workflow. Turning a
// recipe on or off is an admin concern served by the active flag, so a terminal REVOKED
// status is not needed.
//
// ⛔ The RPC itself is deliberately KEPT — removing it from the proto contract would be a
// breaking change, and rows already sitting in REVOKED must stay readable. The SURFACE is
// what was switched off: this method now refuses every call with a plain-language
// BaseResponse (⛔ not a raw gRPC error).
func (h *MBHeadHandler) RevokeMBHead(_ context.Context, _ *financev1.RevokeMBHeadRequest) (*financev1.RevokeMBHeadResponse, error) {
	RecordMBHeadOperation("revoke", false)
	return &financev1.RevokeMBHeadResponse{Base: featureRemovedResponse(revokeRemovedMessage)}, nil
}

// RejectMBHead turns down a submitted MB Head (SUBMITTED -> REJECTED, user decision K-2).
// The reason is mandatory; the domain rejects an empty one.
func (h *MBHeadHandler) RejectMBHead(ctx context.Context, req *financev1.RejectMBHeadRequest) (*financev1.RejectMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("reject", false)
		return &financev1.RejectMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("reject", false)
		return &financev1.RejectMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.rejectHandler.Handle(ctx, appmbhead.RejectCommand{MbhID: id, Reason: req.Reason, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("reject", false)
		return &financev1.RejectMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("reject", true)
	return &financev1.RejectMBHeadResponse{
		Base: successResponse("MB head rejected successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// ReturnMBHeadToDraft sends a rejected MB Head back for editing (REJECTED -> DRAFT,
// user decision K-29).
//
// The reason is OPTIONAL here — unlike RejectMBHead above, which requires one. The proto
// field carries no min_len and the domain never returns ErrReasonRequired; when the reason
// is empty the existing stateReason is PRESERVED. Do NOT add a reason-required check.
func (h *MBHeadHandler) ReturnMBHeadToDraft(ctx context.Context, req *financev1.ReturnMBHeadToDraftRequest) (*financev1.ReturnMBHeadToDraftResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("return_to_draft", false)
		return &financev1.ReturnMBHeadToDraftResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("return_to_draft", false)
		return &financev1.ReturnMBHeadToDraftResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.returnToDraftHandler.Handle(ctx, appmbhead.ReturnToDraftCommand{MbhID: id, Reason: req.Reason, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("return_to_draft", false)
		return &financev1.ReturnMBHeadToDraftResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("return_to_draft", true)
	return &financev1.ReturnMBHeadToDraftResponse{
		Base: successResponse("MB head returned to draft successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// UnrevokeMBHead sends a revoked MB Head back for editing (REVOKED -> DRAFT, user
// decision 2026-08-31). Gated at the permission-interceptor level by
// finance.mb.head.unrevoke, granted only to Super Admin.
//
// The reason is OPTIONAL here, same rationale as ReturnMBHeadToDraft above: when it is
// empty the existing stateReason is PRESERVED. Do NOT add a reason-required check.
func (h *MBHeadHandler) UnrevokeMBHead(ctx context.Context, req *financev1.UnrevokeMBHeadRequest) (*financev1.UnrevokeMBHeadResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("unrevoke", false)
		return &financev1.UnrevokeMBHeadResponse{Base: baseResp}, nil
	}

	id, err := uuid.Parse(req.MbhId)
	if err != nil {
		RecordMBHeadOperation("unrevoke", false)
		return &financev1.UnrevokeMBHeadResponse{Base: invalidIDResponse("mbh_id")}, nil //nolint:nilerr // BaseResponse pattern: error returned in response body
	}

	entity, err := h.unrevokeHandler.Handle(ctx, appmbhead.UnrevokeCommand{MbhID: id, Reason: req.Reason, ActorUserID: getUserFromContext(ctx)})
	if err != nil {
		RecordMBHeadOperation("unrevoke", false)
		return &financev1.UnrevokeMBHeadResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("unrevoke", true)
	return &financev1.UnrevokeMBHeadResponse{
		Base: successResponse("MB head unrevoked successfully"),
		Data: mbHeadEntityToProto(entity),
	}, nil
}

// ListMBHeads lists MB head records with search, filter, and pagination.
func (h *MBHeadHandler) ListMBHeads(ctx context.Context, req *financev1.ListMBHeadsRequest) (*financev1.ListMBHeadsResponse, error) {
	page := int(req.Page)
	if page == 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize == 0 {
		pageSize = 10
	}

	query := appmbhead.ListQuery{
		Page:      page,
		PageSize:  pageSize,
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		t := true
		query.IsActive = &t
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		f := false
		query.IsActive = &f
	default:
	}

	// R16: 0 (proto3 default) means "no filter", matching the existing product_type_id
	// convention on ListCostProductMastersRequest — never send an explicit filter for it.
	if req.CostProductId != 0 {
		costProductID := req.CostProductId
		query.CostProductID = &costProductID
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordMBHeadOperation("list", false)
		return &financev1.ListMBHeadsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("list", true)

	items := make([]*financev1.MBHead, len(result.Items))
	for i, e := range result.Items {
		items[i] = mbHeadEntityToProto(e)
	}

	return &financev1.ListMBHeadsResponse{
		Base: successResponse("MB heads retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// ExportMBHeads exports MB Heads to an Excel file.
func (h *MBHeadHandler) ExportMBHeads(ctx context.Context, req *financev1.ExportMBHeadsRequest) (*financev1.ExportMBHeadsResponse, error) {
	// IncludeRejected forwarded verbatim: proto3 zero value (false) EXCLUDES rejected
	// documents, matching this export's behavior before the field existed.
	query := appmbhead.ExportQuery{IncludeRejected: req.GetIncludeRejected()}

	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		query.IsActive = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// Export all - no filter
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordMBHeadOperation("export", false)
		return &financev1.ExportMBHeadsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("export", true)

	return &financev1.ExportMBHeadsResponse{
		Base:        successResponse("MB Heads exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// newExportFullHandler returns nil when no reader was wired, so ExportMBRecipeFull can
// report a configuration error instead of dereferencing nil.
func newExportFullHandler(reader appmbhead.RecipeFullReader) *appmbhead.ExportFullHandler {
	if reader == nil {
		return nil
	}
	return appmbhead.NewExportFullHandler(reader)
}

// ExportMBRecipeFull exports the denormalized full MB recipe (recipe + composition + MB
// cost) to Excel, one row per composition line (P12, items C1 + C2).
//
// ⛔ Read-only: this path issues no UPDATE. cst_mb_cost and cst_product_cost are joined
// for display only.
func (h *MBHeadHandler) ExportMBRecipeFull(ctx context.Context, req *financev1.ExportMBRecipeFullRequest) (*financev1.ExportMBRecipeFullResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("export_full", false)
		return &financev1.ExportMBRecipeFullResponse{Base: baseResp}, nil
	}
	if h.exportFullH == nil {
		RecordMBHeadOperation("export_full", false)
		return &financev1.ExportMBRecipeFullResponse{
			Base: &commonv1.BaseResponse{
				IsSuccess:  false,
				StatusCode: "500",
				Message:    "Full recipe export is not configured",
			},
		}, nil
	}

	cmd := appmbhead.ExportFullCommand{
		Period:   req.Period,
		CostType: req.CostType,
		// Forwarded VERBATIM. Empty = no derived-check-status filter (ALL rows, the
		// NULL/"Belum dihitung" heads included), which is exactly what this export did
		// before the filter existed. ⛔ Never defaulted here.
		CheckStatusCalc: req.CheckStatusCalc,
		// IncludeRejected forwarded verbatim: proto3 zero value (false) EXCLUDES rejected
		// documents, matching this export's behavior before the field existed.
		IncludeRejected: req.GetIncludeRejected(),
	}
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		active := true
		cmd.ActiveOnly = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		active := false
		cmd.ActiveOnly = &active
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
		// No active filter — absence stays absence.
	}

	content, fileName, err := h.exportFullH.Handle(ctx, cmd)
	if err != nil {
		RecordMBHeadOperation("export_full", false)
		return &financev1.ExportMBRecipeFullResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("export_full", true)
	return &financev1.ExportMBRecipeFullResponse{
		Base:        successResponse("MB recipe full export generated successfully"),
		FileContent: content,
		FileName:    fileName,
	}, nil
}

// ImportMBHeads imports MB Heads from an Excel file.
func (h *MBHeadHandler) ImportMBHeads(ctx context.Context, req *financev1.ImportMBHeadsRequest) (*financev1.ImportMBHeadsResponse, error) {
	if baseResp := h.validation.ValidateRequest(req); baseResp != nil {
		RecordMBHeadOperation("import", false)
		return &financev1.ImportMBHeadsResponse{Base: baseResp}, nil
	}

	cmd := appmbhead.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	}

	result, err := h.importHandler.Handle(ctx, cmd)
	if err != nil {
		RecordMBHeadOperation("import", false)
		return &financev1.ImportMBHeadsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordMBHeadOperation("import", true)

	importErrors := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		importErrors[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	return &financev1.ImportMBHeadsResponse{
		Base:         successResponse("Import completed"),
		SuccessCount: result.SuccessCount,
		SkippedCount: result.SkippedCount,
		FailedCount:  result.FailedCount,
		Errors:       importErrors,
	}, nil
}

// DownloadMBHeadTemplate downloads the Excel import template for MB Heads.
func (h *MBHeadHandler) DownloadMBHeadTemplate(_ context.Context, _ *financev1.DownloadMBHeadTemplateRequest) (*financev1.DownloadMBHeadTemplateResponse, error) {
	result, err := h.templateHandler.Handle()
	if err != nil {
		return &financev1.DownloadMBHeadTemplateResponse{Base: InternalErrorResponse(err.Error())}, nil
	}

	return &financev1.DownloadMBHeadTemplateResponse{
		Base:        successResponse("Template generated successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// mbHeadEntityToProto converts a domain MBHead entity to its proto representation.
func mbHeadEntityToProto(e *mbhead.Entity) *financev1.MBHead {
	p := &financev1.MBHead{
		MbhId:        e.ID().String(),
		MbhMbCosting: e.MBCosting(),
		MbhIsActive:  e.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: e.CreatedAt().Format(time.RFC3339),
			CreatedBy: e.CreatedBy(),
		},
	}
	if e.OracleSysID() != nil {
		p.MbhOracleSysId = *e.OracleSysID()
	}
	if e.MgtName() != nil {
		p.MbhMgtName = *e.MgtName()
	}
	p.MbhDenier = e.Denier()
	if e.Filament() != nil {
		v := safeconv.IntToInt32(*e.Filament())
		p.MbhFilament = &v
	}
	p.MbhDozing = e.Dozing()
	// Both check-status columns are forwarded, side by side (user decision, plan §11
	// item 42 = option 2). MbhCheckStatus is the FROZEN Oracle import trace, shown
	// read-only on the detail page only; MbhCheckStatusCalc is the DERIVED value and
	// is the primary column in table/filter/export.
	//
	// ⛔ MbhCheckStatusCalc stays nil when never calculated (207 legacy rows,
	// permanently — there is no backfill). It is NOT coerced to "" (D13): absent must
	// stay absent so the client can render "belum dihitung" instead of a blank that
	// reads like a stored empty value.
	p.MbhCheckStatus = e.MBHCheckStatus()
	p.MbhCheckStatusCalc = e.MBHCheckStatusCalc()
	p.MbhStatus = e.MBHStatus()
	p.MbhLdrPrsn = e.MBHLdrPrsn()
	p.MbhRunLdrPct = e.MBHRunLdrPct()
	p.MbhFinalProduct = e.MBHFinalProduct()
	p.MbhCode = e.MBHCode()
	p.IsBoughtout = e.IsBoughtout()
	p.DevCode = e.DevCode()
	p.ShadeCode = e.ShadeCode()
	p.ShadeName = e.ShadeName()
	p.CrossSection = e.CrossSection()
	p.LustureCode = e.LustureCode()
	p.EntryStatus = e.EntryStatus()
	p.CurrentVersion = e.CurrentVersion()
	if e.MachineFixedTotal() != nil {
		p.MachineFixedTotal = *e.MachineFixedTotal()
	}
	if e.MachineID() != nil {
		id := e.MachineID().String()
		p.MachineId = &id
	}
	p.StateReason = e.StateReason()
	p.CostProductId = e.CostProductID()
	if e.CostGeneratedAt() != nil {
		p.CostGeneratedAt = *e.CostGeneratedAt()
	}
	p.CostGeneratedBy = e.CostGeneratedBy()
	if e.ParamWaste() != nil {
		p.ParamWaste = *e.ParamWaste()
	}
	if e.ParamQualityLoss() != nil {
		p.ParamQualityLoss = *e.ParamQualityLoss()
	}
	if e.ParamEfficiency() != nil {
		p.ParamEfficiency = *e.ParamEfficiency()
	}
	if e.ParamDevExpense() != nil {
		p.ParamDevExpense = *e.ParamDevExpense()
	}
	if e.ParamPacking() != nil {
		p.ParamPacking = *e.ParamPacking()
	}
	if e.ParamMBProdPerDay() != nil {
		p.ParamMbProdPerDay = *e.ParamMBProdPerDay()
	}
	p.ParamThroughputPerHour = e.ParamThroughputPerHour()
	p.ParamNoOfProcess = e.ParamNoOfProcess()
	mbHeadRecipeFieldsToProto(p, e)
	if e.UpdatedAt() != nil {
		p.Audit.UpdatedAt = e.UpdatedAt().Format(time.RFC3339)
	}
	if e.UpdatedBy() != nil {
		p.Audit.UpdatedBy = *e.UpdatedBy()
	}
	return p
}

// parseOptionalMachineID parses an optional *string mbh_machine_id into a *uuid.UUID.
// Returns (nil, nil) when the input is nil or empty. Returns (nil, baseResponse) when
// the input is non-empty but invalid.
func parseOptionalMachineID(raw *string) (*uuid.UUID, *commonv1.BaseResponse) {
	if raw == nil || *raw == "" {
		return nil, nil
	}
	id, err := uuid.Parse(*raw)
	if err != nil {
		return nil, ErrorResponse("400", "invalid mbh_machine_id: "+err.Error())
	}
	return &id, nil
}

// mbHeadRecipeFieldsToProto fills MBHead fields 38-43 and 45 (P5 recipe + lock
// columns, including mbh_unlock_reason). Field 44 (mbh_check_status_calc) is NOT
// filled here — it is mapped by mbHeadEntityToProto itself.
//
// ⚠ Pointer fields are passed through unchanged: a NULL column must surface as an
// absent proto field, ⛔ never as "" or 0 (D13). That applies to
// mbh_unlock_reason too: absent means "no unlock request was ever recorded".
//
// ⛔ mbh_is_locked / mbh_unlock_requested_* / mbh_unlock_reason are MAPPED ONLY.
// Lock and unlock BEHAVIOR lives in the domain and application layers, not here.
// The NULL → false coalescing for mbh_is_locked (J12) is done by the repository's
// COALESCE(mbh_is_locked, FALSE) read; this function only forwards the result.
// mbh_unlock_reason is deliberately never cleared on grant or reject (principle
// U-2), so a reason may still be present on an already-unlocked head — that is
// the audit trail, not stale data.
func mbHeadRecipeFieldsToProto(p *financev1.MBHead, e *mbhead.Entity) {
	p.MbhVsNumber = e.VSNumber()
	p.MbhNoOfProcess = e.NoOfProcess()

	locked := e.IsLocked()
	p.MbhIsLocked = &locked

	if at := e.UnlockRequestedAt(); at != nil {
		s := at.Format(time.RFC3339)
		p.MbhUnlockRequestedAt = &s
	}
	p.MbhUnlockRequestedBy = e.UnlockRequestedBy()
	p.MbhUnlockReason = e.UnlockReason()

	shades := e.AdditionalShades()
	if len(shades) == 0 {
		return
	}
	p.AdditionalShades = make([]*financev1.MBHeadShade, 0, len(shades))
	for _, s := range shades {
		p.AdditionalShades = append(p.AdditionalShades, &financev1.MBHeadShade{
			MbhsSeqNo:     s.SeqNo,
			MbhsShadeCode: s.Code,
			MbhsShadeName: s.Name,
		})
	}
}

// shadeInputsToDomain converts proto shade inputs into domain shades.
//
// Returns nil for a nil/empty input so the application layer can tell "field
// absent" from "explicitly empty list" — the distinction that keeps a legacy
// payload from clearing stored shades.
func shadeInputsToDomain(in []*financev1.MBHeadShadeInput) []mbhead.Shade {
	if len(in) == 0 {
		return nil
	}
	out := make([]mbhead.Shade, 0, len(in))
	for _, s := range in {
		if s == nil {
			continue
		}
		out = append(out, mbhead.Shade{
			SeqNo: s.GetMbhsSeqNo(),
			Code:  s.GetMbhsShadeCode(),
			Name:  s.GetMbhsShadeName(),
		})
	}
	return out
}
