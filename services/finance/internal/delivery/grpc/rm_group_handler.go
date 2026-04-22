// Package grpc provides gRPC server implementation for finance service.
package grpc

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	apprmcost "github.com/mutugading/goapps-backend/services/finance/internal/application/rmcost"
	appgroup "github.com/mutugading/goapps-backend/services/finance/internal/application/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
	rmgroupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// Metric operation constants (shared across rm_group_handler + rm_cost_handler).
const (
	opCreate           = "create"
	opGet              = "get"
	opUpdate           = "update"
	opDelete           = "delete"
	opList             = "list"
	opAddItems         = "add_items"
	opRemoveItems      = "remove_items"
	opListUngrouped    = "list_ungrouped"
	opGetItemRates     = "get_item_rates"
	opTrigger          = "trigger"
	opCalculate        = "calculate"
	opListHistory      = "list_history"
	opExport           = "export"
	opImport           = "import"
	opTemplate         = "template"
	msgRMGroupSuccess  = "RM Group retrieved successfully"
	msgRMCostRetrieved = "RM Cost retrieved successfully"
)

// ItemMetadataLookup provides item metadata from sync data for enriching
// newly created group details. Without this, detail rows would have empty
// name/grade/UOM columns. GetItemByCodeGrade is preferred — it can pick the
// exact (item_code, grade_code) variant the user selected instead of an
// arbitrary one; GetItemByCode retained for callers that don't have a grade.
type ItemMetadataLookup interface {
	GetItemByCode(ctx context.Context, itemCode string) (*syncdata.ItemConsStockPO, error)
	GetItemByCodeGrade(ctx context.Context, itemCode, gradeCode string) (*syncdata.ItemConsStockPO, error)
}

// buildAddItemInputs translates an AddItemsRequest into the application-layer
// AddItemInput slice, preferring the structured `selections` field over the
// legacy `item_codes`. For each item it tries to enrich with sync-feed
// metadata keyed on (item_code, grade_code) so multi-variant items pick the
// exact row the operator saw in the picker.
func (h *RMGroupHandler) buildAddItemInputs(ctx context.Context, req *financev1.AddItemsRequest) []appgroup.AddItemInput {
	if len(req.Selections) > 0 {
		out := make([]appgroup.AddItemInput, len(req.Selections))
		for i, sel := range req.Selections {
			out[i] = h.enrichItemInput(ctx, sel.ItemCode, sel.GradeCode)
		}
		return out
	}
	out := make([]appgroup.AddItemInput, len(req.ItemCodes))
	for i, code := range req.ItemCodes {
		out[i] = h.enrichItemInput(ctx, code, "")
	}
	return out
}

func (h *RMGroupHandler) enrichItemInput(ctx context.Context, itemCode, gradeCode string) appgroup.AddItemInput {
	in := appgroup.AddItemInput{ItemCode: itemCode, GradeCode: gradeCode}
	if h.itemLookup == nil {
		return in
	}
	syncItem, err := h.itemLookup.GetItemByCodeGrade(ctx, itemCode, gradeCode)
	if err != nil || syncItem == nil {
		return in
	}
	in.ItemName = syncItem.ItemName
	if in.GradeCode == "" {
		in.GradeCode = syncItem.GradeCode
	}
	in.ItemGrade = syncItem.GradeName
	in.UOMCode = syncItem.UOM
	return in
}

// RMGroupHandler implements the RMGroupServiceServer interface.
type RMGroupHandler struct {
	financev1.UnimplementedRMGroupServiceServer
	createHandler      *appgroup.CreateHandler
	getHandler         *appgroup.GetHandler
	updateHandler      *appgroup.UpdateHandler
	deleteHandler      *appgroup.DeleteHandler
	listHandler        *appgroup.ListHandler
	addItemsHandler    *appgroup.AddItemsHandler
	removeItemsHandler *appgroup.RemoveItemsHandler
	ungroupedHandler   *appgroup.UngroupedHandler
	ungroupedExport    *appgroup.UngroupedExportHandler
	itemRatesHandler   *appgroup.GroupItemRatesHandler
	exportHandler      *appgroup.ExportHandler
	importHandler      *appgroup.ImportHandler
	importItemsHandler *appgroup.ImportGroupItemsHandler
	templateHandler    *appgroup.TemplateHandler
	itemsTemplate      *appgroup.GroupItemsTemplateHandler
	itemLookup         ItemMetadataLookup
	recalc             *RecalcChain
	validationHelper   *ValidationHelper
}

// PeriodLister returns the set of known periods, newest first. Used by
// RecalcChain to pick the period to recalculate when the caller did not
// specify one.
type PeriodLister func(ctx context.Context) ([]string, error)

