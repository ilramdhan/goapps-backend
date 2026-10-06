package erprule

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func TestNormalizePage(t *testing.T) {
	p, s := normalizePage(0, 0)
	assert.Equal(t, 1, p)
	assert.Equal(t, DefaultPageSize, s)
	p, s = normalizePage(3, 10000)
	assert.Equal(t, 3, p)
	assert.Equal(t, MaxPageSize, s)
	p, s = normalizePage(2, 50)
	assert.Equal(t, 2, p)
	assert.Equal(t, 50, s)
}

func TestListVallossRules(t *testing.T) {
	repo := newFakeRules()
	seedRule(t, repo, "Type 1", "POY", "BC", "COST", "0")
	r2 := seedRule(t, repo, "Type 2", "PTY", "NS", "SPPTY", "1")
	require.NoError(t, repo.rows[r2.ID()].Deactivate("u", fixedNow))
	h := NewListVallossRulesHandler(repo)
	ctx := context.Background()

	res, err := h.Handle(ctx, ListVallossRulesQuery{FgType: "Type 1", ProdType: "POY", GradeGroup: "BC", Basis: "COST"})
	require.NoError(t, err)
	assert.Equal(t, int64(1), res.Total)
	assert.Equal(t, 1, res.Page)
	assert.Equal(t, DefaultPageSize, res.PageSize)
	assert.Equal(t, domain.FgType("Type 1"), repo.lastList.FgType)
	assert.Equal(t, domain.ProdTypePOY, repo.lastList.ProdType)
	assert.Equal(t, domain.GradeGroupBC, repo.lastList.GradeGroup)
	assert.Equal(t, domain.BasisCost, repo.lastList.Basis)

	res, err = h.Handle(ctx, ListVallossRulesQuery{IncludeInactive: true, Page: 1, PageSize: 5})
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.Total)
	assert.Equal(t, domain.VallossRuleFilter{IncludeInactive: true, Page: 1, PageSize: 5}, repo.lastList)

	for _, q := range []ListVallossRulesQuery{
		{FgType: "this fg type is too long"}, {ProdType: "X"}, {GradeGroup: "AX"}, {Basis: "Y"},
	} {
		_, err := h.Handle(ctx, q)
		require.Error(t, err)
	}

	repo.listErr = errors.New("db")
	_, err = h.Handle(ctx, ListVallossRulesQuery{})
	require.EqualError(t, err, "db")
}

func TestListSellPrices(t *testing.T) {
	repo := newFakePrices()
	repo.rows[domain.BasisSPPTY] = domain.ReconstructSellPrice(domain.BasisSPPTY, dec("1.95"), true, fixedNow, "s", nil, "")
	repo.rows[domain.BasisSPITY] = domain.ReconstructSellPrice(domain.BasisSPITY, dec("2"), false, fixedNow, "s", nil, "")
	h := NewListSellPricesHandler(repo)

	out, err := h.Handle(context.Background(), ListSellPricesQuery{})
	require.NoError(t, err)
	assert.Len(t, out, 1)
	out, err = h.Handle(context.Background(), ListSellPricesQuery{IncludeInactive: true})
	require.NoError(t, err)
	assert.Len(t, out, 2)
}

func TestListGradeGroupsAndUnassignedWorklist(t *testing.T) {
	repo := newFakeGrades()
	repo.add("A1", "a1", gg(domain.GradeGroupNS))
	repo.add("B1", "b1", nil)
	repo.add("C1", "c1", nil)
	ctx := context.Background()

	res, err := NewListGradeGroupsHandler(repo).Handle(ctx, ListGradeGroupsQuery{Search: "  1 ", PageSize: 9999})
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.Total)
	assert.Equal(t, "1", repo.lastList.Search)
	assert.Equal(t, MaxPageSize, repo.lastList.PageSize)

	res, err = NewListUnassignedGradesHandler(repo).Handle(ctx, "", 0, 0)
	require.NoError(t, err)
	assert.True(t, repo.lastList.UnassignedOnly)
	assert.Equal(t, int64(2), res.Total)
	codes := []string{res.Items[0].Code(), res.Items[1].Code()}
	assert.Equal(t, []string{"B1", "C1"}, codes)

	repo.listErr = errors.New("db")
	_, err = NewListGradeGroupsHandler(repo).Handle(ctx, ListGradeGroupsQuery{})
	require.EqualError(t, err, "db")
}

type exportFixture struct {
	rules  *fakeRules
	prices *fakePrices
	grades *fakeGrades
	loader *fakeLoader
	h      *ExportRulesHandler
}

