package mbactivesync

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/google/uuid"

	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbparam"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// HeadReader is the part of mbhead.Repository the workflow adapter needs.
type HeadReader interface {
	GetByID(ctx context.Context, id uuid.UUID) (*mbhead.Entity, error)
}

// SpinLister is the part of mbspin.Repository used to find the head's root spin.
type SpinLister interface {
	List(ctx context.Context, filter mbspin.ListFilter) ([]*mbspin.Entity, int64, error)
}

// HeadSubmitter is the existing Submit use case (application/mbhead SubmitHandler).
type HeadSubmitter interface {
	Handle(ctx context.Context, cmd appmbhead.SubmitCommand) (*mbhead.Entity, error)
}

// HeadValidator is the existing Validate use case (application/mbhead ValidateHandler).
type HeadValidator interface {
	HandleWithOutcome(ctx context.Context, cmd appmbhead.ValidateCommand) (*mbhead.Entity, *mbhead.SupersedeOutcome, error)
}

// HandlerHeadWorkflow implements HeadWorkflow on top of the EXISTING Submit/Validate
// use cases, in the same order as the UI (Submit → Validate; a boughtout head validates
// straight from DRAFT) — design part 2 §5.5. Each handler is its own transaction, so
// every step checks the live entry status first and a rerun resumes where it stopped
// (e.g. a head left SUBMITTED by a failed Validate is only validated).
//
// No lookup-sync trigger is attached to the Validate handler: like the rest of the
// apply path, products repointed by a confirmed supersede are listed in after_json
// (affected_product_ids) for the operator's recalc.
type HandlerHeadWorkflow struct {
	heads    HeadReader
	spins    SpinLister
	submit   HeadSubmitter
	validate HeadValidator
}

// NewHandlerHeadWorkflow builds the adapter.
func NewHandlerHeadWorkflow(heads HeadReader, spins SpinLister, submit HeadSubmitter, validate HeadValidator) *HandlerHeadWorkflow {
	return &HandlerHeadWorkflow{heads: heads, spins: spins, submit: submit, validate: validate}
}

var _ HeadWorkflow = (*HandlerHeadWorkflow)(nil)

// headWorkflowImage is the before/after image of an EnsureValidated item.
type headWorkflowImage struct {
	EntryStatus           string      `json:"mbh_entry_status"`
	CostProductID         int64       `json:"mbh_cost_product_id"`
	IsActive              bool        `json:"mbh_is_active"`
	RootSpinID            *uuid.UUID  `json:"root_spin_id"`
	SupersedeBatchID      *uuid.UUID  `json:"supersede_batch_id,omitempty"`
	SupersededSpinIDs     []uuid.UUID `json:"superseded_spin_ids,omitempty"`
	DeactivatedHeadIDs    []uuid.UUID `json:"deactivated_head_ids,omitempty"`
	DeactivatedProductIDs []int64     `json:"deactivated_product_ids,omitempty"`
	AffectedProductIDs    []int64     `json:"affected_product_ids,omitempty"`
}

func (i headWorkflowImage) json() string {
	b, err := json.Marshal(i)
	if err != nil {
		return ""
	}
	return string(b)
}

// validateReady mirrors the Validate handler's accepted origin states (own production
// vs boughtout); a DRAFT own-production head is submitted first. UNLOCK_REQUESTED is
// deliberately NOT accepted even for boughtout (the handler's boughtout map lets it
// through only by accident of the state map): design part 2 §5.5 routes it to the UI
// as validate_gate.
func validateReady(h *mbhead.Entity) bool {
	switch h.EntryStatus() {
	case mbhead.StatusSubmitted, mbhead.StatusApproved:
		return true
	case mbhead.StatusDraft:
		return h.IsBoughtout()
	}
	return false
}

// EnsureValidated implements HeadWorkflow. A non-nil error is an unexpected failure
// (the applier reports FAILED db_error); business gates come back as FAILED results.
func (w *HandlerHeadWorkflow) EnsureValidated(ctx context.Context, headID uuid.UUID, actor string, confirmSupersede bool) (ItemResult, error) {
	res := ItemResult{HeadID: headID.String()}
	head, err := w.heads.GetByID(ctx, headID)
	if errors.Is(err, mbhead.ErrNotFound) {
		return failResult(res, ResultFailed, ReasonStalePlan, "head missing or deleted"), nil
	}
	if err != nil {
		return res, fmt.Errorf("load head: %w", err)
	}
	before, err := w.image(ctx, head)
	if err != nil {
		return res, err
	}
	res.BeforeJSON = before.json()
	if head.SupersededByHeadID() != nil {
		return failResult(res, ResultSkipped, ReasonHeadSuperseded, "head is superseded"), nil
	}
	if head.CostProductID() != 0 {
		// Already validated once (cost product generated); a missing root spin is
		// GenerateRootSpin's job (K7), not a re-validate.
		if before.RootSpinID != nil {
			res.SpinID = before.RootSpinID.String()
		}
		return failResult(res, ResultSkipped, ReasonAlreadyInTarget, "head already has its MB cost product"), nil
	}

	submitted := false
	if head.EntryStatus() == mbhead.StatusDraft && !head.IsBoughtout() {
		if _, err := w.submit.Handle(ctx, appmbhead.SubmitCommand{MbhID: headID, ActorUserID: actor}); err != nil {
			return mapWorkflowErr(res, "submit", err)
		}
		submitted = true
		if head, err = w.heads.GetByID(ctx, headID); err != nil {
			return res, fmt.Errorf("reload head: %w", err)
		}
	}
	if !validateReady(head) {
		return failResult(res, ResultFailed, ReasonValidateGate,
			fmt.Sprintf("entry status %s cannot be validated (handle in the UI)", head.EntryStatus())), nil
	}
	_, outcome, err := w.validate.HandleWithOutcome(ctx, appmbhead.ValidateCommand{
		MbhID: headID, ActorUserID: actor, ConfirmSupersede: confirmSupersede,
	})
	if err != nil {
		out, rerr := mapWorkflowErr(res, "validate", err)
		if submitted && rerr == nil {
			out.Message += " (head was submitted; a rerun only validates)"
		}
		return out, rerr
	}
	return w.applied(ctx, res, headID, outcome)
}

