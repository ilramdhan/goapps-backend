// Package costproductrequest holds application use cases for the Phase A request aggregate.
package costproductrequest

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	fillDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	routeDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/requesthistory"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/iamclient"
)

// CreateCommand is the create-time input.
type CreateCommand struct {
	RequestTypeID         int32
	Title                 string
	Description           string
	CustomerName          string
	CustomerCode          string
	ProductClassification string
	TargetVolume          string
	TargetPriceRange      string
	UrgencyLevel          string
	NeededByDate          string
	RequesterUserID       string
	Spec                  *domain.SpecInput
}

// CreateHandler creates a draft request.
type CreateHandler struct {
	repo        domain.Repository
	cprNotifier CPRNotifier // optional; if nil, CPR notification emission is skipped
}

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

// WithCPRNotifier attaches a CPRNotifier to CreateHandler. Returns receiver for chaining.
func (h *CreateHandler) WithCPRNotifier(n CPRNotifier) *CreateHandler {
	h.cprNotifier = n
	return h
}

// Handle executes the create.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*domain.Request, error) {
	req, err := domain.New(domain.NewInput{
		RequestTypeID:         cmd.RequestTypeID,
		Title:                 cmd.Title,
		Description:           cmd.Description,
		CustomerName:          cmd.CustomerName,
		CustomerCode:          cmd.CustomerCode,
		ProductClassification: cmd.ProductClassification,
		TargetVolume:          cmd.TargetVolume,
		TargetPriceRange:      cmd.TargetPriceRange,
		UrgencyLevel:          cmd.UrgencyLevel,
		NeededByDate:          cmd.NeededByDate,
		RequesterUserID:       cmd.RequesterUserID,
		Spec:                  cmd.Spec,
	})
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, req); err != nil {
		return nil, err
	}
	// Notify submitters (finance) that a new draft is waiting for their submission.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_DRAFT_CREATED",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.product.request.submit"},
		},
	})
	// Acknowledge to the creator that their draft was saved.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_DRAFT_CREATED_ACK",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	return req, nil
}

// GetQuery loads by id or request_no.
type GetQuery struct {
	RequestID int64
	RequestNo string
}

// GetHandler returns the aggregate.
type GetHandler struct{ repo domain.Repository }

// NewGetHandler constructs a GetHandler.
func NewGetHandler(r domain.Repository) *GetHandler { return &GetHandler{repo: r} }

// Handle resolves by id (preferred) or by no.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*domain.Request, error) {
	if q.RequestID > 0 {
		return h.repo.GetByID(ctx, q.RequestID)
	}
	return h.repo.GetByNo(ctx, q.RequestNo)
}

// UpdateCommand mirrors domain.UpdateInput plus request_id.
type UpdateCommand struct {
	RequestID             int64
	Title                 string
	Description           string
	CustomerName          string
	CustomerCode          string
	ProductClassification string
	TargetVolume          string
	TargetPriceRange      string
	UrgencyLevel          string
	NeededByDate          string
	Spec                  *domain.SpecInput
}

// UpdateHandler mutates DRAFT fields.
type UpdateHandler struct{ repo domain.Repository }

// NewUpdateHandler constructs an UpdateHandler.
func NewUpdateHandler(r domain.Repository) *UpdateHandler { return &UpdateHandler{repo: r} }

// Handle executes the update.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*domain.Request, error) {
	req, err := h.repo.GetByID(ctx, cmd.RequestID)
	if err != nil {
		return nil, err
	}
	if err := req.Update(domain.UpdateInput{
		Title:                 cmd.Title,
		Description:           cmd.Description,
		CustomerName:          cmd.CustomerName,
		CustomerCode:          cmd.CustomerCode,
		ProductClassification: cmd.ProductClassification,
		TargetVolume:          cmd.TargetVolume,
		TargetPriceRange:      cmd.TargetPriceRange,
		UrgencyLevel:          cmd.UrgencyLevel,
		NeededByDate:          cmd.NeededByDate,
		Spec:                  cmd.Spec,
	}); err != nil {
		return nil, err
	}
	if err := h.repo.Save(ctx, req); err != nil {
		return nil, err
	}
	return req, nil
}

