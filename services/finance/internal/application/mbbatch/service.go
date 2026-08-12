// Package mbbatch implements the MB_BATCH cost calculation orchestration: computing
// cst_product_cost rows for every VALIDATED MB Head's auto-gen'd product, in nested-MB
// dependency order (design doc §10.3, PRD §8).
package mbbatch

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"maps"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
	"github.com/mutugading/goapps-backend/services/finance/pkg/safeconv"
)

// mbBatchCalcTypes are the 3 calc types computed for every MB, in the order required by
// step 3-4-5 of design doc §10.3: ACTUAL first (anchors the SHARED formulas), then
// SELLING/FORECAST (reuse the SHARED outputs via CAPP pre-seeding).
var mbBatchCalcTypes = []costcalcdom.CalculationType{
	costcalcdom.CalcTypeActual,
	costcalcdom.CalcTypeSelling,
	costcalcdom.CalcTypeForecast,
}

// Service runs the MB_BATCH compute orchestration.
type Service struct {
	db           *postgres.DB
	headReader   MBHeadReader
	edgeReader   MBEdgeReader
	resultWriter ResultWriter
	loader       costcalc.ProductLoader
	evalCache    *evaluator.Cache
}

// NewService constructs a Service.
func NewService(db *postgres.DB, headReader MBHeadReader, edgeReader MBEdgeReader, resultWriter ResultWriter, loader costcalc.ProductLoader, evalCache *evaluator.Cache) *Service {
	return &Service{
		db:           db,
		headReader:   headReader,
		edgeReader:   edgeReader,
		resultWriter: resultWriter,
		loader:       loader,
		evalCache:    evalCache,
	}
}

// BatchResult summarizes an MB_BATCH run's outcome, collecting per-MB failures rather than
// aborting the whole batch on the first error (mirrors mbpush.ExecuteResult).
type BatchResult struct {
	Period   string
	MBCount  int32
	RowCount int32
	Errors   []BatchError
}

// BatchError records one MB's compute-and-persist failure within a batch run.
type BatchError struct {
	MBHID string
	Error string
}

// RunMBBatch computes cst_product_cost rows (ACTUAL/SELLING/FORECAST) for every VALIDATED
// MB Head's auto-gen'd product, for the given period, in nested-MB dependency order
// (design doc §10.3).
//
// Nested MBs resolve in ONE run at any depth: results are written through tx, which the
// *sql.DB-backed loader cannot see under READ COMMITTED, so each MB's freshly computed
// cost is also published to a per-run batchCosts cache and overlaid on the loader's result
// (see batchcosts.go). Because candidates are topologically sorted deepest-child-first,
// a parent always finds its children already published. This keeps costcalc.ProductLoader
// — shared with the yarn path — untouched.
//
// A single MB's failure is recorded in the returned BatchResult.Errors and does not abort
// the remaining MBs in the batch (mirrors mbpush.ExecuteHandler.executeBatch).
// jobID is the cal_job row this batch runs under; it is stamped onto every persisted
// cst_product_cost row so results stay traceable to the job that produced them.
func (s *Service) RunMBBatch(ctx context.Context, period string, jobID int64) (*BatchResult, error) {
	candidates, err := BuildDAG(ctx, s.headReader, s.edgeReader)
	if err != nil {
		return nil, fmt.Errorf("run mb batch: %w", err)
	}
	result := &BatchResult{Period: period}
	if len(candidates) == 0 {
		return result, nil
	}
	err = s.db.Transaction(ctx, func(tx *sql.Tx) error {
		return s.runBatch(ctx, tx, candidates, period, jobID, result)
	})
	if err != nil {
		return nil, fmt.Errorf("run mb batch: %w", err)
	}
	return result, nil
}

func (s *Service) runBatch(ctx context.Context, tx *sql.Tx, candidates []MBHeadCandidate, period string, jobID int64, result *BatchResult) error {
	if _, err := tx.ExecContext(ctx, `SELECT pg_advisory_xact_lock(hashtext($1))`, "mb_batch:"+period); err != nil {
		return fmt.Errorf("acquire mb batch lock for period %s: %w", period, err)
	}
	costs := newBatchCosts(candidates)
	for _, c := range candidates {
		if err := s.runOneMB(ctx, tx, c, period, jobID, costs); err != nil {
			result.Errors = append(result.Errors, BatchError{MBHID: c.MBHID, Error: err.Error()})
			continue
		}
		result.MBCount++
		result.RowCount += safeconv.IntToInt32(len(mbBatchCalcTypes))
	}
	return nil
}

