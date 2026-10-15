package lookupsync

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// --- fakes ---

type fakeResolver struct {
	results map[string]FillResult // master|key|trigger
	calls   map[string]int
}

func (f *fakeResolver) Resolve(_ context.Context, master, key, trigger string) (FillResult, error) {
	k := master + "|" + key + "|" + trigger
	f.calls[k]++
	res, ok := f.results[k]
	if !ok {
		return FillResult{}, machine.ErrNotFound
	}
	return res, nil
}

type fakeChildren map[string][]*parameter.Parameter

func (f fakeChildren) GetByFillGroup(_ context.Context, code string) ([]*parameter.Parameter, error) {
	return f[code], nil
}

type fakeReader struct {
	rows        []TriggerRow
	states      map[ProductParamKey]ChildState
	orionCounts map[string]int
	gradeNames  map[string]string
}

func (f *fakeReader) ListTriggerRows(_ context.Context, _ Scope, after int64, limit int) ([]TriggerRow, error) {
	var out []TriggerRow
	seen := map[int64]struct{}{}
	for _, r := range f.rows {
		if r.ProductSysID <= after {
			continue
		}
		if _, ok := seen[r.ProductSysID]; !ok {
			if len(seen) == limit {
				break
			}
			seen[r.ProductSysID] = struct{}{}
		}
		out = append(out, r)
	}
	return out, nil
}

func (f *fakeReader) LoadChildStates(_ context.Context, products []int64, params []uuid.UUID) (map[ProductParamKey]ChildState, error) {
	out := map[ProductParamKey]ChildState{}
	for _, p := range products {
		for _, id := range params {
			k := ProductParamKey{ProductSysID: p, ParamID: id}
			if s, ok := f.states[k]; ok {
				out[k] = s
			}
		}
	}
	return out, nil
}

func (f *fakeReader) CountMBSpinsByOrionCode(_ context.Context, code string) (int, error) {
	return f.orionCounts[code], nil
}

func (f *fakeReader) ProductGradeCodeByName(_ context.Context, name string) (string, bool, error) {
	c, ok := f.gradeNames[name]
	return c, ok, nil
}

// --- helpers ---

func newParam(t *testing.T, code string, dt parameter.DataType, group string) *parameter.Parameter {
	t.Helper()
	c, err := parameter.NewCode(code)
	require.NoError(t, err)
	p, err := parameter.NewParameter(c, code+" name", "", dt, parameter.ParamCategoryInput, nil, nil, nil, nil,
		parameter.CostingMetadata{LookupFillGroupCode: group}, "test")
	require.NoError(t, err)
	return p
}

func num(s string) *StoredValue {
	d := decimal.RequireFromString(s)
	return &StoredValue{Numeric: &d}
}

func txt(s string) *StoredValue { return &StoredValue{Text: &s} }

type fixture struct {
	res    *fakeResolver
	kids   fakeChildren
	reader *fakeReader
	speed  *parameter.Parameter // MC_SPEED numeric child of MC_CODE
	label  *parameter.Parameter // MC_LABEL text child of MC_CODE
	rpm    *parameter.Parameter // MC_RPM numeric child of MC_CODE (cleared when absent)
}

func newFixture(t *testing.T) *fixture {
	f := &fixture{
		res:    &fakeResolver{results: map[string]FillResult{}, calls: map[string]int{}},
		kids:   fakeChildren{},
		reader: &fakeReader{states: map[ProductParamKey]ChildState{}, orionCounts: map[string]int{}, gradeNames: map[string]string{}},
	}
	f.speed = newParam(t, "MC_SPEED", parameter.DataTypeNumber, "MC_CODE")
	f.label = newParam(t, "MC_LABEL", parameter.DataTypeText, "MC_CODE")
	f.rpm = newParam(t, "MC_RPM", parameter.DataTypeNumber, "MC_CODE")
	f.kids["MC_CODE"] = []*parameter.Parameter{f.speed, f.label, f.rpm}
	return f
}