// ListQuery is the list input.
type ListQuery struct {
	Search          string
	Status          string
	RequestTypeID   int32
	RequesterUserID string
	AssigneeUserID  string
	Page            int
	PageSize        int
	SortBy          string
	SortOrder       string
}

// ListResult bundles items + total.
type ListResult struct {
	Items []*domain.Request
	Total int64
}

// ListHandler returns a paginated list.
type ListHandler struct{ repo domain.Repository }

// NewListHandler constructs a ListHandler.
func NewListHandler(r domain.Repository) *ListHandler { return &ListHandler{repo: r} }

// Handle executes the list.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (ListResult, error) {
	items, total, err := h.repo.List(ctx, domain.Filter(q))
	if err != nil {
		return ListResult{}, err
	}
	return ListResult{Items: items, Total: total}, nil
}

// =============================================================================
// State transitions (one handler per transition keeps the gRPC handler tidy).
// =============================================================================

// AuditEmitter is the minimal interface TransitionHandler needs from the audit
// log emitter. Decoupled here so the costauditlog package isn't a hard dependency.
type AuditEmitter interface {
	Emit(ctx context.Context, in AuditEntry) error
}

// NotificationEmitter is the minimal interface TransitionHandler needs to fire
// in-app notifications. Matches the Emit signature of costnotification.Emitter
// so the wiring in main.go is a direct assignment with no adapter.
type NotificationEmitter interface {
	Emit(ctx context.Context, in NotificationInput) error
}

// NotificationInput is a decoupled mirror of costnotification.NewInput.
// The costnotification package is not imported here to preserve Clean Architecture
// layering (application/costproductrequest must not import application/costnotification).
type NotificationInput struct {
	RecipientUserID string
	TriggerType     string
	RequestID       int64
	Payload         string
}

// CPRNotifier dispatches rule-based, multi-recipient notifications for CPR
// lifecycle events. Implemented by iamnotifier.CPRNotifier in infrastructure.
// Replaces NotificationEmitter for new code; NotificationEmitter is kept for
// backward compatibility during migration.
type CPRNotifier interface {
	NotifyEvent(ctx context.Context, event CPREvent) error
}

// CPREvent describes a CPR lifecycle notification to be dispatched.
type CPREvent struct {
	// EventType identifies the event, e.g. "CPR_DRAFT_CREATED", "CPR_SUBMITTED_REVIEWER".
	EventType string
	RequestID int64
	RequestNo string
	// RequesterUserID is the user who created the request, used for BY_USER_ID rules.
	RequesterUserID string
	// Rules defines who receives the notification. When empty the default rule
	// for the event type is applied by the implementation.
	Rules []CPRNotifRule
	// ActorName is the display name of the user who triggered the event.
	// Used in comment/mention notifications to say "X commented on...".
	ActorName string
	// MentionedUserIDs is populated for CPR_COMMENT_ADDED events when the
	// comment body contains @mentions. Each UUID receives a separate
	// CPR_MENTIONED notification.
	MentionedUserIDs []string
}

// CPRNotifRule is a single recipient resolution rule embedded in a CPREvent.
type CPRNotifRule struct {
	// RuleType is one of "BY_PERMISSION", "BY_USER_ID", "BY_DEPT", "BY_ROLE".
	RuleType string
	Value    string
}

// AuditEntry is the shape the transition handler hands off to the emitter.
// Mirrors costauditlog.NewInput field-for-field so the wiring in main.go is a
// pure adapter (no domain leakage into this package).
type AuditEntry struct {
	EntityType string
	EntityID   int64
	Operation  string
	BeforeData string
	AfterData  string
	UserID     string
}

// FillTaskCreator creates fill tasks for all route levels of a request before
// the ROUTING_DEFINED → PARAMETER_PENDING transition is committed.
// Implemented by costfillassignment.CreateAllTasksHandler.
type FillTaskCreator interface {
	CreateForRequest(ctx context.Context, requestID, productSysID, routeHeadID int64, routeLevels []int32, perLevelTotals map[int32]int32, requestNo string) error
}

