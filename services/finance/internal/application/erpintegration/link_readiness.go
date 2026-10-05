package erpintegration

// link_readiness.go implements P3-T9: the ERP link-readiness report, the
// GoApps equivalent of recon §9a–§9f (runbook §2.3, design Part 2 §8
// GetErpItemLinkReport). It is read-only: it never writes PostgreSQL and
// never reaches Oracle (the demand comes from the batch's cst_erp_adj_demand
// snapshot, which LoadDemand already read through the read-only user).
//
// Link key (D-LINK): (cpm_erp_item_code, cpm_shade_code) on active AX
// products, trimmed and upper-cased; cpm_erp_grade_code_1/2 are never read.
//
// Sections:
//
//   - §9a the demand combos (item, shade) of the batch, with qty
//   - §9b demand combos with no active AX product (NO_PRODUCT)
//   - §9c attribute gaps of linked products (missing fg_type blocks the
//     derivation with NO_FG_TYPE; an item absent from the replica is flagged
//     once the replica holds rows); other missing attributes are shown but
//     do not block
//   - §9d per §9b combo: LINK_CANDIDATE (unlinked active AX products with the
//     same shade, a V-12-compatible type and a similar name) or CREATE_NEW
//   - §9e every duplicate group on the D-LINK key, with all product sys ids
//     and a TRIAL-* flag; this is the worklist that ungates migration 000556
//   - "linked but no shade": active linked products with an empty
//     cpm_shade_code, with their ACTUAL row count for the period (F-5/F-9)
//   - §9f V-12 type mismatches (CMB item on a non-MB product, or an MB
//     product on a non-CMB item)
//
// Pass (runbook §2.3): §9b has 0 rows with qty > 0, §9c = 0, §9e = 0 and
// §9f = 0.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/shopspring/decimal"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// ErrInvalidLinkReadinessQuery is returned for a query with neither a batch
// nor a valid period, or with a period that contradicts the batch.
var ErrInvalidLinkReadinessQuery = errors.New("erpintegration: invalid link-readiness query")

// ErrLinkReadinessNotConfigured is returned when the handler has no source.
var ErrLinkReadinessNotConfigured = errors.New("erpintegration: link-readiness report not configured")

// LinkSuggestion is the §9d classification of a §9b combo.
type LinkSuggestion string

// §9d suggestions.
const (
	SuggestLinkCandidate LinkSuggestion = "LINK_CANDIDATE"
	SuggestCreateNew     LinkSuggestion = "CREATE_NEW"
)

// TypeMismatchDirection names the V-12 violation of a §9f line.
type TypeMismatchDirection string

// §9f directions.
const (
	MismatchCmbOnNonMB TypeMismatchDirection = "CMB_ON_NON_MB"
	MismatchMBOnYarn   TypeMismatchDirection = "MB_ON_NON_CMB"
)

// Attribute-gap scopes.
const (
	GapScopeDemand = "DEMAND" // products mapped by the batch's demand
	GapScopeAll    = "ALL"    // every linked product (no demand available)
)

// trialItemPrefix marks trial/dev ERP item codes (F-8).
const trialItemPrefix = "TRIAL"

// Candidate matching (§9d): name similarity is a token Jaccard index in
// whole percent (integer math only).
const (
	minCandidateScore = 40
	maxCandidates     = 5
)

// LinkedAxProduct is one active AX product with a non-blank
// cpm_erp_item_code, as read by the source.
type LinkedAxProduct struct {
	SysID       int64
	ProductCode string
	ProductName string
	ItemCode    string // trimmed cpm_erp_item_code
	ShadeCode   string // trimmed cpm_shade_code ("" when NULL)
	TypeCode    string // cost_product_type.cpt_type_code
	Attrs       domain.AttrValues
	InReplica   bool // the item exists in cost_erp_item
	ActualRows  int  // non-SUPERSEDED ACTUAL cst_product_cost rows for the period
}

// UnlinkedAxProduct is one active AX product with no ERP item code.
type UnlinkedAxProduct struct {
	SysID       int64
	ProductCode string
	ProductName string
	ShadeCode   string
	TypeCode    string
}