func (f *fixture) applicable(product int64, p *parameter.Parameter, v *StoredValue) {
	f.reader.states[ProductParamKey{ProductSysID: product, ParamID: p.ID()}] = ChildState{Stored: v}
}

func (f *fixture) planner() *Planner { return NewPlanner(f.res, f.kids, f.reader) }

func mcRow(product int64, key string) TriggerRow {
	return TriggerRow{ProductSysID: product, ProductCode: "P", TriggerParamCode: "MC_CODE", LookupMasterCode: MasterMachine, StoredKey: key}
}

func plan(t *testing.T, p *Planner) Plan {
	t.Helper()
	pl, err := p.PlanSync(context.Background(), Scope{})
	require.NoError(t, err)
	return pl
}

func changeByCode(it Item, code string) (ValueChange, bool) {
	for _, c := range it.Changes {
		if c.ParamCode == code {
			return c, true
		}
	}
	return ValueChange{}, false
}

// --- tests ---

func TestPlanSync_Machine_SetNumericTextAndClear(t *testing.T) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 1200.5}, Text: map[string]string{"MC_LABEL": "Fast"}}
	f.reader.rows = []TriggerRow{mcRow(1, "M1")}
	f.applicable(1, f.speed, num("1000"))
	f.applicable(1, f.label, nil)
	f.applicable(1, f.rpm, num("30"))

	pl := plan(t, f.planner())
	require.Len(t, pl.Items, 1)
	it := pl.Items[0]
	assert.Empty(t, it.SkipReason)
	require.Len(t, it.Changes, 3)

	c, _ := changeByCode(it, "MC_SPEED")
	assert.Equal(t, ChangeSet, c.Kind)
	assert.Equal(t, "1000", c.OldValue)
	assert.Equal(t, "1200.5", c.NewValue)
	require.NotNil(t, c.NewNumeric)

	c, _ = changeByCode(it, "MC_LABEL")
	assert.Equal(t, ChangeSet, c.Kind)
	assert.Equal(t, "Fast", c.NewValue)
	require.NotNil(t, c.NewText)

	c, _ = changeByCode(it, "MC_RPM")
	assert.Equal(t, ChangeClear, c.Kind)
	assert.Equal(t, "30", c.OldValue)
	assert.Empty(t, c.NewValue)

	assert.Equal(t, 1, pl.Summary.TotalProducts)
	assert.Equal(t, 1, pl.Summary.ProductsWithChanges)
	assert.Equal(t, 3, pl.Summary.TotalValueChanges)
	assert.Len(t, pl.Summary.Fingerprint, 64)
}

func TestPlanSync_UnchangedAndClearWithoutRow_NoChange(t *testing.T) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 1200.5}, Text: map[string]string{"MC_LABEL": "Fast"}}
	f.reader.rows = []TriggerRow{mcRow(1, "M1")}
	f.applicable(1, f.speed, num("1200.500000"))
	f.applicable(1, f.label, txt("Fast"))
	f.applicable(1, f.rpm, nil) // no row → nothing to clear

	pl := plan(t, f.planner())
	assert.Empty(t, pl.Items[0].Changes)
	assert.Equal(t, 0, pl.Summary.ProductsWithChanges)
	assert.Equal(t, 0, pl.Summary.TotalValueChanges)
}

func TestPlanSync_NonApplicableChildIgnored(t *testing.T) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 5, "MC_RPM": 7}}
	f.reader.rows = []TriggerRow{mcRow(1, "M1")}
	f.applicable(1, f.speed, nil) // only MC_SPEED applicable

	pl := plan(t, f.planner())
	require.Len(t, pl.Items[0].Changes, 1)
	assert.Equal(t, "MC_SPEED", pl.Items[0].Changes[0].ParamCode)
	assert.Equal(t, 2, pl.Summary.NotApplicableChildren)
}

