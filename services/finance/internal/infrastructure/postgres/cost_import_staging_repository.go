package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
)

// CostImportStagingRepository streams parsed bulk-import rows into the UNLOGGED
// stg_import_* tables using pgx COPY. Rows are pulled lazily from the supplied
// producer so peak memory stays bounded regardless of the total row count.
type CostImportStagingRepository struct{ db *DB }

// NewCostImportStagingRepository constructs the staging repository. It is wired
// from the *DB connection used everywhere else in this service; the underlying
// pgx connection (needed for COPY) is extracted per call via database/sql's Raw
// conn accessor.
func NewCostImportStagingRepository(db *DB) *CostImportStagingRepository {
	return &CostImportStagingRepository{db: db}
}

var _ costimportetl.StagingRepository = (*CostImportStagingRepository)(nil)

// stagingCopyBuffer bounds the channel that bridges the push-based row producer
// to pgx's pull-based CopyFromFunc; it caps in-flight rows without accumulating
// the whole input.
const stagingCopyBuffer = 1024

// Staging column lists. job_id + row_num are prepended to every staged row, so
// these lists begin with those two columns and continue in migration 000428
// declaration order.
var (
	stgProductMasterColumns = []string{
		"job_id", "row_num",
		"legacy_oracle_sys_id", "product_type_code", "product_name", "shade_code",
		"shade_name", "grade_code", "description", "erp_item_code",
		"legacy_erp_compound_key", "legacy_type_label", "is_active",
	}
	stgProductParameterColumns = []string{
		"job_id", "row_num",
		"legacy_oracle_sys_id", "param_code", "data_type", "value_numeric",
		"value_text", "value_flag",
	}
	stgApplicableParamColumns = []string{
		"job_id", "row_num",
		"legacy_oracle_sys_id", "param_code", "is_required", "display_order",
	}
	stgRouteHeadColumns = []string{
		"job_id", "row_num",
		"legacy_oracle_sys_id", "routing_status", "notes",
	}
	stgRouteSeqColumns = []string{
		"job_id", "row_num",
		"route_head_legacy_product_id", "node_product_legacy_id", "route_level",
		"route_seq", "route_name", "route_item_code", "route_shade_code",
		"route_shade_name",
	}
	stgRouteRMColumns = []string{
		"job_id", "row_num",
		"route_head_legacy_product_id", "route_level", "route_seq", "rm_type",
		"ratio", "rm_product_legacy_id", "rm_item_code", "rm_group_code",
		"rm_name", "rm_shade_code", "rm_shade_name", "sub_type", "notes",
	}
)

// CopyStagingProductMaster streams rows into stg_import_product_master.
func (r *CostImportStagingRepository) CopyStagingProductMaster(ctx context.Context, jobID int64, produce costimportetl.RowProducer) (int64, error) {
	return r.copyStaging(ctx, jobID, "stg_import_product_master", stgProductMasterColumns, produce)
}

// CopyStagingProductParameter streams rows into stg_import_product_parameter.
func (r *CostImportStagingRepository) CopyStagingProductParameter(ctx context.Context, jobID int64, produce costimportetl.RowProducer) (int64, error) {
	return r.copyStaging(ctx, jobID, "stg_import_product_parameter", stgProductParameterColumns, produce)
}

// CopyStagingApplicableParam streams rows into stg_import_applicable_param.
func (r *CostImportStagingRepository) CopyStagingApplicableParam(ctx context.Context, jobID int64, produce costimportetl.RowProducer) (int64, error) {
	return r.copyStaging(ctx, jobID, "stg_import_applicable_param", stgApplicableParamColumns, produce)
}

// CopyStagingRouteHead streams rows into stg_import_route_head.
func (r *CostImportStagingRepository) CopyStagingRouteHead(ctx context.Context, jobID int64, produce costimportetl.RowProducer) (int64, error) {
	return r.copyStaging(ctx, jobID, "stg_import_route_head", stgRouteHeadColumns, produce)
}

// CopyStagingRouteSeq streams rows into stg_import_route_seq.
func (r *CostImportStagingRepository) CopyStagingRouteSeq(ctx context.Context, jobID int64, produce costimportetl.RowProducer) (int64, error) {
	return r.copyStaging(ctx, jobID, "stg_import_route_seq", stgRouteSeqColumns, produce)
}