// LinkReadinessSource is the read-only PG port of the report. Every method is
// a SELECT.
type LinkReadinessSource interface {
	// ListLinkedAxProducts returns every active AX product with a non-blank
	// cpm_erp_item_code, with its ACTUAL row count for the period.
	ListLinkedAxProducts(ctx context.Context, period string) ([]LinkedAxProduct, error)
	// ListUnlinkedAxProducts returns the active AX products with no ERP item
	// code whose normalized shade is one of shadeKeys.
	ListUnlinkedAxProducts(ctx context.Context, shadeKeys []string) ([]UnlinkedAxProduct, error)
	// CountReplicaItems returns the number of cost_erp_item rows.
	CountReplicaItems(ctx context.Context) (int64, error)
}

// LinkReadinessQuery selects the report scope: a batch (its demand), or a
// period (the newest batch of the period that holds demand; without one the
// demand sections are empty and only the master sections are computed).
type LinkReadinessQuery struct {
	BatchID int64
	Period  string
}

// ProductRef identifies a product in a report line.
type ProductRef struct {
	SysID       int64  `json:"product_sys_id"`
	ProductCode string `json:"product_code"`
	ShadeCode   string `json:"shade_code,omitempty"`
}

// LinkCandidate is one §9d suggestion.
type LinkCandidate struct {
	SysID       int64  `json:"product_sys_id"`
	ProductCode string `json:"product_code"`
	ProductName string `json:"product_name"`
	ShadeCode   string `json:"shade_code"`
	TypeCode    string `json:"type_code"`
	Score       int    `json:"score"` // name similarity, 0-100
}

// NoProductLine is one §9b combo with its §9d suggestion.
type NoProductLine struct {
	Kind       domain.ItemKind `json:"kind"`
	ItemCode   string          `json:"item_code"`
	ItemName   string          `json:"item_name,omitempty"`
	ShadeCode  string          `json:"shade_code"`
	GradeCodes []string        `json:"grade_codes"`
	QtyKg      decimal.Decimal `json:"qty_kg"`
	Suggestion LinkSuggestion  `json:"suggestion"`
	Candidates []LinkCandidate `json:"candidates,omitempty"`
	// SameItemOtherShade lists products linked to the same item with a
	// different shade (a relink may be the fix).
	SameItemOtherShade []ProductRef `json:"same_item_other_shade,omitempty"`
}

// AttributeGapLine is one §9c product.
type AttributeGapLine struct {
	SysID         int64              `json:"product_sys_id"`
	ProductCode   string             `json:"product_code"`
	ItemCode      string             `json:"item_code"`
	ShadeCode     string             `json:"shade_code"`
	Missing       []domain.AttrField `json:"missing"`
	MissingFgType bool               `json:"missing_fg_type"`
	NotInReplica  bool               `json:"not_in_replica"`
	Blocking      bool               `json:"blocking"`
	QtyKg         decimal.Decimal    `json:"qty_kg"`
}

// DuplicateGroup is one §9e group: > 1 active AX product on one key.
type DuplicateGroup struct {
	ItemCode    string          `json:"item_code"`
	ShadeCode   string          `json:"shade_code"`
	Products    []ProductRef    `json:"products"`
	Trial       bool            `json:"trial"`
	InDemand    bool            `json:"in_demand"`
	DemandQtyKg decimal.Decimal `json:"demand_qty_kg"`
}

// NoShadeLine is one "linked but no shade" product.
type NoShadeLine struct {
	SysID       int64  `json:"product_sys_id"`
	ProductCode string `json:"product_code"`
	ItemCode    string `json:"item_code"`
	TypeCode    string `json:"type_code"`
	ActualRows  int    `json:"actual_rows"`
	// SameItemNoShade is how many linked products share this item with an
	// empty shade (e.g. ACY0000068 x7).
	SameItemNoShade int `json:"same_item_no_shade"`
	// ItemInDemand is true when the batch's demand has this item (any shade).
	ItemInDemand bool `json:"item_in_demand"`
}

// TypeMismatchLine is one §9f V-12 violation.
type TypeMismatchLine struct {
	SysID       int64                 `json:"product_sys_id"`
	ProductCode string                `json:"product_code"`
	ItemCode    string                `json:"item_code"`
	ShadeCode   string                `json:"shade_code"`
	TypeCode    string                `json:"type_code"`
	Direction   TypeMismatchDirection `json:"direction"`
	InDemand    bool                  `json:"in_demand"`
}