func TestPlanSync_OilRateNeverTouched(t *testing.T) {
	f := newFixture(t)
	oil := newParam(t, "OIL_RATE", parameter.DataTypeNumber, "OPU_CODE")
	opu := newParam(t, "OPU", parameter.DataTypeNumber, "OPU_CODE")
	f.kids["OPU_CODE"] = []*parameter.Parameter{oil, opu}
	f.res.results["MB_HEAD|H1|OPU_CODE"] = FillResult{Numeric: map[string]float64{"OPU": 2}}
	f.reader.rows = []TriggerRow{{ProductSysID: 1, TriggerParamCode: "OPU_CODE", LookupMasterCode: MasterMBHead, StoredKey: "H1"}}
	f.applicable(1, oil, num("5")) // would be CLEARed if not excluded
	f.applicable(1, opu, num("1"))

	pl := plan(t, f.planner())
	require.Len(t, pl.Items[0].Changes, 1)
	assert.Equal(t, "OPU", pl.Items[0].Changes[0].ParamCode)
}

func TestPlanSync_LockedSkipped(t *testing.T) {
	f := newFixture(t)
	row := mcRow(1, "M1")
	row.IsLocked = true
	f.reader.rows = []TriggerRow{row}
	f.applicable(1, f.speed, num("1"))

	pl := plan(t, f.planner())
	assert.Equal(t, SkipLocked, pl.Items[0].SkipReason)
	assert.Empty(t, pl.Items[0].Changes)
	assert.Equal(t, 1, pl.Summary.LockedProducts)
	assert.Equal(t, 1, pl.Summary.SkipCounts[SkipLocked])
	assert.Empty(t, f.res.calls, "locked products must not be resolved")
}

func TestPlanSync_KeyEmptyAndNotFound(t *testing.T) {
	f := newFixture(t)
	f.reader.rows = []TriggerRow{mcRow(1, ""), mcRow(2, "GONE")}
	pl := plan(t, f.planner())
	assert.Equal(t, SkipKeyEmpty, pl.Items[0].SkipReason)
	assert.Equal(t, SkipKeyNotFound, pl.Items[1].SkipReason)
	assert.Equal(t, 1, pl.Summary.SkipCounts[SkipKeyEmpty])
	assert.Equal(t, 1, pl.Summary.SkipCounts[SkipKeyNotFound])
}

