// Package costproductrequest holds application use cases for the Phase A request aggregate.
package costproductrequest

import (
	"context"
	"errors"
	"fmt"

	fillDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costfillassignment"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	routeDomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
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
type CreateHandler struct{ repo domain.Repository }

// NewCreateHandler constructs a CreateHandler.
func NewCreateHandler(r domain.Repository) *CreateHandler { return &CreateHandler{repo: r} }

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
	CreateForRequest(ctx context.Context, requestID, productSysID, routeHeadID int64, routeLevels []int32, totalParams int32) error
}

// TransitionHandler wraps a single state transition: load → mutate → save.
// Optionally emits a CAL_ audit row after a successful save. Audit failures are
// LOGGED (caller decides) but never block the business operation.
type TransitionHandler struct {
	repo        domain.Repository
	emitter     AuditEmitter
	routeRepo   routeDomain.Repository // optional; used by MarkParameterPending
	fillCreator FillTaskCreator        // optional; if nil, fill task creation is skipped
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

// applyOpts carries the per-call audit metadata.
type applyOpts struct {
	operation string // CAL operation tag (e.g. STATUS_CHANGE, FEASIBILITY)
	actorID   string
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
	return req, nil
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

// Submit transitions DRAFT → SUBMITTED.
func (h *TransitionHandler) Submit(ctx context.Context, requestID int64, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Submit() }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// StartReview transitions SUBMITTED → UNDER_REVIEW.
func (h *TransitionHandler) StartReview(ctx context.Context, requestID int64, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.StartReview() }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// VerifyClassification sets verified_classification + (required) override_reason.
func (h *TransitionHandler) VerifyClassification(ctx context.Context, requestID int64, verified, reason, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.VerifyClassification(verified, reason) }, applyOpts{operation: auditOpClassificationOverride, actorID: actor})
}

// DecideFeasibility advances UNDER_REVIEW → ROUTING_DEFINED or REJECTED.
func (h *TransitionHandler) DecideFeasibility(ctx context.Context, requestID int64, decision, note, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.DecideFeasibility(decision, note, actor) }, applyOpts{operation: auditOpFeasibility, actorID: actor})
}

// MarkParameterPending advances ROUTING_DEFINED → PARAMETER_PENDING.
// Invoked automatically by the PromoteHandler after the first routing draft
// successfully promotes — see costroutingdraft/handlers.go PromoteHandler.
//
// When a FillTaskCreator is configured via WithFillCreator, fill tasks are created
// for every route level BEFORE the state transition is committed. If config is
// missing for any level, the method returns an error and the transition is aborted.
func (h *TransitionHandler) MarkParameterPending(ctx context.Context, requestID int64, actor string) (*domain.Request, error) {
	if h.fillCreator != nil {
		if err := h.createFillTasksForRequest(ctx, requestID); err != nil {
			return nil, fmt.Errorf("create fill tasks: %w", err)
		}
	}
	return h.apply(ctx, requestID, func(r *domain.Request) error {
		return r.MarkParameterPending()
	}, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// createFillTasksForRequest loads the request + linked route graph, extracts the
// distinct route levels, and calls fillCreator.CreateForRequest.
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
	productSysID := graph.Head.ProductSysID
	if err := h.fillCreator.CreateForRequest(ctx, requestID, productSysID, routeHeadID, levels, 0); err != nil {
		if errors.Is(err, fillDomain.ErrConfigNotFound) {
			return fmt.Errorf("assignment config missing for one or more levels: %w", err)
		}
		return err
	}
	return nil
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

// MarkParameterComplete advances PARAMETER_PENDING → PARAMETER_COMPLETE. The
// gRPC layer is responsible for asserting no required params are missing via
// cost_product_parameter.CheckMissingRequiredParams before calling this.
func (h *TransitionHandler) MarkParameterComplete(ctx context.Context, requestID int64, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error {
		return r.MarkParameterComplete()
	}, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// UseExistingCosting jumps UNDER_REVIEW → QUOTE_READY, recording the reused
// product master on the request so QUOTE_READY traces back to a concrete product.
func (h *TransitionHandler) UseExistingCosting(ctx context.Context, requestID int64, existingProductSysID int64, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error {
		return r.UseExistingCosting(existingProductSysID)
	}, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// Reject sends to REJECTED with a reason.
func (h *TransitionHandler) Reject(ctx context.Context, requestID int64, reason, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Reject(reason) }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// Revise re-submits a REJECTED request.
func (h *TransitionHandler) Revise(ctx context.Context, requestID int64, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Revise() }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// Reopen moves a CLOSED request back to DRAFT.
func (h *TransitionHandler) Reopen(ctx context.Context, requestID int64, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Reopen() }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// Cancel closes a non-CLOSED request with closed_substatus = cancelled.
func (h *TransitionHandler) Cancel(ctx context.Context, requestID int64, reason, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Cancel(reason) }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// Close sets closed_substatus.
func (h *TransitionHandler) Close(ctx context.Context, requestID int64, substatus, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Close(substatus) }, applyOpts{operation: auditOpStatusChange, actorID: actor})
}

// Assign updates assignee_user_id.
func (h *TransitionHandler) Assign(ctx context.Context, requestID int64, assignee, actor string) (*domain.Request, error) {
	return h.apply(ctx, requestID, func(r *domain.Request) error { return r.Assign(assignee) }, applyOpts{operation: auditOpAssign, actorID: actor})
}
