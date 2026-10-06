package erpintegration

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/xuri/excelize/v2"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// fakeReadinessSource is an in-memory LinkReadinessSource.
type fakeReadinessSource struct {
	linked     []LinkedAxProduct
	unlinked   []UnlinkedAxProduct
	replica    int64
	gotPeriod  string
	gotShades  []string
	linkedErr  error
	unlinkErr  error
	replicaErr error
}

func (f *fakeReadinessSource) ListLinkedAxProducts(_ context.Context, period string) ([]LinkedAxProduct, error) {
	f.gotPeriod = period
	return f.linked, f.linkedErr
}

func (f *fakeReadinessSource) ListUnlinkedAxProducts(_ context.Context, shades []string) ([]UnlinkedAxProduct, error) {
	f.gotShades = shades
	return f.unlinked, f.unlinkErr
}

func (f *fakeReadinessSource) CountReplicaItems(context.Context) (int64, error) {
	return f.replica, f.replicaErr
}

// readinessBatches extends memBatches with a real ListByPeriod.
type readinessBatches struct {
	*memBatches
	order []int64
}

func (r *readinessBatches) ListByPeriod(ctx context.Context, period string) ([]*domain.Batch, error) {
	var out []*domain.Batch
	for _, id := range r.order {
		b, err := r.GetByID(ctx, id)
		if err != nil {
			return nil, err
		}
		if b.Period() == period {
			out = append(out, b)
		}
	}
	return out, nil
}

// memDemandRepo is an in-memory DemandRepository.
type memDemandRepo struct{ lines map[int64][]domain.DemandLine }

func (m *memDemandRepo) Replace(_ context.Context, id int64, l []domain.DemandLine) (int64, error) {
	m.lines[id] = l
	return int64(len(l)), nil
}

func (m *memDemandRepo) List(_ context.Context, id int64) ([]domain.DemandLine, error) {
	return m.lines[id], nil
}

func (m *memDemandRepo) Count(_ context.Context, id int64) (int64, error) {
	return int64(len(m.lines[id])), nil
}

func dl(item, name, grade, shade, qty string) domain.DemandLine {
	return domain.DemandLine{
		Period: tPeriod, Kind: domain.ItemKindForCode(item), ItemCode: item, ItemName: name,
		GradeCode: grade, ShadeCode: shade, QtyKg: decimal.RequireFromString(qty),
	}
}

func fullAttrs() domain.AttrValues {
	return domain.AttrValues{FgType: "DTY", ChpItemCode: "CHP1", MsBatchItem: "1", ItemType: "DTY", PrdPerDay: "10"}
}

func lp(id int64, item, shade, typ string) LinkedAxProduct {
	return LinkedAxProduct{
		SysID: id, ProductCode: "P" + decimal.NewFromInt(id).String(), ItemCode: item, ShadeCode: shade,
		TypeCode: typ, Attrs: fullAttrs(), InReplica: true,
	}
}

type readinessFixture struct {
	h       *LinkReadinessHandler
	src     *fakeReadinessSource
	batches *readinessBatches
	demand  *memDemandRepo
}

func newReadinessFixture(src *fakeReadinessSource) *readinessFixture {
	b := &readinessBatches{memBatches: newMemBatches()}
	d := &memDemandRepo{lines: map[int64][]domain.DemandLine{}}
	h := NewLinkReadinessHandler(b, d, src).WithClock(func() time.Time { return testNow })
	return &readinessFixture{h: h, src: src, batches: b, demand: d}
}

func (f *readinessFixture) batch(period string, lines ...domain.DemandLine) int64 {
	id := f.batches.put(domain.BatchState{Period: period, Status: domain.StatusDraft, CreatedAt: testNow, UpdatedAt: testNow})
	f.batches.order = append([]int64{id}, f.batches.order...) // newest first
	for i := range lines {
		lines[i].BatchID = id
	}
	f.demand.lines[id] = lines
	return id
}

