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
// KNOWN GAP (pre-existing, not introduced by ENG-MB-02): the dependency ordering does NOT
// currently make a child's cost visible to its parent within the same run. Results are
// written through tx (persistResult), but the parent's upstream read goes through
// s.loader, which holds the *sql.DB pool rather than this transaction
// (costcalc.productLoader). Under READ COMMITTED the uncommitted in-batch writes are
// invisible, so a parent whose child is computed in this same batch reads the previous
// committed run's value — or fails with ErrMissingUpstreamCost if no committed row exists
// yet. Ordering only pays off across runs. Closing this needs the loader to accept a
// transaction, which changes the interface shared with the yarn path; tracked separately.
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
	for _, c := range candidates {
		if err := s.runOneMB(ctx, tx, c, period, jobID); err != nil {
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
func (s *Service) runOneMB(ctx context.Context, tx *sql.Tx, c MBHeadCandidate, period string, jobID int64) error {
	const savepoint = "sp_mb_batch"
	if _, err := tx.ExecContext(ctx, "SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("savepoint: %w", err)
	}
	if err := s.computeAndPersist(ctx, tx, c, period, jobID); err != nil {
		if _, rbErr := tx.ExecContext(ctx, "ROLLBACK TO SAVEPOINT "+savepoint); rbErr != nil {
			return fmt.Errorf("rollback to savepoint after %w: %w", err, rbErr)
		}
		return err
	}
	if _, err := tx.ExecContext(ctx, "RELEASE SAVEPOINT "+savepoint); err != nil {
		return fmt.Errorf("release savepoint: %w", err)
	}
	return nil
}

func (s *Service) computeAndPersist(ctx context.Context, tx *sql.Tx, c MBHeadCandidate, period string, jobID int64) error {
	productSysID := c.CostProductID

	cappByProduct, err := s.loader.LoadCAPP(ctx, []int64{productSysID})
	if err != nil {
		return fmt.Errorf("load capp: %w", err)
	}
	formulasByProduct, err := s.loader.LoadFormulas(ctx, []int64{productSysID})
	if err != nil {
		return fmt.Errorf("load formulas: %w", err)
	}
	routesByProduct, err := s.loader.LoadRoutesByProducts(ctx, []int64{productSysID})
	if err != nil {
		return fmt.Errorf("load route: %w", err)
	}
	route, ok := routesByProduct[productSysID]
	if !ok || route == nil {
		return fmt.Errorf("no COMPLETE/LOCKED route found for product %d", productSysID)
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
			return fmt.Errorf("load rm costs (%s): %w", calcType, err)
		}
		upstream, err := s.loadUpstreamCosts(ctx, nestedMBProducts, period, string(calcType))
		if err != nil {
			return err
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
			return fmt.Errorf("compute %s: %w", calcType, err)
		}
		outputs[calcType] = out

		if calcType == costcalcdom.CalcTypeActual {
			sharedVals = sharedOutputs(out.ParamSnapshot)
		}
	}

	for _, calcType := range mbBatchCalcTypes {
		if err := s.persistResult(ctx, tx, productSysID, period, calcType, route.Head.HeadID, jobID, outputs[calcType]); err != nil {
			return fmt.Errorf("persist %s: %w", calcType, err)
		}
	}
	return nil
}

// loadUpstreamCosts resolves this MB's nested-MB PRODUCT-type RM references from
// cst_product_cost, for the given calc type. No PRODUCT-type RMs returns an empty map.
func (s *Service) loadUpstreamCosts(ctx context.Context, products []int64, period, calcType string) (map[int64]float64, error) {
	if len(products) == 0 {
		return map[int64]float64{}, nil
	}
	upstream, err := s.loader.LoadUpstreamCosts(ctx, products, period, calcType)
	if err != nil {
		return nil, fmt.Errorf("load upstream costs (%s): %w", calcType, err)
	}
	return upstream, nil
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