// RecalcChain encapsulates the fire-and-forget enqueue of an RM cost job
// triggered from group CRUD operations (create/update head, add/remove items).
// Failures are logged and swallowed so the CRUD response never fails on them.
type RecalcChain struct {
	jobRepo     job.Repository
	publisher   apprmcost.JobPublisher
	costPeriods PeriodLister
	syncPeriods PeriodLister
}

// NewRecalcChain builds a RecalcChain. Any of the deps may be nil (all-nil
// makes Publish a no-op — useful when RabbitMQ is unavailable).
func NewRecalcChain(jobRepo job.Repository, publisher apprmcost.JobPublisher, costPeriods, syncPeriods PeriodLister) *RecalcChain {
	return &RecalcChain{jobRepo: jobRepo, publisher: publisher, costPeriods: costPeriods, syncPeriods: syncPeriods}
}

// Publish enqueues a single-group recalculation for groupHeadID with the given
// reason. Returns nil on success or if chain is disabled; logs and returns nil
// on any step failure to keep CRUD responses unaffected.
func (c *RecalcChain) Publish(ctx context.Context, groupHeadID uuid.UUID, reason, createdBy string) {
	if c == nil || c.publisher == nil || c.jobRepo == nil {
		return
	}
	period, err := c.resolvePeriod(ctx)
	if err != nil {
		log.Warn().Err(err).Msg("recalc chain: resolve period failed")
		return
	}
	if period == "" {
		// Nothing to recalculate against yet — no cost rows and no synced data.
		return
	}

	// Skip if another active job for (type, period) already exists.
	active, err := c.jobRepo.HasActiveJob(ctx, job.TypeRMCostCalculation, period)
	if err != nil {
		log.Warn().Err(err).Msg("recalc chain: check active job failed")
		return
	}
	if active {
		log.Debug().Str("period", period).Msg("recalc chain: active job already queued, skipping")
		return
	}

	exec, err := job.NewExecution(job.TypeRMCostCalculation, groupHeadID.String(), period, createdBy, 5, nil)
	if err != nil {
		log.Warn().Err(err).Msg("recalc chain: new execution failed")
		return
	}
	if err := c.jobRepo.Create(ctx, exec); err != nil {
		log.Warn().Err(err).Msg("recalc chain: persist execution failed")
		return
	}
	gid := groupHeadID
	if err := c.publisher.PublishRMCostCalculation(ctx, exec.ID().String(), period, &gid, reason, createdBy); err != nil {
		if failErr := exec.Fail("publish failed: " + err.Error()); failErr == nil {
			if updErr := c.jobRepo.UpdateStatus(ctx, exec); updErr != nil {
				log.Warn().Err(updErr).Msg("recalc chain: mark failed status failed")
			}
		}
		log.Warn().Err(err).Msg("recalc chain: publish failed (operator can recalc manually)")
		return
	}
	log.Info().Str("job_id", exec.ID().String()).Str("period", period).
		Str("group_head_id", groupHeadID.String()).Str("reason", reason).
		Msg("recalc chain: job enqueued")
}

func (c *RecalcChain) resolvePeriod(ctx context.Context) (string, error) {
	if c.costPeriods != nil {
		periods, err := c.costPeriods(ctx)
		if err != nil {
			return "", err
		}
		if len(periods) > 0 {
			return periods[0], nil
		}
	}
	if c.syncPeriods != nil {
		periods, err := c.syncPeriods(ctx)
		if err != nil {
			return "", err
		}
		if len(periods) > 0 {
			return periods[0], nil
		}
	}
	return "", nil
}

// NewRMGroupHandler builds an RMGroupHandler.
func NewRMGroupHandler(
	repo rmgroupdomain.Repository,
	ungroupedReader appgroup.UngroupedItemsReader,
	itemRatesReader appgroup.GroupItemRatesReader,
	itemLookup ItemMetadataLookup,
	costChecker appgroup.CostChecker,
	importLookup appgroup.ImportItemLookup,
	recalc *RecalcChain,
) (*RMGroupHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &RMGroupHandler{
		createHandler:      appgroup.NewCreateHandler(repo),
		getHandler:         appgroup.NewGetHandler(repo),
		updateHandler:      appgroup.NewUpdateHandler(repo),
		deleteHandler:      appgroup.NewDeleteHandler(repo, costChecker),
		listHandler:        appgroup.NewListHandler(repo),
		addItemsHandler:    appgroup.NewAddItemsHandler(repo),
		removeItemsHandler: appgroup.NewRemoveItemsHandler(repo),
		ungroupedHandler:   appgroup.NewUngroupedHandler(ungroupedReader),
		ungroupedExport:    appgroup.NewUngroupedExportHandler(ungroupedReader),
		itemRatesHandler:   appgroup.NewGroupItemRatesHandler(itemRatesReader),
		exportHandler:      appgroup.NewExportHandler(repo),
		importHandler:      appgroup.NewImportHandler(repo, importLookup),
		importItemsHandler: appgroup.NewImportGroupItemsHandler(appgroup.NewAddItemsHandler(repo), importLookup),
		templateHandler:    appgroup.NewTemplateHandler(),
		itemsTemplate:      appgroup.NewGroupItemsTemplateHandler(),
		itemLookup:         itemLookup,
		recalc:             recalc,
		validationHelper:   v,
	}, nil
}

