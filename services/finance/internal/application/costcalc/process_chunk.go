package costcalc

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// Block-reason constants written into cal_job_product.cjp_block_reason. Kept
// in one place so the UI / metrics dashboard can rely on a stable taxonomy.
const (
	blockReasonMissingRoute    = "MISSING_ROUTE"
	blockReasonMissingCAPP     = "MISSING_CAPP_VALUE"
	blockReasonMissingRMCost   = "MISSING_RM_COST"
	blockReasonMissingUpstream = "MISSING_UPSTREAM_COST"
	blockReasonFormulaError    = "FORMULA_ERROR"
)

// ProcessChunkInput is the slice of work passed into ProcessChunk.
type ProcessChunkInput struct {
	JobID    int64
	ChunkID  int64 // 0 means inline path with no chunk row to update
	Period   string
	CalcType costcalcdom.CalculationType
	Products []int64
	Actor    string
}

// ProcessChunkOutput summarizes the chunk-level outcome.
type ProcessChunkOutput struct {
	Success int
	Failed  int
	Blocked int
}

// ProcessChunk loads everything for the chunk's products in bulk and computes
// each product in turn. Per-product failures are isolated — one bad product
// does not abort the chunk. When ChunkID != 0 the chunk row is transitioned
// PROCESSING -> SUCCESS / PARTIAL_FAILED / FAILED based on the outcome.
//
// Transactional isolation per product via SAVEPOINT lands in S8c when real
// multi-product chunks arrive; for the S8b inline SINGLE_PRODUCT path the
// resultRepo.UpsertWithSupersede already runs its own transaction.
func (s *Service) ProcessChunk(ctx context.Context, in ProcessChunkInput) (*ProcessChunkOutput, error) {
	if len(in.Products) == 0 {
		return &ProcessChunkOutput{}, nil
	}

	if in.ChunkID != 0 {
		if err := s.chunkRepo.UpdateStatus(ctx, in.ChunkID, costcalcdom.ChunkStatusProcessing, "inline"); err != nil {
			return nil, fmt.Errorf("mark chunk processing: %w", err)
		}
	}

	loaded, err := s.bulkLoad(ctx, in)
	if err != nil {
		return nil, fmt.Errorf("bulk load: %w", err)
	}

	out := &ProcessChunkOutput{}
	for _, pid := range in.Products {
		switch s.computeOne(ctx, in, pid, loaded) {
		case productOutcomeSuccess:
			out.Success++
		case productOutcomeBlocked:
			out.Blocked++
		case productOutcomeFailed:
			out.Failed++
		}
	}

	if in.ChunkID != 0 {
		status := finalChunkStatus(out)
		if err := s.chunkRepo.UpdateResult(ctx, in.ChunkID, status, out.Success, out.Failed, 0, ""); err != nil {
			return nil, fmt.Errorf("finalize chunk: %w", err)
		}
	}
	return out, nil
}

// finalChunkStatus picks the terminal status from the per-product tallies.
func finalChunkStatus(out *ProcessChunkOutput) costcalcdom.ChunkStatus {
	switch {
	case out.Failed == 0 && out.Blocked == 0:
		return costcalcdom.ChunkStatusSuccess
	case out.Success == 0:
		return costcalcdom.ChunkStatusFailed
	default:
		return costcalcdom.ChunkStatusPartialFailed
	}
}

// loadedBundle holds the pre-fetched dependencies for a chunk.
type loadedBundle struct {
	routes        map[int64]*costroute.Graph
	capp          map[int64]map[string]float64
	formulas      map[int64][]Formula
	rmCosts       map[string]float64
	upstreamCosts map[int64]float64
}

func (s *Service) bulkLoad(ctx context.Context, in ProcessChunkInput) (*loadedBundle, error) {
	routes, err := s.loader.LoadRoutesByProducts(ctx, in.Products)
	if err != nil {
		return nil, fmt.Errorf("load routes: %w", err)
	}
	capp, err := s.loader.LoadCAPP(ctx, in.Products)
	if err != nil {
		return nil, fmt.Errorf("load CAPP: %w", err)
	}
	formulas, err := s.loader.LoadFormulas(ctx, in.Products)
	if err != nil {
		return nil, fmt.Errorf("load formulas: %w", err)
	}

	itemCodes := collectRMCodes(routes)
	rmCosts, err := s.loader.LoadRMCosts(ctx, itemCodes, in.Period)
	if err != nil {
		return nil, fmt.Errorf("load RM costs: %w", err)
	}

	upstreamIDs := collectUpstreamProducts(routes)
	upstreamCosts, err := s.loader.LoadUpstreamCosts(ctx, upstreamIDs, in.Period, string(in.CalcType))
	if err != nil {
		return nil, fmt.Errorf("load upstream costs: %w", err)
	}

	return &loadedBundle{
		routes:        routes,
		capp:          capp,
		formulas:      formulas,
		rmCosts:       rmCosts,
		upstreamCosts: upstreamCosts,
	}, nil
}

