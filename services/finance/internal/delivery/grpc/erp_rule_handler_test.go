package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpruleapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erprule"
	domainerp "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

type fakeRuleCreate struct {
	r   *domain.VallossRule
	err error
}

func (f fakeRuleCreate) Handle(context.Context, erpruleapp.CreateVallossRuleCommand) (*domain.VallossRule, error) {
	return f.r, f.err
}

type fakeRuleDelete struct{ err error }

func (f fakeRuleDelete) Handle(context.Context, erpruleapp.DeleteVallossRuleCommand) error {
	return f.err
}

type fakeRuleBatches struct{}

func (fakeRuleBatches) GetByID(context.Context, int64) (*domainerp.Batch, error) {
	return nil, domainerp.ErrBatchNotFound
}

func TestErpRuleHandler_CreateValLossRule_Happy(t *testing.T) {
	key, err := domain.NewRuleKey("Type 1", "POY", "BC")
	require.NoError(t, err)
	_ = key
	h := NewErpRuleGRPCHandler(ErpRuleDeps{Create: fakeRuleCreate{err: domain.ErrDuplicateRule}})
	resp, err := h.CreateValLossRule(context.Background(), &financev1.CreateValLossRuleRequest{})
	require.NoError(t, err)
	assert.Equal(t, "409", resp.GetBase().GetStatusCode())
}

func TestErpRuleHandler_DeleteInUse_412(t *testing.T) {
	h := NewErpRuleGRPCHandler(ErpRuleDeps{Delete: fakeRuleDelete{err: domain.ErrRuleInUse}})
	resp, err := h.DeleteValLossRule(context.Background(), &financev1.DeleteValLossRuleRequest{Id: 1})
	require.NoError(t, err)
	assert.Equal(t, "412", resp.GetBase().GetStatusCode())
}

func TestErpRuleHandler_DeleteOK(t *testing.T) {
	h := NewErpRuleGRPCHandler(ErpRuleDeps{Delete: fakeRuleDelete{}})
	resp, err := h.DeleteValLossRule(context.Background(), &financev1.DeleteValLossRuleRequest{Id: 1})
	require.NoError(t, err)
	assert.True(t, resp.GetBase().GetIsSuccess())
}

func TestErpRuleHandler_Diff_BatchNotFound(t *testing.T) {
	h := NewErpRuleGRPCHandler(ErpRuleDeps{Batches: fakeRuleBatches{}})
	resp, err := h.GetRuleSnapshotDiff(context.Background(), &financev1.GetRuleSnapshotDiffRequest{FromBatchId: 1, ToBatchId: 2})
	require.NoError(t, err)
	assert.Equal(t, "404", resp.GetBase().GetStatusCode())
}

func TestErpRuleErrBase_Mapping(t *testing.T) {
	cases := map[error]string{
		domain.ErrRuleNotFound:   "404",
		domain.ErrInvalidPercent: "400",
		domain.ErrRuleInactive:   "412",
		fmt.Errorf("boom"):       "500",
	}
	for err, code := range cases {
		assert.Equal(t, code, erpRuleErrBase(err).GetStatusCode(), err.Error())
	}
}

type covRuleLister struct {
	res erpruleapp.ListVallossRulesResult
	err error
}

func (f covRuleLister) Handle(context.Context, erpruleapp.ListVallossRulesQuery) (erpruleapp.ListVallossRulesResult, error) {
	return f.res, f.err
}

type covRuleGetter struct {
	r   *domain.VallossRule
	err error
}

func (f covRuleGetter) GetByID(context.Context, int64) (*domain.VallossRule, error) {
	return f.r, f.err
}

type covRuleUpdater struct{ err error }

func (f covRuleUpdater) Handle(_ context.Context, c erpruleapp.UpdateVallossRuleCommand) (*domain.VallossRule, error) {
	return covRule(c.ID, true), f.err
}

type covPriceLister struct{ err error }

func (f covPriceLister) Handle(context.Context, erpruleapp.ListSellPricesQuery) ([]*domain.SellPrice, error) {
	now := time.Now()
	return []*domain.SellPrice{domain.ReconstructSellPrice("SPPTY", decimal.NewFromInt(3), true, now, "u", &now, "v")}, f.err
}

type covPriceUpsert struct{ err error }

func (f covPriceUpsert) Handle(context.Context, erpruleapp.UpsertSellPriceCommand) (*domain.SellPrice, error) {
	return domain.ReconstructSellPrice("SPPTY", decimal.NewFromInt(3), true, time.Time{}, "u", nil, ""), f.err
}

type covGradeLister struct{ err error }

func (f covGradeLister) Handle(context.Context, erpruleapp.ListGradeGroupsQuery) (erpruleapp.ListGradeGroupsResult, error) {
	grp := domain.GradeGroup("BC")
	return erpruleapp.ListGradeGroupsResult{
		Items: []*domain.Grade{domain.ReconstructGrade("G1", "Grade", true, &grp), domain.ReconstructGrade("G2", "Grade2", true, nil)},
		Total: 2, Page: 1, PageSize: 20,
	}, f.err
}

type covGradeAssign struct{ err error }

func (f covGradeAssign) Handle(context.Context, erpruleapp.AssignGradeGroupCommand) (*domain.Grade, error) {
	return domain.ReconstructGrade("G1", "Grade", true, nil), f.err
}

type covExport struct{ err error }

func (f covExport) Handle(context.Context, erpruleapp.ExportRulesQuery) (*erpruleapp.ExportRulesResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &erpruleapp.ExportRulesResult{FileContent: []byte("x"), FileName: "r.xlsx"}, nil
}

