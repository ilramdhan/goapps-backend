package erpintegration

// backfill_attributes.go implements P3-T6: the one-off ERP attribute backfill
// (design Part 2 §9.2 attr_backfill, §10 subtype attr_backfill with
// params.dry_run default true, gap M-7, I-5).
//
//   - Source: MGTDAT.OT_STD_COST_PRODUCTS_MGT, read (SELECT only) through the
//     read-only LegacyStdReader.
//   - Target: the five 000551 cpm_erp_* columns of cost_product_master, for
//     products that are ALREADY linked (active, AX, non-blank
//     cpm_erp_item_code). The backfill never creates, changes or removes a
//     link and never writes cpm_erp_grade_code_1/2 (D-LINK).
//   - Match key: (cpm_erp_item_code, cpm_shade_code) = (FG_ITEM_CODE,
//     FG_ITEM_SHADE), trimmed and case-insensitive (D-LINK). The AX row is the
//     source; for a CMB item with no AX row the MB row (grade A) is used.
//   - Legacy duplicates are possible (no unique key). When two source rows
//     disagree on a non-empty value, that field is CONFLICT and never guessed.
//   - dry_run (default) returns the report and writes nothing. apply fills
//     NULL columns only; a column that already has a value is never
//     overwritten, even when the legacy value differs (reported as DIFFERS).
//   - Oracle is never written; apply writes PostgreSQL only.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/rs/zerolog/log"
	"github.com/shopspring/decimal"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	cpmdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// SubtypeAttrBackfill is the erp_integration job subtype of the backfill
// (D-J1). The job params carry dry_run (default true) and period.
const SubtypeAttrBackfill = "attr_backfill"

// auditEntityProductMaster is the audit entity of a filled product.
const auditEntityProductMaster = "cost_product_master"

// ErrInvalidBackfillMode is returned for a mode other than dry_run or apply.
var ErrInvalidBackfillMode = errors.New("erpintegration: invalid attribute backfill mode")

// BackfillMode selects dry_run or apply.
type BackfillMode string

// Backfill modes.
const (
	BackfillModeDryRun BackfillMode = "dry_run"
	BackfillModeApply  BackfillMode = "apply"
)

// ParseBackfillMode normalizes a mode; empty means dry_run.
func ParseBackfillMode(s string) (BackfillMode, error) {
	switch BackfillMode(strings.ToLower(strings.TrimSpace(s))) {
	case "", BackfillModeDryRun:
		return BackfillModeDryRun, nil
	case BackfillModeApply:
		return BackfillModeApply, nil
	}
	return "", fmt.Errorf("%w: %q (dry_run or apply)", ErrInvalidBackfillMode, s)
}

// BackfillModeFromDryRun maps the RPC/job flag (dry_run, default true) to a mode.
func BackfillModeFromDryRun(dryRun *bool) BackfillMode {
	if dryRun != nil && !*dryRun {
		return BackfillModeApply
	}
	return BackfillModeDryRun
}

// FieldOutcome is the per-attribute decision.
type FieldOutcome string

// Field outcomes.
const (
	// FieldFill: the column is NULL and the legacy value is filled (apply) or
	// would be filled (dry_run).
	FieldFill FieldOutcome = "FILL"
	// FieldHasValue: the column already holds the legacy value.
	FieldHasValue FieldOutcome = "HAS_VALUE"
	// FieldDiffers: the column holds a different value; it is kept.
	FieldDiffers FieldOutcome = "DIFFERS"
	// FieldNoSource: the legacy rows have no value for this field.
	FieldNoSource FieldOutcome = "NO_SOURCE_VALUE"
	// FieldConflict: legacy rows disagree; nothing is guessed.
	FieldConflict FieldOutcome = "CONFLICT"
	// FieldInvalid: the legacy value fails the column limits (e.g. > 12 chars
	// for ms_batch_item, C-11).
	FieldInvalid FieldOutcome = "INVALID_VALUE"
	// FieldStale: apply found the product changed (unlinked, relinked,
	// deactivated) or the column was filled concurrently; nothing written.
	FieldStale FieldOutcome = "STALE"
)

