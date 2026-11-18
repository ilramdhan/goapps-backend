package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

type fakeReportSrc struct {
	products []postgres.BackfillReportProduct
	fgIDs    []int64
	poy      map[int64]string
	cur      map[int64]postgres.BackfillCurrentSPCode
	batchMax int
}

func (f *fakeReportSrc) ListReportProducts(context.Context) ([]postgres.BackfillReportProduct, error) {
	return f.products, nil
}

func (f *fakeReportSrc) BackfillFgProductIDs(context.Context) ([]int64, error) { return f.fgIDs, nil }

func (f *fakeReportSrc) ActivePoyShades(_ context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	for _, id := range ids {
		if s, ok := f.poy[id]; ok {
			out[id] = s
		}
	}
	return out, nil
}

func (f *fakeReportSrc) CurrentSPCodes(_ context.Context, ids []int64) (map[int64]postgres.BackfillCurrentSPCode, error) {
	if len(ids) > f.batchMax {
		f.batchMax = len(ids)
	}
	out := map[int64]postgres.BackfillCurrentSPCode{}
	for _, id := range ids {
		if c, ok := f.cur[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

type fakeCandFinder struct {
	m        map[int64][]int64
	batchMax int
}

func (f *fakeCandFinder) FgCandidates(_ context.Context, ids []int64) (map[int64][]int64, error) {
	if len(ids) > f.batchMax {
		f.batchMax = len(ids)
	}
	out := map[int64][]int64{}
	for _, id := range ids {
		if c, ok := f.m[id]; ok {
			out[id] = c
		}
	}
	return out, nil
}

type fakeShadeFg struct {
	m   map[mbsource.ShadeFgKey]*mbspin.Entity
	err error
}

func (f *fakeShadeFg) FindByShadeAndFg(_ context.Context, keys []mbsource.ShadeFgKey) (map[mbsource.ShadeFgKey]*mbspin.Entity, error) {
	if f.err != nil {
		return nil, f.err
	}
	out := map[mbsource.ShadeFgKey]*mbspin.Entity{}
	for _, k := range keys {
		if e, ok := f.m[k]; ok {
			out[k] = e
		}
	}
	return out, nil
}

type fakeGetter map[uuid.UUID]*mbspin.Entity

func (f fakeGetter) GetByID(_ context.Context, id uuid.UUID) (*mbspin.Entity, error) {
	if e, ok := f[id]; ok {
		return e, nil
	}
	return nil, mbspin.ErrNotFound
}

type noParams struct{}

func (noParams) GetByFillGroup(context.Context, string) ([]*parameter.Parameter, error) {
	return nil, nil
}

func testSpin(t *testing.T, orion, shade string, active bool, fg *int64) *mbspin.Entity {
	t.Helper()
	o, s := orion, shade
	e := mbspin.Reconstruct(uuid.New(), nil, &o, uuid.New(), "DYE", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil,
		active, time.Now(), "t", nil, nil, nil, nil)
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{ShadeCode: &s, LDRType: "NOT_CALCULATED"})
	e.HydrateFg(fg, nil, nil, mbspin.FgSupersede{})
	return e
}

func prod(id int64, shade string) postgres.BackfillReportProduct {
	return postgres.BackfillReportProduct{ProductSysID: id, ProductCode: "P", ProductName: "N", TypeCode: "FDY", ShadeCode: shade}
}

func k(s string, fg int64) mbsource.ShadeFgKey { return mbsource.ShadeFgKey{Shade: s, FgProductID: fg} }

func TestDiffProduct_Rules(t *testing.T) {
	s1, s2 := uuid.New(), uuid.New()
	fg := int64(7)
	res := mbsource.Resolution{SPCode: "NEW", SpinID: &s1, FgProductID: &fg}
	cases := []struct {
		name    string
		cur     postgres.BackfillCurrentSPCode
		changed bool
		change  string
	}{
		{"no current value", postgres.BackfillCurrentSPCode{}, true, changeNew},
		{"same pointer", postgres.BackfillCurrentSPCode{Text: "OLD", SpinID: &s1}, false, ""},
		{"other pointer", postgres.BackfillCurrentSPCode{Text: "NEW", SpinID: &s2}, true, changeChange},
		{"no pointer same text", postgres.BackfillCurrentSPCode{Text: "NEW"}, false, ""},
		{"no pointer other text", postgres.BackfillCurrentSPCode{Text: "OLD"}, true, changeChange},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			row, changed := diffProduct(prod(1, "A"), tc.cur, res)
			assert.Equal(t, tc.changed, changed)
			if changed {
				assert.Equal(t, tc.change, row.Change)
				assert.Equal(t, int64(7), row.FgProductID)
				assert.Equal(t, "NEW", row.NewSPCode)
			}
		})
	}
}