type productOutcome int

const (
	productOutcomeSuccess productOutcome = iota
	productOutcomeBlocked
	productOutcomeFailed
)

// computeOne runs the full per-product pipeline: gate on route presence,
// compute, persist, mark job_product.
func (s *Service) computeOne(ctx context.Context, in ProcessChunkInput, pid int64, loaded *loadedBundle) productOutcome {
	route, ok := loaded.routes[pid]
	if !ok || route == nil || route.Head == nil {
		_ = s.productRepo.MarkBlocked(ctx, in.JobID, pid, blockReasonMissingRoute, nil)
		return productOutcomeBlocked
	}

	out, err := ComputeProduct(ctx, ComputeInput{
		ProductSysID:  pid,
		Period:        in.Period,
		CalcType:      in.CalcType,
		Route:         route,
		CAPP:          loaded.capp[pid],
		Formulas:      loaded.formulas[pid],
		RMCosts:       loaded.rmCosts,
		UpstreamCosts: loaded.upstreamCosts,
		EvalCache:     s.cache,
	})
	if err != nil {
		return s.recordComputeError(ctx, in, pid, err)
	}

	if persistErr := s.persistResult(ctx, in, pid, route, out); persistErr != nil {
		_ = s.productRepo.MarkFailed(ctx, in.JobID, pid, persistErr.Error(), logBytes(persistErr))
		return productOutcomeFailed
	}
	return productOutcomeSuccess
}

// recordComputeError maps sentinel domain errors to BLOCKED with a structured
// block_reason; everything else marks FAILED.
func (s *Service) recordComputeError(ctx context.Context, in ProcessChunkInput, pid int64, err error) productOutcome {
	switch {
	case errors.Is(err, costcalcdom.ErrMissingCAPPValue):
		_ = s.productRepo.MarkBlocked(ctx, in.JobID, pid, blockReasonMissingCAPP, logBytes(err))
		return productOutcomeBlocked
	case errors.Is(err, costcalcdom.ErrMissingRMCost):
		_ = s.productRepo.MarkBlocked(ctx, in.JobID, pid, blockReasonMissingRMCost, logBytes(err))
		return productOutcomeBlocked
	case errors.Is(err, costcalcdom.ErrMissingUpstreamCost):
		_ = s.productRepo.MarkBlocked(ctx, in.JobID, pid, blockReasonMissingUpstream, logBytes(err))
		return productOutcomeBlocked
	case errors.Is(err, costcalcdom.ErrFormulaEval):
		_ = s.productRepo.MarkBlocked(ctx, in.JobID, pid, blockReasonFormulaError, logBytes(err))
		return productOutcomeBlocked
	default:
		_ = s.productRepo.MarkFailed(ctx, in.JobID, pid, err.Error(), logBytes(err))
		return productOutcomeFailed
	}
}

// persistResult upserts the cost result, writes audit-history on supersede,
// and marks the job_product row SUCCESS with a compact calc-log blob.
func (s *Service) persistResult(ctx context.Context, in ProcessChunkInput, pid int64, route *costroute.Graph, out *ComputeOutput) error {
	r := costcalcdom.NewResult(
		pid, in.Period, in.CalcType, route.Head.HeadID, 1,
		out.CostPerUnit, out.TotalRMCost, out.TotalConversion, out.TotalCost,
		0, "IDR",
		jsonOrNil(out.CostByLevel), jsonOrNil(out.RMCostDetail),
		jsonOrNil(out.ParamSnapshot), jsonOrNil(out.FormulaTrace),
		out.InputHash,
		in.JobID, in.Actor,
	)

	newID, prevVer, prevTotal, prevID, err := s.resultRepo.UpsertWithSupersede(ctx, r)
	if err != nil {
		return fmt.Errorf("upsert result: %w", err)
	}

	if prevID != 0 {
		s.writeRecomputeAudit(ctx, in, pid, newID, prevVer, prevTotal, prevID, out.CostPerUnit)
	}

	if err := s.productRepo.MarkSuccess(ctx, in.JobID, pid, newID, 0, buildCalculationLog(out)); err != nil {
		return fmt.Errorf("mark product success: %w", err)
	}
	return nil
}