// CreateRMGroup creates a new RM group head.
func (h *RMGroupHandler) CreateRMGroup(ctx context.Context, req *financev1.CreateRMGroupRequest) (*financev1.CreateRMGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opCreate, false)
		return &financev1.CreateRMGroupResponse{Base: baseResp}, nil
	}

	head, err := h.createHandler.Handle(ctx, appgroup.CreateCommand{
		Code:           req.GroupCode,
		Name:           req.GroupName,
		Description:    req.Description,
		Colorant:       req.Colourant,
		CIName:         req.CiName,
		CostPercentage: req.CostPercentage,
		CostPerKg:      req.CostPerKg,
		CreatedBy:      getUserFromContext(ctx),
	})
	if err != nil {
		RecordRMGroupOperation(opCreate, false)
		return &financev1.CreateRMGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opCreate, true)
	h.recalc.Publish(ctx, head.ID(), string(apprmcost.TriggerGroupUpdate), getUserFromContext(ctx))
	return &financev1.CreateRMGroupResponse{
		Base: successResponse("RM Group created successfully"),
		Data: rmGroupHeadToProto(head),
	}, nil
}

// GetRMGroup retrieves a group head with its details.
func (h *RMGroupHandler) GetRMGroup(ctx context.Context, req *financev1.GetRMGroupRequest) (*financev1.GetRMGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opGet, false)
		return &financev1.GetRMGroupResponse{Base: baseResp}, nil
	}

	result, err := h.getHandler.Handle(ctx, appgroup.GetQuery{
		HeadID:      req.GroupHeadId,
		WithDetails: true,
		ActiveOnly:  false,
	})
	if err != nil {
		RecordRMGroupOperation(opGet, false)
		return &financev1.GetRMGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	details := make([]*financev1.RMGroupDetail, len(result.Details))
	for i, d := range result.Details {
		details[i] = rmGroupDetailToProto(d)
	}

	RecordRMGroupOperation(opGet, true)
	return &financev1.GetRMGroupResponse{
		Base: successResponse(msgRMGroupSuccess),
		Data: &financev1.RMGroupHeadWithDetails{
			Head:    rmGroupHeadToProto(result.Head),
			Details: details,
		},
	}, nil
}

// UpdateRMGroup applies a partial update to a head.
func (h *RMGroupHandler) UpdateRMGroup(ctx context.Context, req *financev1.UpdateRMGroupRequest) (*financev1.UpdateRMGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opUpdate, false)
		return &financev1.UpdateRMGroupResponse{Base: baseResp}, nil
	}

	cmd := appgroup.UpdateCommand{
		HeadID:                 req.GroupHeadId,
		Name:                   req.GroupName,
		Description:            req.Description,
		Colorant:               req.Colourant,
		CIName:                 req.CiName,
		CostPercentage:         req.CostPercentage,
		CostPerKg:              req.CostPerKg,
		InitValValuation:       req.InitValValuation,
		InitValMarketing:       req.InitValMarketing,
		InitValSimulation:      req.InitValSimulation,
		ClearInitValValuation:  req.ClearInitValValuation,
		ClearInitValMarketing:  req.ClearInitValMarketing,
		ClearInitValSimulation: req.ClearInitValSimulation,
		IsActive:               req.IsActive,
		UpdatedBy:              getUserFromContext(ctx),
	}
	if req.FlagValuation != nil {
		s := protoFlagToString(*req.FlagValuation)
		cmd.FlagValuation = &s
	}
	if req.FlagMarketing != nil {
		s := protoFlagToString(*req.FlagMarketing)
		cmd.FlagMarketing = &s
	}
	if req.FlagSimulation != nil {
		s := protoFlagToString(*req.FlagSimulation)
		cmd.FlagSimulation = &s
	}

	head, err := h.updateHandler.Handle(ctx, cmd)
	if err != nil {
		RecordRMGroupOperation(opUpdate, false)
		return &financev1.UpdateRMGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opUpdate, true)
	h.recalc.Publish(ctx, head.ID(), string(apprmcost.TriggerGroupUpdate), getUserFromContext(ctx))
	return &financev1.UpdateRMGroupResponse{
		Base: successResponse("RM Group updated successfully"),
		Data: rmGroupHeadToProto(head),
	}, nil
}

