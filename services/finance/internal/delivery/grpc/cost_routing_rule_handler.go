package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costroutingrule"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroutingrule"
)

// CostRoutingRuleHandler implements financev1.CostRoutingRuleServiceServer.
type CostRoutingRuleHandler struct {
	financev1.UnimplementedCostRoutingRuleServiceServer
	createH    *app.CreateHandler
	getH       *app.GetHandler
	updateH    *app.UpdateHandler
	deleteH    *app.DeleteHandler
	listH      *app.ListHandler
	validation *ValidationHelper
}

// NewCostRoutingRuleHandler constructs the handler.
func NewCostRoutingRuleHandler(repo domain.Repository) (*CostRoutingRuleHandler, error) {
	v, err := NewValidationHelper()
	if err != nil {
		return nil, err
	}
	return &CostRoutingRuleHandler{
		createH: app.NewCreateHandler(repo), getH: app.NewGetHandler(repo),
		updateH: app.NewUpdateHandler(repo), deleteH: app.NewDeleteHandler(repo),
		listH: app.NewListHandler(repo), validation: v,
	}, nil
}

// CreateCostRoutingRule creates a rule.
func (h *CostRoutingRuleHandler) CreateCostRoutingRule(ctx context.Context, req *financev1.CreateCostRoutingRuleRequest) (*financev1.CreateCostRoutingRuleResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.CreateCostRoutingRuleResponse{Base: b}, nil
	}
	actor, _ := GetUserIDFromCtx(ctx)
	r, err := h.createH.Handle(ctx, app.CreateCommand{
		Priority: req.GetPriority(), Condition: req.GetCondition(),
		ActionType: req.GetActionType(), ActionTarget: req.GetActionTarget(), CreatedBy: actor,
	})
	if err != nil {
		return &financev1.CreateCostRoutingRuleResponse{Base: ruleErrToBase(err)}, nil
	}
	return &financev1.CreateCostRoutingRuleResponse{Base: successResponse("Rule created"), Data: ruleToProto(r)}, nil
}

// GetCostRoutingRule returns one rule.
func (h *CostRoutingRuleHandler) GetCostRoutingRule(ctx context.Context, req *financev1.GetCostRoutingRuleRequest) (*financev1.GetCostRoutingRuleResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.GetCostRoutingRuleResponse{Base: b}, nil
	}
	r, err := h.getH.Handle(ctx, app.GetQuery{RuleID: req.GetRuleId()})
	if err != nil {
		return &financev1.GetCostRoutingRuleResponse{Base: ruleErrToBase(err)}, nil
	}
	return &financev1.GetCostRoutingRuleResponse{Base: successResponse("OK"), Data: ruleToProto(r)}, nil
}

// UpdateCostRoutingRule updates a rule.
func (h *CostRoutingRuleHandler) UpdateCostRoutingRule(ctx context.Context, req *financev1.UpdateCostRoutingRuleRequest) (*financev1.UpdateCostRoutingRuleResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.UpdateCostRoutingRuleResponse{Base: b}, nil
	}
	r, err := h.updateH.Handle(ctx, app.UpdateCommand{
		RuleID: req.GetRuleId(), Priority: req.GetPriority(), Condition: req.GetCondition(),
		ActionType: req.GetActionType(), ActionTarget: req.GetActionTarget(), IsActive: req.GetIsActive(),
	})
	if err != nil {
		return &financev1.UpdateCostRoutingRuleResponse{Base: ruleErrToBase(err)}, nil
	}
	return &financev1.UpdateCostRoutingRuleResponse{Base: successResponse("Rule updated"), Data: ruleToProto(r)}, nil
}

// DeleteCostRoutingRule removes a rule.
func (h *CostRoutingRuleHandler) DeleteCostRoutingRule(ctx context.Context, req *financev1.DeleteCostRoutingRuleRequest) (*financev1.DeleteCostRoutingRuleResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.DeleteCostRoutingRuleResponse{Base: b}, nil
	}
	if err := h.deleteH.Handle(ctx, app.DeleteCommand{RuleID: req.GetRuleId()}); err != nil {
		return &financev1.DeleteCostRoutingRuleResponse{Base: ruleErrToBase(err)}, nil
	}
	return &financev1.DeleteCostRoutingRuleResponse{Base: successResponse("Rule deleted")}, nil
}

// ListCostRoutingRules paginates rules by ascending priority.
func (h *CostRoutingRuleHandler) ListCostRoutingRules(ctx context.Context, req *financev1.ListCostRoutingRulesRequest) (*financev1.ListCostRoutingRulesResponse, error) {
	if b := h.validation.ValidateRequest(req); b != nil {
		return &financev1.ListCostRoutingRulesResponse{Base: b}, nil
	}
	page, pageSize := paginationFromProto(req.Pagination)
	res, err := h.listH.Handle(ctx, app.ListQuery{
		ActiveFilter: req.GetActiveFilter(), Page: int(page), PageSize: int(pageSize),
	})
	if err != nil {
		return &financev1.ListCostRoutingRulesResponse{Base: ruleErrToBase(err)}, nil
	}
	data := make([]*financev1.CostRoutingRule, 0, len(res.Items))
	for _, r := range res.Items {
		data = append(data, ruleToProto(r))
	}
	return &financev1.ListCostRoutingRulesResponse{
		Base: successResponse("OK"), Data: data,
		Pagination: paginationResponse(page, pageSize, res.Total),
	}, nil
}

func ruleToProto(r *domain.Rule) *financev1.CostRoutingRule {
	return &financev1.CostRoutingRule{
		RuleId: r.RuleID, Priority: r.Priority, Condition: r.Condition,
		ActionType: r.ActionType, ActionTarget: r.ActionTarget,
		IsActive: r.IsActive, CreatedBy: r.CreatedBy,
		CreatedAt: r.CreatedAt.Format(time.RFC3339),
	}
}

func ruleErrToBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidCondition), errors.Is(err, domain.ErrInvalidAction):
		return ErrorResponse("400", err.Error())
	default:
		return InternalErrorResponse(err.Error())
	}
}