func TestPlanSync_EachMasterRoutesByCode(t *testing.T) {
	for _, master := range SyncableMasters {
		if master == MasterMBSpin || master == MasterProductGrade {
			continue // special-cased below
		}
		t.Run(master, func(t *testing.T) {
			f := newFixture(t)
			f.res.results[master+"|K|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 3}}
			row := mcRow(1, "K")
			row.LookupMasterCode = master
			f.reader.rows = []TriggerRow{row}
			f.applicable(1, f.speed, nil)
			pl := plan(t, f.planner())
			require.Len(t, pl.Items[0].Changes, 1)
			assert.Equal(t, "3", pl.Items[0].Changes[0].NewValue)
		})
	}
}

func gradeFixture(t *testing.T) (*fixture, *parameter.Parameter) {
	f := newFixture(t)
	bc := newParam(t, "BC_PERC", parameter.DataTypeNumber, "PG_CODE")
	f.kids["PG_CODE"] = []*parameter.Parameter{bc}
	f.res.results["PRODUCT_GRADE|A|PG_CODE"] = FillResult{Numeric: map[string]float64{"BC_PERC": 1.25}}
	f.applicable(1, bc, nil)
	return f, bc
}

func TestPlanSync_ProductGrade_CodeThenLegacyName(t *testing.T) {
	f, _ := gradeFixture(t)
	f.reader.gradeNames["Grade A"] = "A"
	f.reader.rows = []TriggerRow{
		{ProductSysID: 1, TriggerParamCode: "PG_CODE", LookupMasterCode: MasterProductGrade, StoredKey: "A"},
		{ProductSysID: 2, TriggerParamCode: "PG_CODE", LookupMasterCode: MasterProductGrade, StoredKey: "Grade A"},
		{ProductSysID: 3, TriggerParamCode: "PG_CODE", LookupMasterCode: MasterProductGrade, StoredKey: "Unknown"},
	}
	for _, p := range []int64{2, 3} {
		f.reader.states[ProductParamKey{ProductSysID: p, ParamID: f.kids["PG_CODE"][0].ID()}] = ChildState{}
	}
	pl := plan(t, f.planner())
	require.Len(t, pl.Items, 3)
	assert.Equal(t, "1.25", pl.Items[0].Changes[0].NewValue)
	assert.Empty(t, pl.Items[1].SkipReason)
	assert.Equal(t, "1.25", pl.Items[1].Changes[0].NewValue)
	assert.Equal(t, "Grade A", pl.Items[1].MasterKey)
	assert.Equal(t, SkipKeyNotFound, pl.Items[2].SkipReason)
}

func mbSpinFixture(t *testing.T) (*fixture, *parameter.Parameter, uuid.UUID) {
	f := newFixture(t)
	rate := newParam(t, "MB_RATE", parameter.DataTypeNumber, "MB_SP_CODE")
	f.kids["MB_SP_CODE"] = []*parameter.Parameter{rate}
	id := uuid.New()
	f.res.results["MB_SPIN|"+id.String()+"|MB_SP_CODE"] = FillResult{Numeric: map[string]float64{"MB_RATE": 9}}
	f.res.results["MB_SPIN|ORI1|MB_SP_CODE"] = FillResult{Numeric: map[string]float64{"MB_RATE": 1}}
	f.reader.orionCounts["ORI1"] = 2
	return f, rate, id
}

func TestPlanSync_MBSpin_IDWinsOverText(t *testing.T) {
	f, rate, id := mbSpinFixture(t)
	f.reader.rows = []TriggerRow{{ProductSysID: 1, TriggerParamCode: "MB_SP_CODE", LookupMasterCode: MasterMBSpin, StoredKey: "ORI1", MBSpinID: &id}}
	f.applicable(1, rate, nil)
	pl := plan(t, f.planner())
	assert.Empty(t, pl.Items[0].SkipReason)
	require.Len(t, pl.Items[0].Changes, 1)
	assert.Equal(t, "9", pl.Items[0].Changes[0].NewValue)
	assert.Zero(t, f.res.calls["MB_SPIN|ORI1|MB_SP_CODE"])
}

func TestPlanSync_MBSpin_AmbiguousTextWithoutID(t *testing.T) {
	f, rate, _ := mbSpinFixture(t)
	f.reader.orionCounts["ORI2"] = 1
	f.res.results["MB_SPIN|ORI2|MB_SP_CODE"] = FillResult{Numeric: map[string]float64{"MB_RATE": 4}}
	f.reader.rows = []TriggerRow{
		{ProductSysID: 1, TriggerParamCode: "MB_SP_CODE", LookupMasterCode: MasterMBSpin, StoredKey: "ORI1"},
		{ProductSysID: 2, TriggerParamCode: "MB_SP_CODE", LookupMasterCode: MasterMBSpin, StoredKey: "ORI2"},
	}
	f.applicable(1, rate, nil)
	f.applicable(2, rate, nil)
	pl := plan(t, f.planner())
	assert.Equal(t, SkipMBSpinAmbiguous, pl.Items[0].SkipReason)
	assert.Equal(t, 1, pl.Summary.SkipCounts[SkipMBSpinAmbiguous])
	assert.Empty(t, pl.Items[1].SkipReason)
	assert.Equal(t, "4", pl.Items[1].Changes[0].NewValue)
}

func TestPlanSync_MemoizesResolvePerMasterKeyTrigger(t *testing.T) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 1}}
	for p := int64(1); p <= 5; p++ {
		f.reader.rows = append(f.reader.rows, mcRow(p, "M1"))
		f.applicable(p, f.speed, nil)
	}
	f.reader.rows = append(f.reader.rows, mcRow(6, "GONE"), mcRow(7, "GONE"))
	pl, err := f.planner().WithPageSize(2).PlanSync(context.Background(), Scope{})
	require.NoError(t, err)
	assert.Equal(t, 7, pl.Summary.TotalProducts)
	assert.Equal(t, 5, pl.Summary.ProductsWithChanges)
	assert.Equal(t, 1, f.res.calls["MACHINE|M1|MC_CODE"])
	assert.Equal(t, 1, f.res.calls["MACHINE|GONE|MC_CODE"], "not-found is memoized too")
}