// DeleteRMGroup soft-deletes a group head (cascade to details).
func (h *RMGroupHandler) DeleteRMGroup(ctx context.Context, req *financev1.DeleteRMGroupRequest) (*financev1.DeleteRMGroupResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opDelete, false)
		return &financev1.DeleteRMGroupResponse{Base: baseResp}, nil
	}

	if err := h.deleteHandler.Handle(ctx, appgroup.DeleteCommand{
		HeadID:    req.GroupHeadId,
		DeletedBy: getUserFromContext(ctx),
	}); err != nil {
		RecordRMGroupOperation(opDelete, false)
		return &financev1.DeleteRMGroupResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opDelete, true)
	return &financev1.DeleteRMGroupResponse{
		Base: successResponse("RM Group deleted successfully"),
	}, nil
}

// ListRMGroups returns a paginated list of group heads.
func (h *RMGroupHandler) ListRMGroups(ctx context.Context, req *financev1.ListRMGroupsRequest) (*financev1.ListRMGroupsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opList, false)
		return &financev1.ListRMGroupsResponse{Base: baseResp}, nil
	}

	query := appgroup.ListQuery{
		Page:      int(req.Page),
		PageSize:  int(req.PageSize),
		Search:    req.Search,
		SortBy:    req.SortBy,
		SortOrder: req.SortOrder,
	}
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		v := true
		query.IsActive = &v
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		v := false
		query.IsActive = &v
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
	}

	result, err := h.listHandler.Handle(ctx, query)
	if err != nil {
		RecordRMGroupOperation(opList, false)
		return &financev1.ListRMGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.RMGroupHead, len(result.Heads))
	for i, head := range result.Heads {
		items[i] = rmGroupHeadToProto(head)
	}

	RecordRMGroupOperation(opList, true)
	return &financev1.ListRMGroupsResponse{
		Base: successResponse("RM Groups retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// AddItems assigns items to a group head.
// It enriches each item with metadata (name, grade, UOM) from sync data
// so the detail rows are human-readable without a separate JOIN.
func (h *RMGroupHandler) AddItems(ctx context.Context, req *financev1.AddItemsRequest) (*financev1.AddItemsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opAddItems, false)
		return &financev1.AddItemsResponse{Base: baseResp}, nil
	}

	// Prefer the structured `selections` field (carries grade_code) over the
	// legacy `item_codes`. When both are supplied, selections wins so new
	// frontends can migrate without breaking older callers.
	items := h.buildAddItemInputs(ctx, req)

	result, err := h.addItemsHandler.Handle(ctx, appgroup.AddItemsCommand{
		HeadID:    req.GroupHeadId,
		CreatedBy: getUserFromContext(ctx),
		Items:     items,
	})
	if err != nil {
		RecordRMGroupOperation(opAddItems, false)
		return &financev1.AddItemsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	added := make([]*financev1.RMGroupDetail, len(result.Added))
	for i, d := range result.Added {
		added[i] = rmGroupDetailToProto(d)
	}
	skipped := make([]*financev1.SkippedItem, len(result.Skipped))
	for i, s := range result.Skipped {
		skipped[i] = skippedItemToProto(s)
	}

	RecordRMGroupOperation(opAddItems, true)
	if len(result.Added) > 0 {
		if headID, parseErr := uuid.Parse(req.GroupHeadId); parseErr == nil {
			h.recalc.Publish(ctx, headID, string(apprmcost.TriggerDetailChange), getUserFromContext(ctx))
		}
	}
	return &financev1.AddItemsResponse{
		Base:    successResponse("Items processed"),
		Added:   added,
		Skipped: skipped,
	}, nil
}

// RemoveItems removes details from a group head.
func (h *RMGroupHandler) RemoveItems(ctx context.Context, req *financev1.RemoveItemsRequest) (*financev1.RemoveItemsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opRemoveItems, false)
		return &financev1.RemoveItemsResponse{Base: baseResp}, nil
	}

	result, err := h.removeItemsHandler.Handle(ctx, appgroup.RemoveItemsCommand{
		HeadID:    req.GroupHeadId,
		DetailIDs: req.GroupDetailIds,
		Mode:      removeModeFromProto(req.Mode),
		RemovedBy: getUserFromContext(ctx),
	})
	if err != nil {
		RecordRMGroupOperation(opRemoveItems, false)
		return &financev1.RemoveItemsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opRemoveItems, true)
	if len(result.Removed) > 0 {
		if headID, parseErr := uuid.Parse(req.GroupHeadId); parseErr == nil {
			h.recalc.Publish(ctx, headID, string(apprmcost.TriggerDetailChange), getUserFromContext(ctx))
		}
	}
	return &financev1.RemoveItemsResponse{
		Base:         successResponse("Items removed"),
		RemovedCount: int32(len(result.Removed)), //nolint:gosec // slice length capped by request validation
	}, nil
}