// FillCompletionChecker checks whether all regular fill levels (below the
// completion chain at L100+) are approved for a given request.
// Implemented by costfillassignment.TaskRepository.
type FillCompletionChecker interface {
	CountNonApprovedBelow(ctx context.Context, requestID int64, maxLevel int32) (int, error)
}

// ApplicableParamCounter counts applicable params for a set of product sys IDs.
// Used when creating fill tasks to populate cft_total_params per route level.
// Implemented by the costproductparameter.Repository postgres adapter.
type ApplicableParamCounter interface {
	CountApplicableForProducts(ctx context.Context, productSysIDs []int64) (int32, error)
}

// TransitionHandler wraps a single state transition: load → mutate → save.
// Optionally emits a CAL_ audit row and/or in-app notifications after a
// successful save. Failures of both are best-effort (logged, never blocking).
type TransitionHandler struct {
	repo         domain.Repository
	emitter      AuditEmitter
	notifier     NotificationEmitter       // optional; if nil, notification emission is skipped
	cprNotifier  CPRNotifier               // optional; if nil, CPR notification emission is skipped
	routeRepo    routeDomain.Repository    // optional; used by MarkParameterPending
	fillCreator  FillTaskCreator           // optional; if nil, fill task creation is skipped
	fillChecker  FillCompletionChecker     // optional; if nil, fill guard is skipped
	paramCounter ApplicableParamCounter    // optional; used to set cft_total_params on task creation
	wflClient    iamclient.WorkflowClient  // optional; if nil, IAM workflow wiring is skipped
	historyRepo  requesthistory.Repository // optional; if nil, approval trace recording is skipped
}

// NewTransitionHandler constructs a TransitionHandler. Pass emitter=nil to skip auditing.
func NewTransitionHandler(r domain.Repository) *TransitionHandler {
	return &TransitionHandler{repo: r}
}

// WithAudit attaches an audit emitter. Returns the receiver for chaining.
func (h *TransitionHandler) WithAudit(emitter AuditEmitter) *TransitionHandler {
	h.emitter = emitter
	return h
}

// WithNotifier attaches a notification emitter. Returns the receiver for chaining.
// Pass nil (or omit) to disable in-app notification emission.
func (h *TransitionHandler) WithNotifier(n NotificationEmitter) *TransitionHandler {
	h.notifier = n
	return h
}

// WithCPRNotifier attaches a CPRNotifier to TransitionHandler. Returns receiver for chaining.
func (h *TransitionHandler) WithCPRNotifier(n CPRNotifier) *TransitionHandler {
	h.cprNotifier = n
	return h
}

// WithRouteRepo attaches the route repository used by MarkParameterPending to
// resolve route levels for fill-task creation. Returns the receiver for chaining.
func (h *TransitionHandler) WithRouteRepo(rr routeDomain.Repository) *TransitionHandler {
	h.routeRepo = rr
	return h
}

// WithFillCreator attaches the fill-task creator called before the state
// transition in MarkParameterPending. Returns the receiver for chaining.
func (h *TransitionHandler) WithFillCreator(c FillTaskCreator) *TransitionHandler {
	h.fillCreator = c
	return h
}

// WithFillChecker attaches a fill-completion checker used by MarkParameterComplete
// to guard against premature completion when regular fill levels are not yet approved.
func (h *TransitionHandler) WithFillChecker(c FillCompletionChecker) *TransitionHandler {
	h.fillChecker = c
	return h
}

// WithParamCounter attaches an applicable-param counter used by createFillTasksForRequest
// to populate cft_total_params per route level on fill task creation.
func (h *TransitionHandler) WithParamCounter(c ApplicableParamCounter) *TransitionHandler {
	h.paramCounter = c
	return h
}

// WithWorkflowClient attaches an IAM workflow client used by Submit to start an
// approval instance after a successful DRAFT → SUBMITTED transition. This is
// best-effort: if the client returns an error the submit still succeeds.
// Pass nil (or omit) to disable IAM workflow wiring.
func (h *TransitionHandler) WithWorkflowClient(c iamclient.WorkflowClient) *TransitionHandler {
	h.wflClient = c
	return h
}