// ProductOutcome is the per-product decision.
type ProductOutcome string

// Product outcomes.
const (
	ProductFilled      ProductOutcome = "FILLED"        // apply wrote at least one field
	ProductWouldFill   ProductOutcome = "WOULD_FILL"    // dry_run: at least one FILL
	ProductNothingToDo ProductOutcome = "NOTHING_TO_DO" // no FILL field
	ProductNoLegacyRow ProductOutcome = "NO_LEGACY_ROW" // no source row for the key
	ProductStale       ProductOutcome = "STALE"         // apply: changed since planning
)

// BackfillField is one attribute line of the report.
type BackfillField struct {
	Field    domain.AttrField `json:"field"`
	Outcome  FieldOutcome     `json:"outcome"`
	Current  string           `json:"current,omitempty"`
	Legacy   string           `json:"legacy,omitempty"`
	Variants []string         `json:"variants,omitempty"` // CONFLICT only
}

// BackfillProduct is one product line of the report.
type BackfillProduct struct {
	ProductSysID int64           `json:"product_sys_id"`
	ProductCode  string          `json:"product_code"`
	ErpItemCode  string          `json:"erp_item_code"`
	ShadeCode    string          `json:"shade_code"`
	SourceGrade  string          `json:"source_grade,omitempty"`
	SourceRows   int             `json:"source_rows"`
	Outcome      ProductOutcome  `json:"outcome"`
	Fields       []BackfillField `json:"fields,omitempty"`
}

// BackfillSummary holds the report counters.
type BackfillSummary struct {
	LegacyRows        int                      `json:"legacy_rows"`
	LinkedProducts    int                      `json:"linked_products"`
	WithLegacyRow     int                      `json:"with_legacy_row"`
	NoLegacyRow       int                      `json:"no_legacy_row"`
	ProductsToFill    int                      `json:"products_to_fill"`
	ProductsFilled    int                      `json:"products_filled"`
	ProductsStale     int                      `json:"products_stale"`
	FieldsToFill      int                      `json:"fields_to_fill"`
	FieldsFilled      int                      `json:"fields_filled"`
	FieldOutcomes     map[FieldOutcome]int     `json:"field_outcomes"`
	FieldsToFillByKey map[domain.AttrField]int `json:"fields_to_fill_by_field"`
}

// BackfillReport is the result of one run (dry_run or apply).
type BackfillReport struct {
	Mode     BackfillMode      `json:"mode"`
	Period   string            `json:"period,omitempty"`
	Summary  BackfillSummary   `json:"summary"`
	Products []BackfillProduct `json:"products"`
}

// BackfillAttributesCommand runs the backfill.
type BackfillAttributesCommand struct {
	// Period is YYYYMM (legacy rows created up to that month) or "" for all.
	Period string
	Mode   BackfillMode
	Actor  string
}

// BackfillAttributesHandler plans and optionally applies the backfill.
type BackfillAttributesHandler struct {
	reader domain.LegacyStdReader
	repo   domain.AttrBackfillRepository
	audit  AuditSink
}

// NewBackfillAttributesHandler builds the handler. reader and repo are
// required (ErrAttrBackfillNotConfigured otherwise); audit may be nil.
func NewBackfillAttributesHandler(reader domain.LegacyStdReader, repo domain.AttrBackfillRepository, audit AuditSink) *BackfillAttributesHandler {
	return &BackfillAttributesHandler{reader: reader, repo: repo, audit: audit}
}