func TestBuildReport_CurrentStateFiltersByFgSetAndDiffs(t *testing.T) {
	sA := testSpin(t, "SP-A", "A", true, ptr(int64(100)))
	src := &fakeReportSrc{
		products: []postgres.BackfillReportProduct{prod(1, "a"), prod(2, "A"), prod(3, "A"), prod(4, "A")},
		fgIDs:    []int64{100},
		cur: map[int64]postgres.BackfillCurrentSPCode{
			2: {Text: "OLD"},
			3: {Text: "SP-A"},
		},
	}
	cands := &fakeCandFinder{m: map[int64][]int64{1: {100}, 2: {100}, 3: {100}, 4: {999}}}
	d := reportDeps{src: src, cands: cands, spins: &fakeShadeFg{m: map[mbsource.ShadeFgKey]*mbspin.Entity{k("A", 100): sA}},
		getter: fakeGetter{}, params: noParams{}}
	rows, st, err := buildReport(context.Background(), d, nil, false)
	require.NoError(t, err)
	assert.Equal(t, 4, st.Products)
	assert.Equal(t, 3, st.Considered, "product 4 has no candidate in the FG set")
	require.Len(t, rows, 2)
	assert.Equal(t, int64(1), rows[0].Product.ProductSysID)
	assert.Equal(t, changeNew, rows[0].Change)
	assert.Equal(t, int64(2), rows[1].Product.ProductSysID)
	assert.Equal(t, changeChange, rows[1].Change)
	assert.Equal(t, 1, st.New)
	assert.Equal(t, 1, st.Change)
}

func TestBuildReport_EmptyFgSetNoWork(t *testing.T) {
	cands := &fakeCandFinder{}
	d := reportDeps{src: &fakeReportSrc{products: []postgres.BackfillReportProduct{prod(1, "A")}}, cands: cands,
		spins: &fakeShadeFg{}, getter: fakeGetter{}, params: noParams{}}
	rows, st, err := buildReport(context.Background(), d, nil, false)
	require.NoError(t, err)
	assert.Empty(t, rows)
	assert.Zero(t, st.Considered)
	assert.Zero(t, cands.batchMax, "no candidate query without an FG set")
}

func TestBuildReport_SimulationOverlaysOnlyValidApplyRows(t *testing.T) {
	ok := testSpin(t, "SIM-OK", "B", true, nil)
	inactive := testSpin(t, "INACT", "B", false, nil)
	linked := testSpin(t, "LINKED", "B", true, ptr(int64(9)))
	wrongShade := testSpin(t, "WS", "Z", true, nil)
	dup := testSpin(t, "DUP", "B", true, nil)
	getter := fakeGetter{ok.ID(): ok, inactive.ID(): inactive, linked.ID(): linked, wrongShade.ID(): wrongShade, dup.ID(): dup}
	src := &fakeReportSrc{
		products: []postgres.BackfillReportProduct{prod(10, "b")},
		poy:      map[int64]string{200: "B"},
	}
	entries := []applyEntry{
		{SpinID: ok.ID().String(), PoySys: 200},
		{SpinID: inactive.ID().String(), PoySys: 200},
		{SpinID: linked.ID().String(), PoySys: 200},
		{SpinID: wrongShade.ID().String(), PoySys: 200},
		{SpinID: dup.ID().String(), PoySys: 200},
		{SpinID: uuid.New().String(), PoySys: 200},
		{SpinID: ok.ID().String(), PoySys: 300},
		{Err: errors.New("bad")},
	}
	d := reportDeps{src: src, cands: &fakeCandFinder{m: map[int64][]int64{10: {200}}}, spins: &fakeShadeFg{},
		getter: getter, params: noParams{}}
	rows, st, err := buildReport(context.Background(), d, entries, true)
	require.NoError(t, err)
	assert.Equal(t, 1, st.Overlay)
	assert.Equal(t, map[string]int{
		overlaySkipNotLive: 1, overlaySkipFgSet: 1, overlaySkipShade: 1, overlaySkipDuplicate: 1,
		overlaySkipSpinMissing: 1, overlaySkipPoyInvalid: 1, overlaySkipMalformed: 1,
	}, st.OverlaySkp)
	require.Len(t, rows, 1)
	assert.Equal(t, "SIM-OK", rows[0].NewSPCode)
	assert.Equal(t, int64(200), rows[0].FgProductID)
	assert.Equal(t, changeNew, rows[0].Change)
}

func TestBuildReport_BatchesAtMost500(t *testing.T) {
	src := &fakeReportSrc{fgIDs: []int64{1}}
	cands := &fakeCandFinder{m: map[int64][]int64{}}
	for i := int64(1); i <= 1201; i++ {
		src.products = append(src.products, prod(i, "A"))
		cands.m[i] = []int64{1}
	}
	d := reportDeps{src: src, cands: cands, spins: &fakeShadeFg{}, getter: fakeGetter{}, params: noParams{}}
	_, st, err := buildReport(context.Background(), d, nil, false)
	require.NoError(t, err)
	assert.Equal(t, 1201, st.Considered)
	assert.Equal(t, reportBatchSize, cands.batchMax)
}

func TestWriteReportCSV_Header(t *testing.T) {
	var buf bytes.Buffer
	s := uuid.New()
	require.NoError(t, writeReportCSV(&buf, []reportRow{{Product: prod(5, "A"), NewSPCode: "X", NewSpinID: &s, FgProductID: 9, Change: changeNew}}))
	recs, err := csv.NewReader(&buf).ReadAll()
	require.NoError(t, err)
	require.Len(t, recs, 2)
	assert.Equal(t, reportHeader, recs[0])
	assert.Equal(t, []string{"5", "P", "N", "FDY", "A", "false", "", "", "X", s.String(), "9", changeNew}, recs[1])
}

func ptr[T any](v T) *T { return &v }