// WithHistoryRepo attaches an approval trace repository so that every successful
// state transition is recorded as a history entry. Pass nil (or omit) to disable.
func (h *TransitionHandler) WithHistoryRepo(r requesthistory.Repository) *TransitionHandler {
	h.historyRepo = r
	return h
}

// applyOpts carries the per-call audit metadata.
type applyOpts struct {
	operation string // CAL operation tag (e.g. STATUS_CHANGE, FEASIBILITY)
	actorID   string
	actorName string
}

func (h *TransitionHandler) apply(ctx context.Context, requestID int64, mutate func(*domain.Request) error, opts applyOpts) (*domain.Request, error) {
	req, err := h.repo.GetByID(ctx, requestID)
	if err != nil {
		return nil, err
	}
	beforeStatus := req.Status()
	if err := mutate(req); err != nil {
		return nil, err
	}
	if err := h.repo.Save(ctx, req); err != nil {
		return nil, err
	}
	h.emitAudit(ctx, req, beforeStatus, opts)
	h.insertHistory(ctx, req, beforeStatus, opts)
	return req, nil
}

// insertHistory records a status transition in the approval trace (best-effort).
func (h *TransitionHandler) insertHistory(ctx context.Context, req *domain.Request, beforeStatus string, opts applyOpts) {
	if h.historyRepo == nil {
		return
	}
	actorName := opts.actorName
	if actorName == "" {
		actorName = opts.actorID
	}
	entry := &requesthistory.Entry{
		RequestID:   req.RequestID(),
		FromStatus:  beforeStatus,
		ToStatus:    req.Status(),
		ActorUserID: opts.actorID,
		ActorName:   actorName,
	}
	if insertErr := h.historyRepo.Insert(ctx, entry); insertErr != nil {
		log.Warn().Err(insertErr).Int64("request_id", req.RequestID()).
			Msg("TransitionHandler: history insert failed")
	}
}

func (h *TransitionHandler) emitAudit(ctx context.Context, req *domain.Request, beforeStatus string, opts applyOpts) {
	if h.emitter == nil || opts.operation == "" {
		return
	}
	entry := AuditEntry{
		EntityType: "cost_product_request",
		EntityID:   req.RequestID(),
		Operation:  opts.operation,
		BeforeData: marshalStatusSnapshot(beforeStatus),
		AfterData:  marshalStatusSnapshot(req.Status()),
		UserID:     opts.actorID,
	}
	// Best-effort: ignore the emit error here; the repo logs it. Business
	// operation already succeeded.
	if e := h.emitter.Emit(ctx, entry); e != nil {
		_ = e
	}
}

func marshalStatusSnapshot(status string) string {
	// Inline JSON to avoid pulling encoding/json import here for a 1-field object.
	if status == "" {
		return ""
	}
	return `{"status":"` + status + `"}`
}

// Audit operation tags — emitted via the AuditEmitter when one is configured.
const (
	auditOpStatusChange           = "STATUS_CHANGE"
	auditOpFeasibility            = "FEASIBILITY"
	auditOpClassificationOverride = "CLASSIFICATION_OVERRIDE"
	auditOpAssign                 = "ASSIGN"
)

// Notification trigger type constants — mirror costnotification domain constants
// without importing that package (layering constraint).
const (
	notifTriggerStatusChange    = "STATUS_CHANGE"
	notifTriggerAssigned        = "ASSIGNED"
	notifTriggerFeasibility     = "FEASIBILITY"
	notifTriggerRequestRejected = "REQUEST_REJECTED"
)

// emitNotification fires a best-effort in-app notification.
// Failures are logged as warnings and never propagate to the caller.
func (h *TransitionHandler) emitNotification(ctx context.Context, in NotificationInput) {
	if h.notifier == nil || in.RecipientUserID == "" {
		return
	}
	if e := h.notifier.Emit(ctx, in); e != nil {
		log.Warn().
			Err(e).
			Str("recipient", in.RecipientUserID).
			Str("trigger", in.TriggerType).
			Int64("request_id", in.RequestID).
			Msg("notification emit failed (non-fatal)")
	}
}