// Handle runs the backfill. dry_run performs reads only.
func (h *BackfillAttributesHandler) Handle(ctx context.Context, cmd BackfillAttributesCommand) (BackfillReport, error) {
	rep := BackfillReport{Mode: cmd.Mode, Period: strings.TrimSpace(cmd.Period)}
	if h == nil || h.reader == nil || h.repo == nil {
		return rep, domain.ErrAttrBackfillNotConfigured
	}
	if rep.Mode == "" {
		rep.Mode = BackfillModeDryRun
	}
	if rep.Mode != BackfillModeDryRun && rep.Mode != BackfillModeApply {
		return rep, fmt.Errorf("%w: %q", ErrInvalidBackfillMode, rep.Mode)
	}
	if rep.Period != "" {
		if err := domain.ValidateBatchPeriod(rep.Period); err != nil {
			return rep, err
		}
	}
	legacy, err := h.reader.List(ctx, rep.Period)
	if err != nil {
		return rep, fmt.Errorf("erp attr backfill: read legacy std: %w", err)
	}
	products, err := h.repo.ListLinkedProducts(ctx)
	if err != nil {
		return rep, fmt.Errorf("erp attr backfill: list linked products: %w", err)
	}
	idx := indexLegacyRows(legacy)
	rep.Summary = newBackfillSummary(len(legacy), len(products))
	rep.Products = make([]BackfillProduct, 0, len(products))
	for _, p := range products {
		line, fill := planProduct(p, idx)
		if rep.Mode == BackfillModeApply && fill != (domain.AttrValues{}) {
			if err := h.applyProduct(ctx, cmd.Actor, p, fill, &line); err != nil {
				return rep, err
			}
		}
		rep.Summary.add(line)
		rep.Products = append(rep.Products, line)
	}
	return rep, nil
}

// applyProduct fills one product and rewrites its report line with what the
// repository actually did.
func (h *BackfillAttributesHandler) applyProduct(ctx context.Context, actor string, p domain.LinkedProductAttrs, fill domain.AttrValues, line *BackfillProduct) error {
	res, err := h.repo.FillNullAttributes(ctx, domain.AttrFillWrite{
		ProductSysID: p.ProductSysID,
		ErpItemCode:  p.ErpItemCode,
		ShadeKey:     cpmdomain.NormalizeShadeKey(p.ShadeCode),
		Values:       fill,
		Actor:        actor,
	})
	if err != nil {
		return fmt.Errorf("erp attr backfill: fill product %d: %w", p.ProductSysID, err)
	}
	filled := map[domain.AttrField]bool{}
	for _, f := range res.Filled {
		filled[f] = true
	}
	for i := range line.Fields {
		if line.Fields[i].Outcome == FieldFill && (res.Stale || !filled[line.Fields[i].Field]) {
			line.Fields[i].Outcome = FieldStale
		}
	}
	switch {
	case res.Stale:
		line.Outcome = ProductStale
	case len(res.Filled) > 0:
		line.Outcome = ProductFilled
		h.emitAudit(ctx, actor, p.ProductSysID, res)
	default:
		line.Outcome = ProductStale
	}
	return nil
}

// legacyKey is the D-LINK key form.
type legacyKey struct{ item, shade string }

func keyOf(item, shade string) legacyKey {
	return legacyKey{item: strings.ToUpper(strings.TrimSpace(item)), shade: cpmdomain.NormalizeShadeKey(shade)}
}

// legacyGroup holds the source rows of one key, split by grade.
type legacyGroup struct {
	ax []domain.LegacyStdRow
	mb []domain.LegacyStdRow // grade A rows (legacy MB block E)
}

// mbSourceGrade is the grade the legacy MB block inserts (FG_ITEM_GRADE='A').
const mbSourceGrade = "A"

func indexLegacyRows(rows []domain.LegacyStdRow) map[legacyKey]*legacyGroup {
	idx := map[legacyKey]*legacyGroup{}
	for _, r := range rows {
		grade := strings.ToUpper(strings.TrimSpace(r.GradeCode))
		isAX := cpmdomain.IsAxGrade(grade)
		isMB := grade == mbSourceGrade && cpmdomain.IsCmbItem(r.ItemCode)
		if !isAX && !isMB {
			continue // derived grades copy the AX attributes; the AX row is the source
		}
		k := keyOf(r.ItemCode, r.ShadeCode)
		g := idx[k]
		if g == nil {
			g = &legacyGroup{}
			idx[k] = g
		}
		if isAX {
			g.ax = append(g.ax, r)
		} else {
			g.mb = append(g.mb, r)
		}
	}
	return idx
}

