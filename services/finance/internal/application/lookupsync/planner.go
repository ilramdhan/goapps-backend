package lookupsync

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// Skip reasons of a plan item (LookupSyncItem.skip_reason).
const (
	SkipLocked          = "LOCKED"
	SkipKeyEmpty        = "KEY_EMPTY"
	SkipKeyNotFound     = "KEY_NOT_FOUND"
	SkipMBSpinAmbiguous = "MB_SPIN_AMBIGUOUS"
	// SkipChildNotApplicable is not an item skip reason: it names the
	// per-child case where a fill-group child is not in
	// cost_product_applicable_param for the product. Such children are ignored
	// (the sync never adds applicability) and counted in
	// Summary.NotApplicableChildren.
	SkipChildNotApplicable = "CHILD_NOT_APPLICABLE"
)

// Change kinds (LookupSyncValueChange.change_kind).
const (
	ChangeSet   = "SET"
	ChangeClear = "CLEAR"
)

// numericScale is the scale of cost_product_parameter.cpp_value_numeric
// (NUMERIC(20,6)); values are compared and rendered at this scale.
const numericScale = 6

// defaultPageSize is the number of products read per SyncReader page.
const defaultPageSize = 200

// engineResolvedChildren mirrors ENGINE_RESOLVED_CHILDREN in the frontend
// lookup-fill.ts: children the calc engine resolves per period, never written
// or cleared by a lookup selection.
var engineResolvedChildren = map[string]struct{}{"OIL_RATE": {}}

// SyncableMasters are the lookup masters the sync re-applies (proto
// LookupMasterRef.lookup_master_code). RM_GROUP_OIL is deliberately absent: its
// children are engine/formula resolved.
var SyncableMasters = []string{
	MasterMachine, MasterIntermingling, MasterProductGrade,
	MasterMBHead, MasterMBSpin, MasterBoxBobbinCost,
}

// ValueChange is one child value that differs from the master.
type ValueChange struct {
	ParamID   uuid.UUID
	ParamCode string
	ParamName string
	// OldValue / NewValue are display strings ("" when empty / cleared).
	OldValue string
	NewValue string
	// Kind is ChangeSet or ChangeClear. CLEAR means DELETE the
	// cost_product_parameter row (cpp_one_value_chk forbids an all-NULL row).
	Kind string
	// NewNumeric / NewText carry the typed value to write for ChangeSet
	// (exactly one non-nil). Both nil for ChangeClear.
	NewNumeric *decimal.Decimal
	NewText    *string
}

// Item is one (product, trigger) pair of a plan.
type Item struct {
	ProductSysID     int64
	ProductCode      string
	ProductName      string
	TriggerParamCode string
	LookupMasterCode string
	MasterKey        string
	// SkipReason is "" when the item is applicable.
	SkipReason string
	Changes    []ValueChange
}

// Summary aggregates a whole plan.
type Summary struct {
	TotalProducts       int
	ProductsWithChanges int
	TotalValueChanges   int
	LockedProducts      int
	// SkipCounts counts items by SkipReason.
	SkipCounts map[string]int
	// NotApplicableChildren counts fill-group children ignored because they
	// are not applicable to the product (SkipChildNotApplicable).
	NotApplicableChildren int
	// Fingerprint is the plan fingerprint (see fingerprint.go).
	Fingerprint string
}

// Plan is a fully materialized plan.
type Plan struct {
	Summary Summary
	Items   []Item
}

// Planner computes what a lookup sync would change.
//
// It is stateless apart from its dependencies: every PlanSync / Walk call uses
// a fresh memo, so one Planner can be shared and used concurrently. The worker
// (P6) re-plans inside its transaction by calling ForReader with a tx-scoped
// SyncReader — this keeps one code path for preview, apply-time drift check
// and the write itself, instead of a separate PlanProduct helper with its own
// semantics.
type Planner struct {
	resolver FillResolver
	children FillGroupLister
	reader   SyncReader
	pageSize int
}

// NewPlanner creates a Planner. resolver is normally *Resolver; children is
// normally the parameter.Repository.
func NewPlanner(resolver FillResolver, children FillGroupLister, reader SyncReader) *Planner {
	return &Planner{resolver: resolver, children: children, reader: reader, pageSize: defaultPageSize}
}