func (w *HandlerHeadWorkflow) applied(ctx context.Context, res ItemResult, headID uuid.UUID, outcome *mbhead.SupersedeOutcome) (ItemResult, error) {
	head, err := w.heads.GetByID(ctx, headID)
	if err != nil {
		return res, fmt.Errorf("reload validated head: %w", err)
	}
	after, err := w.image(ctx, head)
	if err != nil {
		return res, err
	}
	if outcome != nil {
		batch := outcome.BatchID
		after.SupersedeBatchID = &batch
		after.SupersededSpinIDs = outcome.OldSpinIDs
		after.DeactivatedHeadIDs = outcome.DeactivatedHeadIDs
		after.DeactivatedProductIDs = outcome.DeactivatedProductIDs
		after.AffectedProductIDs = outcome.AffectedProductIDs
	}
	res.AfterJSON = after.json()
	if after.RootSpinID != nil {
		res.SpinID = after.RootSpinID.String()
	}
	res.Result, res.Reason = ResultApplied, ""
	return res, nil
}

func (w *HandlerHeadWorkflow) image(ctx context.Context, head *mbhead.Entity) (headWorkflowImage, error) {
	img := headWorkflowImage{EntryStatus: head.EntryStatus(), CostProductID: head.CostProductID(), IsActive: head.IsActive()}
	root, err := w.rootSpin(ctx, head.ID())
	if err != nil {
		return img, err
	}
	if root != uuid.Nil {
		img.RootSpinID = &root
	}
	return img, nil
}

// rootSpin returns the head's oldest live (not superseded) root spin, or uuid.Nil.
func (w *HandlerHeadWorkflow) rootSpin(ctx context.Context, headID uuid.UUID) (uuid.UUID, error) {
	filter := mbspin.ListFilter{HeadID: headID, Page: 1, PageSize: 100, SortBy: "created_at", SortOrder: "asc"}
	spins, _, err := w.spins.List(ctx, filter)
	if err != nil {
		return uuid.Nil, fmt.Errorf("list head spins: %w", err)
	}
	for _, s := range spins {
		if s.ParentSpinID() == nil {
			return s.ID(), nil
		}
	}
	return uuid.Nil, nil
}

func failResult(res ItemResult, status, reason, msg string) ItemResult {
	res.Result, res.Reason, res.Message = status, reason, msg
	return res
}

// mapWorkflowErr maps the Submit/Validate business errors to item verdicts (design
// part 2 §5.9); anything else is returned as an error (FAILED db_error).
func mapWorkflowErr(res ItemResult, step string, err error) (ItemResult, error) {
	msg := step + ": " + err.Error()
	switch {
	case errors.Is(err, mbspin.ErrSupersedeConfirmationRequired):
		return failResult(res, ResultFailed, ReasonSupersedeConfirmRequired, msg), nil
	case errors.Is(err, mbspin.ErrSpinShadeFgExists):
		return failResult(res, ResultFailed, ReasonFGConflict, msg), nil
	case errors.Is(err, mbspin.ErrFgNotPoy):
		return failResult(res, ResultFailed, ReasonPOYInvalid, msg), nil
	case errors.Is(err, mbspin.ErrFgShadeMismatch):
		return failResult(res, ResultFailed, ReasonShadeMismatch, msg), nil
	case errors.Is(err, mbhead.ErrNotFound):
		return failResult(res, ResultFailed, ReasonStalePlan, msg), nil
	case isValidateGateErr(err):
		return failResult(res, ResultFailed, ReasonValidateGate, msg), nil
	}
	return res, fmt.Errorf("%s head: %w", step, err)
}

func isValidateGateErr(err error) bool {
	for _, target := range []error{
		mbhead.ErrInvalidTransition, mbhead.ErrHeadLocked, mbhead.ErrRecipeFieldRequired,
		mbcomposition.ErrCompositionEmpty, mbcomposition.ErrCompositionSumInvalid,
		mbparam.ErrParamNotFound, mbparam.ErrNotFound,
		costproductparameter.ErrInvalidValueShape, costproductparameter.ErrInvalidDataType,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}