func TestPlanSync_FingerprintStableAndOrderIndependent(t *testing.T) {
	build := func(order []int64, pageSize int) string {
		f := newFixture(t)
		f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 2}, Text: map[string]string{"MC_LABEL": "x"}}
		for _, p := range order {
			f.reader.rows = append(f.reader.rows, mcRow(p, "M1"))
			f.applicable(p, f.speed, nil)
			f.applicable(p, f.label, nil)
		}
		pl, err := f.planner().WithPageSize(pageSize).PlanSync(context.Background(), Scope{})
		require.NoError(t, err)
		return pl.Summary.Fingerprint
	}
	// Product order and page size do not matter (param ids differ per fixture,
	// so compare within one fixture's params via digests below).
	a := []byte("1|p|SET|N:2")
	b := []byte("2|p|SET|N:2")
	assert.Equal(t, fingerprint([][]byte{a, b}), fingerprint([][]byte{b, a}))
	assert.NotEqual(t, fingerprint([][]byte{a}), fingerprint([][]byte{a, b}))
	assert.Len(t, build([]int64{1, 2, 3}, 1), 64)

	// Same fixture planned twice with different page sizes → same fingerprint.
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 2}}
	for p := int64(1); p <= 4; p++ {
		f.reader.rows = append(f.reader.rows, mcRow(p, "M1"))
		f.applicable(p, f.speed, num("1"))
	}
	p1, err := f.planner().WithPageSize(1).PlanSync(context.Background(), Scope{})
	require.NoError(t, err)
	p2, err := f.planner().WithPageSize(100).PlanSync(context.Background(), Scope{})
	require.NoError(t, err)
	assert.Equal(t, p1.Summary.Fingerprint, p2.Summary.Fingerprint)

	// Numeric normalization: 2 and 2.000000 hash identically.
	d1, d2 := decimal.RequireFromString("2"), decimal.RequireFromString("2.000000")
	id := uuid.New()
	assert.Equal(t,
		changeDigest(1, ValueChange{ParamID: id, Kind: ChangeSet, NewNumeric: &d1}),
		changeDigest(1, ValueChange{ParamID: id, Kind: ChangeSet, NewNumeric: &d2}))

	// A different value changes the fingerprint.
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 3}}
	p3, err := f.planner().PlanSync(context.Background(), Scope{})
	require.NoError(t, err)
	assert.NotEqual(t, p1.Summary.Fingerprint, p3.Summary.Fingerprint)
}

func TestPlanChunk_ForReader_UsesGivenReader(t *testing.T) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 2}}
	other := &fakeReader{states: map[ProductParamKey]ChildState{}, rows: []TriggerRow{mcRow(9, "M1")}}
	other.states[ProductParamKey{ProductSysID: 9, ParamID: f.speed.ID()}] = ChildState{}
	pl, err := f.planner().ForReader(other).PlanChunk(context.Background(), Scope{}, []int64{9})
	require.NoError(t, err)
	require.Len(t, pl.Items, 1)
	assert.Equal(t, int64(9), pl.Items[0].ProductSysID)
}

func TestWalk_NilVisitReturnsSummary(t *testing.T) {
	f := newFixture(t)
	f.res.results["MACHINE|M1|MC_CODE"] = FillResult{Numeric: map[string]float64{"MC_SPEED": 2}}
	f.reader.rows = []TriggerRow{mcRow(1, "M1")}
	f.applicable(1, f.speed, nil)
	sum, err := f.planner().Walk(context.Background(), Scope{}, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, sum.TotalValueChanges)
}