func newExportFixture(t *testing.T) *exportFixture {
	t.Helper()
	f := &exportFixture{rules: newFakeRules(), prices: newFakePrices(), grades: newFakeGrades()}
	f.loader = &fakeLoader{rules: f.rules, prices: f.prices, grades: f.grades}
	seedRule(t, f.rules, "Type 1", "POY", "BC", "SPPTY", "0.05")
	r := seedRule(t, f.rules, "Type 3", "ITY", "NS", "COST", "1")
	require.NoError(t, f.rules.rows[r.ID()].Deactivate("u", fixedNow))
	f.prices.rows[domain.BasisSPPTY] = domain.ReconstructSellPrice(domain.BasisSPPTY, dec("1.95"), true, fixedNow, "s", nil, "")
	f.grades.add("A1", "Grade A1", gg(domain.GradeGroupBC))
	f.grades.add("B1", "Grade B1", nil)
	f.h = NewExportRulesHandler(f.rules, f.prices, f.grades, f.loader)
	f.h.now = clock
	return f
}

func TestExportRules_Workbook(t *testing.T) {
	f := newExportFixture(t)
	res, err := f.h.Handle(context.Background(), ExportRulesQuery{})
	require.NoError(t, err)
	assert.Equal(t, "erp_rules_20260929_100000.xlsx", res.FileName)
	assert.Len(t, res.RuleHash, 64)
	rs, err := f.loader.LoadRuleSet(context.Background())
	require.NoError(t, err)
	assert.Equal(t, rs.Hash(), res.RuleHash)

	x, err := excelize.OpenReader(bytes.NewReader(res.FileContent))
	require.NoError(t, err)
	defer func() { _ = x.Close() }()
	assert.Equal(t, []string{SheetVallossRules, SheetSellPrices, SheetGradeGroups, SheetRuleSet}, x.GetSheetList())

	rows, err := x.GetRows(SheetVallossRules)
	require.NoError(t, err)
	require.Len(t, rows, 2, "inactive rule excluded by default")
	assert.Equal(t, "cevr_val_loss", rows[0][6])
	assert.Equal(t, []string{"1", "1", "Type 1", "POY", "BC", "SPPTY", "0.050000", "TRUE"}, rows[1][:8])

	rows, err = x.GetRows(SheetSellPrices)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, "1.950000", rows[1][2])

	rows, err = x.GetRows(SheetGradeGroups)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, []string{"1", "A1", "Grade A1", "TRUE", "BC"}, rows[1])
	assert.Equal(t, "B1", rows[2][1])

	rows, err = x.GetRows(SheetRuleSet)
	require.NoError(t, err)
	assert.Equal(t, []string{"rule_hash", res.RuleHash}, rows[1])
	assert.Equal(t, []string{"active_rules", "1"}, rows[2])
	assert.Equal(t, []string{"generated_at", "2026-09-29 10:00:00"}, rows[5])
}

func TestExportRules_IncludeInactive(t *testing.T) {
	f := newExportFixture(t)
	res, err := f.h.Handle(context.Background(), ExportRulesQuery{IncludeInactive: true})
	require.NoError(t, err)
	x, err := excelize.OpenReader(bytes.NewReader(res.FileContent))
	require.NoError(t, err)
	defer func() { _ = x.Close() }()
	rows, err := x.GetRows(SheetVallossRules)
	require.NoError(t, err)
	require.Len(t, rows, 3)
	assert.Equal(t, "FALSE", rows[2][7])
	assert.Equal(t, "2026-09-29 10:00:00", rows[2][10])
	assert.Equal(t, 0, f.rules.lastList.PageSize, "export reads every row")
}

func TestExportRules_Errors(t *testing.T) {
	ctx := context.Background()
	f := newExportFixture(t)
	f.rules.listErr = errors.New("rules")
	_, err := f.h.Handle(ctx, ExportRulesQuery{})
	require.ErrorContains(t, err, "rules")

	f = newExportFixture(t)
	f.prices.listErr = errors.New("prices")
	_, err = f.h.Handle(ctx, ExportRulesQuery{})
	require.ErrorContains(t, err, "prices")

	f = newExportFixture(t)
	f.grades.listErr = errors.New("grades")
	_, err = f.h.Handle(ctx, ExportRulesQuery{})
	require.ErrorContains(t, err, "grades")

	f = newExportFixture(t)
	f.loader.loadErr = errors.New("snapshot")
	_, err = f.h.Handle(ctx, ExportRulesQuery{})
	require.ErrorContains(t, err, "snapshot")
}

func TestFormatTime(t *testing.T) {
	assert.Empty(t, formatTimePtr(nil))
	assert.Empty(t, formatTime(time.Time{}))
	assert.Equal(t, "2026-09-29 10:00:00", formatTimePtr(&fixedNow))
}