func TestLinkReadiness_AllSections(t *testing.T) {
	src := &fakeReadinessSource{
		replica: 100,
		linked: []LinkedAxProduct{
			lp(1, "DTY0000001", "S1", "DTY"), // mapped, clean
			lp(2, "TRIAL-X", "S9", "DTY"),    // dup (trial)
			lp(3, "TRIAL-X", "s9 ", "DTY"),   // dup (trial), normalized key
			lp(4, "POY0000275", "X419T", "POY"),
			lp(5, "POY0000275", "X419T", "POY"), // dup, in demand
			lp(6, "ACY0000068", "", "ACY"),      // no shade
			lp(7, "ACY0000068", "", "ACY"),      // no shade
			lp(8, "CMB0000010", "M1", "DTY"),    // §9f CMB on non-MB
			lp(9, "DTY0000009", "S3", "MB"),     // §9f MB on yarn
		},
		unlinked: []UnlinkedAxProduct{
			{SysID: 20, ProductCode: "U20", ProductName: "DTY 150/48 RAW WHITE", ShadeCode: "S2", TypeCode: "DTY"},
			{SysID: 21, ProductCode: "U21", ProductName: "DTY 150/48 RAW WHITE", ShadeCode: "S2", TypeCode: "MB"}, // V-12 excluded
			{SysID: 22, ProductCode: "U22", ProductName: "UNRELATED NAME", ShadeCode: "S2", TypeCode: "DTY"},      // low score
		},
	}
	src.linked[0].Attrs.FgType = "" // §9c blocking
	src.linked[5].ActualRows = 2
	src.linked[8].InReplica = false
	f := newReadinessFixture(src)
	id := f.batch(tPeriod,
		dl("DTY0000001", "DTY 1", "AX", "S1", "10"),
		dl("DTY0000001", "DTY 1", "AB", "S1", "5"),
		dl("DTY0000002", "DTY 150/48 RAW WHITE", "AX", "S2", "7"), // §9b LINK_CANDIDATE
		dl("DTY0000003", "NOTHING ALIKE", "AX", "S7", "0"),        // §9b CREATE_NEW qty 0
		dl("POY0000275", "POY", "AX", "X419T", "3"),               // dup in demand
		dl("ACY0000068", "ACY", "AX", "Z", "1"),                   // no product (other shade)
	)

	rep, err := f.h.Handle(context.Background(), LinkReadinessQuery{BatchID: id})
	require.NoError(t, err)
	assert.Equal(t, tPeriod, rep.Period)
	assert.Equal(t, tPeriod, src.gotPeriod)
	assert.True(t, rep.DemandAvailable)
	assert.Equal(t, testNow, rep.GeneratedAt)

	// §9b / §9d
	require.Len(t, rep.NoProduct, 3)
	byItem := map[string]NoProductLine{}
	for _, l := range rep.NoProduct {
		byItem[l.ItemCode] = l
	}
	cand := byItem["DTY0000002"]
	assert.Equal(t, SuggestLinkCandidate, cand.Suggestion)
	require.Len(t, cand.Candidates, 1)
	assert.Equal(t, int64(20), cand.Candidates[0].SysID)
	assert.Equal(t, 100, cand.Candidates[0].Score)
	assert.Equal(t, SuggestCreateNew, byItem["DTY0000003"].Suggestion)
	acy := byItem["ACY0000068"]
	require.Len(t, acy.SameItemOtherShade, 2)
	assert.Equal(t, []string{"S2", "S7", "Z"}, src.gotShades)

	// §9c: product 1 missing fg_type (blocking); no other mapped demand product has gaps.
	require.Len(t, rep.AttributeGaps, 1)
	g := rep.AttributeGaps[0]
	assert.Equal(t, int64(1), g.SysID)
	assert.True(t, g.MissingFgType)
	assert.True(t, g.Blocking)
	assert.Equal(t, []domain.AttrField{domain.AttrFgType}, g.Missing)
	assert.Equal(t, "15", g.QtyKg.String())

	// §9e
	require.Len(t, rep.Duplicates, 3)
	assert.Equal(t, "ACY0000068", rep.Duplicates[0].ItemCode)
	assert.Equal(t, "POY0000275", rep.Duplicates[1].ItemCode)
	assert.True(t, rep.Duplicates[1].InDemand)
	assert.Equal(t, "3", rep.Duplicates[1].DemandQtyKg.String())
	assert.Equal(t, []ProductRef{{SysID: 4, ProductCode: "P4", ShadeCode: "X419T"}, {SysID: 5, ProductCode: "P5", ShadeCode: "X419T"}}, rep.Duplicates[1].Products)
	assert.True(t, rep.Duplicates[2].Trial)
	assert.False(t, rep.Duplicates[1].Trial)

	// linked but no shade
	require.Len(t, rep.LinkedNoShade, 2)
	assert.Equal(t, 2, rep.LinkedNoShade[0].ActualRows)
	assert.Equal(t, 2, rep.LinkedNoShade[0].SameItemNoShade)
	assert.True(t, rep.LinkedNoShade[0].ItemInDemand)

	// §9f
	require.Len(t, rep.TypeMismatches, 2)
	assert.Equal(t, MismatchCmbOnNonMB, rep.TypeMismatches[0].Direction)
	assert.Equal(t, MismatchMBOnYarn, rep.TypeMismatches[1].Direction)

	s := rep.Summary
	assert.Equal(t, 5, s.DemandCombos)
	assert.Equal(t, 1, s.MappedCombos)
	assert.Equal(t, 2, s.NoProductWithQty)
	assert.Equal(t, 1, s.LinkCandidates)
	assert.Equal(t, 2, s.CreateNew)
	assert.Equal(t, 3, s.DuplicateGroups)
	assert.Equal(t, 6, s.DuplicateProducts)
	assert.Equal(t, 1, s.TrialDuplicateGroups)
	assert.Equal(t, 1, s.DemandDuplicates)
	assert.Equal(t, 1, s.LinkedNoShadeActual)
	assert.Equal(t, GapScopeDemand, s.GapScope)
	assert.Equal(t, "26", s.DemandQtyKg.String())
	assert.False(t, s.Ready)
	assert.False(t, s.UniqueIndexReady)
}