// ListUngroupedItems returns items from the sync feed with no active group.
func (h *RMGroupHandler) ListUngroupedItems(ctx context.Context, req *financev1.ListUngroupedItemsRequest) (*financev1.ListUngroupedItemsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opListUngrouped, false)
		return &financev1.ListUngroupedItemsResponse{Base: baseResp}, nil
	}

	result, err := h.ungroupedHandler.Handle(ctx, appgroup.UngroupedQuery{
		Page:     int(req.Page),
		PageSize: int(req.PageSize),
		Period:   req.Period,
		Search:   req.Search,
	})
	if err != nil {
		RecordRMGroupOperation(opListUngrouped, false)
		return &financev1.ListUngroupedItemsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	items := make([]*financev1.UngroupedItem, len(result.Items))
	for i, it := range result.Items {
		items[i] = ungroupedItemToProto(it)
	}

	RecordRMGroupOperation(opListUngrouped, true)
	return &financev1.ListUngroupedItemsResponse{
		Base: successResponse("Ungrouped items retrieved successfully"),
		Data: items,
		Pagination: &commonv1.PaginationResponse{
			CurrentPage: result.CurrentPage,
			PageSize:    result.PageSize,
			TotalItems:  result.TotalItems,
			TotalPages:  result.TotalPages,
		},
	}, nil
}

// GetRMGroupItemRates returns per-item per-stage rates for a group + period.
func (h *RMGroupHandler) GetRMGroupItemRates(ctx context.Context, req *financev1.GetRMGroupItemRatesRequest) (*financev1.GetRMGroupItemRatesResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opGetItemRates, false)
		return &financev1.GetRMGroupItemRatesResponse{Base: baseResp}, nil
	}

	rows, err := h.itemRatesHandler.Handle(ctx, appgroup.GroupItemRatesQuery{
		HeadID: req.GroupHeadId,
		Period: req.Period,
	})
	if err != nil {
		RecordRMGroupOperation(opGetItemRates, false)
		return &financev1.GetRMGroupItemRatesResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	out := make([]*financev1.RMGroupItemRates, len(rows))
	for i, r := range rows {
		out[i] = groupItemRatesToProto(r)
	}

	RecordRMGroupOperation(opGetItemRates, true)
	return &financev1.GetRMGroupItemRatesResponse{
		Base: successResponse("Group item rates retrieved successfully"),
		Data: out,
	}, nil
}

func groupItemRatesToProto(r *appgroup.GroupItemRates) *financev1.RMGroupItemRates {
	return &financev1.RMGroupItemRates{
		ItemCode:    r.ItemCode,
		ItemName:    r.ItemName,
		GradeCode:   r.GradeCode,
		ItemGrade:   r.ItemGrade,
		UomCode:     r.UOMCode,
		IsActive:    r.IsActive,
		IsDummy:     r.IsDummy,
		Period:      r.Period,
		ConsQty:     r.ConsQty,
		ConsVal:     r.ConsVal,
		ConsRate:    r.ConsRate,
		StoresQty:   r.StoresQty,
		StoresVal:   r.StoresVal,
		StoresRate:  r.StoresRate,
		DeptQty:     r.DeptQty,
		DeptVal:     r.DeptVal,
		DeptRate:    r.DeptRate,
		LastPoQty1:  r.LastPOQty1,
		LastPoVal1:  r.LastPOVal1,
		LastPoRate1: r.LastPORate1,
		LastPoQty2:  r.LastPOQty2,
		LastPoVal2:  r.LastPOVal2,
		LastPoRate2: r.LastPORate2,
		LastPoQty3:  r.LastPOQty3,
		LastPoVal3:  r.LastPOVal3,
		LastPoRate3: r.LastPORate3,
	}
}

// =============================================================================
// Entity → proto mappers
// =============================================================================

