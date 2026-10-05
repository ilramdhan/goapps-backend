package grpc

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

type covSchedule struct {
	view erpapp.ScheduleView
	err  error
	upd  error
}

func (f covSchedule) Get(context.Context, bool) (erpapp.ScheduleView, error) { return f.view, f.err }
func (f covSchedule) Update(context.Context, erpapp.UpdateScheduleCommand) (erpapp.ScheduleView, error) {
	return f.view, f.upd
}

type covPreviewGet struct {
	p   domain.ValuationPreview
	err error
}

func (f covPreviewGet) Get(context.Context, string) (domain.ValuationPreview, error) {
	return f.p, f.err
}

type covStd struct {
	rows []domain.StdRow
	err  error
}

func (f covStd) List(context.Context, int64) ([]domain.StdRow, error) { return f.rows, f.err }

type covOracle struct {
	rows []domain.OracleCallRow
	err  error
}

func (f covOracle) ListByBatch(context.Context, int64) ([]domain.OracleCallRow, error) {
	return f.rows, f.err
}

type covSanity struct{ err error }

func (f covSanity) Report(context.Context, erpapp.CurrencySanityFilter) (*erpapp.CurrencySanityReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &erpapp.CurrencySanityReport{
		Labels:   []erpapp.CurrencyLabelCount{{Period: "202606", Currency: "USD", Rows: 3}},
		Outliers: []erpapp.CurrencyOutlier{{Currency: "IDR", Period: "202606", ErpItemCode: "I1", ProductCode: "P1", CostPerUnit: decimal.NewFromInt(5)}},
	}, nil
}

type covLinkRep struct{ err error }

func (f covLinkRep) Handle(context.Context, erpapp.LinkReadinessQuery) (*erpapp.LinkReadinessReport, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &erpapp.LinkReadinessReport{
		NoProduct:      []erpapp.NoProductLine{{ItemCode: "A"}},
		AttributeGaps:  []erpapp.AttributeGapLine{{ItemCode: "B", SysID: 1}},
		Duplicates:     []erpapp.DuplicateGroup{{ItemCode: "C"}},
		LinkedNoShade:  []erpapp.NoShadeLine{{ItemCode: "D", SysID: 2}},
		TypeMismatches: []erpapp.TypeMismatchLine{{ItemCode: "E", SysID: 3}},
	}, nil
}

type covBacktest struct {
	rep domain.BacktestReport
	err error
}

func (f covBacktest) Get(context.Context, int64) (domain.BacktestReport, error) { return f.rep, f.err }

func TestErpHandler_ScheduleRPCs(t *testing.T) {
	now := time.Now()
	withSetting := erpapp.ScheduleView{
		Effective: erpapp.ResolvedSchedule{Enabled: true, Source: "db"},
		Setting:   &domain.ScheduleSetting{Mode: domain.ScheduleCron, Cron: "0 0 1 * * *", DayOfMonth: 3, Timezone: "UTC"},
		NextRun:   &now,
	}
	noSetting := erpapp.ScheduleView{Effective: erpapp.ResolvedSchedule{Mode: domain.ScheduleEndOfMonth, Timezone: "Asia/Jakarta"}}
	c := ctxWith("finance.cost.erpintegration.update")

	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Schedule: covSchedule{view: withSetting}})
	g, _ := h.GetErpIntegrationSchedule(c, &financev1.GetErpIntegrationScheduleRequest{})
	assert.Equal(t, "UTC", g.Data.Timezone)
	assert.NotEmpty(t, g.Data.NextRunAt)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Schedule: covSchedule{view: noSetting}})
	g, _ = h.GetErpIntegrationSchedule(c, &financev1.GetErpIntegrationScheduleRequest{})
	assert.Equal(t, "Asia/Jakarta", g.Data.Timezone)
	assert.Equal(t, financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_END_OF_MONTH, g.Data.Mode)
	u, _ := h.UpdateErpIntegrationSchedule(c, &financev1.UpdateErpIntegrationScheduleRequest{Enabled: true, Mode: financev1.ErpScheduleMode_ERP_SCHEDULE_MODE_CRON})
	assert.True(t, u.Base.IsSuccess)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Schedule: covSchedule{view: withSetting}})
	u, _ = h.UpdateErpIntegrationSchedule(c, &financev1.UpdateErpIntegrationScheduleRequest{})
	assert.True(t, u.Base.IsSuccess)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Schedule: covSchedule{err: domain.ErrInvalidSchedule}})
	g, _ = h.GetErpIntegrationSchedule(c, &financev1.GetErpIntegrationScheduleRequest{})
	assert.Equal(t, "400", g.Base.StatusCode)
	u, _ = h.UpdateErpIntegrationSchedule(c, &financev1.UpdateErpIntegrationScheduleRequest{})
	assert.Equal(t, "400", u.Base.StatusCode)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Schedule: covSchedule{view: noSetting, upd: domain.ErrInvalidSchedule}})
	u, _ = h.UpdateErpIntegrationSchedule(c, &financev1.UpdateErpIntegrationScheduleRequest{})
	assert.Equal(t, "400", u.Base.StatusCode)
}