// runOneMB computes and persists all 3 calc-type rows for one MB's auto-gen'd product,
// isolated in its own savepoint so this MB's failure does not need to abort the whole batch
// caller's transaction (mirrors mbpush.ExecuteHandler.pushOneMB).
func (s *Service) runOneMB(ctx context.Context, tx *sql.Tx, c MBHeadCandidate, period string, jobID int64, costs *batchCosts) error {
	const savepoint = "sp_mb_batch"
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("savepoint: %w", err)
	}
	computed, err := s.computeAndPersist(ctx, tx, c, period, jobID, costs)
	if err != nil {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return fmt.Errorf("rollback to savepoint after %w: %w", err, rbErr)
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("release savepoint: %w", err)
	}
	// Published only after RELEASE: a rolled-back MB must not feed a value to its parent.
	costs.publish(c.CostProductID, computed)
	return nil
}

func (s *Service) computeAndPersist(ctx context.Context, tx *sql.Tx, c MBHeadCandidate, period string, jobID int64, costs *batchCosts) (map[costcalcdom.CalculationType]float64, error) {
	productSysID := c.CostProductID

	cappByProduct, err := s.loader.LoadCAPP(ctx, []int64{productSysID})
	if err != nil {
		return nil, fmt.Errorf("load capp: %w", err)
	}
	formulasByProduct, err := s.loader.LoadFormulas(ctx, []int64{productSysID})
	if err != nil {
		return nil, fmt.Errorf("load formulas: %w", err)
	}
	routesByProduct, err := s.loader.LoadRoutesByProducts(ctx, []int64{productSysID})
	if err != nil {
		return nil, fmt.Errorf("load route: %w", err)
	}
	route, ok := routesByProduct[productSysID]
	if !ok || route == nil {
		return nil, fmt.Errorf("no COMPLETE/LOCKED route found for product %d", productSysID)
	}

	allFormulas := formulasByProduct[productSysID]
	_, perType := partitionFormulas(allFormulas)
	capp := cappByProduct[productSysID]
	groupCodes := collectGroupCodes(route)
	nestedMBProducts := collectNestedMBProducts(route)

	outputs := make(map[costcalcdom.CalculationType]*costcalc.ComputeOutput, len(mbBatchCalcTypes))
	var sharedVals map[string]float64

	for _, calcType := range mbBatchCalcTypes {
		rmCosts, err := s.loader.LoadRMCosts(ctx, groupCodes, period, string(calcType))
		if err != nil {
			return nil, fmt.Errorf("load rm costs (%s): %w", calcType, err)
		}
		upstream, err := s.loadUpstreamCosts(ctx, upstreamRequest{
			products: nestedMBProducts,
			period:   period,
			calcType: calcType,
			parent:   c,
			jobID:    jobID,
			costs:    costs,
		})
		if err != nil {
			return nil, err
		}

		typeCAPP := capp
		formulas := perType
		if calcType == costcalcdom.CalcTypeActual {
			formulas = allFormulas
		} else {
			typeCAPP = mergeCAPP(capp, sharedVals)
		}

		out, err := costcalc.ComputeProduct(ctx, costcalc.ComputeInput{
			ProductSysID:  productSysID,
			Period:        period,
			CalcType:      calcType,
			Route:         route,
			CAPP:          typeCAPP,
			Formulas:      formulas,
			RMCosts:       rmCosts,
			UpstreamCosts: upstream,
			EvalCache:     s.evalCache,
		})
		if err != nil {
			return nil, fmt.Errorf("compute %s: %w", calcType, err)
		}
		outputs[calcType] = out

		if calcType == costcalcdom.CalcTypeActual {
			sharedVals = sharedOutputs(out.ParamSnapshot)
		}
	}

	computed := make(map[costcalcdom.CalculationType]float64, len(outputs))
	for _, calcType := range mbBatchCalcTypes {
		if err := s.persistResult(ctx, tx, productSysID, period, calcType, route.Head.HeadID, jobID, outputs[calcType]); err != nil {
			return nil, fmt.Errorf("persist %s: %w", calcType, err)
		}
		computed[calcType] = outputs[calcType].CostPerUnit
	}
	return computed, nil
}

// upstreamRequest carries everything loadUpstreamCosts needs, including the parent MB and
// job so a fallback can be logged with enough identity for finance to act on it.
type upstreamRequest struct {
	products []int64
	period   string
	calcType costcalcdom.CalculationType
	parent   MBHeadCandidate
	jobID    int64
	costs    *batchCosts
}