func rmGroupHeadToProto(h *rmgroupdomain.Head) *financev1.RMGroupHead {
	out := &financev1.RMGroupHead{
		GroupHeadId:       h.ID().String(),
		GroupCode:         h.Code().String(),
		GroupName:         h.Name(),
		Description:       h.Description(),
		Colourant:         h.Colorant(),
		CiName:            h.CIName(),
		CostPercentage:    h.CostPercentage(),
		CostPerKg:         h.CostPerKg(),
		FlagValuation:     flagToProto(h.FlagValuation()),
		FlagMarketing:     flagToProto(h.FlagMarketing()),
		FlagSimulation:    flagToProto(h.FlagSimulation()),
		InitValValuation:  h.InitValValuation(),
		InitValMarketing:  h.InitValMarketing(),
		InitValSimulation: h.InitValSimulation(),
		IsActive:          h.IsActive(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: h.CreatedAt().Format(time.RFC3339),
			CreatedBy: h.CreatedBy(),
		},
	}
	if h.UpdatedAt() != nil {
		out.Audit.UpdatedAt = h.UpdatedAt().Format(time.RFC3339)
	}
	if h.UpdatedBy() != nil {
		out.Audit.UpdatedBy = *h.UpdatedBy()
	}
	return out
}

func rmGroupDetailToProto(d *rmgroupdomain.Detail) *financev1.RMGroupDetail {
	out := &financev1.RMGroupDetail{
		GroupDetailId:    d.ID().String(),
		GroupHeadId:      d.HeadID().String(),
		ItemCode:         d.ItemCode().String(),
		ItemName:         d.ItemName(),
		ItemTypeCode:     d.ItemTypeCode(),
		GradeCode:        d.GradeCode(),
		ItemGrade:        d.ItemGrade(),
		UomCode:          d.UOMCode(),
		MarketPercentage: d.MarketPercentage(),
		MarketValueRp:    d.MarketValueRp(),
		SortOrder:        d.SortOrder(),
		IsActive:         d.IsActive(),
		IsDummy:          d.IsDummy(),
		Audit: &commonv1.AuditInfo{
			CreatedAt: d.CreatedAt().Format(time.RFC3339),
			CreatedBy: d.CreatedBy(),
		},
	}
	if d.UpdatedAt() != nil {
		out.Audit.UpdatedAt = d.UpdatedAt().Format(time.RFC3339)
	}
	if d.UpdatedBy() != nil {
		out.Audit.UpdatedBy = *d.UpdatedBy()
	}
	return out
}

func ungroupedItemToProto(it *syncdata.ItemConsStockPO) *financev1.UngroupedItem {
	out := &financev1.UngroupedItem{
		Period:    it.Period,
		ItemCode:  it.ItemCode,
		ItemName:  it.ItemName,
		GradeCode: it.GradeCode,
		ItemGrade: it.GradeName,
		UomCode:   it.UOM,
	}
	assignF64(&out.ConsQty, it.ConsQty)
	assignF64(&out.ConsVal, it.ConsVal)
	assignF64(&out.ConsRate, it.ConsRate)
	assignF64(&out.StoresQty, it.StoresQty)
	assignF64(&out.StoresVal, it.StoresVal)
	assignF64(&out.StoresRate, it.StoresRate)
	assignF64(&out.DeptQty, it.DeptQty)
	assignF64(&out.DeptVal, it.DeptVal)
	assignF64(&out.DeptRate, it.DeptRate)
	assignF64(&out.LastPoQty1, it.LastPOQty1)
	assignF64(&out.LastPoVal1, it.LastPOVal1)
	assignF64(&out.LastPoRate1, it.LastPORate1)
	assignF64(&out.LastPoQty2, it.LastPOQty2)
	assignF64(&out.LastPoVal2, it.LastPOVal2)
	assignF64(&out.LastPoRate2, it.LastPORate2)
	assignF64(&out.LastPoQty3, it.LastPOQty3)
	assignF64(&out.LastPoVal3, it.LastPOVal3)
	assignF64(&out.LastPoRate3, it.LastPORate3)
	return out
}

func assignF64(dst *float64, src *float64) {
	if src != nil {
		*dst = *src
	}
}

func skippedItemToProto(s appgroup.SkippedItem) *financev1.SkippedItem {
	out := &financev1.SkippedItem{
		ItemCode: s.ItemCode,
	}
	if s.OwningGroupID != nil {
		out.OwningGroupHeadId = s.OwningGroupID.String()
	}
	if s.OwningDetailID != nil {
		out.OwningGroupDetailId = s.OwningDetailID.String()
	}
	return out
}

