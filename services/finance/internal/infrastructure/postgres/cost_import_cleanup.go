package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
)

var _ costimportetl.StagingMaintainer = (*CostImportStagingRepository)(nil)

// stagingCleanupStatements deletes every staged row for a single job. It covers
// the six stg_import_* data tables plus stg_import_error; each statement is a
// static DELETE scoped to job_id ($1) so no table name is ever interpolated.
var stagingCleanupStatements = []string{
	`DELETE FROM stg_import_product_master WHERE job_id = $1`,
	`DELETE FROM stg_import_product_parameter WHERE job_id = $1`,
	`DELETE FROM stg_import_applicable_param WHERE job_id = $1`,
	`DELETE FROM stg_import_route_head WHERE job_id = $1`,
	`DELETE FROM stg_import_route_seq WHERE job_id = $1`,
	`DELETE FROM stg_import_route_rm WHERE job_id = $1`,
	`DELETE FROM stg_import_error WHERE job_id = $1`,
}

// CollectErrors returns every row-level error captured in stg_import_error for
// jobID, ordered by sheet then row number so the caller can group them into the
// per-sheet results consumed by the costbulkimport error-report generator. NULL
// text columns are normalized to the empty string and a NULL row number to zero.
func (r *CostImportStagingRepository) CollectErrors(ctx context.Context, jobID int64) ([]costimportetl.StagingError, error) {
	const query = `
SELECT sheet, row_num, key_info, error_message
FROM stg_import_error
WHERE job_id = $1
ORDER BY sheet, row_num`

	rows, err := r.db.QueryContext(ctx, query, jobID)
	if err != nil {
		return nil, fmt.Errorf("query staging errors: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var out []costimportetl.StagingError
	for rows.Next() {
		var (
			sheet   sql.NullString
			rowNum  sql.NullInt32
			keyInfo sql.NullString
			message sql.NullString
		)
		if scanErr := rows.Scan(&sheet, &rowNum, &keyInfo, &message); scanErr != nil {
			return nil, fmt.Errorf("scan staging error: %w", scanErr)
		}
		out = append(out, costimportetl.StagingError{
			Sheet:     sheet.String,
			RowNumber: rowNum.Int32,
			KeyInfo:   keyInfo.String,
			Message:   message.String,
		})
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate staging errors: %w", rowsErr)
	}
	return out, nil
}

// CleanupStaging deletes every staged row belonging to jobID across all
// stg_import_* tables (data and error) inside one transaction, so a finished job
// leaves no rows behind in the UNLOGGED staging area.
func (r *CostImportStagingRepository) CleanupStaging(ctx context.Context, jobID int64) error {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin staging cleanup tx: %w", err)
	}
	defer rollbackOnErr(tx)

	for _, stmt := range stagingCleanupStatements {
		if _, execErr := tx.ExecContext(ctx, stmt, jobID); execErr != nil {
			return fmt.Errorf("cleanup staging: %w", execErr)
		}
	}

	if commitErr := tx.Commit(); commitErr != nil {
		return fmt.Errorf("commit staging cleanup: %w", commitErr)
	}
	return nil
}