func TestLinkReadiness_CleanPasses(t *testing.T) {
	src := &fakeReadinessSource{replica: 1, linked: []LinkedAxProduct{lp(1, "DTY0000001", "S1", "DTY"), lp(2, "CMB0000001", "M1", "MB")}}
	f := newReadinessFixture(src)
	f.batch(tPeriod, dl("DTY0000001", "a", "AX", "S1", "1"), dl("CMB0000001", "b", "A", "M1", "2"), dl("DTY0000005", "c", "AX", "S5", "0"))
	rep, err := f.h.Handle(context.Background(), LinkReadinessQuery{Period: tPeriod})
	require.NoError(t, err)
	assert.True(t, rep.Summary.Pass.NoProductWithQty) // only a qty-0 no-product row
	assert.True(t, rep.Summary.Ready)
	assert.True(t, rep.Summary.UniqueIndexReady)
	assert.Len(t, rep.NoProduct, 1)
}

func TestLinkReadiness_PeriodPicksNewestBatchWithDemand(t *testing.T) {
	src := &fakeReadinessSource{}
	f := newReadinessFixture(src)
	older := f.batch(tPeriod, dl("DTY0000001", "a", "AX", "S1", "1"))
	f.batch(tPeriod) // newest, no demand
	rep, err := f.h.Handle(context.Background(), LinkReadinessQuery{Period: tPeriod})
	require.NoError(t, err)
	assert.Equal(t, older, rep.BatchID)
	assert.Len(t, rep.NoProduct, 1)
}

func TestLinkReadiness_NoDemandScopesAllLinked(t *testing.T) {
	p := lp(1, "DTY0000001", "S1", "DTY")
	p.Attrs.ChpItemCode = ""
	src := &fakeReadinessSource{linked: []LinkedAxProduct{p}}
	f := newReadinessFixture(src)
	rep, err := f.h.Handle(context.Background(), LinkReadinessQuery{Period: tPeriod})
	require.NoError(t, err)
	assert.False(t, rep.DemandAvailable)
	assert.Equal(t, GapScopeAll, rep.Summary.GapScope)
	require.Len(t, rep.AttributeGaps, 1)
	assert.False(t, rep.AttributeGaps[0].Blocking) // non-fg attribute only; replica empty
	assert.True(t, rep.Summary.Ready)
	assert.Nil(t, src.gotShades)
}

