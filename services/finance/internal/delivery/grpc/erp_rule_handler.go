package grpc

import (
	"context"
	"errors"
	"time"

	commonv1 "github.com/mutugading/goapps-backend/gen/common/v1"
	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpruleapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erprule"
	domainerp "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// Narrow seams over the erprule application handlers (fakeable in tests).
type (
	erpRuleLister interface {
		Handle(ctx context.Context, q erpruleapp.ListVallossRulesQuery) (erpruleapp.ListVallossRulesResult, error)
	}
	erpRuleCreator interface {
		Handle(ctx context.Context, cmd erpruleapp.CreateVallossRuleCommand) (*domain.VallossRule, error)
	}
	erpRuleUpdater interface {
		Handle(ctx context.Context, cmd erpruleapp.UpdateVallossRuleCommand) (*domain.VallossRule, error)
	}
	erpRuleGetter interface {
		GetByID(ctx context.Context, id int64) (*domain.VallossRule, error)
	}
	erpRuleDeleter interface {
		Handle(ctx context.Context, cmd erpruleapp.DeleteVallossRuleCommand) error
	}
	erpSellPriceLister interface {
		Handle(ctx context.Context, q erpruleapp.ListSellPricesQuery) ([]*domain.SellPrice, error)
	}
	erpSellPriceUpserter interface {
		Handle(ctx context.Context, cmd erpruleapp.UpsertSellPriceCommand) (*domain.SellPrice, error)
	}
	erpGradeLister interface {
		Handle(ctx context.Context, q erpruleapp.ListGradeGroupsQuery) (erpruleapp.ListGradeGroupsResult, error)
	}
	erpGradeAssigner interface {
		Handle(ctx context.Context, cmd erpruleapp.AssignGradeGroupCommand) (*domain.Grade, error)
	}
	erpRuleBatchReader interface {
		GetByID(ctx context.Context, id int64) (*domainerp.Batch, error)
	}
	erpRuleExporter interface {
		Handle(ctx context.Context, q erpruleapp.ExportRulesQuery) (*erpruleapp.ExportRulesResult, error)
	}
)

// ErpRuleDeps groups the collaborators of ErpRuleHandler.
type ErpRuleDeps struct {
	List        erpRuleLister
	Create      erpRuleCreator
	Update      erpRuleUpdater
	Rules       erpRuleGetter
	Delete      erpRuleDeleter
	ListPrices  erpSellPriceLister
	UpsertPrice erpSellPriceUpserter
	ListGrades  erpGradeLister
	AssignGrade erpGradeAssigner
	Batches     erpRuleBatchReader
	Export      erpRuleExporter
}

// ErpRuleHandler implements financev1.ErpRuleServiceServer.
type ErpRuleHandler struct {
	financev1.UnimplementedErpRuleServiceServer
	d ErpRuleDeps
}

// NewErpRuleGRPCHandler builds the handler.
func NewErpRuleGRPCHandler(d ErpRuleDeps) *ErpRuleHandler { return &ErpRuleHandler{d: d} }

// erpRuleErrBase maps an erprule error to a BaseResponse: not found -> 404,
// validation -> 400, duplicate -> 409, in-use / inactive -> 412.
func erpRuleErrBase(err error) *commonv1.BaseResponse {
	switch {
	case errors.Is(err, domain.ErrRuleNotFound), errors.Is(err, domain.ErrSellPriceNotFound),
		errors.Is(err, domain.ErrGradeNotFound), errors.Is(err, domainerp.ErrBatchNotFound):
		return NotFoundResponse(err.Error())
	case errors.Is(err, domain.ErrInvalidPercent), errors.Is(err, domain.ErrInvalidPrice),
		errors.Is(err, domain.ErrInvalidBasis), errors.Is(err, domain.ErrInvalidProdType),
		errors.Is(err, domain.ErrInvalidGradeGroup), errors.Is(err, domain.ErrAxRule),
		errors.Is(err, domain.ErrInvalidFgType), errors.Is(err, domain.ErrInvalidGradeCode),
		errors.Is(err, domain.ErrUserRequired):
		return BadRequestResponse(err.Error())
	case errors.Is(err, domain.ErrDuplicateRule):
		return ConflictResponse(err.Error())
	case errors.Is(err, domain.ErrRuleInUse), errors.Is(err, domain.ErrRuleInactive):
		return ErrorResponse("412", err.Error())
	}
	return InternalErrorResponse(err.Error())
}

func erpRuleTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func erpRuleTimePtr(t *time.Time) string {
	if t == nil {
		return ""
	}
	return erpRuleTime(*t)
}

func vallossToProto(r *domain.VallossRule) *financev1.ValLossRule {
	if r == nil {
		return nil
	}
	k := r.Key()
	return &financev1.ValLossRule{
		Id: r.ID(), FgType: string(k.FgType), ProdType: string(k.ProdType), GradeGroup: string(k.GradeGroup),
		Basis: string(r.Basis()), ValLoss: r.ValLoss().StringFixed(6), IsActive: r.IsActive(),
		CreatedAt: erpRuleTime(r.CreatedAt()), CreatedBy: r.CreatedBy(),
		UpdatedAt: erpRuleTimePtr(r.UpdatedAt()), UpdatedBy: r.UpdatedBy(),
	}
}

func sellPriceToProto(p *domain.SellPrice) *financev1.SellPrice {
	if p == nil {
		return nil
	}
	return &financev1.SellPrice{
		Basis: string(p.Basis()), Price: p.Price().StringFixed(6), IsActive: p.IsActive(),
		CreatedAt: erpRuleTime(p.CreatedAt()), CreatedBy: p.CreatedBy(),
		UpdatedAt: erpRuleTimePtr(p.UpdatedAt()), UpdatedBy: p.UpdatedBy(),
	}
}

func gradeToProto(g *domain.Grade) *financev1.GradeGroup {
	if g == nil {
		return nil
	}
	out := &financev1.GradeGroup{
		GradeCode: g.Code(), GradeName: g.Name(), IsActive: g.IsActive(), AssignedBy: g.AssignedBy(),
	}
	if grp := g.Group(); grp != nil {
		out.GradeGroup = string(*grp)
	}
	return out
}

func ruleChangeValuesToProto(vs []domain.ChangeValue) []*financev1.ChangeValue {
	out := make([]*financev1.ChangeValue, 0, len(vs))
	for _, v := range vs {
		if v.Basis != "" {
			out = append(out, &financev1.ChangeValue{Name: "basis", Value: string(v.Basis)})
		}
		out = append(out, &financev1.ChangeValue{Name: "value", Value: v.Value.StringFixed(6)})
		if v.Group != "" {
			out = append(out, &financev1.ChangeValue{Name: "grade_group", Value: string(v.Group)})
		}
	}
	return out
}

// ListValLossRules lists val-loss rules.
func (h *ErpRuleHandler) ListValLossRules(ctx context.Context, req *financev1.ListValLossRulesRequest) (*financev1.ListValLossRulesResponse, error) {
	page, size := paginationFromProto(req.GetPagination())
	res, err := h.d.List.Handle(ctx, erpruleapp.ListVallossRulesQuery{
		FgType: req.GetFgType(), ProdType: req.GetProdType(), GradeGroup: req.GetGradeGroup(),
		IncludeInactive: req.IsActive == nil || !req.GetIsActive(), Page: int(page), PageSize: int(size),
	})
	if err != nil {
		return &financev1.ListValLossRulesResponse{Base: erpRuleErrBase(err)}, nil
	}
	rows := make([]*financev1.ValLossRule, 0, len(res.Items))
	for _, r := range res.Items {
		// is_active narrows to active / inactive rows; unset keeps all.
		if req.IsActive != nil && req.GetIsActive() != r.IsActive() {
			continue
		}
		rows = append(rows, vallossToProto(r))
	}
	return &financev1.ListValLossRulesResponse{
		Base: erpOK("OK"), Data: rows,
		Pagination: paginationResponse(safeIntToInt32(res.Page), safeIntToInt32(res.PageSize), res.Total),
	}, nil
}

// CreateValLossRule creates a rule.
func (h *ErpRuleHandler) CreateValLossRule(ctx context.Context, req *financev1.CreateValLossRuleRequest) (*financev1.CreateValLossRuleResponse, error) {
	r, err := h.d.Create.Handle(ctx, erpruleapp.CreateVallossRuleCommand{
		FgType: req.GetFgType(), ProdType: req.GetProdType(), GradeGroup: req.GetGradeGroup(),
		Basis: req.GetBasis(), ValLoss: req.GetValLoss(), User: erpActor(ctx),
	})
	if err != nil {
		return &financev1.CreateValLossRuleResponse{Base: erpRuleErrBase(err)}, nil
	}
	return &financev1.CreateValLossRuleResponse{Base: erpOK("Rule created"), Data: vallossToProto(r)}, nil
}

// UpdateValLossRule updates a rule. The request carries no basis, so the
// current basis is read first and kept; only the value loss changes.
func (h *ErpRuleHandler) UpdateValLossRule(ctx context.Context, req *financev1.UpdateValLossRuleRequest) (*financev1.UpdateValLossRuleResponse, error) {
	cur, err := h.d.Rules.GetByID(ctx, req.GetId())
	if err != nil {
		return &financev1.UpdateValLossRuleResponse{Base: erpRuleErrBase(err)}, nil
	}
	r, err := h.d.Update.Handle(ctx, erpruleapp.UpdateVallossRuleCommand{
		ID: req.GetId(), Basis: string(cur.Basis()), ValLoss: req.GetValLoss(), User: erpActor(ctx),
	})
	if err != nil {
		return &financev1.UpdateValLossRuleResponse{Base: erpRuleErrBase(err)}, nil
	}
	return &financev1.UpdateValLossRuleResponse{Base: erpOK("Rule updated"), Data: vallossToProto(r)}, nil
}

// DeleteValLossRule soft-deletes a rule.
func (h *ErpRuleHandler) DeleteValLossRule(ctx context.Context, req *financev1.DeleteValLossRuleRequest) (*financev1.DeleteValLossRuleResponse, error) {
	if err := h.d.Delete.Handle(ctx, erpruleapp.DeleteVallossRuleCommand{ID: req.GetId(), User: erpActor(ctx)}); err != nil {
		return &financev1.DeleteValLossRuleResponse{Base: erpRuleErrBase(err)}, nil
	}
	return &financev1.DeleteValLossRuleResponse{Base: erpOK("Rule deleted")}, nil
}

// ListSellPrices lists the sell prices.
func (h *ErpRuleHandler) ListSellPrices(ctx context.Context, _ *financev1.ListSellPricesRequest) (*financev1.ListSellPricesResponse, error) {
	ps, err := h.d.ListPrices.Handle(ctx, erpruleapp.ListSellPricesQuery{IncludeInactive: true})
	if err != nil {
		return &financev1.ListSellPricesResponse{Base: erpRuleErrBase(err)}, nil
	}
	rows := make([]*financev1.SellPrice, 0, len(ps))
	for _, p := range ps {
		rows = append(rows, sellPriceToProto(p))
	}
	return &financev1.ListSellPricesResponse{
		Base: erpOK("OK"), Data: rows,
		Pagination: paginationResponse(1, safeIntToInt32(len(rows)), int64(len(rows))),
	}, nil
}

// UpsertSellPrice creates or updates the sell price of a basis.
func (h *ErpRuleHandler) UpsertSellPrice(ctx context.Context, req *financev1.UpsertSellPriceRequest) (*financev1.UpsertSellPriceResponse, error) {
	p, err := h.d.UpsertPrice.Handle(ctx, erpruleapp.UpsertSellPriceCommand{
		Basis: req.GetBasis(), Price: req.GetPrice(), User: erpActor(ctx),
	})
	if err != nil {
		return &financev1.UpsertSellPriceResponse{Base: erpRuleErrBase(err)}, nil
	}
	return &financev1.UpsertSellPriceResponse{Base: erpOK("Sell price saved"), Data: sellPriceToProto(p)}, nil
}

// ListGradeGroups lists ERP grades and their groups.
func (h *ErpRuleHandler) ListGradeGroups(ctx context.Context, req *financev1.ListGradeGroupsRequest) (*financev1.ListGradeGroupsResponse, error) {
	page, size := paginationFromProto(req.GetPagination())
	res, err := h.d.ListGrades.Handle(ctx, erpruleapp.ListGradeGroupsQuery{
		UnassignedOnly: req.GetUnassignedOnly(), Page: int(page), PageSize: int(size),
	})
	if err != nil {
		return &financev1.ListGradeGroupsResponse{Base: erpRuleErrBase(err)}, nil
	}
	rows := make([]*financev1.GradeGroup, 0, len(res.Items))
	for _, g := range res.Items {
		rows = append(rows, gradeToProto(g))
	}
	return &financev1.ListGradeGroupsResponse{
		Base: erpOK("OK"), Data: rows,
		Pagination: paginationResponse(safeIntToInt32(res.Page), safeIntToInt32(res.PageSize), res.Total),
	}, nil
}

// UpdateGradeGroup assigns (or clears, with an empty group) a grade group.
func (h *ErpRuleHandler) UpdateGradeGroup(ctx context.Context, req *financev1.UpdateGradeGroupRequest) (*financev1.UpdateGradeGroupResponse, error) {
	g, err := h.d.AssignGrade.Handle(ctx, erpruleapp.AssignGradeGroupCommand{
		GradeCode: req.GetGradeCode(), GradeGroup: req.GetGradeGroup(), User: erpActor(ctx),
	})
	if err != nil {
		return &financev1.UpdateGradeGroupResponse{Base: erpRuleErrBase(err)}, nil
	}
	return &financev1.UpdateGradeGroupResponse{Base: erpOK("Grade group updated"), Data: gradeToProto(g)}, nil
}

// GetRuleSnapshotDiff diffs the rule snapshots of two batches.
func (h *ErpRuleHandler) GetRuleSnapshotDiff(ctx context.Context, req *financev1.GetRuleSnapshotDiffRequest) (*financev1.GetRuleSnapshotDiffResponse, error) {
	prev, err := h.d.Batches.GetByID(ctx, req.GetFromBatchId())
	if err != nil {
		return &financev1.GetRuleSnapshotDiffResponse{Base: erpRuleErrBase(err)}, nil
	}
	cur, err := h.d.Batches.GetByID(ctx, req.GetToBatchId())
	if err != nil {
		return &financev1.GetRuleSnapshotDiffResponse{Base: erpRuleErrBase(err)}, nil
	}
	changes, err := domain.DiffSnapshots(prev.RuleSnapshot(), cur.RuleSnapshot())
	if err != nil {
		return &financev1.GetRuleSnapshotDiffResponse{Base: BadRequestResponse(err.Error())}, nil
	}
	rows := make([]*financev1.RuleChange, 0, len(changes))
	for _, c := range changes {
		rows = append(rows, &financev1.RuleChange{
			Subject: string(c.Subject), Kind: string(c.Kind), Key: c.Key,
			Before: ruleChangeValuesToProto(c.Before), After: ruleChangeValuesToProto(c.After),
		})
	}
	return &financev1.GetRuleSnapshotDiffResponse{Base: erpOK("OK"), Data: rows}, nil
}

// ExportErpRules exports the rule master as xlsx.
func (h *ErpRuleHandler) ExportErpRules(ctx context.Context, _ *financev1.ExportErpRulesRequest) (*financev1.ExportErpRulesResponse, error) {
	res, err := h.d.Export.Handle(ctx, erpruleapp.ExportRulesQuery{IncludeInactive: true})
	if err != nil {
		return &financev1.ExportErpRulesResponse{Base: erpRuleErrBase(err)}, nil
	}
	return &financev1.ExportErpRulesResponse{Base: erpOK("OK"), FileContent: res.FileContent, FileName: res.FileName}, nil
}