// ForReader returns a copy of the Planner reading through r (e.g. a
// transaction-scoped reader in the worker).
func (p *Planner) ForReader(r SyncReader) *Planner {
	cp := *p
	cp.reader = r
	return &cp
}

// WithPageSize overrides the products-per-page of SyncReader reads (n > 0).
func (p *Planner) WithPageSize(n int) *Planner {
	if n > 0 {
		p.pageSize = n
	}
	return p
}

// PlanSync materializes the full plan for scope. Use Walk when the caller only
// needs a page of items (preview) to keep memory bounded.
func (p *Planner) PlanSync(ctx context.Context, scope Scope) (Plan, error) {
	var items []Item
	sum, err := p.Walk(ctx, scope, func(it Item) error {
		items = append(items, it)
		return nil
	})
	if err != nil {
		return Plan{}, err
	}
	return Plan{Summary: sum, Items: items}, nil
}

// PlanChunk plans scope restricted to productSysIDs (intersection). The worker
// uses it with ForReader to re-plan one chunk inside its transaction.
func (p *Planner) PlanChunk(ctx context.Context, scope Scope, productSysIDs []int64) (Plan, error) {
	scope.ProductSysIDs = productSysIDs
	return p.PlanSync(ctx, scope)
}

// Walk streams every plan item of scope to visit (in product order) and
// returns the summary of the whole plan. Products are read page by page.
//
// visit may be nil when only the summary is needed (CountLookupUsage, the
// Apply fingerprint check).
func (p *Planner) Walk(ctx context.Context, scope Scope, visit func(Item) error) (Summary, error) {
	if visit == nil {
		visit = func(Item) error { return nil }
	}
	run := newPlanRun(p)
	var after int64
	for {
		rows, err := p.reader.ListTriggerRows(ctx, scope, after, p.pageSize)
		if err != nil {
			return Summary{}, fmt.Errorf("list lookup trigger rows: %w", err)
		}
		if len(rows) == 0 {
			break
		}
		distinct, last, err := run.planPage(ctx, rows, visit)
		if err != nil {
			return Summary{}, err
		}
		after = last
		if distinct < p.pageSize {
			break
		}
	}
	return run.summary(), nil
}

// fillOutcome is a memoized Resolve result.
type fillOutcome struct {
	res      FillResult
	notFound bool
}

// planRun holds the per-call memo and running totals.
type planRun struct {
	p           *Planner
	fills       map[string]fillOutcome
	children    map[string][]*parameter.Parameter
	orionCounts map[string]int
	gradeNames  map[string]gradeName
	sum         Summary
	digests     [][]byte
}

type gradeName struct {
	code string
	ok   bool
}

func newPlanRun(p *Planner) *planRun {
	return &planRun{
		p:           p,
		fills:       map[string]fillOutcome{},
		children:    map[string][]*parameter.Parameter{},
		orionCounts: map[string]int{},
		gradeNames:  map[string]gradeName{},
		sum:         Summary{SkipCounts: map[string]int{}},
	}
}

// planPage plans one page of trigger rows. It returns the number of distinct
// products and the last product id of the page.
func (r *planRun) planPage(ctx context.Context, rows []TriggerRow, visit func(Item) error) (int, int64, error) {
	states, err := r.loadStates(ctx, rows)
	if err != nil {
		return 0, 0, err
	}
	distinct := 0
	for i := 0; i < len(rows); {
		j := i
		for j < len(rows) && rows[j].ProductSysID == rows[i].ProductSysID {
			j++
		}
		distinct++
		if err := r.planProduct(ctx, rows[i:j], states, visit); err != nil {
			return 0, 0, err
		}
		i = j
	}
	return distinct, rows[len(rows)-1].ProductSysID, nil
}

