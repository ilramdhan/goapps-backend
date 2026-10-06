// Package validation implements the ERP standard cost validators V-01..V-12
// (design Part 2 §7). Every validator is a pure function of an Input: the
// application layer (P5-T2 validate step) loads the I/O-backed facts (the PG
// replica sets for V-06, the source cost labels for V-09, the demand
// timestamp and the read-only posted probe for V-10) and passes them in.
// Nothing here performs I/O, reads a clock or touches Oracle.
//
// Money comparisons use github.com/shopspring/decimal only (forbidigo).
package validation

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Validation codes (design §7). V-03, V-06, V-08 and V-08w reuse the codes
// the derivation engine already attaches to std rows.
const (
	CodeV01  erpintegration.IssueCode = "V-01"
	CodeV02  erpintegration.IssueCode = "V-02"
	CodeV03                           = erpintegration.IssueV03
	CodeV04  erpintegration.IssueCode = "V-04"
	CodeV05  erpintegration.IssueCode = "V-05"
	CodeV06                           = erpintegration.IssueV06
	CodeV07  erpintegration.IssueCode = "V-07"
	CodeV08                           = erpintegration.IssueV08
	CodeV08w                          = erpintegration.IssueV08w
	CodeV09  erpintegration.IssueCode = "V-09"
	CodeV10  erpintegration.IssueCode = "V-10"
	CodeV11  erpintegration.IssueCode = "V-11"
	CodeV12  erpintegration.IssueCode = "V-12"
)

// codeOrder is the deterministic output order of the codes.
var codeOrder = []erpintegration.IssueCode{
	CodeV01, CodeV02, CodeV03, CodeV04, CodeV05, CodeV06, CodeV07,
	CodeV08, CodeV08w, CodeV09, CodeV10, CodeV11, CodeV12,
}

// Severity of a finding. Errors block VALIDATED; warnings need an ack by a
// user with the ack permission; info is reported only (V-05 first-seen, Q11).
type Severity = erpintegration.Severity

// Severities. SeverityInfo exists only here: it never blocks and needs no ack.
const (
	SeverityError   = erpintegration.SeverityError
	SeverityWarning = erpintegration.SeverityWarning
	SeverityInfo    = erpintegration.Severity("info")
)

// Scope is what a finding is attached to (design §7 "Scope" column).
type Scope string

// Scopes.
const (
	ScopeBatch     Scope = "batch"
	ScopeCoverage  Scope = "coverage"
	ScopeRow       Scope = "row"
	ScopePeriodSet Scope = "period_set"
)

var scopeRank = map[Scope]int{ScopeBatch: 0, ScopeCoverage: 1, ScopeRow: 2, ScopePeriodSet: 3}

// Finding is one validation result. Key is the std row key (ScopeRow), the
// (item, shade) combo with an empty grade (ScopeCoverage) or empty (batch).
// HeadSysID is set only for ScopePeriodSet findings (V-07 G7a).
type Finding struct {
	Code      erpintegration.IssueCode
	Severity  Severity
	Scope     Scope
	Key       erpintegration.ErpKey
	HeadSysID int64
	Message   string
}

// RequiresAck reports whether the finding is a warning that must be acked.
func (f Finding) RequiresAck() bool { return f.Severity == SeverityWarning }

// ReplicaSets are the PG ERP master replicas used by V-06: codes present in
// cost_erp_item, cost_erp_shade and cost_erp_grade (exact, trimmed codes).
type ReplicaSets struct {
	Items  map[string]struct{}
	Shades map[string]struct{}
	Grades map[string]struct{}
}

// SourceCost is the currency label and value of the source cst_product_cost
// row of a std row (V-09), keyed by cost id in Input.SourceCosts.
type SourceCost struct {
	Currency    string
	CostPerUnit decimal.Decimal
}

// CurrencyPolicy is the V-09 interim acceptance rule (P0-T10a helper
// CoverageCurrencyOK in the application layer): it decides whether a source
// cost label/value is acceptable for the period before 000557 is applied.
type CurrencyPolicy func(period, erpItemCode string, c SourceCost) bool

// DefaultDemandMaxAge is the V-10 demand freshness bound (Q11).
const DefaultDemandMaxAge = 24 * time.Hour

// Input is everything the validators read. Nil maps / pointers mean "not
// provided"; validators that need a missing fact fail closed with an error.
type Input struct {
	Period string
	Now    time.Time

	// Rows are the derived std rows of the batch (cst_erp_std_cost).
	Rows []erpintegration.StdRow
	// Coverage is the batch's coverage (cst_erp_coverage).
	Coverage []erpintegration.CoverageLine
	// Demand is the batch's ADJ demand snapshot (cst_erp_adj_demand).
	Demand []erpintegration.DemandLine

	// PrevStd is the std cost of the previous active batch per key (V-05);
	// a missing key is a first-seen combo (info only).
	PrevStd map[erpintegration.ErpKey]decimal.Decimal

	// Replica is required by V-06.
	Replica *ReplicaSets

	// SourceCosts maps cesc_ax_cost_sys_id to its label/value (V-09).
	SourceCosts map[int64]SourceCost
	// CurrencyRelabelApplied is true once 000557 relabelled the period: V-09
	// is then strict USD.
	CurrencyRelabelApplied bool
	// CurrencyPolicy is the interim rule; nil means strict USD (fail closed).
	CurrencyPolicy CurrencyPolicy

	// DemandLoadedAt is ceib_demand_loaded_at (V-10).
	DemandLoadedAt *time.Time
	// DemandMaxAge overrides DefaultDemandMaxAge when > 0.
	DemandMaxAge time.Duration
	// PostedHeads is the live read-only posted-head probe of the period (V-10);
	// nil means the probe was not run.
	PostedHeads *int64
}

