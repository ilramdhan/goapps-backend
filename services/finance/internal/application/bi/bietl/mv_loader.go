// Package bietl provides the Oracle MV → bi_fact_metric ETL loader.
//
// MVLoader fetches from MGTDAT.MV_DASH_{MIS,DELMAR,SALES}_MGT Oracle materialized
// views and upserts the results into the local bi_fact_metric table in batches.
// Each fact row is keyed by its DimensionKey so re-runs are fully idempotent.
package bietl

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
	oracleinfra "github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

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

// LoadMIS fetches MGTDAT.MV_DASH_MIS_MGT (EBITDA) and upserts into bi_fact_metric.
// Returns the number of rows upserted.
func (l *MVLoader) LoadMIS(ctx context.Context) (int, error) {
	rows, err := l.oracleRepo.FetchMIS(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch MIS MV: %w", err)
	}
	return l.upsertBatched(ctx, rows)
}

// LoadDeliveryMargin fetches MGTDAT.MV_DASH_DELMAR_MGT and upserts into bi_fact_metric.
// Returns the number of rows upserted.
func (l *MVLoader) LoadDeliveryMargin(ctx context.Context) (int, error) {
	rows, err := l.oracleRepo.FetchDeliveryMargin(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch DELMAR MV: %w", err)
	}
	return l.upsertBatched(ctx, rows)
}

// LoadSales fetches MGTDAT.MV_DASH_SALES_MGT and upserts into bi_fact_metric.
// Returns the number of rows upserted.
func (l *MVLoader) LoadSales(ctx context.Context) (int, error) {
	rows, err := l.oracleRepo.FetchSales(ctx)
	if err != nil {
		return 0, fmt.Errorf("fetch SALES MV: %w", err)
	}
	return l.upsertBatched(ctx, rows)
}

// upsertBatched converts BIMVRows to FactMetric values and upserts them in
// chunks of batchSize to bound per-transaction memory usage.
func (l *MVLoader) upsertBatched(ctx context.Context, rows []oracleinfra.BIMVRow) (int, error) {
	total := 0
	for i := 0; i < len(rows); i += l.batchSize {
		end := min(i+l.batchSize, len(rows))
		batch := make([]factmetric.FactMetric, 0, end-i)
		for _, r := range rows[i:end] {
			batch = append(batch, r.ToFactMetric())
		}
		if err := l.factRepo.Upsert(ctx, batch); err != nil {
			return total, fmt.Errorf("upsert batch at offset %d: %w", i, err)
		}
		total += len(batch)
	}
	return total, nil
}