func covRule(id int64, active bool) *domain.VallossRule {
	key, _ := domain.NewRuleKey("Type 1", "POY", "BC")
	now := time.Now()
	return domain.ReconstructVallossRule(id, key, "SPPTY", decimal.NewFromInt(2), active, now, "u", &now, "v")
}

func TestErpRuleHandler_Reads(t *testing.T) {
	ctx := context.Background()
	yes, no := true, false
	h := NewErpRuleGRPCHandler(ErpRuleDeps{
		List: covRuleLister{res: erpruleapp.ListVallossRulesResult{
			Items: []*domain.VallossRule{covRule(1, true), covRule(2, false)}, Total: 2, Page: 1, PageSize: 20}},
		ListPrices: covPriceLister{}, UpsertPrice: covPriceUpsert{}, ListGrades: covGradeLister{},
		AssignGrade: covGradeAssign{}, Export: covExport{},
	})
	l, _ := h.ListValLossRules(ctx, &financev1.ListValLossRulesRequest{})
	assert.Len(t, l.Data, 2)
	l, _ = h.ListValLossRules(ctx, &financev1.ListValLossRulesRequest{IsActive: &yes})
	require.Len(t, l.Data, 1)
	assert.Equal(t, "2.000000", l.Data[0].ValLoss)
	l, _ = h.ListValLossRules(ctx, &financev1.ListValLossRulesRequest{IsActive: &no})
	assert.Len(t, l.Data, 1)

	sp, _ := h.ListSellPrices(ctx, &financev1.ListSellPricesRequest{})
	assert.Len(t, sp.Data, 1)
	up, _ := h.UpsertSellPrice(ctx, &financev1.UpsertSellPriceRequest{Basis: "SPPTY"})
	assert.Equal(t, "SPPTY", up.Data.Basis)
	gl, _ := h.ListGradeGroups(ctx, &financev1.ListGradeGroupsRequest{})
	require.Len(t, gl.Data, 2)
	assert.Equal(t, "BC", gl.Data[0].GradeGroup)
	ug, _ := h.UpdateGradeGroup(ctx, &financev1.UpdateGradeGroupRequest{GradeCode: "G1"})
	assert.Equal(t, "G1", ug.Data.GradeCode)
	ex, _ := h.ExportErpRules(ctx, &financev1.ExportErpRulesRequest{})
	assert.Equal(t, "r.xlsx", ex.FileName)

	assert.Nil(t, vallossToProto(nil))
	assert.Nil(t, sellPriceToProto(nil))
	assert.Nil(t, gradeToProto(nil))
	assert.Empty(t, erpRuleTimePtr(nil))
	assert.Empty(t, erpRuleTime(time.Time{}))
	cv := ruleChangeValuesToProto([]domain.ChangeValue{{Basis: "SPPTY", Value: decimal.NewFromInt(1), Group: "BC"}, {Value: decimal.Zero}})
	assert.Len(t, cv, 4)
}

func TestErpRuleHandler_ReadErrors(t *testing.T) {
	ctx := context.Background()
	boom := fmt.Errorf("boom")
	h := NewErpRuleGRPCHandler(ErpRuleDeps{
		List: covRuleLister{err: boom}, ListPrices: covPriceLister{err: boom}, UpsertPrice: covPriceUpsert{err: domain.ErrInvalidPrice},
		ListGrades: covGradeLister{err: boom}, AssignGrade: covGradeAssign{err: domain.ErrGradeNotFound}, Export: covExport{err: boom},
	})
	l, _ := h.ListValLossRules(ctx, &financev1.ListValLossRulesRequest{})
	assert.Equal(t, "500", l.Base.StatusCode)
	sp, _ := h.ListSellPrices(ctx, &financev1.ListSellPricesRequest{})
	assert.Equal(t, "500", sp.Base.StatusCode)
	up, _ := h.UpsertSellPrice(ctx, &financev1.UpsertSellPriceRequest{})
	assert.Equal(t, "400", up.Base.StatusCode)
	gl, _ := h.ListGradeGroups(ctx, &financev1.ListGradeGroupsRequest{})
	assert.Equal(t, "500", gl.Base.StatusCode)
	ug, _ := h.UpdateGradeGroup(ctx, &financev1.UpdateGradeGroupRequest{})
	assert.Equal(t, "404", ug.Base.StatusCode)
	ex, _ := h.ExportErpRules(ctx, &financev1.ExportErpRulesRequest{})
	assert.Equal(t, "500", ex.Base.StatusCode)
}

func TestErpRuleHandler_UpdateValLossRule(t *testing.T) {
	ctx := context.Background()
	h := NewErpRuleGRPCHandler(ErpRuleDeps{Rules: covRuleGetter{r: covRule(5, true)}, Update: covRuleUpdater{}})
	r, _ := h.UpdateValLossRule(ctx, &financev1.UpdateValLossRuleRequest{Id: 5, ValLoss: "1"})
	assert.True(t, r.Base.IsSuccess)
	assert.Equal(t, int64(5), r.Data.Id)

	h = NewErpRuleGRPCHandler(ErpRuleDeps{Rules: covRuleGetter{err: domain.ErrRuleNotFound}})
	r, _ = h.UpdateValLossRule(ctx, &financev1.UpdateValLossRuleRequest{})
	assert.Equal(t, "404", r.Base.StatusCode)

	h = NewErpRuleGRPCHandler(ErpRuleDeps{Rules: covRuleGetter{r: covRule(5, true)}, Update: covRuleUpdater{err: domain.ErrRuleInactive}})
	r, _ = h.UpdateValLossRule(ctx, &financev1.UpdateValLossRuleRequest{})
	assert.Equal(t, "412", r.Base.StatusCode)
}