// Validator is one rule.
type Validator interface {
	Code() erpintegration.IssueCode
	Validate(in *Input) []Finding
}

// validatorFunc adapts a function to Validator.
type validatorFunc struct {
	code erpintegration.IssueCode
	fn   func(in *Input) []Finding
}

func (v validatorFunc) Code() erpintegration.IssueCode { return v.code }
func (v validatorFunc) Validate(in *Input) []Finding   { return v.fn(in) }

// Registry runs a fixed list of validators.
type Registry struct {
	validators []Validator
}

// NewRegistry builds a registry of the given validators.
func NewRegistry(vs ...Validator) *Registry {
	return &Registry{validators: append([]Validator(nil), vs...)}
}

// DefaultRegistry returns V-01..V-12 (V-08 includes V-08w).
func DefaultRegistry() *Registry {
	return NewRegistry(
		V01(), V02(), V03(), V04(), V05(), V06(), V07(),
		V08(), V09(), V10(), V11(), V12(),
	)
}

// Codes lists the codes of the registered validators, in registration order.
func (r *Registry) Codes() []erpintegration.IssueCode {
	out := make([]erpintegration.IssueCode, len(r.validators))
	for i, v := range r.validators {
		out[i] = v.Code()
	}
	return out
}

// Run runs every validator and returns the findings in a deterministic order.
func (r *Registry) Run(in Input) Result {
	var all []Finding
	for _, v := range r.validators {
		all = append(all, v.Validate(&in)...)
	}
	SortFindings(all)
	return Result{Findings: all}
}

// Validate runs the default registry.
func Validate(in Input) Result { return DefaultRegistry().Run(in) }

// SortFindings orders findings by code (V-01..V-12), scope, key, head id and
// message, in place.
func SortFindings(fs []Finding) {
	sort.SliceStable(fs, func(i, j int) bool {
		a, b := fs[i], fs[j]
		if ra, rb := codeRank(a.Code), codeRank(b.Code); ra != rb {
			return ra < rb
		}
		if a.Scope != b.Scope {
			return scopeRank[a.Scope] < scopeRank[b.Scope]
		}
		if ka, kb := a.Key.String(), b.Key.String(); ka != kb {
			return ka < kb
		}
		if a.HeadSysID != b.HeadSysID {
			return a.HeadSysID < b.HeadSysID
		}
		if a.Severity != b.Severity {
			return a.Severity < b.Severity
		}
		return a.Message < b.Message
	})
}

func codeRank(c erpintegration.IssueCode) int {
	for i, v := range codeOrder {
		if v == c {
			return i
		}
	}
	return len(codeOrder)
}

// Result is the ordered outcome of a validation run.
type Result struct {
	Findings []Finding
}

func (r Result) filter(s Severity) []Finding {
	var out []Finding
	for _, f := range r.Findings {
		if f.Severity == s {
			out = append(out, f)
		}
	}
	return out
}

// Errors returns the error findings.
func (r Result) Errors() []Finding { return r.filter(SeverityError) }

// Warnings returns the warning findings (each requires an ack).
func (r Result) Warnings() []Finding { return r.filter(SeverityWarning) }

// Infos returns the info findings.
func (r Result) Infos() []Finding { return r.filter(SeverityInfo) }

// HasErrors reports whether any finding is an error.
func (r Result) HasErrors() bool { return len(r.Errors()) > 0 }

// NeedsAck reports whether any warning requires an ack.
func (r Result) NeedsAck() bool { return len(r.Warnings()) > 0 }

// Counts returns the number of findings per severity.
func (r Result) Counts() map[Severity]int {
	out := map[Severity]int{SeverityError: 0, SeverityWarning: 0, SeverityInfo: 0}
	for _, f := range r.Findings {
		out[f.Severity]++
	}
	return out
}

// CountsByCode returns the number of findings per code.
func (r Result) CountsByCode() map[erpintegration.IssueCode]int {
	out := map[erpintegration.IssueCode]int{}
	for _, f := range r.Findings {
		out[f.Code]++
	}
	return out
}

// RowIssues groups row-scope findings by std row key as the Issues stored in
// cesc_validation.
func (r Result) RowIssues() map[erpintegration.ErpKey][]erpintegration.Issue {
	out := map[erpintegration.ErpKey][]erpintegration.Issue{}
	for _, f := range r.Findings {
		if f.Scope != ScopeRow {
			continue
		}
		out[f.Key] = append(out[f.Key], erpintegration.Issue{Key: f.Key, Code: f.Code, Severity: f.Severity, Message: f.Message})
	}
	return out
}

// WarningSetHash is the sha256 (hex) of the ordered warning set. An ack is
// bound to this hash, so a re-validation that changes the warnings
// invalidates the ack. Empty when there are no warnings.
func (r Result) WarningSetHash() string {
	ws := r.Warnings()
	if len(ws) == 0 {
		return ""
	}
	h := sha256.New()
	for _, f := range ws {
		_, _ = h.Write([]byte(strings.Join([]string{
			string(f.Code), string(f.Scope), f.Key.String(),
			strconv.FormatInt(f.HeadSysID, 10), f.Message,
		}, "\x1f") + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

// rowFinding builds a row-scope finding.
func rowFinding(code erpintegration.IssueCode, sev Severity, key erpintegration.ErpKey, msg string) Finding {
	return Finding{Code: code, Severity: sev, Scope: ScopeRow, Key: key, Message: msg}
}

// coverageKey is the ErpKey form of a coverage combo (grade left empty).
func coverageKey(l erpintegration.CoverageLine) erpintegration.ErpKey {
	return erpintegration.ErpKey{ItemCode: l.ItemCode, ShadeCode: l.ShadeCode}
}