// emitCPREvent fires a CPR notification best-effort (logs on failure, never returns error).
func (h *TransitionHandler) emitCPREvent(ctx context.Context, event CPREvent) {
	if h.cprNotifier == nil {
		return
	}
	if err := h.cprNotifier.NotifyEvent(ctx, event); err != nil {
		log.Warn().Err(err).Str("event_type", event.EventType).
			Int64("request_id", event.RequestID).
			Msg("TransitionHandler: CPR notification failed (non-fatal)")
	}
}

// emitCPREvent fires a CPR notification best-effort (logs on failure, never returns error).
func (h *CreateHandler) emitCPREvent(ctx context.Context, event CPREvent) {
	if h.cprNotifier == nil {
		return
	}
	if err := h.cprNotifier.NotifyEvent(ctx, event); err != nil {
		log.Warn().Err(err).Str("event_type", event.EventType).
			Int64("request_id", event.RequestID).
			Msg("CreateHandler: CPR notification failed (non-fatal)")
	}
}

// Submit transitions DRAFT → SUBMITTED.
// After a successful transition, if a WorkflowClient is configured, it
// attempts to start an IAM approval instance. This is best-effort: a failure
// to reach IAM is logged as a warning but does not roll back the transition.
// If a NotificationEmitter is configured, a STATUS_CHANGE notification is sent
// to the assigned reviewer (if any); falls back to the requester if unassigned.
func (h *TransitionHandler) Submit(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.Submit() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	h.startWorkflowInstance(ctx, req, actor)
	// Notify reviewer (assigned user) that a new request is awaiting review.
	// Falls back to the requester themselves if no assignee is set yet.
	recipient := req.AssignedToUserID()
	if recipient == "" {
		recipient = req.RequesterUserID()
	}
	h.emitNotification(ctx, NotificationInput{
		RecipientUserID: recipient,
		TriggerType:     notifTriggerStatusChange,
		RequestID:       req.RequestID(),
		Payload:         `{"status":"SUBMITTED","request_no":"` + req.RequestNo() + `"}`,
	})
	// Rule-based notification: notify users with review permission that a
	// submitted request awaits their review.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_SUBMITTED_REVIEWER",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.product.request.review"},
		},
	})
	// Acknowledge to the creator that their request was submitted.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_SUBMITTED_ACK",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	return req, nil
}

// startWorkflowInstance fires the IAM workflow engine for the submitted request.
// It is always best-effort: errors are logged as warnings and never propagated.
// The instance ID is logged; persistence into cpr_wfl_instance_id will be wired
// in T18 once the full gRPC delivery layer and repository method are in place.
func (h *TransitionHandler) startWorkflowInstance(ctx context.Context, req *domain.Request, actor string) {
	if h.wflClient == nil {
		return
	}
	instanceID, wflErr := h.wflClient.StartInstance(
		ctx,
		"CPR_APPROVAL",
		"COST_PRODUCT_REQUEST",
		req.RequestNo(),
		actor,
	)
	if wflErr != nil {
		log.Warn().
			Err(wflErr).
			Int64("request_id", req.RequestID()).
			Str("request_no", req.RequestNo()).
			Msg("workflow start failed (non-fatal); CPR submit succeeded")
		return
	}
	if instanceID != "" {
		log.Info().
			Str("instance_id", instanceID).
			Int64("request_id", req.RequestID()).
			Str("request_no", req.RequestNo()).
			Msg("IAM workflow instance started for CPR submit")
	}
}

// StartReview transitions SUBMITTED → UNDER_REVIEW.
// Emits an ASSIGNED notification to the reviewer (actor) and a STATUS_CHANGE
// notification to the requester informing them the review has started.
func (h *TransitionHandler) StartReview(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.StartReview() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify the reviewer (actor) that they own this review.
	h.emitNotification(ctx, NotificationInput{
		RecipientUserID: actor,
		TriggerType:     notifTriggerAssigned,
		RequestID:       req.RequestID(),
		Payload:         `{"status":"UNDER_REVIEW","request_no":"` + req.RequestNo() + `"}`,
	})
	// Notify the requester that their submission is now under review.
	h.emitNotification(ctx, NotificationInput{
		RecipientUserID: req.RequesterUserID(),
		TriggerType:     notifTriggerStatusChange,
		RequestID:       req.RequestID(),
		Payload:         `{"status":"UNDER_REVIEW","request_no":"` + req.RequestNo() + `"}`,
	})
	// Rule-based notification: inform the requester that review has started.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_UNDER_REVIEW",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	return req, nil
}