// flagToProto maps a domain Flag to its proto counterpart.
func flagToProto(f rmgroupdomain.Flag) financev1.RMGroupFlag {
	switch f {
	case rmgroupdomain.FlagInit:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_INIT
	case rmgroupdomain.FlagCons:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_CONS
	case rmgroupdomain.FlagStores:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_STORES
	case rmgroupdomain.FlagDept:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_DEPT
	case rmgroupdomain.FlagPO1:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_PO_1
	case rmgroupdomain.FlagPO2:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_PO_2
	case rmgroupdomain.FlagPO3:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_PO_3
	default:
		return financev1.RMGroupFlag_RM_GROUP_FLAG_UNSPECIFIED
	}
}

// protoFlagToString maps proto flag to its canonical string form (domain Flag underlying value).
func protoFlagToString(f financev1.RMGroupFlag) string {
	switch f {
	case financev1.RMGroupFlag_RM_GROUP_FLAG_INIT:
		return string(rmgroupdomain.FlagInit)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_CONS:
		return string(rmgroupdomain.FlagCons)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_STORES:
		return string(rmgroupdomain.FlagStores)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_DEPT:
		return string(rmgroupdomain.FlagDept)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_PO_1:
		return string(rmgroupdomain.FlagPO1)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_PO_2:
		return string(rmgroupdomain.FlagPO2)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_PO_3:
		return string(rmgroupdomain.FlagPO3)
	case financev1.RMGroupFlag_RM_GROUP_FLAG_UNSPECIFIED:
		return ""
	default:
		return ""
	}
}

func removeModeFromProto(m financev1.RemoveItemsMode) appgroup.RemoveMode {
	switch m {
	case financev1.RemoveItemsMode_REMOVE_ITEMS_MODE_DEACTIVATE:
		return appgroup.RemoveModeDeactivate
	case financev1.RemoveItemsMode_REMOVE_ITEMS_MODE_SOFT_DELETE:
		return appgroup.RemoveModeSoftDelete
	case financev1.RemoveItemsMode_REMOVE_ITEMS_MODE_UNSPECIFIED:
		return appgroup.RemoveModeSoftDelete
	default:
		return appgroup.RemoveModeSoftDelete
	}
}

