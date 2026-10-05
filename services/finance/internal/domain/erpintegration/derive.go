package erpintegration

// derive.go is the pure derivation engine (design Part 2 §6.1-6.3, plan-05
// P4-T1). It reproduces the legacy std-value insert procedure + std-cost trigger
// formulas with shopspring/decimal and Round5 (half away from zero, 5 dp)
// applied only at the points the design lists. No I/O, no clock, no globals.

import (
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// DeriveStatus is the derivation outcome of one std row (cesc_status).
type DeriveStatus string

// Derive statuses (the chk on cst_erp_std_cost.cesc_status).
const (
	DeriveOK           DeriveStatus = "OK"
	DeriveNoAX         DeriveStatus = "NO_AX"
	DeriveNoRule       DeriveStatus = "NO_RULE"
	DeriveNoFgType     DeriveStatus = "NO_FG_TYPE"
	DeriveNoGradeGroup DeriveStatus = "NO_GRADE_GROUP"
	DeriveNoSellPrice  DeriveStatus = "NO_SELL_PRICE"
	DeriveInvalid      DeriveStatus = "INVALID"
)

var allDeriveStatuses = []DeriveStatus{
	DeriveOK, DeriveNoAX, DeriveNoRule, DeriveNoFgType,
	DeriveNoGradeGroup, DeriveNoSellPrice, DeriveInvalid,
}

// ErrInvalidDeriveStatus is returned for an unknown derive status.
var ErrInvalidDeriveStatus = errors.New("erpintegration: invalid derive status")

// ParseDeriveStatus parses an exact stored value.
func ParseDeriveStatus(s string) (DeriveStatus, error) {
	for _, v := range allDeriveStatuses {
		if string(v) == s {
			return v, nil
		}
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidDeriveStatus, s)
}

// String returns the stored value.
func (s DeriveStatus) String() string { return string(s) }

// StdSource is the origin of a std row (cesc_source / GSC_SOURCE).
type StdSource string

// Std sources.
const (
	SourceAX      StdSource = "GOAPPS_AX"
	SourceDerived StdSource = "GOAPPS_DERIVED"
	SourceMB      StdSource = "GOAPPS_MB"
)

// String returns the stored value.
func (s StdSource) String() string { return string(s) }

// maxPushCodeLen is the ERP-side width of item / grade / shade codes.
const maxPushCodeLen = 12

// maxStdNameLen is the width of GSC_ITEM_NAME / GSC_SHADE_NAME.
const maxStdNameLen = 240

// ErpKey is the ERP natural key of a std row: (item, grade, shade).
type ErpKey struct {
	ItemCode  string
	GradeCode string
	ShadeCode string
}

// String renders the key for messages: "POY123/AX/NL".
func (k ErpKey) String() string {
	return k.ItemCode + "/" + k.GradeCode + "/" + k.ShadeCode
}

// PushSafe reports whether every code is non-empty, at most 12 characters,
// printable ASCII and without surrounding blanks (V-06 / C-11).
func (k ErpKey) PushSafe() bool {
	return pushSafeCode(k.ItemCode) && pushSafeCode(k.GradeCode) && pushSafeCode(k.ShadeCode)
}

func pushSafeCode(c string) bool {
	if c == "" || len(c) > maxPushCodeLen || strings.TrimSpace(c) != c {
		return false
	}
	for i := 0; i < len(c); i++ {
		if c[i] < 0x20 || c[i] > 0x7e {
			return false
		}
	}
	return true
}

// IssueCode is a validation code attached by the derivation engine.
type IssueCode string

// Issue codes raised by Derive (the rest of V-01..V-12 run at validate).
const (
	IssueV03  IssueCode = "V-03" // numeric component outside the FLEX range
	IssueV06  IssueCode = "V-06" // key not push-safe
	IssueV08  IssueCode = "V-08" // missing AX / rule / group / fg type / price
	IssueV08w IssueCode = "V-08w"
)

// Severity is the severity of an Issue.
type Severity string

// Severities.
const (
	SeverityError   Severity = "error"
	SeverityWarning Severity = "warning"
)

// Issue is one validation result of a std row (stored in cesc_validation).
type Issue struct {
	Key      ErpKey
	Code     IssueCode
	Severity Severity
	Message  string
}

// AxComponents are the cost inputs read from the active APPROVED ACTUAL
// cst_product_cost row of the linked AX product (design §6.1).
type AxComponents struct {
	CostID       int64
	Version      int32
	ProductSysID int64
	CostPerUnit  decimal.Decimal // cpc_cost_per_unit (USD/kg)
	TotalRMCost  decimal.Decimal // cpc_total_rm_cost
	FgType       string
	ChpItemCode  string
	MsBatchItem  string
	ItemType     string
	PrdPerDay    *decimal.Decimal
}

// DeriveInput is one (item, grade, shade) to value. Ax is nil when coverage
// found no usable AX cost (NO_AX). An empty Kind is derived from the item
// code (CMB… is MB).
type DeriveInput struct {
	Key       ErpKey
	Kind      ItemKind
	Ax        *AxComponents
	ItemName  string
	ShadeName string
}

// StdRow is one derived standard cost row (cst_erp_std_cost). Numeric
// components are NULL (Valid=false) when the status prevented computing
// them. Built only by Derive.
type StdRow struct {
	Key        ErpKey
	Kind       ItemKind
	ItemName   string
	ShadeName  string
	Source     StdSource
	Status     DeriveStatus
	ProdType   erprule.ProdType   // empty for AX and MB rows
	GradeGroup erprule.GradeGroup // AX for AX and MB rows
	FgType     string
	Basis      erprule.Basis

	AxCostSysID   *int64
	AxCostVersion *int32
	ProductSysID  *int64

	ChpItemCode string
	MsBatchItem string
	ItemType    string
	PrdPerDay   decimal.NullDecimal

	ChpConKg     decimal.NullDecimal
	ChpCost      decimal.NullDecimal
	AxConvCost   decimal.NullDecimal
	ConvCost     decimal.NullDecimal
	ConvCost1    decimal.NullDecimal
	ConvCost2    decimal.NullDecimal
	ConvCost4    decimal.NullDecimal
	ConvCost5    decimal.NullDecimal
	SellingPrice decimal.NullDecimal
	ValueLoss    decimal.NullDecimal
	AxCost       decimal.NullDecimal
	StdCost      decimal.NullDecimal
	ProdValLoss  decimal.NullDecimal

	Issues []Issue
}

// NumericComponents returns every cost component with its column name, in a
// stable order. These are the values bounded by the FLEX / NUMBER range
// check (V-03); prd_per_day is a pass-through attribute, not a cost, and is
// not included.
func (r StdRow) NumericComponents() []NamedAmount {
	return []NamedAmount{
		{"chp_con_kg", r.ChpConKg}, {"chp_cost", r.ChpCost},
		{"ax_conv_cost", r.AxConvCost}, {"conv_cost", r.ConvCost},
		{"conv_cost1", r.ConvCost1}, {"conv_cost2", r.ConvCost2},
		{"conv_cost4", r.ConvCost4}, {"conv_cost5", r.ConvCost5},
		{"selling_price", r.SellingPrice}, {"value_loss", r.ValueLoss},
		{"ax_cost", r.AxCost}, {"std_cost", r.StdCost},
		{"prod_value_loss", r.ProdValLoss},
	}
}

// NamedAmount is a named nullable numeric component.
type NamedAmount struct {
	Name  string
	Value decimal.NullDecimal
}

// HasErrors reports whether any issue of the row is an error.
func (r StdRow) HasErrors() bool {
	for _, is := range r.Issues {
		if is.Severity == SeverityError {
			return true
		}
	}
	return false
}

// flexLimit is the smallest magnitude FM990D00000 cannot render.
var flexLimit = decimal.NewFromInt(1000)

// Derive values every input with the rule set. It returns one row per input,
// in input order, and the concatenation of the rows' issues. A nil rule set
// is treated as empty (every derived grade is NO_GRADE_GROUP).
func Derive(inputs []DeriveInput, rs *erprule.RuleSet) ([]StdRow, []Issue) {
	rows := make([]StdRow, 0, len(inputs))
	var issues []Issue
	for _, in := range inputs {
		row := DeriveRow(in, rs)
		rows = append(rows, row)
		issues = append(issues, row.Issues...)
	}
	return rows, issues
}

// DeriveRow values one input (design §6.2).
func DeriveRow(in DeriveInput, rs *erprule.RuleSet) StdRow {
	row := StdRow{
		Key:       in.Key,
		Kind:      in.Kind,
		ItemName:  truncateRunes(strings.TrimSpace(in.ItemName), maxStdNameLen),
		ShadeName: truncateRunes(strings.TrimSpace(in.ShadeName), maxStdNameLen),
	}
	if row.Kind == "" {
		row.Kind = ItemKindForCode(in.Key.ItemCode)
	}
	if _, err := ParseItemKind(string(row.Kind)); err != nil {
		return row.withStatus(DeriveInvalid, IssueV06, "invalid item kind "+fmt.Sprintf("%q", row.Kind))
	}

	switch {
	case row.Kind == ItemKindMB:
		row.Source = SourceMB
	case in.Key.GradeCode == GradeAX:
		row.Source = SourceAX
	default:
		row.Source = SourceDerived
	}

	if in.Ax == nil {
		return row.withStatus(DeriveNoAX, IssueV08, "no active approved AX cost for "+in.Key.ItemCode+"/"+in.Key.ShadeCode)
	}
	row.copyAx(in.Ax)

	switch row.Source {
	case SourceMB:
		if in.Key.GradeCode != GradeMBA {
			return row.withStatus(DeriveNoRule, IssueV08, "masterbatch is valued for grade A only")
		}
		row.valueAsBase(in.Ax)
	case SourceAX:
		row.valueAsBase(in.Ax)
	default:
		row.valueDerived(in.Ax, rs)
	}
	if row.Status == DeriveOK {
		row.checkPushable()
	}
	return row
}

// copyAx copies the AX identifiers and pass-through attributes.
func (r *StdRow) copyAx(ax *AxComponents) {
	id, ver, prod := ax.CostID, ax.Version, ax.ProductSysID
	r.AxCostSysID, r.AxCostVersion, r.ProductSysID = &id, &ver, &prod
	r.FgType = strings.TrimSpace(ax.FgType)
	r.ChpItemCode = strings.TrimSpace(ax.ChpItemCode)
	r.MsBatchItem = strings.TrimSpace(ax.MsBatchItem)
	r.ItemType = strings.TrimSpace(ax.ItemType)
	if r.ItemType == "" {
		r.ItemType = itemPrefix(r.Key.ItemCode)
	}
	if ax.PrdPerDay != nil {
		r.PrdPerDay = valid(*ax.PrdPerDay) // pass-through; not an R5 point (§6.2)
	}
}

// axParts are the AX-derived components shared by every source.
type axParts struct {
	chpKg   decimal.Decimal // chp_cost × chp_con_kg
	axConv  decimal.Decimal // R5(CostPerUnit − TotalRMCost)
	axCost  decimal.Decimal // R5(chp×kg + R5(ax_conv))
	chpCost decimal.Decimal
	conKg   decimal.Decimal
}

func computeAxParts(ax *AxComponents) axParts {
	chp := Round5(ax.TotalRMCost)
	kg := decimal.NewFromInt(1) // C-7: legacy RM_NORM literal 1
	axConv := Round5(ax.CostPerUnit.Sub(ax.TotalRMCost))
	chpKg := chp.Mul(kg)
	return axParts{
		chpKg:   chpKg,
		axConv:  axConv,
		axCost:  Round5(chpKg.Add(axConv)),
		chpCost: chp,
		conKg:   kg,
	}
}

func (r *StdRow) setAxParts(p axParts) {
	r.ChpCost = valid(p.chpCost)
	r.ChpConKg = valid(p.conKg)
	r.AxConvCost = valid(p.axConv)
	r.AxCost = valid(p.axCost)
}

// valueAsBase values a yarn AX row or an MB grade-A row: basis COST, no
// value loss, no selling price, conv = R5(ax_conv), tiers = conv (T-4).
// Yarn AX: std = R5(chp×kg + conv), pvl = R5(std − ax_cost) (= 0).
func (r *StdRow) valueAsBase(ax *AxComponents) {
	p := computeAxParts(ax)
	r.setAxParts(p)
	r.Status = DeriveOK
	r.GradeGroup = erprule.GradeGroupAX
	r.Basis = erprule.BasisCost
	r.ValueLoss = valid(decimal.Zero)
	r.SellingPrice = valid(decimal.Zero)
	conv := p.axConv
	r.ConvCost = valid(conv)
	r.setTiers(conv)
	if r.Source == SourceMB {
		// Legacy block E takes CMCH_COST_VALUATION as-is (the std trigger
		// skips CMB rows): std = R5(CostPerUnit), prod value loss = 0.
		r.StdCost = valid(Round5(ax.CostPerUnit))
		r.ProdValLoss = valid(decimal.Zero)
		return
	}
	std := Round5(p.chpKg.Add(conv))
	r.StdCost = valid(std)
	r.ProdValLoss = valid(Round5(std.Sub(p.axCost)))
}

// valueDerived values a non-AX yarn grade through the rule set.
func (r *StdRow) valueDerived(ax *AxComponents, rs *erprule.RuleSet) {
	p := computeAxParts(ax)
	r.setAxParts(p)
	b := resolveBasis(r.Key.ItemCode, r.Key.GradeCode, ax.FgType, rs)
	r.ProdType = b.prodType
	r.GradeGroup = b.gradeGroup
	if b.prodIssue != nil {
		is := *b.prodIssue
		is.Key = r.Key
		r.Issues = append(r.Issues, is)
	}
	if b.status != DeriveOK {
		r.Basis = b.basis
		*r = r.withStatus(b.status, IssueV08, b.failure)
		return
	}
	r.Status = DeriveOK
	r.Basis = b.basis
	loss := Round5(b.valLoss)
	r.ValueLoss = valid(loss)

	var std decimal.Decimal
	if b.basis == erprule.BasisCost {
		conv := Round5(p.axConv.Sub(loss))
		r.SellingPrice = valid(decimal.Zero)
		r.ConvCost = valid(conv)
		r.setTiers(conv) // interim tier_n = R5(ax_conv): R5(tier_n − value_loss) = conv
		std = Round5(p.chpKg.Add(conv))
	} else {
		sell := Round5(b.sellPrice)
		r.SellingPrice = valid(sell)
		r.ConvCost = valid(decimal.Zero)
		r.setTiers(decimal.Zero)
		std = Round5(sell.Sub(loss))
	}
	r.StdCost = valid(std)
	r.ProdValLoss = valid(Round5(std.Sub(p.axCost)))
	if r.MsBatchItem == "" {
		r.MsBatchItem = "0" // legacy NVL(AX.FG_MS_BATCH_ITEM,0) on derived rows
	}
}

func (r *StdRow) setTiers(v decimal.Decimal) {
	r.ConvCost1, r.ConvCost2, r.ConvCost4, r.ConvCost5 = valid(v), valid(v), valid(v), valid(v)
}

// checkPushable turns an OK row INVALID when its key is not push-safe
// (V-06) or a numeric component cannot be rendered by FM990D00000 (V-03).
func (r *StdRow) checkPushable() {
	if !r.Key.PushSafe() {
		*r = r.withStatus(DeriveInvalid, IssueV06, "ERP key "+fmt.Sprintf("%q", r.Key.String())+" is not push-safe (non-empty, <= 12 ASCII)")
		return
	}
	for _, c := range r.NumericComponents() {
		if c.Value.Valid && c.Value.Decimal.Abs().GreaterThanOrEqual(flexLimit) {
			*r = r.withStatus(DeriveInvalid, IssueV03, c.Name+" "+c.Value.Decimal.String()+" is outside [-999.99999, 999.99999]")
			return
		}
	}
	if !r.ChpConKg.Valid || !r.ChpConKg.Decimal.IsPositive() {
		*r = r.withStatus(DeriveInvalid, IssueV03, "chp_con_kg must be > 0")
	}
}

// withStatus sets a non-OK status and appends the matching error issue.
func (r StdRow) withStatus(status DeriveStatus, code IssueCode, msg string) StdRow {
	r.Status = status
	r.Issues = append(r.Issues, Issue{Key: r.Key, Code: code, Severity: SeverityError, Message: string(status) + ": " + msg})
	return r
}

func valid(d decimal.Decimal) decimal.NullDecimal {
	return decimal.NullDecimal{Decimal: d, Valid: true}
}

func truncateRunes(s string, n int) string {
	rs := []rune(s)
	if len(rs) <= n {
		return s
	}
	return string(rs[:n])
}