func TestErpHandler_AdjPreviewAndBacktest(t *testing.T) {
	c := ctxWith()
	tot, err := json.Marshal(erpapp.PreviewTotals{
		ProjectedVal: decimal.NewFromInt(7),
		Heads:        []erpapp.PreviewHead{{HeadSysID: 1, TxnCode: "T1", Items: 2, ProjectedVal: decimal.NewFromInt(7)}},
	})
	require.NoError(t, err)
	p := domain.ValuationPreview{ID: "pv", BatchID: 4, Operation: domain.AdjOpValuate, HeadCount: 1, Totals: tot, CreatedAt: time.Now(), ExpiresAt: time.Now()}

	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Previews: covPreviewGet{p: p}})
	r, _ := h.GetErpAdjPreview(c, &financev1.GetErpAdjPreviewRequest{PreviewId: "pv"})
	require.True(t, r.Base.IsSuccess)
	assert.Len(t, r.Data.Heads, 1)
	assert.Equal(t, "7", r.Data.TotalAmount)

	bad := p
	bad.Totals = []byte("{")
	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Previews: covPreviewGet{p: bad}})
	r, _ = h.GetErpAdjPreview(c, &financev1.GetErpAdjPreviewRequest{})
	assert.Equal(t, "500", r.Base.StatusCode)
	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Previews: covPreviewGet{err: domain.ErrBatchNotFound}})
	r, _ = h.GetErpAdjPreview(c, &financev1.GetErpAdjPreviewRequest{})
	assert.Equal(t, "404", r.Base.StatusCode)

	rep := domain.BacktestReport{
		Counts: map[domain.BacktestClass]int{"MATCH": 1},
		Lines: []domain.BacktestLine{{Key: domain.ErpKey{ItemCode: "I"}, Basis: "SPPTY", Fail: true,
			GoApps: decimal.NullDecimal{Decimal: decimal.NewFromInt(1), Valid: true}}},
		Failed: true,
	}
	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Backtest: covBacktest{rep: rep}})
	b, _ := h.GetErpBacktestReport(c, &financev1.GetErpBacktestReportRequest{BatchId: 4})
	assert.True(t, b.Data.Failed)
	assert.Len(t, b.Data.Lines, 1)
	assert.Equal(t, int64(1), b.Data.Counts["MATCH"])
	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{Backtest: covBacktest{err: domain.ErrBatchNotFound}})
	b, _ = h.GetErpBacktestReport(c, &financev1.GetErpBacktestReportRequest{})
	assert.Equal(t, "404", b.Base.StatusCode)
}

func TestErpHandler_Lists(t *testing.T) {
	c := ctxWith()
	boom := errors.New("boom")
	fin := time.Now()
	h := NewErpIntegrationGRPCHandler(ErpIntegrationDeps{
		StdCost: covStd{rows: []domain.StdRow{
			{Key: domain.ErpKey{ItemCode: "I1"}, Basis: "SPPTY", Status: "OK", Source: "DERIVED", StdCost: decimal.NullDecimal{Decimal: decimal.NewFromInt(2), Valid: true}},
			{Key: domain.ErpKey{ItemCode: "I2"}, Basis: "AX", Status: "OK"},
		}},
		OracleCall: covOracle{rows: []domain.OracleCallRow{{ID: 1, StatementKey: "W1", StartedAt: fin, FinishedAt: &fin}, {ID: 2, StartedAt: fin}}},
		Sanity:     covSanity{}, LinkReport: covLinkRep{},
	})
	s, _ := h.ListErpStdCost(c, &financev1.ListErpStdCostRequest{BatchId: 1})
	assert.Len(t, s.Data, 2)
	s, _ = h.ListErpStdCost(c, &financev1.ListErpStdCostRequest{BatchId: 1, Basis: "AX", Status: "OK"})
	assert.Len(t, s.Data, 1)
	s, _ = h.ListErpStdCost(c, &financev1.ListErpStdCostRequest{BatchId: 1, Basis: "ZZ"})
	assert.Empty(t, s.Data)
	s, _ = h.ListErpStdCost(c, &financev1.ListErpStdCostRequest{BatchId: 1, Status: "BAD"})
	assert.Empty(t, s.Data)
	s, _ = h.ListErpStdCost(c, &financev1.ListErpStdCostRequest{BatchId: 1, Source: financev1.ErpStdSource(99)})
	assert.Empty(t, s.Data)

	o, _ := h.ListErpOracleCalls(c, &financev1.ListErpOracleCallsRequest{BatchId: 1})
	require.Len(t, o.Data, 2)
	assert.NotEmpty(t, o.Data[0].FinishedAt)
	assert.Empty(t, o.Data[1].FinishedAt)

	cs, _ := h.GetErpCurrencySanity(c, &financev1.GetErpCurrencySanityRequest{Period: "202606"})
	assert.Len(t, cs.Data, 2)
	lr, _ := h.GetErpItemLinkReport(c, &financev1.GetErpItemLinkReportRequest{})
	assert.Len(t, lr.Data, 5)

	h = NewErpIntegrationGRPCHandler(ErpIntegrationDeps{
		StdCost: covStd{err: boom}, OracleCall: covOracle{err: boom}, Sanity: covSanity{err: boom}, LinkReport: covLinkRep{err: boom},
	})
	s, _ = h.ListErpStdCost(c, &financev1.ListErpStdCostRequest{})
	assert.Equal(t, "500", s.Base.StatusCode)
	o, _ = h.ListErpOracleCalls(c, &financev1.ListErpOracleCallsRequest{})
	assert.Equal(t, "500", o.Base.StatusCode)
	cs, _ = h.GetErpCurrencySanity(c, &financev1.GetErpCurrencySanityRequest{})
	assert.Equal(t, "500", cs.Base.StatusCode)
	lr, _ = h.GetErpItemLinkReport(c, &financev1.GetErpItemLinkReportRequest{})
	assert.Equal(t, "500", lr.Base.StatusCode)
}