// planProduct decides every field of one product. It returns the report line
// and the values to fill (only FILL fields set).
func planProduct(p domain.LinkedProductAttrs, idx map[legacyKey]*legacyGroup) (BackfillProduct, domain.AttrValues) {
	line := BackfillProduct{
		ProductSysID: p.ProductSysID, ProductCode: p.ProductCode,
		ErpItemCode: p.ErpItemCode, ShadeCode: p.ShadeCode,
	}
	var src []domain.LegacyStdRow
	if g := idx[keyOf(p.ErpItemCode, p.ShadeCode)]; g != nil {
		switch {
		case len(g.ax) > 0:
			src, line.SourceGrade = g.ax, "AX"
		case len(g.mb) > 0:
			src, line.SourceGrade = g.mb, mbSourceGrade
		}
	}
	line.SourceRows = len(src)
	if len(src) == 0 {
		line.Outcome = ProductNoLegacyRow
		return line, domain.AttrValues{}
	}
	var fill domain.AttrValues
	for _, f := range domain.AllAttrFields() {
		bf := decideField(f, strings.TrimSpace(p.Attrs.Get(f)), src)
		if bf.Outcome == FieldFill {
			fill = fill.With(f, bf.Legacy)
		}
		line.Fields = append(line.Fields, bf)
	}
	line.Outcome = ProductNothingToDo
	if fill != (domain.AttrValues{}) {
		line.Outcome = ProductWouldFill
	}
	return line, fill
}

// decideField picks the outcome of one attribute.
func decideField(f domain.AttrField, current string, src []domain.LegacyStdRow) BackfillField {
	bf := BackfillField{Field: f, Current: current}
	variants := distinctValues(f, src)
	switch len(variants) {
	case 0:
		bf.Outcome = FieldNoSource
		return bf
	case 1:
		bf.Legacy = variants[0]
	default:
		bf.Outcome = FieldConflict
		bf.Variants = variants
		return bf
	}
	switch {
	case current != "" && sameValue(f, current, bf.Legacy):
		bf.Outcome = FieldHasValue
	case current != "":
		bf.Outcome = FieldDiffers
	case validAttr(f, bf.Legacy) != nil:
		bf.Outcome = FieldInvalid
	default:
		bf.Outcome = FieldFill
	}
	return bf
}