func TestLinkReadiness_Validation(t *testing.T) {
	f := newReadinessFixture(&fakeReadinessSource{})
	_, err := f.h.Handle(context.Background(), LinkReadinessQuery{})
	require.ErrorIs(t, err, ErrInvalidLinkReadinessQuery)
	_, err = f.h.Handle(context.Background(), LinkReadinessQuery{BatchID: -1})
	require.ErrorIs(t, err, ErrInvalidLinkReadinessQuery)
	_, err = f.h.Handle(context.Background(), LinkReadinessQuery{BatchID: 99})
	require.ErrorIs(t, err, domain.ErrBatchNotFound)
	id := f.batch(tPeriod)
	_, err = f.h.Handle(context.Background(), LinkReadinessQuery{BatchID: id, Period: "202601"})
	require.ErrorIs(t, err, ErrInvalidLinkReadinessQuery)

	var nilH *LinkReadinessHandler
	_, err = nilH.Handle(context.Background(), LinkReadinessQuery{Period: tPeriod})
	require.ErrorIs(t, err, ErrLinkReadinessNotConfigured)
	_, err = NewLinkReadinessHandler(nil, nil, &fakeReadinessSource{}).Handle(context.Background(), LinkReadinessQuery{BatchID: 1})
	require.ErrorIs(t, err, ErrLinkReadinessNotConfigured)
	rep, err := NewLinkReadinessHandler(nil, nil, &fakeReadinessSource{}).Handle(context.Background(), LinkReadinessQuery{Period: tPeriod})
	require.NoError(t, err)
	assert.False(t, rep.DemandAvailable)
}

func TestLinkReadiness_SourceErrors(t *testing.T) {
	boom := errors.New("boom")
	for name, src := range map[string]*fakeReadinessSource{
		"linked":   {linkedErr: boom},
		"replica":  {replicaErr: boom},
		"unlinked": {unlinkErr: boom},
	} {
		t.Run(name, func(t *testing.T) {
			f := newReadinessFixture(src)
			f.batch(tPeriod, dl("DTY0000001", "a", "AX", "S1", "1"))
			_, err := f.h.Handle(context.Background(), LinkReadinessQuery{Period: tPeriod})
			require.ErrorIs(t, err, boom)
		})
	}
}

func TestJaccardAndTokens(t *testing.T) {
	assert.Equal(t, 0, jaccardPercent(nameTokens(""), nameTokens("A B")))
	assert.Equal(t, 66, jaccardPercent(nameTokens("DTY 150/48"), nameTokens("dty-150 x")))
	assert.True(t, typeCompatible(domain.ItemKindMB, " mb "))
	assert.False(t, typeCompatible(domain.ItemKindYarn, "MB"))
}

func TestLinkReadiness_Export(t *testing.T) {
	src := &fakeReadinessSource{
		replica: 1,
		linked:  []LinkedAxProduct{lp(1, "TRIAL-A", "S1", "DTY"), lp(2, "TRIAL-A", "S1", "DTY"), lp(3, "CMB1", "", "DTY")},
		unlinked: []UnlinkedAxProduct{
			{SysID: 9, ProductCode: "U9", ProductName: "NEW ITEM", ShadeCode: "S2", TypeCode: "DTY"},
		},
	}
	src.linked[2].InReplica = false
	f := newReadinessFixture(src)
	f.batch(tPeriod, dl("DTY0000002", "NEW ITEM", "AX", "S2", "1.5"), dl("CMB1", "m", "A", "", "2"))
	out, err := f.h.Export(context.Background(), LinkReadinessQuery{Period: tPeriod})
	require.NoError(t, err)
	assert.Equal(t, "erp_link_readiness_202607_20260929_090000.xlsx", out.FileName)

	x, err := excelize.OpenReader(bytes.NewReader(out.FileContent))
	require.NoError(t, err)
	t.Cleanup(func() { _ = x.Close() })
	assert.Equal(t, []string{SheetReadinessSummary, SheetReadinessNoProduct, SheetReadinessAttrGaps, SheetReadinessDups, SheetReadinessNoShade, SheetReadinessTypes}, x.GetSheetList())
	rows, err := x.GetRows(SheetReadinessDups)
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, []string{"1", "TRIAL-A", "S1", "2", "1,2", "P1,P2", "TRUE", "FALSE", "0.0000000"}, rows[1])
	np, err := x.GetRows(SheetReadinessNoProduct)
	require.NoError(t, err)
	require.Len(t, np, 2)
	assert.Equal(t, "1.5000000", np[1][6])
	assert.Equal(t, "9:U9(DTY,100%)", np[1][8])
	gaps, err := x.GetRows(SheetReadinessAttrGaps)
	require.NoError(t, err)
	require.Len(t, gaps, 2) // CMB1 not in replica
	types, err := x.GetRows(SheetReadinessTypes)
	require.NoError(t, err)
	require.Len(t, types, 2)
	assert.Equal(t, string(MismatchCmbOnNonMB), types[1][6])

	_, err = RenderLinkReadinessXLSX(nil)
	require.Error(t, err)
}