// loadStates loads the applicable child states of every product in the page.
func (r *planRun) loadStates(ctx context.Context, rows []TriggerRow) (map[ProductParamKey]ChildState, error) {
	productSet := map[int64]struct{}{}
	paramSet := map[uuid.UUID]struct{}{}
	productIDs := make([]int64, 0, len(rows))
	var paramIDs []uuid.UUID
	for _, row := range rows {
		if _, ok := productSet[row.ProductSysID]; !ok {
			productSet[row.ProductSysID] = struct{}{}
			productIDs = append(productIDs, row.ProductSysID)
		}
		kids, err := r.childrenOf(ctx, row.TriggerParamCode)
		if err != nil {
			return nil, err
		}
		for _, k := range kids {
			if _, ok := paramSet[k.ID()]; !ok {
				paramSet[k.ID()] = struct{}{}
				paramIDs = append(paramIDs, k.ID())
			}
		}
	}
	if len(paramIDs) == 0 {
		return map[ProductParamKey]ChildState{}, nil
	}
	states, err := r.p.reader.LoadChildStates(ctx, productIDs, paramIDs)
	if err != nil {
		return nil, fmt.Errorf("load child states: %w", err)
	}
	return states, nil
}

// planProduct plans all triggers of one product.
func (r *planRun) planProduct(ctx context.Context, rows []TriggerRow, states map[ProductParamKey]ChildState, visit func(Item) error) error {
	r.sum.TotalProducts++
	if rows[0].IsLocked {
		r.sum.LockedProducts++
	}
	hasChanges := false
	for _, row := range rows {
		it, err := r.planTrigger(ctx, row, states)
		if err != nil {
			return err
		}
		if it.SkipReason != "" {
			r.sum.SkipCounts[it.SkipReason]++
		} else if len(it.Changes) > 0 {
			hasChanges = true
			r.sum.TotalValueChanges += len(it.Changes)
			for _, c := range it.Changes {
				r.digests = append(r.digests, changeDigest(it.ProductSysID, c))
			}
		}
		if err := visit(it); err != nil {
			return err
		}
	}
	if hasChanges {
		r.sum.ProductsWithChanges++
	}
	return nil
}

func (r *planRun) summary() Summary {
	s := r.sum
	s.Fingerprint = fingerprint(r.digests)
	return s
}

// planTrigger plans one (product, trigger) pair.
func (r *planRun) planTrigger(ctx context.Context, row TriggerRow, states map[ProductParamKey]ChildState) (Item, error) {
	it := Item{
		ProductSysID:     row.ProductSysID,
		ProductCode:      row.ProductCode,
		ProductName:      row.ProductName,
		TriggerParamCode: row.TriggerParamCode,
		LookupMasterCode: row.LookupMasterCode,
		MasterKey:        displayKey(row),
	}
	if row.IsLocked {
		it.SkipReason = SkipLocked
		return it, nil
	}
	res, skip, err := r.resolveRow(ctx, row)
	if err != nil {
		return Item{}, err
	}
	if skip != "" {
		it.SkipReason = skip
		return it, nil
	}
	kids, err := r.childrenOf(ctx, row.TriggerParamCode)
	if err != nil {
		return Item{}, err
	}
	for _, child := range kids {
		state, applicable := states[ProductParamKey{ProductSysID: row.ProductSysID, ParamID: child.ID()}]
		if !applicable {
			r.sum.NotApplicableChildren++
			continue
		}
		if c, changed := childChange(child, res, state); changed {
			it.Changes = append(it.Changes, c)
		}
	}
	return it, nil
}

// displayKey is the key shown for a row: the stored text, else the spin id.
func displayKey(row TriggerRow) string {
	if row.StoredKey != "" {
		return row.StoredKey
	}
	if row.MBSpinID != nil {
		return row.MBSpinID.String()
	}
	return ""
}

// resolveRow resolves the master values of a row. It returns a skip reason
// instead of a result when the row cannot be synced.
func (r *planRun) resolveRow(ctx context.Context, row TriggerRow) (FillResult, string, error) {
	switch row.LookupMasterCode {
	case MasterMBSpin:
		return r.resolveMBSpinRow(ctx, row)
	case MasterProductGrade:
		return r.resolveGradeRow(ctx, row)
	default:
		if row.StoredKey == "" {
			return FillResult{}, SkipKeyEmpty, nil
		}
		return r.resolveKey(ctx, row.LookupMasterCode, row.StoredKey, row.TriggerParamCode)
	}
}