// LinkReadinessPass holds the runbook §2.3 pass criteria.
type LinkReadinessPass struct {
	NoProductWithQty bool `json:"s9b_no_product_with_qty_zero"`
	AttributeGaps    bool `json:"s9c_zero"`
	Duplicates       bool `json:"s9e_zero"`
	TypeMismatches   bool `json:"s9f_zero"`
}

// LinkReadinessSummary holds the report counters.
type LinkReadinessSummary struct {
	LinkedProducts       int               `json:"linked_products"`
	ReplicaItems         int64             `json:"replica_items"`
	DemandCombos         int               `json:"demand_combos"`
	DemandQtyKg          decimal.Decimal   `json:"demand_qty_kg"`
	MappedCombos         int               `json:"mapped_combos"`
	NoProduct            int               `json:"no_product"`
	NoProductWithQty     int               `json:"no_product_with_qty"`
	LinkCandidates       int               `json:"link_candidates"`
	CreateNew            int               `json:"create_new"`
	GapScope             string            `json:"gap_scope"`
	AttributeGapProducts int               `json:"attribute_gap_products"`
	BlockingGapProducts  int               `json:"blocking_gap_products"`
	MissingFgType        int               `json:"missing_fg_type"`
	NotInReplica         int               `json:"not_in_replica"`
	DuplicateGroups      int               `json:"duplicate_groups"`
	DuplicateProducts    int               `json:"duplicate_products"`
	TrialDuplicateGroups int               `json:"trial_duplicate_groups"`
	DemandDuplicates     int               `json:"demand_duplicate_groups"`
	LinkedNoShade        int               `json:"linked_no_shade"`
	LinkedNoShadeActual  int               `json:"linked_no_shade_with_actual"`
	TypeMismatches       int               `json:"type_mismatches"`
	Pass                 LinkReadinessPass `json:"pass"`
	// Ready is true when every pass criterion holds.
	Ready bool `json:"ready"`
	// UniqueIndexReady is true when §9e = 0 (migration 000556 may deploy).
	UniqueIndexReady bool `json:"unique_index_ready"`
}

// LinkReadinessReport is the whole report.
type LinkReadinessReport struct {
	Period          string               `json:"period"`
	BatchID         int64                `json:"batch_id,omitempty"`
	DemandAvailable bool                 `json:"demand_available"`
	GeneratedAt     time.Time            `json:"generated_at"`
	Summary         LinkReadinessSummary `json:"summary"`
	NoProduct       []NoProductLine      `json:"no_product"`
	AttributeGaps   []AttributeGapLine   `json:"attribute_gaps"`
	Duplicates      []DuplicateGroup     `json:"duplicates"`
	LinkedNoShade   []NoShadeLine        `json:"linked_no_shade"`
	TypeMismatches  []TypeMismatchLine   `json:"type_mismatches"`
}

// LinkReadinessHandler builds the report. Read-only.
type LinkReadinessHandler struct {
	batches domain.BatchRepository
	demand  domain.DemandRepository
	source  LinkReadinessSource
	now     func() time.Time
}

// NewLinkReadinessHandler builds the handler. batches and demand may be nil
// only for period-only reports (no demand sections).
func NewLinkReadinessHandler(batches domain.BatchRepository, demand domain.DemandRepository, source LinkReadinessSource) *LinkReadinessHandler {
	return &LinkReadinessHandler{batches: batches, demand: demand, source: source, now: time.Now}
}

// WithClock overrides the clock (tests).
func (h *LinkReadinessHandler) WithClock(now func() time.Time) *LinkReadinessHandler {
	h.now = now
	return h
}

