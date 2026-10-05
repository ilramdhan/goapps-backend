package erpintegration

import (
	"context"
	"errors"
	"strings"
	"testing"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

type fakeLegacyReader struct {
	rows    []domain.LegacyStdRow
	err     error
	periods []string
}

func (f *fakeLegacyReader) List(_ context.Context, period string) ([]domain.LegacyStdRow, error) {
	f.periods = append(f.periods, period)
	return f.rows, f.err
}

// fakeBackfillRepo models the real repository: FillNullAttributes fills only
// columns that are still empty and reports stale when the key changed.
type fakeBackfillRepo struct {
	products []domain.LinkedProductAttrs
	listErr  error
	fillErr  error
	writes   []domain.AttrFillWrite
	staleIDs map[int64]bool
	// current overrides the stored attrs at write time (concurrent edits).
	current map[int64]domain.AttrValues
}

func (r *fakeBackfillRepo) ListLinkedProducts(context.Context) ([]domain.LinkedProductAttrs, error) {
	return r.products, r.listErr
}

func (r *fakeBackfillRepo) FillNullAttributes(_ context.Context, w domain.AttrFillWrite) (domain.AttrFillResult, error) {
	r.writes = append(r.writes, w)
	if r.fillErr != nil {
		return domain.AttrFillResult{}, r.fillErr
	}
	var before domain.AttrValues
	for _, p := range r.products {
		if p.ProductSysID == w.ProductSysID {
			before = p.Attrs
		}
	}
	if c, ok := r.current[w.ProductSysID]; ok {
		before = c
	}
	res := domain.AttrFillResult{Before: before, After: before}
	if r.staleIDs[w.ProductSysID] {
		res.Stale = true
		return res, nil
	}
	for _, f := range domain.AllAttrFields() {
		if v := w.Values.Get(f); v != "" && before.Get(f) == "" {
			res.After = res.After.With(f, v)
			res.Filled = append(res.Filled, f)
		}
	}
	return res, nil
}

func legacyFixture() []domain.LegacyStdRow {
	return []domain.LegacyStdRow{
		// P1: full AX row plus a derived BX row (ignored).
		{ItemCode: "FGX0001", GradeCode: "AX", ShadeCode: "sh01", ItemType: "FGX", FgType: "DTY", ChpItemCode: "CHPX001", MsBatchItem: "MB0001", PrdPerDay: "1250.50"},
		{ItemCode: "FGX0001", GradeCode: "BX", ShadeCode: "SH01", ItemType: "FGX", FgType: "OTHER"},
		// P2: duplicate AX rows that disagree on fg_type, agree on prd (numerically).
		{ItemCode: "FGX0002", GradeCode: "AX", ItemType: "FGX", FgType: "DTY", PrdPerDay: "10"},
		{ItemCode: "FGX0002", GradeCode: "", ItemType: "FGX", FgType: "FDY", PrdPerDay: "10.000"},
		// P3: MB item, grade A only.
		{ItemCode: "CMBX001", GradeCode: "A", ShadeCode: "NL", ItemType: "CMB", FgType: "MB"},
		// P4: ms batch too long for VARCHAR(12).
		{ItemCode: "FGX0004", GradeCode: "AX", ShadeCode: "S4", MsBatchItem: "MB0123456789XYZ", ItemType: "FGX"},
		// Unlinked in PG: must never be touched.
		{ItemCode: "FGX0099", GradeCode: "AX", ShadeCode: "S9", FgType: "DTY"},
	}
}

func productsFixture() []domain.LinkedProductAttrs {
	return []domain.LinkedProductAttrs{
		{ProductSysID: 1, ProductCode: "P1", ErpItemCode: "fgx0001", ShadeCode: "SH01 ",
			Attrs: domain.AttrValues{FgType: "KEEP"}},
		{ProductSysID: 2, ProductCode: "P2", ErpItemCode: "FGX0002"},
		{ProductSysID: 3, ProductCode: "P3", ErpItemCode: "CMBX001", ShadeCode: "nl"},
		{ProductSysID: 4, ProductCode: "P4", ErpItemCode: "FGX0004", ShadeCode: "S4",
			Attrs: domain.AttrValues{ItemType: "FGX"}},
		{ProductSysID: 5, ProductCode: "P5", ErpItemCode: "FGX0005", ShadeCode: "S5"},
	}
}

func fieldOutcome(t *testing.T, p BackfillProduct, f domain.AttrField) BackfillField {
	t.Helper()
	for _, bf := range p.Fields {
		if bf.Field == f {
			return bf
		}
	}
	t.Fatalf("product %d has no field %s", p.ProductSysID, f)
	return BackfillField{}
}

func TestBackfillAttributes_DryRun_ZeroWritesAndReport(t *testing.T) {
	reader := &fakeLegacyReader{rows: legacyFixture()}
	repo := &fakeBackfillRepo{products: productsFixture()}
	audit := &recordingAudit{}
	h := NewBackfillAttributesHandler(reader, repo, audit)

	rep, err := h.Handle(context.Background(), BackfillAttributesCommand{Period: "202608"})
	if err != nil {
		t.Fatal(err)
	}
	if len(repo.writes) != 0 || len(audit.inputs) != 0 {
		t.Fatalf("dry run wrote: writes=%d audits=%d", len(repo.writes), len(audit.inputs))
	}
	if rep.Mode != BackfillModeDryRun || reader.periods[0] != "202608" {
		t.Errorf("mode=%s periods=%v", rep.Mode, reader.periods)
	}
	p1, p2, p3, p4, p5 := rep.Products[0], rep.Products[1], rep.Products[2], rep.Products[3], rep.Products[4]

	if p1.Outcome != ProductWouldFill || p1.SourceGrade != "AX" || p1.SourceRows != 1 {
		t.Errorf("p1 = %+v", p1)
	}
	if bf := fieldOutcome(t, p1, domain.AttrFgType); bf.Outcome != FieldDiffers || bf.Current != "KEEP" || bf.Legacy != "DTY" {
		t.Errorf("p1 fg_type = %+v (existing value must be kept)", bf)
	}
	if bf := fieldOutcome(t, p1, domain.AttrPrdPerDay); bf.Outcome != FieldFill || bf.Legacy != "1250.5" {
		t.Errorf("p1 prd = %+v", bf)
	}
	if bf := fieldOutcome(t, p2, domain.AttrFgType); bf.Outcome != FieldConflict || len(bf.Variants) != 2 {
		t.Errorf("p2 fg_type = %+v", bf)
	}
	if bf := fieldOutcome(t, p2, domain.AttrPrdPerDay); bf.Outcome != FieldFill || bf.Legacy != "10" {
		t.Errorf("p2 prd = %+v", bf)
	}
	if bf := fieldOutcome(t, p2, domain.AttrChpItemCode); bf.Outcome != FieldNoSource {
		t.Errorf("p2 chp = %+v", bf)
	}
	if p3.SourceGrade != mbSourceGrade || fieldOutcome(t, p3, domain.AttrFgType).Outcome != FieldFill {
		t.Errorf("p3 MB = %+v", p3)
	}
	if bf := fieldOutcome(t, p4, domain.AttrMsBatchItem); bf.Outcome != FieldInvalid {
		t.Errorf("p4 ms = %+v", bf)
	}
	if bf := fieldOutcome(t, p4, domain.AttrItemType); bf.Outcome != FieldHasValue {
		t.Errorf("p4 item_type = %+v", bf)
	}
	if p4.Outcome != ProductNothingToDo {
		t.Errorf("p4 outcome = %s", p4.Outcome)
	}
	if p5.Outcome != ProductNoLegacyRow || len(p5.Fields) != 0 {
		t.Errorf("p5 = %+v", p5)
	}
	s := rep.Summary
	if s.LegacyRows != 7 || s.LinkedProducts != 5 || s.NoLegacyRow != 1 || s.WithLegacyRow != 4 ||
		s.ProductsToFill != 3 || s.ProductsFilled != 0 || s.FieldsFilled != 0 {
		t.Errorf("summary = %+v", s)
	}
	// p1: chp, ms, item_type, prd; p2: item_type, prd; p3: fg_type, item_type.
	if s.FieldsToFill != 8 || s.FieldOutcomes[FieldConflict] != 1 || s.FieldOutcomes[FieldInvalid] != 1 {
		t.Errorf("field counters = %+v", s)
	}
}

func TestBackfillAttributes_Apply_FillsNullsOnly(t *testing.T) {
	repo := &fakeBackfillRepo{products: productsFixture()}
	audit := &recordingAudit{}
	h := NewBackfillAttributesHandler(&fakeLegacyReader{rows: legacyFixture()}, repo, audit)

	rep, err := h.Handle(context.Background(), BackfillAttributesCommand{Mode: BackfillModeApply, Actor: "u-1"})
	if err != nil {
		t.Fatal(err)
	}
	// Only products with a FILL field are written (p1, p2, p3); never p4/p5.
	if len(repo.writes) != 3 {
		t.Fatalf("writes = %d, want 3", len(repo.writes))
	}
	w1 := repo.writes[0]
	if w1.ProductSysID != 1 || w1.ShadeKey != "SH01" || w1.Values.FgType != "" ||
		w1.Values.ChpItemCode != "CHPX001" || w1.Values.PrdPerDay != "1250.5" || w1.Actor != "u-1" {
		t.Errorf("write p1 = %+v (fg_type must not be sent: it has a value)", w1)
	}
	if w2 := repo.writes[1]; w2.Values.FgType != "" {
		t.Errorf("conflicting fg_type was sent: %+v", w2)
	}
	for _, w := range repo.writes {
		if w.ProductSysID == 4 || w.ProductSysID == 5 {
			t.Errorf("unexpected write for %d", w.ProductSysID)
		}
	}
	if rep.Summary.ProductsFilled != 3 || rep.Summary.FieldsFilled != 8 || rep.Summary.FieldsToFill != 0 {
		t.Errorf("summary = %+v", rep.Summary)
	}
	if len(audit.inputs) != 3 {
		t.Fatalf("audits = %d", len(audit.inputs))
	}
	a := audit.inputs[0]
	if a.Operation != auditdomain.OpErpAttrBackfill || a.EntityType != "cost_product_master" || a.EntityID != 1 || a.UserID != "u-1" {
		t.Errorf("audit = %+v", a)
	}
	if !strings.Contains(a.AfterData, `"erp_chp_item_code":"CHPX001"`) || !strings.Contains(a.BeforeData, `"erp_fg_type":"KEEP"`) ||
		strings.Contains(a.BeforeData, "CHPX001") {
		t.Errorf("audit data before=%s after=%s", a.BeforeData, a.AfterData)
	}
}

func TestBackfillAttributes_Apply_StaleAndConcurrentFill(t *testing.T) {
	repo := &fakeBackfillRepo{
		products: productsFixture(),
		staleIDs: map[int64]bool{2: true},
		// p3's fg_type was filled by someone else after planning, item_type still NULL.
		current: map[int64]domain.AttrValues{3: {FgType: "OTHER"}},
	}
	audit := &recordingAudit{err: errors.New("audit down")}
	h := NewBackfillAttributesHandler(&fakeLegacyReader{rows: legacyFixture()}, repo, audit)

	rep, err := h.Handle(context.Background(), BackfillAttributesCommand{Mode: BackfillModeApply})
	if err != nil {
		t.Fatalf("audit failure must be best effort: %v", err)
	}
	p2, p3 := rep.Products[1], rep.Products[2]
	if p2.Outcome != ProductStale || fieldOutcome(t, p2, domain.AttrPrdPerDay).Outcome != FieldStale {
		t.Errorf("p2 = %+v", p2)
	}
	if p3.Outcome != ProductFilled || fieldOutcome(t, p3, domain.AttrFgType).Outcome != FieldStale ||
		fieldOutcome(t, p3, domain.AttrItemType).Outcome != FieldFill {
		t.Errorf("p3 = %+v", p3)
	}
	if rep.Summary.ProductsStale != 1 || rep.Summary.ProductsFilled != 2 {
		t.Errorf("summary = %+v", rep.Summary)
	}
	if audit.inputs[len(audit.inputs)-1].UserID != systemActor {
		t.Errorf("default actor = %q", audit.inputs[len(audit.inputs)-1].UserID)
	}
}

func TestBackfillAttributes_Errors(t *testing.T) {
	ctx := context.Background()
	reader := &fakeLegacyReader{rows: legacyFixture()}
	repo := &fakeBackfillRepo{products: productsFixture()}

	if _, err := NewBackfillAttributesHandler(nil, repo, nil).Handle(ctx, BackfillAttributesCommand{}); !errors.Is(err, domain.ErrAttrBackfillNotConfigured) {
		t.Errorf("nil reader err = %v", err)
	}
	if _, err := NewBackfillAttributesHandler(reader, nil, nil).Handle(ctx, BackfillAttributesCommand{}); !errors.Is(err, domain.ErrAttrBackfillNotConfigured) {
		t.Errorf("nil repo err = %v", err)
	}
	var nilH *BackfillAttributesHandler
	if _, err := nilH.Handle(ctx, BackfillAttributesCommand{}); !errors.Is(err, domain.ErrAttrBackfillNotConfigured) {
		t.Errorf("nil handler err = %v", err)
	}
	h := NewBackfillAttributesHandler(reader, repo, nil)
	if _, err := h.Handle(ctx, BackfillAttributesCommand{Mode: "overwrite"}); !errors.Is(err, ErrInvalidBackfillMode) {
		t.Errorf("bad mode err = %v", err)
	}
	if _, err := h.Handle(ctx, BackfillAttributesCommand{Period: "2026-8"}); err == nil {
		t.Error("bad period accepted")
	}
	boom := errors.New("boom")
	if _, err := NewBackfillAttributesHandler(&fakeLegacyReader{err: boom}, repo, nil).Handle(ctx, BackfillAttributesCommand{}); !errors.Is(err, boom) {
		t.Errorf("reader err = %v", err)
	}
	if _, err := NewBackfillAttributesHandler(reader, &fakeBackfillRepo{listErr: boom}, nil).Handle(ctx, BackfillAttributesCommand{}); !errors.Is(err, boom) {
		t.Errorf("list err = %v", err)
	}
	fr := &fakeBackfillRepo{products: productsFixture(), fillErr: boom}
	if _, err := NewBackfillAttributesHandler(reader, fr, nil).Handle(ctx, BackfillAttributesCommand{Mode: BackfillModeApply}); !errors.Is(err, boom) {
		t.Errorf("fill err = %v", err)
	}
	if len(repo.writes) != 0 {
		t.Error("error paths wrote")
	}
}

func TestBackfillMode_Parse(t *testing.T) {
	for in, want := range map[string]BackfillMode{"": BackfillModeDryRun, " DRY_RUN ": BackfillModeDryRun, "apply": BackfillModeApply} {
		got, err := ParseBackfillMode(in)
		if err != nil || got != want {
			t.Errorf("ParseBackfillMode(%q) = %s, %v", in, got, err)
		}
	}
	if _, err := ParseBackfillMode("force"); !errors.Is(err, ErrInvalidBackfillMode) {
		t.Errorf("force err = %v", err)
	}
	f, tr := false, true
	if BackfillModeFromDryRun(nil) != BackfillModeDryRun || BackfillModeFromDryRun(&tr) != BackfillModeDryRun || BackfillModeFromDryRun(&f) != BackfillModeApply {
		t.Error("BackfillModeFromDryRun default must be dry_run")
	}
}