// resolveMBSpinRow: cpp_value_mb_spin_id wins over the text; a text-only
// reference to an ORION code shared by several spins is ambiguous.
func (r *planRun) resolveMBSpinRow(ctx context.Context, row TriggerRow) (FillResult, string, error) {
	if row.MBSpinID != nil {
		return r.resolveKey(ctx, MasterMBSpin, row.MBSpinID.String(), row.TriggerParamCode)
	}
	if row.StoredKey == "" {
		return FillResult{}, SkipKeyEmpty, nil
	}
	if _, err := uuid.Parse(row.StoredKey); err != nil {
		n, err := r.orionCount(ctx, row.StoredKey)
		if err != nil {
			return FillResult{}, "", err
		}
		if n > 1 {
			return FillResult{}, SkipMBSpinAmbiguous, nil
		}
	}
	return r.resolveKey(ctx, MasterMBSpin, row.StoredKey, row.TriggerParamCode)
}

// resolveGradeRow tries the stored key as pg_code, then as a legacy grade name.
func (r *planRun) resolveGradeRow(ctx context.Context, row TriggerRow) (FillResult, string, error) {
	if row.StoredKey == "" {
		return FillResult{}, SkipKeyEmpty, nil
	}
	res, skip, err := r.resolveKey(ctx, MasterProductGrade, row.StoredKey, row.TriggerParamCode)
	if err != nil || skip != SkipKeyNotFound {
		return res, skip, err
	}
	gn, err := r.gradeCodeByName(ctx, row.StoredKey)
	if err != nil {
		return FillResult{}, "", err
	}
	if !gn.ok || gn.code == row.StoredKey {
		return FillResult{}, SkipKeyNotFound, nil
	}
	return r.resolveKey(ctx, MasterProductGrade, gn.code, row.TriggerParamCode)
}

// resolveKey calls the Resolver once per (master, key, trigger) per run.
func (r *planRun) resolveKey(ctx context.Context, master, key, trigger string) (FillResult, string, error) {
	memoKey := master + "\x00" + key + "\x00" + trigger
	out, ok := r.fills[memoKey]
	if !ok {
		res, err := r.p.resolver.Resolve(ctx, master, key, trigger)
		switch {
		case err == nil:
			out = fillOutcome{res: res}
		case IsNotFound(err):
			out = fillOutcome{notFound: true}
		default:
			return FillResult{}, "", fmt.Errorf("resolve %s %q for %s: %w", master, key, trigger, err)
		}
		r.fills[memoKey] = out
	}
	if out.notFound {
		return FillResult{}, SkipKeyNotFound, nil
	}
	return out.res, "", nil
}

func (r *planRun) childrenOf(ctx context.Context, trigger string) ([]*parameter.Parameter, error) {
	if kids, ok := r.children[trigger]; ok {
		return kids, nil
	}
	kids, err := r.p.children.GetByFillGroup(ctx, trigger)
	if err != nil {
		return nil, fmt.Errorf("load fill-group children of %s: %w", trigger, err)
	}
	filtered := make([]*parameter.Parameter, 0, len(kids))
	for _, k := range kids {
		if _, skip := engineResolvedChildren[k.Code().String()]; !skip {
			filtered = append(filtered, k)
		}
	}
	r.children[trigger] = filtered
	return filtered, nil
}

func (r *planRun) orionCount(ctx context.Context, code string) (int, error) {
	if n, ok := r.orionCounts[code]; ok {
		return n, nil
	}
	n, err := r.p.reader.CountMBSpinsByOrionCode(ctx, code)
	if err != nil {
		return 0, fmt.Errorf("count mb spins by orion code %q: %w", code, err)
	}
	r.orionCounts[code] = n
	return n, nil
}

func (r *planRun) gradeCodeByName(ctx context.Context, name string) (gradeName, error) {
	if gn, ok := r.gradeNames[name]; ok {
		return gn, nil
	}
	code, ok, err := r.p.reader.ProductGradeCodeByName(ctx, name)
	if err != nil {
		return gradeName{}, fmt.Errorf("product grade by name %q: %w", name, err)
	}
	gn := gradeName{code: code, ok: ok}
	r.gradeNames[name] = gn
	return gn, nil
}