// Handle builds the report.
func (h *LinkReadinessHandler) Handle(ctx context.Context, q LinkReadinessQuery) (*LinkReadinessReport, error) {
	if h == nil || h.source == nil {
		return nil, ErrLinkReadinessNotConfigured
	}
	period, batchID, demand, err := h.resolveDemand(ctx, q)
	if err != nil {
		return nil, err
	}
	linked, err := h.source.ListLinkedAxProducts(ctx, period)
	if err != nil {
		return nil, fmt.Errorf("link readiness: list linked products: %w", err)
	}
	replica, err := h.source.CountReplicaItems(ctx)
	if err != nil {
		return nil, fmt.Errorf("link readiness: count replica items: %w", err)
	}
	in := readinessInput{
		period: period, batchID: batchID, demandAvailable: batchID > 0,
		combos: GroupDemandCombos(batchID, demand), names: demandItemNames(demand),
		linked: linked, replicaItems: replica,
	}
	noProduct := noProductCombos(in)
	if len(noProduct) > 0 {
		in.unlinked, err = h.source.ListUnlinkedAxProducts(ctx, shadeKeysOf(noProduct))
		if err != nil {
			return nil, fmt.Errorf("link readiness: list unlinked products: %w", err)
		}
	}
	rep := BuildLinkReadiness(in)
	rep.GeneratedAt = h.now().UTC()
	return rep, nil
}

// resolveDemand picks the batch and loads its demand.
func (h *LinkReadinessHandler) resolveDemand(ctx context.Context, q LinkReadinessQuery) (string, int64, []domain.DemandLine, error) {
	period := strings.TrimSpace(q.Period)
	if q.BatchID < 0 {
		return "", 0, nil, fmt.Errorf("%w: batch id %d", ErrInvalidLinkReadinessQuery, q.BatchID)
	}
	if q.BatchID == 0 {
		if err := domain.ValidateBatchPeriod(period); err != nil {
			return "", 0, nil, fmt.Errorf("%w: %w", ErrInvalidLinkReadinessQuery, err)
		}
	}
	if h.batches == nil || h.demand == nil {
		if q.BatchID > 0 {
			return "", 0, nil, ErrLinkReadinessNotConfigured
		}
		return period, 0, nil, nil
	}
	if q.BatchID > 0 {
		b, err := h.batches.GetByID(ctx, q.BatchID)
		if err != nil {
			return "", 0, nil, err
		}
		if period != "" && period != b.Period() {
			return "", 0, nil, fmt.Errorf("%w: period %s does not match batch %d (%s)", ErrInvalidLinkReadinessQuery, period, b.ID(), b.Period())
		}
		lines, err := h.demand.List(ctx, b.ID())
		if err != nil {
			return "", 0, nil, fmt.Errorf("link readiness: list demand: %w", err)
		}
		return b.Period(), b.ID(), lines, nil
	}
	return h.latestDemand(ctx, period)
}

// latestDemand returns the newest batch of the period that holds demand.
func (h *LinkReadinessHandler) latestDemand(ctx context.Context, period string) (string, int64, []domain.DemandLine, error) {
	batches, err := h.batches.ListByPeriod(ctx, period)
	if err != nil {
		return "", 0, nil, fmt.Errorf("link readiness: list batches: %w", err)
	}
	for _, b := range batches {
		lines, err := h.demand.List(ctx, b.ID())
		if err != nil {
			return "", 0, nil, fmt.Errorf("link readiness: list demand: %w", err)
		}
		if len(lines) > 0 {
			return period, b.ID(), lines, nil
		}
	}
	return period, 0, nil, nil
}

// readinessInput is everything BuildLinkReadiness needs (pure).
type readinessInput struct {
	period          string
	batchID         int64
	demandAvailable bool
	combos          []domain.CoverageLine
	names           map[ErpProductKey]string
	linked          []LinkedAxProduct
	unlinked        []UnlinkedAxProduct
	replicaItems    int64
}

// readinessIndex groups the linked products.
type readinessIndex struct {
	byKey      map[ErpProductKey][]LinkedAxProduct
	byItem     map[string][]LinkedAxProduct
	demandKeys map[ErpProductKey]decimal.Decimal
	demandItem map[string]bool
}

func buildReadinessIndex(in readinessInput) readinessIndex {
	idx := readinessIndex{
		byKey: map[ErpProductKey][]LinkedAxProduct{}, byItem: map[string][]LinkedAxProduct{},
		demandKeys: map[ErpProductKey]decimal.Decimal{}, demandItem: map[string]bool{},
	}
	for _, p := range in.linked {
		k := NewErpProductKey(p.ItemCode, p.ShadeCode)
		idx.byKey[k] = append(idx.byKey[k], p)
		idx.byItem[k.ItemCode] = append(idx.byItem[k.ItemCode], p)
	}
	for _, c := range in.combos {
		k := NewErpProductKey(c.ItemCode, c.ShadeCode)
		idx.demandKeys[k] = idx.demandKeys[k].Add(c.QtyKg)
		idx.demandItem[k.ItemCode] = true
	}
	return idx
}