// ExportRMGroups generates a 2-sheet Excel (Groups + Items) of all RM groups.
func (h *RMGroupHandler) ExportRMGroups(ctx context.Context, req *financev1.ExportRMGroupsRequest) (*financev1.ExportRMGroupsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opExport, false)
		return &financev1.ExportRMGroupsResponse{Base: baseResp}, nil
	}

	query := appgroup.ExportQuery{}
	switch req.ActiveFilter {
	case financev1.ActiveFilter_ACTIVE_FILTER_ACTIVE:
		v := true
		query.IsActive = &v
	case financev1.ActiveFilter_ACTIVE_FILTER_INACTIVE:
		v := false
		query.IsActive = &v
	case financev1.ActiveFilter_ACTIVE_FILTER_UNSPECIFIED:
	}

	result, err := h.exportHandler.Handle(ctx, query)
	if err != nil {
		RecordRMGroupOperation(opExport, false)
		return &financev1.ExportRMGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opExport, true)
	return &financev1.ExportRMGroupsResponse{
		Base:        successResponse("RM Groups exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportRMGroups parses a 2-sheet Excel and upserts heads + details.
func (h *RMGroupHandler) ImportRMGroups(ctx context.Context, req *financev1.ImportRMGroupsRequest) (*financev1.ImportRMGroupsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opImport, false)
		return &financev1.ImportRMGroupsResponse{Base: baseResp}, nil
	}

	result, err := h.importHandler.Handle(ctx, appgroup.ImportCommand{
		FileContent:     req.FileContent,
		FileName:        req.FileName,
		DuplicateAction: req.DuplicateAction,
		CreatedBy:       getUserFromContext(ctx),
	})
	if err != nil {
		RecordRMGroupOperation(opImport, false)
		return &financev1.ImportRMGroupsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	errs := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		errs[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}

	RecordRMGroupOperation(opImport, true)
	return &financev1.ImportRMGroupsResponse{
		Base:          successResponse("RM Groups imported"),
		GroupsCreated: result.GroupsCreated,
		GroupsUpdated: result.GroupsUpdated,
		GroupsSkipped: result.GroupsSkipped,
		ItemsAdded:    result.ItemsAdded,
		ItemsSkipped:  result.ItemsSkipped,
		FailedCount:   result.FailedCount,
		Errors:        errs,
	}, nil
}

// DownloadRMGroupTemplate returns the blank 2-sheet import template.
func (h *RMGroupHandler) DownloadRMGroupTemplate(_ context.Context, req *financev1.DownloadRMGroupTemplateRequest) (*financev1.DownloadRMGroupTemplateResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opTemplate, false)
		return &financev1.DownloadRMGroupTemplateResponse{Base: baseResp}, nil
	}

	result, err := h.templateHandler.Handle()
	if err != nil {
		RecordRMGroupOperation(opTemplate, false)
		return &financev1.DownloadRMGroupTemplateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opTemplate, true)
	return &financev1.DownloadRMGroupTemplateResponse{
		Base:        successResponse("RM Group template generated"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ExportUngroupedItems exports ungrouped items matching the filter to Excel.
func (h *RMGroupHandler) ExportUngroupedItems(ctx context.Context, req *financev1.ExportUngroupedItemsRequest) (*financev1.ExportUngroupedItemsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opExport, false)
		return &financev1.ExportUngroupedItemsResponse{Base: baseResp}, nil
	}

	result, err := h.ungroupedExport.Handle(ctx, appgroup.UngroupedExportQuery{
		Period: req.Period,
		Search: req.Search,
	})
	if err != nil {
		RecordRMGroupOperation(opExport, false)
		return &financev1.ExportUngroupedItemsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	RecordRMGroupOperation(opExport, true)
	return &financev1.ExportUngroupedItemsResponse{
		Base:        successResponse("Ungrouped items exported successfully"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// ImportGroupItems bulk-adds items to a specific existing group from Excel.
func (h *RMGroupHandler) ImportGroupItems(ctx context.Context, req *financev1.ImportGroupItemsRequest) (*financev1.ImportGroupItemsResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opImport, false)
		return &financev1.ImportGroupItemsResponse{Base: baseResp}, nil
	}

	result, err := h.importItemsHandler.Handle(ctx, appgroup.ImportGroupItemsCommand{
		HeadID:      req.GroupHeadId,
		FileContent: req.FileContent,
		FileName:    req.FileName,
		CreatedBy:   getUserFromContext(ctx),
	})
	if err != nil {
		RecordRMGroupOperation(opImport, false)
		return &financev1.ImportGroupItemsResponse{Base: domainErrorToBaseResponse(err)}, nil
	}

	errs := make([]*financev1.ImportError, len(result.Errors))
	for i, e := range result.Errors {
		errs[i] = &financev1.ImportError{
			RowNumber: e.RowNumber,
			Field:     e.Field,
			Message:   e.Message,
		}
	}
	skipped := make([]*financev1.SkippedItem, len(result.Skipped))
	for i, s := range result.Skipped {
		skipped[i] = skippedItemToProto(s)
	}

	RecordRMGroupOperation(opImport, true)

	// Trigger a recalc if any items were actually added.
	if result.ItemsAdded > 0 {
		if headID, parseErr := uuid.Parse(req.GroupHeadId); parseErr == nil {
			h.recalc.Publish(ctx, headID, string(apprmcost.TriggerDetailChange), getUserFromContext(ctx))
		}
	}

	return &financev1.ImportGroupItemsResponse{
		Base:         successResponse("Items imported"),
		ItemsAdded:   result.ItemsAdded,
		ItemsSkipped: result.ItemsSkipped,
		FailedCount:  result.FailedCount,
		Errors:       errs,
		Skipped:      skipped,
	}, nil
}

// DownloadGroupItemsTemplate returns a one-sheet Excel template matching
// the columns ImportGroupItems expects.
func (h *RMGroupHandler) DownloadGroupItemsTemplate(_ context.Context, req *financev1.DownloadGroupItemsTemplateRequest) (*financev1.DownloadGroupItemsTemplateResponse, error) {
	if baseResp := h.validationHelper.ValidateRequest(req); baseResp != nil {
		RecordRMGroupOperation(opTemplate, false)
		return &financev1.DownloadGroupItemsTemplateResponse{Base: baseResp}, nil
	}
	result, err := h.itemsTemplate.Handle()
	if err != nil {
		RecordRMGroupOperation(opTemplate, false)
		return &financev1.DownloadGroupItemsTemplateResponse{Base: domainErrorToBaseResponse(err)}, nil
	}
	RecordRMGroupOperation(opTemplate, true)
	return &financev1.DownloadGroupItemsTemplateResponse{
		Base:        successResponse("RM Group items template generated"),
		FileContent: result.FileContent,
		FileName:    result.FileName,
	}, nil
}

// RecordRMGroupOperation records an RM Group operation metric.
func RecordRMGroupOperation(operation string, success bool) {
	rmGroupOperationsTotal.WithLabelValues(operation, metricStatus(success)).Inc()
}