// VerifyClassification sets verified_classification + (required) override_reason.
func (h *TransitionHandler) VerifyClassification(ctx context.Context, requestID int64, verified, reason, actor, actorName string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.VerifyClassification(verified, reason) }, applyOpts{operation: auditOpClassificationOverride, actorID: actor, actorName: actorName})
}

// DecideFeasibility advances UNDER_REVIEW → ROUTING_DEFINED or REJECTED.
// Emits a FEASIBILITY notification to the requester with the decision outcome,
// and a STATUS_CHANGE notification to the assigned engineer (if any) on FEASIBLE.
func (h *TransitionHandler) DecideFeasibility(ctx context.Context, requestID int64, decision, note, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.DecideFeasibility(decision, note, actor) }, applyOpts{operation: auditOpFeasibility, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify the requester of the feasibility outcome.
	h.emitNotification(ctx, NotificationInput{
		RecipientUserID: req.RequesterUserID(),
		TriggerType:     notifTriggerFeasibility,
		RequestID:       req.RequestID(),
		Payload:         `{"decision":"` + decision + `","status":"` + req.Status() + `","request_no":"` + req.RequestNo() + `"}`,
	})
	// On FEASIBLE, also notify the assigned engineer that routing can begin.
	if decision == domain.FeasibilityFeasible && req.AssignedToUserID() != "" {
		h.emitNotification(ctx, NotificationInput{
			RecipientUserID: req.AssignedToUserID(),
			TriggerType:     notifTriggerStatusChange,
			RequestID:       req.RequestID(),
			Payload:         `{"status":"ROUTING_DEFINED","request_no":"` + req.RequestNo() + `"}`,
		})
	}
	// Rule-based notification: inform the requester of the feasibility decision.
	eventType := "CPR_NOT_FEASIBLE"
	if decision == domain.FeasibilityFeasible {
		eventType = "CPR_FEASIBLE"
	}
	h.emitCPREvent(ctx, CPREvent{
		EventType:       eventType,
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	// On FEASIBLE, notify routing engineers that they can now define routing.
	if decision == domain.FeasibilityFeasible {
		h.emitCPREvent(ctx, CPREvent{
			EventType:       "CPR_ROUTING_NEEDED",
			RequestID:       req.RequestID(),
			RequestNo:       req.RequestNo(),
			RequesterUserID: req.RequesterUserID(),
			Rules: []CPRNotifRule{
				{RuleType: "BY_PERMISSION", Value: "finance.product.route.create"},
			},
		})
	}
	return req, nil
}

// MarkParameterPending advances ROUTING_DEFINED → PARAMETER_PENDING.
// Invoked automatically by the PromoteHandler after the first routing draft
// successfully promotes — see costroutingdraft/handlers.go PromoteHandler.
//
// When a FillTaskCreator is configured via WithFillCreator, fill tasks are created
// for every route level BEFORE the state transition is committed. If config is
// missing for any level, the method returns an error and the transition is aborted.
func (h *TransitionHandler) MarkParameterPending(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	if h.fillCreator != nil {
		if err := h.createFillTasksForRequest(ctx, requestID); err != nil {
			return nil, fmt.Errorf("create fill tasks: %w", err)
		}
	}
	return h.apply(ctx, requestID, func(r *domain.Request) error {
		return r.MarkParameterPending()
	}, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
}

// createFillTasksForRequest loads the request + linked route graph, extracts the
// distinct route levels, counts applicable params per level, and calls fillCreator.CreateForRequest.
func (h *TransitionHandler) createFillTasksForRequest(ctx context.Context, requestID int64) error {
	req, err := h.repo.GetByID(ctx, requestID)
	if err != nil {
		return fmt.Errorf("load request %d: %w", requestID, err)
	}
	routeHeadID := req.LinkedRouteHeadID()
	if routeHeadID == 0 {
		return fmt.Errorf("request %d has no linked route head", requestID)
	}
	if h.routeRepo == nil {
		return fmt.Errorf("route repository not configured on TransitionHandler")
	}
	graph, err := h.routeRepo.GetGraph(ctx, routeHeadID)
	if err != nil {
		return fmt.Errorf("load route graph %d: %w", routeHeadID, err)
	}
	levels := uniqueRouteLevels(graph)
	if len(levels) == 0 {
		return fmt.Errorf("route head %d has no sequences", routeHeadID)
	}
	perLevelTotals := h.countParamsPerLevel(ctx, graph)
	productSysID := graph.Head.ProductSysID
	if err := h.fillCreator.CreateForRequest(ctx, requestID, productSysID, routeHeadID, levels, perLevelTotals, req.RequestNo()); err != nil {
		if errors.Is(err, fillDomain.ErrConfigNotFound) {
			return fmt.Errorf("assignment config missing for one or more levels: %w", err)
		}
		return err
	}
	return nil
}

// countParamsPerLevel sums applicable params for every product in each route level.
// Returns an empty map (safe to use) when paramCounter is nil or counting fails.
func (h *TransitionHandler) countParamsPerLevel(ctx context.Context, graph *routeDomain.Graph) map[int32]int32 {
	result := make(map[int32]int32, 8) //nolint:gomnd // pre-size for typical route depth
	if h.paramCounter == nil || graph == nil {
		return result
	}
	levelProducts := make(map[int32][]int64, 8) //nolint:gomnd
	for _, seq := range graph.Seqs {
		levelProducts[seq.RouteLevel] = append(levelProducts[seq.RouteLevel], seq.ProductSysID)
	}
	for level, ids := range levelProducts {
		n, countErr := h.paramCounter.CountApplicableForProducts(ctx, ids)
		if countErr != nil {
			log.Warn().Err(countErr).Int32("level", level).
				Msg("countParamsPerLevel: failed to count applicable params (defaulting to 0)")
			continue
		}
		result[level] = n
	}
	return result
}

// uniqueRouteLevels extracts the distinct route level integers from a graph,
// deduplicated and in ascending order.
func uniqueRouteLevels(g *routeDomain.Graph) []int32 {
	if g == nil {
		return nil
	}
	seen := make(map[int32]struct{}, len(g.Seqs))
	for _, s := range g.Seqs {
		if s != nil {
			seen[s.RouteLevel] = struct{}{}
		}
	}
	levels := make([]int32, 0, len(seen))
	for lvl := range seen {
		levels = append(levels, lvl)
	}
	// Sort ascending so tasks are inserted in a consistent order.
	sortInt32Slice(levels)
	return levels
}

// sortInt32Slice sorts a []int32 in ascending order (insertion sort — short slices typical).
func sortInt32Slice(s []int32) {
	for i := 1; i < len(s); i++ {
		key := s[i]
		j := i - 1
		for j >= 0 && s[j] > key {
			s[j+1] = s[j]
			j--
		}
		s[j+1] = key
	}
}

// completionLevelMin is the lowest route level reserved for the automated
// completion chain (L100, L101, L102). Regular fill levels are below this.
const completionLevelMin = int32(100)

// MarkParameterComplete advances PARAMETER_PENDING → PARAMETER_COMPLETE.
// When a FillCompletionChecker is attached via WithFillChecker, all regular
// fill levels (below L100) must be APPROVED before the transition is allowed.
func (h *TransitionHandler) MarkParameterComplete(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	if h.fillChecker != nil {
		pending, checkErr := h.fillChecker.CountNonApprovedBelow(ctx, requestID, completionLevelMin)
		if checkErr != nil {
			return nil, fmt.Errorf("checking fill completion: %w", checkErr)
		}
		if pending > 0 {
			return nil, fmt.Errorf("%w: %d fill level(s) not yet approved", domain.ErrInvalidTransition, pending)
		}
	}
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error {
		return r.MarkParameterComplete()
	}, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify the requester that parameter filling is complete.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_PARAM_COMPLETE_REQUESTER",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	// Notify users with confirm permission that the request awaits confirmation.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_PARAM_COMPLETE_CONFIRM",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.product.request.confirm"},
		},
	})
	return req, nil
}

