package mbbatch

import (
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// batchCosts is the per-run in-memory cost cache that makes a child MB's freshly computed
// cost visible to its parent inside the SAME batch run.
//
// Why it exists: MB results are written through the batch's *sql.Tx, but the parent's
// upstream read goes through costcalc.ProductLoader, which holds the *sql.DB pool. Under
// READ COMMITTED the uncommitted in-batch rows are invisible to that pool, so before this
// cache a parent read the PREVIOUS committed run's child cost — meaning a correction only
// propagated one nesting level per run and users had to trigger MB_BATCH as many times as
// the recipe was deep. Caching the value the batch just computed closes that gap without
// touching ProductLoader, which the yarn calc path shares.
//
// Not safe for concurrent use, and deliberately so: one instance is created per RunMBBatch
// call and is confined to that call's goroutine. It is never stored on Service, so no state
// leaks between runs.
type batchCosts struct {
	// byType is calc type -> product sys id -> cost per unit, populated only after an MB's
	// savepoint is released (a rolled-back MB must not leak a value to its parent).
	byType map[costcalcdom.CalculationType]map[int64]float64
	// inBatch is the product sys id of every VALIDATED MB scheduled in this run. A nested-MB
	// reference outside this set is a child that is not VALIDATED, so it can never be
	// computed here and must fall back to whatever cst_product_cost already holds.
	inBatch map[int64]struct{}
}

// newBatchCosts sizes the cache from the run's candidate set.
func newBatchCosts(candidates []MBHeadCandidate) *batchCosts {
	b := &batchCosts{
		byType:  make(map[costcalcdom.CalculationType]map[int64]float64, len(mbBatchCalcTypes)),
		inBatch: make(map[int64]struct{}, len(candidates)),
	}
	for _, ct := range mbBatchCalcTypes {
		b.byType[ct] = make(map[int64]float64, len(candidates))
	}
	for _, c := range candidates {
		b.inBatch[c.CostProductID] = struct{}{}
	}
	return b
}

// publish records one MB's per-calc-type cost per unit. Called only after that MB's
// savepoint is released, so a failed MB contributes nothing.
func (b *batchCosts) publish(productSysID int64, costs map[costcalcdom.CalculationType]float64) {
	for calcType, v := range costs {
		m, ok := b.byType[calcType]
		if !ok {
			m = map[int64]float64{}
			b.byType[calcType] = m
		}
		m[productSysID] = v
	}
}

// overlay writes this run's cached costs over dst for the requested products. In-batch
// values win over the loader's committed values: they are the same period and calc type,
// computed moments ago from the current masters, so they are strictly fresher.
func (b *batchCosts) overlay(dst map[int64]float64, calcType costcalcdom.CalculationType, products []int64) {
	m := b.byType[calcType]
	if len(m) == 0 {
		return
	}
	for _, pid := range products {
		if v, ok := m[pid]; ok {
			dst[pid] = v
		}
	}
}

// scheduled reports whether a product belongs to an MB that this run computes.
func (b *batchCosts) scheduled(productSysID int64) bool {
	_, ok := b.inBatch[productSysID]
	return ok
}