// distinctValues returns the sorted distinct non-empty values of f (decimals
// compared numerically for prd_per_day).
func distinctValues(f domain.AttrField, rows []domain.LegacyStdRow) []string {
	seen := map[string]string{}
	for _, r := range rows {
		v := strings.TrimSpace(legacyValue(f, r))
		if v == "" {
			continue
		}
		k := v
		if f == domain.AttrPrdPerDay {
			if d, err := decimal.NewFromString(v); err == nil {
				k = d.String()
				v = k
			}
		}
		if _, ok := seen[k]; !ok {
			seen[k] = v
		}
	}
	out := make([]string, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func legacyValue(f domain.AttrField, r domain.LegacyStdRow) string {
	switch f {
	case domain.AttrFgType:
		return r.FgType
	case domain.AttrChpItemCode:
		return r.ChpItemCode
	case domain.AttrMsBatchItem:
		return r.MsBatchItem
	case domain.AttrItemType:
		return r.ItemType
	case domain.AttrPrdPerDay:
		return r.PrdPerDay
	}
	return ""
}

// sameValue compares a current and a legacy value (numerically for prd_per_day).
func sameValue(f domain.AttrField, a, b string) bool {
	if f == domain.AttrPrdPerDay {
		da, errA := decimal.NewFromString(a)
		db, errB := decimal.NewFromString(b)
		if errA == nil && errB == nil {
			return da.Equal(db)
		}
	}
	return a == b
}

// validAttr checks one value against the 000551 column limits by reusing the
// product-master validation.
func validAttr(f domain.AttrField, v string) error {
	var a cpmdomain.ErpAttributes
	switch f {
	case domain.AttrFgType:
		a.FgType = v
	case domain.AttrChpItemCode:
		a.ChpItemCode = v
	case domain.AttrMsBatchItem:
		a.MsBatchItem = v
	case domain.AttrItemType:
		a.ItemType = v
	case domain.AttrPrdPerDay:
		d, err := decimal.NewFromString(v)
		if err != nil {
			return fmt.Errorf("%w: prd_per_day %q", cpmdomain.ErrInvalidErpAttributes, v)
		}
		a.PrdPerDay = decimal.NullDecimal{Decimal: d, Valid: true}
	}
	return a.Validate()
}

func newBackfillSummary(legacyRows, linked int) BackfillSummary {
	return BackfillSummary{
		LegacyRows:        legacyRows,
		LinkedProducts:    linked,
		FieldOutcomes:     map[FieldOutcome]int{},
		FieldsToFillByKey: map[domain.AttrField]int{},
	}
}

func (s *BackfillSummary) add(line BackfillProduct) {
	switch line.Outcome {
	case ProductNoLegacyRow:
		s.NoLegacyRow++
		return
	case ProductFilled:
		s.ProductsFilled++
	case ProductStale:
		s.ProductsStale++
	case ProductWouldFill:
		s.ProductsToFill++
	case ProductNothingToDo:
	}
	s.WithLegacyRow++
	for _, f := range line.Fields {
		s.FieldOutcomes[f.Outcome]++
		switch f.Outcome {
		case FieldFill:
			if line.Outcome == ProductFilled {
				s.FieldsFilled++
			} else {
				s.FieldsToFill++
			}
			s.FieldsToFillByKey[f.Field]++
		default:
		}
	}
}

// backfillAuditData is written to cal_after_data / cal_before_data.
type backfillAuditData struct {
	EventType string             `json:"event_type"`
	Filled    []domain.AttrField `json:"filled,omitempty"`
	FgType    *string            `json:"erp_fg_type"`
	ChpItem   *string            `json:"erp_chp_item_code"`
	MsBatch   *string            `json:"erp_ms_batch_item"`
	ItemType  *string            `json:"erp_item_type"`
	PrdPerDay *string            `json:"erp_prd_per_day"`
}

func auditValues(v domain.AttrValues, filled []domain.AttrField) backfillAuditData {
	p := func(s string) *string {
		if s == "" {
			return nil
		}
		return &s
	}
	return backfillAuditData{
		EventType: auditdomain.OpErpAttrBackfill, Filled: filled,
		FgType: p(v.FgType), ChpItem: p(v.ChpItemCode), MsBatch: p(v.MsBatchItem),
		ItemType: p(v.ItemType), PrdPerDay: p(v.PrdPerDay),
	}
}

// emitAudit records one filled product (best effort: the fill has committed).
func (h *BackfillAttributesHandler) emitAudit(ctx context.Context, actor string, sysID int64, res domain.AttrFillResult) {
	if h.audit == nil {
		return
	}
	if actor == "" {
		actor = systemActor
	}
	before, err := json.Marshal(auditValues(res.Before, nil))
	if err != nil {
		log.Warn().Err(err).Msg("erp attr backfill: marshal audit before")
		return
	}
	after, err := json.Marshal(auditValues(res.After, res.Filled))
	if err != nil {
		log.Warn().Err(err).Msg("erp attr backfill: marshal audit after")
		return
	}
	if err := h.audit.Emit(ctx, auditdomain.NewInput{
		EntityType: auditEntityProductMaster,
		EntityID:   sysID,
		Operation:  auditdomain.OpErpAttrBackfill,
		BeforeData: string(before),
		AfterData:  string(after),
		UserID:     actor,
	}); err != nil {
		log.Warn().Err(err).Int64("product_sys_id", sysID).Msg("erp attr backfill: audit emit failed")
	}
}
