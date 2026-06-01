// Package bietl provides the Oracle MV → bi_fact_metric ETL loader.
//
// MVLoader fetches from MGTDAT.MV_DASH_{MIS,DELMAR,SALES}_MGT Oracle materialized
// views and upserts the results into the local bi_fact_metric table in batches.
// Each fact row is keyed by its DimensionKey so re-runs are fully idempotent.
package bietl

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// Ensure MVLoader satisfies the job.BIETLRunner interface at compile time.
// (job package imports bietl, so we cannot import job here — the check lives in main.go.)

const defaultBatchSize = 500

// MVLoader fetches rows from Oracle BI MVs and upserts them into bi_fact_metric.
type MVLoader struct {
	oracleRepo *oracleinfra.BIMVRepository
	factRepo   factmetric.Repository
	batchSize  int
}

// NewMVLoader constructs an MVLoader with default batch size of 500 rows.
func NewMVLoader(oracleRepo *oracleinfra.BIMVRepository, factRepo factmetric.Repository) *MVLoader {
	return &MVLoader{
		oracleRepo: oracleRepo,
		factRepo:   factRepo,
		batchSize:  defaultBatchSize,
	}
}

// Load implements job.BIETLRunner.
//
// Dispatches by targetType — the value stored in bi_fact_metric.type.
// sourceView is the fully-qualified Oracle MV name (e.g. "MGTDAT.MV_DASH_MIS_MGT").
//
// Adding a new ETL type:
//  1. Add FetchXxx() to BIMVRepository (oracle package).
//  2. Add a case here.
//  3. Create a bi_job via the admin form with the matching Target Type.
//
// No changes to handlers.go or the frontend form are needed.
func (l *MVLoader) Load(ctx context.Context, targetType, sourceView string, sourceID uuid.UUID) (int, error) {
	switch targetType {
	case "MIS":
		return l.loadMIS(ctx, sourceView, sourceID)
	case "DELIVERY MARGIN":
		return l.loadDeliveryMargin(ctx, sourceView, sourceID)
	case "SALES":
		return l.loadSales(ctx, sourceView, sourceID)
	default:
		return 0, fmt.Errorf("unsupported target_type %q: add a new FetchXxx method to BIMVRepository and a case here", targetType)
	}
}

func (l *MVLoader) loadMIS(ctx context.Context, _ string, sourceID uuid.UUID) (int, error) {
	rows, err := l.oracleRepo.FetchMIS(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch MIS MV: %w", err)
	}
	return l.upsertBatched(ctx, rows, sourceID)
}

func (l *MVLoader) loadDeliveryMargin(ctx context.Context, _ string, sourceID uuid.UUID) (int, error) {
	rows, err := l.oracleRepo.FetchDeliveryMargin(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch DELMAR MV: %w", err)
	}
	return l.upsertBatched(ctx, rows, sourceID)
}

func (l *MVLoader) loadSales(ctx context.Context, _ string, sourceID uuid.UUID) (int, error) {
	rows, err := l.oracleRepo.FetchSales(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch SALES MV: %w", err)
	}
	return l.upsertBatched(ctx, rows, sourceID)
}

// upsertBatched converts BIMVRows to FactMetric values and upserts them in
// chunks of batchSize to bound per-transaction memory usage.
func (l *MVLoader) upsertBatched(ctx context.Context, rows []oracleinfra.BIMVRow, sourceID uuid.UUID) (int, error) {
	total := 0
	for i := 0; i < len(rows); i += l.batchSize {
		end := min(i+l.batchSize, len(rows))
		batch := make([]factmetric.FactMetric, 0, end-i)
		for _, r := range rows[i:end] {
			batch = append(batch, r.ToFactMetric(sourceID))
		}
		if err := l.factRepo.Upsert(ctx, batch); err != nil {
			return total, fmt.Errorf("upsert batch at offset %d: %w", i, err)
		}
		total += len(batch)
	}
	return total, nil
}