// BuildLinkReadiness computes every section from already-loaded data (pure).
func BuildLinkReadiness(in readinessInput) *LinkReadinessReport {
	idx := buildReadinessIndex(in)
	rep := &LinkReadinessReport{
		Period: in.period, BatchID: in.batchID, DemandAvailable: in.demandAvailable,
		NoProduct: []NoProductLine{}, AttributeGaps: []AttributeGapLine{}, Duplicates: []DuplicateGroup{},
		LinkedNoShade: []NoShadeLine{}, TypeMismatches: []TypeMismatchLine{},
	}
	rep.NoProduct = buildNoProduct(in, idx)
	rep.AttributeGaps = buildAttributeGaps(in, idx)
	rep.Duplicates = buildDuplicates(idx)
	rep.LinkedNoShade = buildNoShade(in, idx)
	rep.TypeMismatches = buildTypeMismatches(in, idx)
	rep.Summary = summarizeReadiness(in, idx, rep)
	return rep
}

// noProductCombos returns the demand combos with no active AX product.
func noProductCombos(in readinessInput) []domain.CoverageLine {
	idx := buildReadinessIndex(readinessInput{linked: in.linked})
	var out []domain.CoverageLine
	for _, c := range in.combos {
		if len(idx.byKey[NewErpProductKey(c.ItemCode, c.ShadeCode)]) == 0 {
			out = append(out, c)
		}
	}
	return out
}

func shadeKeysOf(combos []domain.CoverageLine) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(combos))
	for _, c := range combos {
		s := NewErpProductKey(c.ItemCode, c.ShadeCode).ShadeCode
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}

func demandItemNames(demand []domain.DemandLine) map[ErpProductKey]string {
	out := map[ErpProductKey]string{}
	for _, d := range demand {
		k := NewErpProductKey(d.ItemCode, d.ShadeCode)
		if _, ok := out[k]; !ok && strings.TrimSpace(d.ItemName) != "" {
			out[k] = strings.TrimSpace(d.ItemName)
		}
	}
	return out
}

// buildNoProduct is §9b + §9d.
func buildNoProduct(in readinessInput, idx readinessIndex) []NoProductLine {
	out := []NoProductLine{}
	for _, c := range in.combos {
		k := NewErpProductKey(c.ItemCode, c.ShadeCode)
		if len(idx.byKey[k]) > 0 {
			continue
		}
		line := NoProductLine{
			Kind: c.Kind, ItemCode: c.ItemCode, ItemName: in.names[k], ShadeCode: c.ShadeCode,
			GradeCodes: c.GradeCodes, QtyKg: c.QtyKg,
		}
		line.Candidates = suggestCandidates(c.Kind, k.ShadeCode, line.ItemName, in.unlinked)
		line.Suggestion = SuggestCreateNew
		if len(line.Candidates) > 0 {
			line.Suggestion = SuggestLinkCandidate
		}
		for _, p := range idx.byItem[k.ItemCode] {
			line.SameItemOtherShade = append(line.SameItemOtherShade, ProductRef{SysID: p.SysID, ProductCode: p.ProductCode, ShadeCode: p.ShadeCode})
		}
		out = append(out, line)
	}
	return out
}

// suggestCandidates ranks unlinked products with the same shade and a
// V-12-compatible type by name similarity. Nothing is ever auto-linked.
func suggestCandidates(kind domain.ItemKind, shadeKey, itemName string, unlinked []UnlinkedAxProduct) []LinkCandidate {
	erpTokens := nameTokens(itemName)
	if len(erpTokens) == 0 {
		return nil
	}
	out := make([]LinkCandidate, 0, len(unlinked))
	for _, p := range unlinked {
		if NewErpProductKey("", p.ShadeCode).ShadeCode != shadeKey || !typeCompatible(kind, p.TypeCode) {
			continue
		}
		score := jaccardPercent(erpTokens, nameTokens(p.ProductName))
		if score < minCandidateScore {
			continue
		}
		out = append(out, LinkCandidate{
			SysID: p.SysID, ProductCode: p.ProductCode, ProductName: p.ProductName,
			ShadeCode: p.ShadeCode, TypeCode: p.TypeCode, Score: score,
		})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score > out[j].Score
		}
		return out[i].SysID < out[j].SysID
	})
	if len(out) > maxCandidates {
		out = out[:maxCandidates]
	}
	return out
}