// loadUpstreamCosts resolves this MB's nested-MB PRODUCT-type RM references, for the given
// calc type. It reads the committed cst_product_cost rows via the loader, then overlays the
// costs this run has already computed (in.costs) so a child computed moments ago in the same
// batch wins over the previous run's value. No PRODUCT-type RMs returns an empty map.
func (s *Service) loadUpstreamCosts(ctx context.Context, in upstreamRequest) (map[int64]float64, error) {
	if len(in.products) == 0 {
		return map[int64]float64{}, nil
	}
	upstream, err := s.loader.LoadUpstreamCosts(ctx, in.products, in.period, string(in.calcType))
	if err != nil {
		return nil, fmt.Errorf("load upstream costs (%s): %w", in.calcType, err)
	}
	if upstream == nil {
		upstream = make(map[int64]float64, len(in.products))
	}
	in.costs.overlay(upstream, in.calcType, in.products)
	s.warnUnscheduledChildren(in, upstream)
	return upstream, nil
}

// warnUnscheduledChildren logs every nested-MB reference this run cannot compute because the
// child MB Head is not VALIDATED, so it was never a batch candidate. Its parent silently
// prices through whatever cst_product_cost already holds — the previous period's or run's
// figure, or nothing at all — and that is exactly the case finance needs told about rather
// than discovering as a wrong number weeks later.
func (s *Service) warnUnscheduledChildren(in upstreamRequest, upstream map[int64]float64) {
	for _, pid := range in.products {
		if in.costs.scheduled(pid) {
			continue
		}
		prior, hasPrior := upstream[pid]
		log.Warn().
			Int64("job_id", in.jobID).
			Str("period", in.period).
			Str("calc_type", string(in.calcType)).
			Str("parent_mbh_id", in.parent.MBHID).
			Str("parent_mb_code", in.parent.Code).
			Int64("parent_product_sys_id", in.parent.CostProductID).
			Int64("child_product_sys_id", pid).
			Bool("has_prior_cost", hasPrior).
			Float64("prior_cost_per_unit", prior).
			Msg("nested MB child is not VALIDATED so it is not computed in this batch: parent prices it from the previously committed cost, validate the child MB Head to have it recomputed here")
	}
}

// mergeCAPP layers sharedVals (the ACTUAL pass's SHARED formula outputs) over base CAPP,
// producing the CAPP map used for the SELLING/FORECAST passes.
func mergeCAPP(base, sharedVals map[string]float64) map[string]float64 {
	out := make(map[string]float64, len(base)+len(sharedVals))
	maps.Copy(out, base)
	maps.Copy(out, sharedVals)
	return out
}

func (s *Service) persistResult(ctx context.Context, tx *sql.Tx, productSysID int64, period string, calcType costcalcdom.CalculationType, routeHeadID, jobID int64, out *costcalc.ComputeOutput) error {
	r := newMBResult(productSysID, period, calcType, routeHeadID, jobID, out)
	_, _, _, _, err := s.resultWriter.UpsertWithSupersedeTx(ctx, tx, r)
	if err != nil {
		return fmt.Errorf("upsert with supersede: %w", err)
	}
	return nil
}

// newMBResult builds the cst_product_cost row for one MB calc-type pass. Pure so the
// argument positions — NewResult takes seven consecutive float64s — can be asserted in
// a test rather than trusted.
//
// captiveCost, deliveryCost, and vb1..vb5 are deliberately 0 (the insert's NULLIF turns
// them into NULL). They are yarn cost-sheet concepts — captive is row 61, delivery row
// 62 — that MB does not compute. Since ENG-MB-02 made LoadUpstreamCosts read
// cpc_cost_per_unit for MB-typed products, nothing reads them for MB either.
func newMBResult(productSysID int64, period string, calcType costcalcdom.CalculationType, routeHeadID, jobID int64, out *costcalc.ComputeOutput) *costcalcdom.Result {
	return costcalcdom.NewResult(
		productSysID, period, calcType, routeHeadID, 1,
		out.CostPerUnit, out.TotalRMCost, out.TotalConversion, out.TotalCost,
		0, "IDR",
		jsonOrNil(out.CostByLevel), jsonOrNil(out.RMCostDetail),
		jsonOrNil(out.ParamSnapshot), jsonOrNil(out.FormulaTrace),
		out.InputHash,
		jobID, "system:mb_batch",
		0, 0, // captiveCost, deliveryCost — yarn-only
		0, 0, 0, 0, 0, // vb1..vb5 del cost — yarn-only
	)
}

// jsonOrNil marshals v to JSON, returning nil on error (mirrors costcalc's process_chunk.go
// helper of the same name, which is unexported and therefore not reusable from mbbatch).
func jsonOrNil(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}