// CopyStagingRouteRM streams rows into stg_import_route_rm.
func (r *CostImportStagingRepository) CopyStagingRouteRM(ctx context.Context, jobID int64, produce costimportetl.RowProducer) (int64, error) {
	return r.copyStaging(ctx, jobID, "stg_import_route_rm", stgRouteRMColumns, produce)
}

// stagedRow carries one assembled COPY tuple (job_id + row_num + data columns)
// or a terminal producer error across the bridge channel.
type stagedRow struct {
	values []any
	err    error
}

// copyStaging runs produce in a goroutine, bridging its pushed rows into pgx's
// pull-based CopyFromFunc over a bounded channel, and COPYs them into table.
// dataCols is len(columns)-2 (job_id + row_num are prepended here). It returns
// the number of rows copied.
func (r *CostImportStagingRepository) copyStaging(ctx context.Context, jobID int64, table string, columns []string, produce costimportetl.RowProducer) (int64, error) {
	dataCols := len(columns) - 2
	rowCh := make(chan stagedRow, stagingCopyBuffer)
	produceCtx, cancelProduce := context.WithCancel(ctx)
	defer cancelProduce()

	go runRowProducer(produceCtx, jobID, dataCols, rowCh, produce)

	source := pgx.CopyFromFunc(newRowPuller(rowCh))

	copied, err := r.copyFrom(ctx, table, columns, source)
	if err != nil {
		// Stop the producer so its goroutine does not leak when COPY aborts early.
		cancelProduce()
		return copied, err
	}
	return copied, nil
}

// runRowProducer invokes produce, prepending job_id + row_num and copying each
// pushed row (the producer may reuse its slice) before sending it downstream.
// It always sends a terminal stagedRow (nil values) carrying the final error,
// then closes the channel.
func runRowProducer(ctx context.Context, jobID int64, dataCols int, rowCh chan<- stagedRow, produce costimportetl.RowProducer) {
	defer close(rowCh)

	var rowNum int
	emit := func(row []string) error {
		rowNum++
		values := make([]any, 0, dataCols+2)
		values = append(values, jobID, rowNum)
		for i := range dataCols {
			if i < len(row) {
				values = append(values, row[i])
			} else {
				values = append(values, nil)
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case rowCh <- stagedRow{values: values}:
			return nil
		}
	}

	err := produce(emit)
	// Surface the terminal error to the puller; a nil error signals clean EOF.
	select {
	case <-ctx.Done():
	case rowCh <- stagedRow{err: err}:
	}
}

// newRowPuller adapts the bridge channel to pgx CopyFromFunc semantics: it
// returns (nil, nil) to signal end-of-rows, or a producer error.
func newRowPuller(rowCh <-chan stagedRow) func() ([]any, error) {
	return func() ([]any, error) {
		row, ok := <-rowCh
		if !ok {
			return nil, nil
		}
		if row.err != nil {
			return nil, fmt.Errorf("produce staging rows: %w", row.err)
		}
		if row.values == nil {
			// Terminal marker with a nil error: clean end of stream.
			return nil, nil
		}
		return row.values, nil
	}
}

// copyFrom extracts the underlying pgx connection from the database/sql pool and
// runs a COPY into table. database/sql is used everywhere in this service, so
// the raw pgx connection is reached via (*sql.Conn).Raw and the stdlib driver
// conn accessor.
func (r *CostImportStagingRepository) copyFrom(ctx context.Context, table string, columns []string, source pgx.CopyFromSource) (copied int64, err error) {
	sqlConn, err := r.db.Conn(ctx)
	if err != nil {
		return 0, fmt.Errorf("acquire conn for COPY into %s: %w", table, err)
	}
	defer func() {
		if cerr := sqlConn.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close conn after COPY into %s: %w", table, cerr)
		}
	}()

	rawErr := sqlConn.Raw(func(driverConn any) error {
		pgxConn, ok := driverConn.(*stdlib.Conn)
		if !ok {
			return errors.New("driver connection is not a pgx stdlib conn")
		}
		n, copyErr := pgxConn.Conn().CopyFrom(ctx, pgx.Identifier{table}, columns, source)
		if copyErr != nil {
			return fmt.Errorf("copy into %s: %w", table, copyErr)
		}
		copied = n
		return nil
	})
	if rawErr != nil {
		return copied, rawErr
	}
	return copied, err
}