// Confirm advances PARAMETER_COMPLETE → CONFIRMED.
func (h *TransitionHandler) Confirm(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.Confirm() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify requester that the request has been confirmed.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_CONFIRMED_REQUESTER",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	// Notify users with approve permission that approval is required.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_CONFIRMED_APPROVE",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.product.request.approve"},
		},
	})
	return req, nil
}

// Approve advances CONFIRMED → APPROVED.
func (h *TransitionHandler) Approve(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.Approve() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify requester that the request has been approved.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_APPROVED_REQUESTER",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	// Notify users with release permission that release is required.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_APPROVED_RELEASE",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.product.request.release"},
		},
	})
	return req, nil
}

// Release advances APPROVED → RELEASED. After release the request is locked
// and the cost calculation engine can proceed.
func (h *TransitionHandler) Release(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.Release() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify requester that the request has been released.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_RELEASED_REQUESTER",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	// Notify costing team that the request is released and ready for calculation.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_RELEASED_CALC",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_PERMISSION", Value: "finance.cost.caljob.trigger"},
		},
	})
	return req, nil
}

// UseExistingCosting jumps UNDER_REVIEW → QUOTE_READY, recording the reused
// product master on the request so QUOTE_READY traces back to a concrete product.
func (h *TransitionHandler) UseExistingCosting(ctx context.Context, requestID int64, existingProductSysID int64, actor, actorName string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error {
		return r.UseExistingCosting(existingProductSysID)
	}, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
}