// writeRecomputeAudit emits the aud_cost_history row for a recompute. Errors
// are swallowed: the cost row already supersedes the previous successfully and
// blocking on audit failure would be worse than losing a history row.
func (s *Service) writeRecomputeAudit(ctx context.Context, in ProcessChunkInput, pid, newID int64, prevVer int, prevTotal float64, prevID int64, newTotal float64) {
	_ = prevVer
	variance := 0.0
	if prevTotal != 0 {
		variance = ((newTotal - prevTotal) / prevTotal) * 100.0
	}
	_ = s.auditRepo.Write(ctx, &costcalcdom.AuditHistoryEntry{
		ProductSysID: pid,
		Period:       in.Period,
		CalcType:     in.CalcType,
		OldCostID:    prevID,
		NewCostID:    newID,
		OldTotal:     prevTotal,
		NewTotal:     newTotal,
		VariancePct:  variance,
		NewJobID:     in.JobID,
		ChangeReason: "CALC_RECALC",
		ChangedBy:    in.Actor,
	})
}

// buildCalculationLog serializes a compact execution trace into JSON for
// cjp_calculation_log. Used by the "drill into product" UI view.
func buildCalculationLog(out *ComputeOutput) []byte {
	doc := map[string]any{
		"cost_per_unit":    out.CostPerUnit,
		"total_rm_cost":    out.TotalRMCost,
		"total_conversion": out.TotalConversion,
		"rm_details":       out.RMCostDetail,
		"formula_trace":    out.FormulaTrace,
		"cost_by_level":    out.CostByLevel,
		"input_hash":       out.InputHash,
	}
	b, err := json.Marshal(doc)
	if err != nil {
		// Never expected — fall back to a minimal envelope so the row still has
		// SOMETHING auditable. We deliberately don't return the error: the
		// product compute already succeeded.
		return []byte(`{"calc_log_error":true}`)
	}
	return b
}

// logBytes wraps an error in a JSON envelope for the calculation_log column.
func logBytes(err error) []byte {
	b, marshalErr := json.Marshal(map[string]string{"error": err.Error()})
	if marshalErr != nil {
		return []byte(`{"error":"unmarshalable"}`)
	}
	return b
}

// jsonOrNil marshals to JSON, returning nil on error so the column stays NULL
// (rather than poisoning the row with invalid JSON).
func jsonOrNil(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	return b
}

// collectRMCodes returns the deduped list of rm_code strings referenced by any
// ITEM or GROUP RM across all routes. Used as input to LoadRMCosts (which
// queries cst_rm_cost.rm_code IN (...)).
func collectRMCodes(routes map[int64]*costroute.Graph) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, g := range routes {
		if g == nil {
			continue
		}
		for _, seq := range g.Seqs {
			if seq == nil {
				continue
			}
			for _, rm := range seq.Rms {
				if rm == nil {
					continue
				}
				code := rmReferenceCode(rm)
				if code == "" {
					continue
				}
				if _, ok := seen[code]; !ok {
					seen[code] = struct{}{}
					out = append(out, code)
				}
			}
		}
	}
	return out
}

func rmReferenceCode(rm *costroute.Rm) string {
	switch rm.RmType {
	case costroute.RmTypeItem:
		return rm.RmItemCode
	case costroute.RmTypeGroup:
		return rm.RmGroupCode
	default:
		return ""
	}
}

// collectUpstreamProducts returns the deduped list of upstream product sys
// IDs referenced as PRODUCT-type RMs.
func collectUpstreamProducts(routes map[int64]*costroute.Graph) []int64 {
	seen := map[int64]struct{}{}
	out := []int64{}
	for _, g := range routes {
		if g == nil {
			continue
		}
		for _, seq := range g.Seqs {
			if seq == nil {
				continue
			}
			for _, rm := range seq.Rms {
				if rm == nil || rm.RmType != costroute.RmTypeProduct {
					continue
				}
				if _, ok := seen[rm.RmProductSysID]; !ok {
					seen[rm.RmProductSysID] = struct{}{}
					out = append(out, rm.RmProductSysID)
				}
			}
		}
	}
	return out
}