// typeCompatible is the V-12 rule: CMB items only on MB products, yarn items
// never on MB products.
func typeCompatible(kind domain.ItemKind, typeCode string) bool {
	isMB := strings.EqualFold(strings.TrimSpace(typeCode), mbProductTypeCode)
	if kind == domain.ItemKindMB {
		return isMB
	}
	return !isMB
}

// nameTokens splits a name into upper-case alphanumeric tokens (len >= 2).
func nameTokens(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, t := range strings.FieldsFunc(strings.ToUpper(s), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}) {
		if len(t) >= 2 {
			out[t] = struct{}{}
		}
	}
	return out
}

// jaccardPercent is |a∩b| * 100 / |a∪b| (integer).
func jaccardPercent(a, b map[string]struct{}) int {
	if len(a) == 0 || len(b) == 0 {
		return 0
	}
	inter := 0
	for t := range a {
		if _, ok := b[t]; ok {
			inter++
		}
	}
	union := len(a) + len(b) - inter
	return inter * 100 / union
}

// gapScopeProducts returns the products §9c looks at: those mapped by the
// demand (single holder of a demand key), or every linked product when no
// demand is available.
func gapScopeProducts(in readinessInput, idx readinessIndex) ([]LinkedAxProduct, map[int64]decimal.Decimal) {
	qty := map[int64]decimal.Decimal{}
	if !in.demandAvailable {
		return in.linked, qty
	}
	out := make([]LinkedAxProduct, 0, len(in.linked))
	for _, p := range in.linked {
		k := NewErpProductKey(p.ItemCode, p.ShadeCode)
		q, ok := idx.demandKeys[k]
		if !ok || len(idx.byKey[k]) != 1 {
			continue
		}
		qty[p.SysID] = q
		out = append(out, p)
	}
	return out, qty
}

// buildAttributeGaps is §9c.
func buildAttributeGaps(in readinessInput, idx readinessIndex) []AttributeGapLine {
	products, qty := gapScopeProducts(in, idx)
	out := []AttributeGapLine{}
	for _, p := range products {
		var missing []domain.AttrField
		for _, f := range domain.AllAttrFields() {
			if strings.TrimSpace(p.Attrs.Get(f)) == "" {
				missing = append(missing, f)
			}
		}
		notInReplica := in.replicaItems > 0 && !p.InReplica
		if len(missing) == 0 && !notInReplica {
			continue
		}
		fg := strings.TrimSpace(p.Attrs.FgType) == ""
		q, ok := qty[p.SysID]
		if !ok {
			q = decimal.Zero
		}
		out = append(out, AttributeGapLine{
			SysID: p.SysID, ProductCode: p.ProductCode, ItemCode: p.ItemCode, ShadeCode: p.ShadeCode,
			Missing: missing, MissingFgType: fg, NotInReplica: notInReplica, Blocking: fg || notInReplica, QtyKg: q,
		})
	}
	return out
}

// buildDuplicates is §9e: every key held by > 1 active AX product.
func buildDuplicates(idx readinessIndex) []DuplicateGroup {
	out := []DuplicateGroup{}
	for k, ps := range idx.byKey {
		if len(ps) < 2 {
			continue
		}
		g := DuplicateGroup{
			ItemCode: ps[0].ItemCode, ShadeCode: ps[0].ShadeCode,
			Trial: strings.HasPrefix(k.ItemCode, trialItemPrefix), DemandQtyKg: decimal.Zero,
		}
		if q, ok := idx.demandKeys[k]; ok {
			g.InDemand, g.DemandQtyKg = true, q
		}
		for _, p := range ps {
			g.Products = append(g.Products, ProductRef{SysID: p.SysID, ProductCode: p.ProductCode, ShadeCode: p.ShadeCode})
		}
		sort.Slice(g.Products, func(i, j int) bool { return g.Products[i].SysID < g.Products[j].SysID })
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := NewErpProductKey(out[i].ItemCode, out[i].ShadeCode), NewErpProductKey(out[j].ItemCode, out[j].ShadeCode)
		if a.ItemCode != b.ItemCode {
			return a.ItemCode < b.ItemCode
		}
		return a.ShadeCode < b.ShadeCode
	})
	return out
}