// Reject sends to REJECTED with a reason.
// Emits a REQUEST_REJECTED notification to the requester.
func (h *TransitionHandler) Reject(ctx context.Context, requestID int64, reason, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.Reject(reason) }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	h.emitNotification(ctx, NotificationInput{
		RecipientUserID: req.RequesterUserID(),
		TriggerType:     notifTriggerRequestRejected,
		RequestID:       req.RequestID(),
		Payload:         `{"status":"REJECTED","request_no":"` + req.RequestNo() + `"}`,
	})
	// Rule-based notification: inform the requester their request was rejected.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_REJECTED",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	return req, nil
}

// Revise re-submits a REJECTED request.
func (h *TransitionHandler) Revise(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Revise() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
}

// Reopen moves a CLOSED request back to DRAFT.
func (h *TransitionHandler) Reopen(ctx context.Context, requestID int64, actor, actorName string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Reopen() }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
}

// Cancel closes a non-CLOSED request with closed_substatus = cancelled.
func (h *TransitionHandler) Cancel(ctx context.Context, requestID int64, reason, actor, actorName string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Cancel(reason) }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
}

// Close sets closed_substatus.
func (h *TransitionHandler) Close(ctx context.Context, requestID int64, substatus, actor, actorName string) (*domain.Request, error) {
	req, err := h.apply(ctx, requestID, func(r *domain.Request) error { return r.Close(substatus) }, applyOpts{operation: auditOpStatusChange, actorID: actor, actorName: actorName})
	if err != nil {
		return nil, err
	}
	// Notify the requester that the request has been closed.
	h.emitCPREvent(ctx, CPREvent{
		EventType:       "CPR_CLOSED",
		RequestID:       req.RequestID(),
		RequestNo:       req.RequestNo(),
		RequesterUserID: req.RequesterUserID(),
		Rules: []CPRNotifRule{
			{RuleType: "BY_USER_ID", Value: req.RequesterUserID()},
		},
	})
	return req, nil
}

// Assign updates assignee_user_id.
func (h *TransitionHandler) Assign(ctx context.Context, requestID int64, assignee, actor, actorName string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Assign(assignee) }, applyOpts{operation: auditOpAssign, actorID: actor, actorName: actorName})
}
