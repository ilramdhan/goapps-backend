package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
)

var _ costimportetl.MasterLookupValidator = (*CostImportStagingRepository)(nil)

// masterLookupCategory is the mst_parameter.param_category value that marks a
// parameter whose value must exist in a registered lookup-master table.
const masterLookupCategory = "MASTER_LOOKUP"

// unknownMasterValueMsgPrefix prefixes the stg_import_error message for a rejected
// MASTER_LOOKUP value. It MUST stay in sync with costbulkimport's report parser
// (unknownMasterValuePrefix) so the "missing_master_values" sheet picks these up.
// Format: "unknown_master_value:<masterCode>:<value>".
const unknownMasterValueMsgPrefix = "unknown_master_value:"

// MasterLookupCandidates returns the distinct (master_code, param_code, value)
// triples staged for jobID whose parameter is a MASTER_LOOKUP type with a
// configured lookup_master_code and a non-empty value. DISTINCT keeps the result
// bounded by the master option space, not the staged row count, so it stays small
// even for multi-million-row imports.
func (r *CostImportStagingRepository) MasterLookupCandidates(ctx context.Context, jobID int64) ([]costimportetl.MasterLookupCandidate, error) {
	const q = `
SELECT DISTINCT p.lookup_master_code, s.param_code, btrim(s.value_text)
FROM stg_import_product_parameter s
JOIN mst_parameter p
      ON p.param_code = s.param_code AND p.deleted_at IS NULL AND p.is_active = TRUE
WHERE s.job_id = $1
  AND p.param_category = $2
  AND COALESCE(p.lookup_master_code, '') <> ''
  AND NULLIF(btrim(s.value_text), '') IS NOT NULL`

	rows, err := r.db.QueryContext(ctx, q, jobID, masterLookupCategory)
	if err != nil {
		return nil, fmt.Errorf("query master-lookup candidates: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	var out []costimportetl.MasterLookupCandidate
	for rows.Next() {
		var c costimportetl.MasterLookupCandidate
		if scanErr := rows.Scan(&c.MasterCode, &c.ParamCode, &c.Value); scanErr != nil {
			return nil, fmt.Errorf("scan master-lookup candidate: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate master-lookup candidates: %w", err)
	}
	return out, nil
}

// RejectMasterLookupValues records one stg_import_error per rejected value and
// deletes every matching staged product-parameter row so ResolveLayer2Params will
// not import it. All inserts and deletes run in one transaction; the returned
// count is the total number of staged rows removed.
func (r *CostImportStagingRepository) RejectMasterLookupValues(ctx context.Context, jobID int64, rejected []costimportetl.MasterLookupCandidate) (int, error) {
	if len(rejected) == 0 {
		return 0, nil
	}

	const errSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
VALUES ($1, $2, 0, $3, $4)`
	const delSQL = `
DELETE FROM stg_import_product_parameter
WHERE job_id = $1 AND param_code = $2 AND btrim(value_text) = $3`

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin master-lookup reject tx: %w", err)
	}
	defer rollbackOnErr(tx)

	var removed int64
	for _, c := range rejected {
		msg := unknownMasterValueMsgPrefix + c.MasterCode + ":" + c.Value
		if _, execErr := tx.ExecContext(ctx, errSQL, jobID, errSheetProductParam, c.Value, msg); execErr != nil {
			return 0, fmt.Errorf("record master-lookup error: %w", execErr)
		}
		res, delErr := tx.ExecContext(ctx, delSQL, jobID, c.ParamCode, c.Value)
		if delErr != nil {
			return 0, fmt.Errorf("delete rejected master-lookup rows: %w", delErr)
		}
		affected, affErr := res.RowsAffected()
		if affErr != nil {
			return 0, fmt.Errorf("master-lookup rows affected: %w", affErr)
		}
		removed += affected
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit master-lookup reject: %w", err)
	}
	return clampRowsAffected(removed), nil
}