// buildNoShade lists active linked products with an empty cpm_shade_code.
func buildNoShade(_ readinessInput, idx readinessIndex) []NoShadeLine {
	out := []NoShadeLine{}
	for k, ps := range idx.byKey {
		if k.ShadeCode != "" {
			continue
		}
		for _, p := range ps {
			out = append(out, NoShadeLine{
				SysID: p.SysID, ProductCode: p.ProductCode, ItemCode: p.ItemCode, TypeCode: p.TypeCode,
				ActualRows: p.ActualRows, SameItemNoShade: len(ps), ItemInDemand: idx.demandItem[k.ItemCode],
			})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].SysID < out[j].SysID })
	return out
}

// buildTypeMismatches is §9f (V-12 both ways).
func buildTypeMismatches(in readinessInput, idx readinessIndex) []TypeMismatchLine {
	out := []TypeMismatchLine{}
	for _, p := range in.linked {
		kind := domain.ItemKindForCode(p.ItemCode)
		if typeCompatible(kind, p.TypeCode) {
			continue
		}
		dir := MismatchMBOnYarn
		if kind == domain.ItemKindMB {
			dir = MismatchCmbOnNonMB
		}
		_, inDemand := idx.demandKeys[NewErpProductKey(p.ItemCode, p.ShadeCode)]
		out = append(out, TypeMismatchLine{
			SysID: p.SysID, ProductCode: p.ProductCode, ItemCode: p.ItemCode, ShadeCode: p.ShadeCode,
			TypeCode: p.TypeCode, Direction: dir, InDemand: inDemand,
		})
	}
	return out
}

func summarizeReadiness(in readinessInput, idx readinessIndex, rep *LinkReadinessReport) LinkReadinessSummary {
	s := LinkReadinessSummary{
		LinkedProducts: len(in.linked), ReplicaItems: in.replicaItems, DemandCombos: len(in.combos),
		DemandQtyKg: decimal.Zero, NoProduct: len(rep.NoProduct), GapScope: GapScopeAll,
		AttributeGapProducts: len(rep.AttributeGaps), DuplicateGroups: len(rep.Duplicates),
		LinkedNoShade: len(rep.LinkedNoShade), TypeMismatches: len(rep.TypeMismatches),
	}
	if in.demandAvailable {
		s.GapScope = GapScopeDemand
	}
	for _, c := range in.combos {
		s.DemandQtyKg = s.DemandQtyKg.Add(c.QtyKg)
		if len(idx.byKey[NewErpProductKey(c.ItemCode, c.ShadeCode)]) == 1 {
			s.MappedCombos++
		}
	}
	for _, l := range rep.NoProduct {
		if l.QtyKg.IsPositive() {
			s.NoProductWithQty++
		}
		if l.Suggestion == SuggestLinkCandidate {
			s.LinkCandidates++
		} else {
			s.CreateNew++
		}
	}
	for _, g := range rep.AttributeGaps {
		s.BlockingGapProducts += boolInt(g.Blocking)
		s.MissingFgType += boolInt(g.MissingFgType)
		s.NotInReplica += boolInt(g.NotInReplica)
	}
	for _, g := range rep.Duplicates {
		s.DuplicateProducts += len(g.Products)
		s.TrialDuplicateGroups += boolInt(g.Trial)
		s.DemandDuplicates += boolInt(g.InDemand)
	}
	for _, l := range rep.LinkedNoShade {
		s.LinkedNoShadeActual += boolInt(l.ActualRows > 0)
	}
	s.Pass = LinkReadinessPass{
		NoProductWithQty: s.NoProductWithQty == 0, AttributeGaps: s.BlockingGapProducts == 0,
		Duplicates: s.DuplicateGroups == 0, TypeMismatches: s.TypeMismatches == 0,
	}
	s.Ready = s.Pass.NoProductWithQty && s.Pass.AttributeGaps && s.Pass.Duplicates && s.Pass.TypeMismatches
	s.UniqueIndexReady = s.Pass.Duplicates
	return s
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
