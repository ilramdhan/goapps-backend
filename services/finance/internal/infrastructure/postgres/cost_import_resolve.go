package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"math"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costimportetl"
)

var _ costimportetl.Resolver = (*CostImportStagingRepository)(nil)

// Resolve-layer SQL implements the set-based resolution described in design §5.
// Every staged dataset is already loaded into the UNLOGGED stg_import_* tables;
// each layer resolves all cross-references at once via JOIN to the real tables,
// so there is no notion of a chunk and no cross-chunk circular dependency.
//
// Each layer runs in its OWN transaction (commit per layer) and follows a
// two-step pattern:
//  1. capture rows whose FK references are missing or whose required cast fails
//     into stg_import_error (LEFT JOIN ... WHERE ref IS NULL);
//  2. write the valid rows into the target table (INNER JOIN ... ON CONFLICT
//     DO UPDATE), returning the number of rows written as the success count.
//
// The SQL mirrors the column lists and ON CONFLICT targets of the existing
// Bulk* repository methods (BulkUpsertByLegacyID / BulkUpsertValues /
// BulkUpsertApplicable / BulkUpsertHeads / BulkUpsertSeqs / BulkReplaceRMs) so
// behavior stays identical; only the row source changes (staging via JOIN
// instead of in-memory slices). Text columns are validated with a regexp guard
// before any ::numeric / ::int cast so a single dirty cell turns into one error
// row instead of aborting the whole layer.

// errSheetProductMaster and the other sheet labels name the staging source in
// stg_import_error rows so the error report can group by sheet.
const (
	errSheetProductMaster   = "product_master"
	errSheetProductParam    = "product_parameter"
	errSheetApplicableParam = "applicable_param"
	errSheetRouteHead       = "route_head"
	errSheetRouteSeq        = "route_seq"
	errSheetRouteRM         = "route_rm"
)

// rollbackOnErr rolls back tx unless it has already been committed, swallowing
// the benign sql.ErrTxDone so the deferred call never masks the real error.
func rollbackOnErr(tx *sql.Tx) {
	if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
		_ = rbErr
	}
}

// clampRowsAffected converts a sql.Result row count into the int return type,
// clamped to a non-negative value that fits a 32-bit int.
func clampRowsAffected(n int64) int {
	if n < 0 {
		return 0
	}
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	return int(n) //nolint:gosec // clamped to [0, MaxInt32] above
}

// resolveLayer runs the two-step resolve (capture errors, then upsert valid
// rows) for one layer inside a single committed transaction. errSQL inserts the
// invalid rows into stg_import_error; upsertSQL writes the valid rows and its
// RowsAffected is returned as the success count. Both statements are scoped to
// jobID ($1) with actor ($2).
func (r *CostImportStagingRepository) resolveLayer(ctx context.Context, jobID int64, actor, layer, errSQL, upsertSQL string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin %s resolve tx: %w", layer, err)
	}
	defer rollbackOnErr(tx)

	if _, err := tx.ExecContext(ctx, errSQL, jobID); err != nil {
		return 0, fmt.Errorf("%s capture errors: %w", layer, err)
	}

	res, err := tx.ExecContext(ctx, upsertSQL, jobID, actor)
	if err != nil {
		return 0, fmt.Errorf("%s upsert valid rows: %w", layer, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows affected: %w", layer, err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit %s resolve: %w", layer, err)
	}
	return clampRowsAffected(affected), nil
}

// ResolveLayer1Products resolves stg_import_product_master into
// cost_product_master, upserting by cpm_product_code (the existing code is
// reused via cpm_flex_02 / cpm_product_code lookups, else a fresh code is
// generated). Rows whose product_type_code is unknown are captured into
// stg_import_error. Mirrors BulkUpsertByLegacyID.
func (r *CostImportStagingRepository) ResolveLayer1Products(ctx context.Context, jobID int64, actor string) (int, error) {
	const errSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT s.job_id, '` + errSheetProductMaster + `', s.row_num, s.legacy_oracle_sys_id,
       'product_type_code tidak dikenal: ' || COALESCE(s.product_type_code, '')
FROM stg_import_product_master s
LEFT JOIN cost_product_type pt
       ON pt.cpt_type_code = s.product_type_code AND pt.cpt_is_active = TRUE
WHERE s.job_id = $1 AND pt.cpt_type_id IS NULL`

	const upsertSQL = `
INSERT INTO cost_product_master (
    cpm_product_type_id, cpm_product_name,
    cpm_shade_code, cpm_grade_code, cpm_description,
    cpm_shade_name, cpm_flex_01, cpm_flex_02, cpm_flex_03,
    cpm_erp_item_code, cpm_is_active,
    cpm_created_at, cpm_created_by, cpm_updated_at, cpm_updated_by,
    cpm_product_code
)
SELECT
    pt.cpt_type_id,
    s.product_name,
    NULLIF(s.shade_code, ''),
    COALESCE(NULLIF(s.grade_code, ''), 'AX'),
    NULLIF(s.description, ''),
    NULLIF(s.shade_name, ''),
    NULLIF(s.legacy_erp_compound_key, ''),
    NULLIF(s.legacy_oracle_sys_id, ''),
    NULLIF(s.legacy_type_label, ''),
    NULLIF(s.erp_item_code, ''),
    CASE
      WHEN s.is_active IS NULL OR btrim(s.is_active) = '' THEN TRUE
      WHEN lower(btrim(s.is_active)) IN ('false', 'f', '0', 'n', 'no') THEN FALSE
      ELSE TRUE
    END,
    now(), $2, now(), $2,
    COALESCE(
        (SELECT m.cpm_product_code FROM cost_product_master m
           WHERE m.cpm_flex_02 = s.legacy_oracle_sys_id AND m.cpm_flex_02 <> '' AND m.cpm_is_active = TRUE),
        (SELECT m.cpm_product_code FROM cost_product_master m
           WHERE m.cpm_product_code = s.legacy_oracle_sys_id AND m.cpm_is_active = TRUE),
        generate_cost_product_code(pt.cpt_type_id, now())
    )
FROM stg_import_product_master s
JOIN cost_product_type pt
     ON pt.cpt_type_code = s.product_type_code AND pt.cpt_is_active = TRUE
WHERE s.job_id = $1
ON CONFLICT (cpm_product_code) DO UPDATE SET
    cpm_product_type_id = EXCLUDED.cpm_product_type_id,
    cpm_product_name    = EXCLUDED.cpm_product_name,
    cpm_shade_code      = EXCLUDED.cpm_shade_code,
    cpm_grade_code      = EXCLUDED.cpm_grade_code,
    cpm_description     = EXCLUDED.cpm_description,
    cpm_shade_name      = EXCLUDED.cpm_shade_name,
    cpm_flex_01         = EXCLUDED.cpm_flex_01,
    cpm_flex_02         = EXCLUDED.cpm_flex_02,
    cpm_flex_03         = EXCLUDED.cpm_flex_03,
    cpm_erp_item_code   = EXCLUDED.cpm_erp_item_code,
    cpm_is_active       = EXCLUDED.cpm_is_active,
    cpm_updated_at      = EXCLUDED.cpm_updated_at,
    cpm_updated_by      = EXCLUDED.cpm_updated_by`

	return r.resolveLayer(ctx, jobID, actor, errSheetProductMaster, errSQL, upsertSQL)
}

// ResolveLayer2Params resolves stg_import_product_parameter into
// cost_product_parameter, joining product master (by cpm_flex_02 = legacy id)
// and mst_parameter (by param_code). Rows referencing an unknown product or
// param, that would not populate exactly one value column, or whose numeric
// value fails to cast are captured into stg_import_error. Mirrors
// BulkUpsertValues.
func (r *CostImportStagingRepository) ResolveLayer2Params(ctx context.Context, jobID int64, actor string) (int, error) {
	const errSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT s.job_id, '` + errSheetProductParam + `', s.row_num, s.legacy_oracle_sys_id,
       CASE
         WHEN cpm.cpm_product_sys_id IS NULL THEN 'produk tidak dikenal: ' || COALESCE(s.legacy_oracle_sys_id, '')
         WHEN p.id IS NULL THEN 'param_code tidak dikenal: ' || COALESCE(s.param_code, '')
         WHEN NULLIF(btrim(s.value_numeric), '') IS NOT NULL AND btrim(s.value_numeric) !~ '^-?[0-9]+(\.[0-9]+)?$'
              THEN 'value_numeric bukan angka valid: ' || COALESCE(s.value_numeric, '')
         ELSE 'harus tepat satu kolom nilai terisi (numeric/text/flag): param ' || COALESCE(s.param_code, '')
       END
FROM stg_import_product_parameter s
LEFT JOIN cost_product_master cpm
       ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
LEFT JOIN mst_parameter p
       ON p.param_code = s.param_code AND p.deleted_at IS NULL AND p.is_active = TRUE
WHERE s.job_id = $1
  AND (
        cpm.cpm_product_sys_id IS NULL
     OR p.id IS NULL
     OR (
          (CASE WHEN NULLIF(btrim(s.value_numeric), '') IS NOT NULL THEN 1 ELSE 0 END)
        + (CASE WHEN NULLIF(s.value_text, '')          IS NOT NULL THEN 1 ELSE 0 END)
        + (CASE WHEN NULLIF(btrim(s.value_flag), '')   IS NOT NULL THEN 1 ELSE 0 END)
        ) <> 1
     OR (NULLIF(btrim(s.value_numeric), '') IS NOT NULL AND btrim(s.value_numeric) !~ '^-?[0-9]+(\.[0-9]+)?$')
      )`

	const upsertSQL = `
INSERT INTO cost_product_parameter (
    cpp_product_sys_id, cpp_param_id,
    cpp_value_numeric, cpp_value_text, cpp_value_flag,
    cpp_filled_at, cpp_filled_by,
    cpp_created_at, cpp_created_by, cpp_updated_at, cpp_updated_by
)
SELECT
    cpm.cpm_product_sys_id, p.id,
    NULLIF(btrim(s.value_numeric), '')::numeric,
    NULLIF(s.value_text, ''),
    CASE WHEN NULLIF(btrim(s.value_flag), '') IS NOT NULL
         THEN (lower(btrim(s.value_flag)) IN ('true', '1', 'y', 'yes'))
         ELSE NULL END,
    now(), $2,
    now(), $2, now(), $2
FROM stg_import_product_parameter s
JOIN cost_product_master cpm
      ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
JOIN mst_parameter p
      ON p.param_code = s.param_code AND p.deleted_at IS NULL AND p.is_active = TRUE
WHERE s.job_id = $1
  AND (
        (CASE WHEN NULLIF(btrim(s.value_numeric), '') IS NOT NULL THEN 1 ELSE 0 END)
      + (CASE WHEN NULLIF(s.value_text, '')          IS NOT NULL THEN 1 ELSE 0 END)
      + (CASE WHEN NULLIF(btrim(s.value_flag), '')   IS NOT NULL THEN 1 ELSE 0 END)
      ) = 1
  AND (NULLIF(btrim(s.value_numeric), '') IS NULL OR btrim(s.value_numeric) ~ '^-?[0-9]+(\.[0-9]+)?$')
ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET
    cpp_value_numeric = EXCLUDED.cpp_value_numeric,
    cpp_value_text    = EXCLUDED.cpp_value_text,
    cpp_value_flag    = EXCLUDED.cpp_value_flag,
    cpp_filled_at     = EXCLUDED.cpp_filled_at,
    cpp_filled_by     = EXCLUDED.cpp_filled_by,
    cpp_updated_at    = EXCLUDED.cpp_updated_at,
    cpp_updated_by    = EXCLUDED.cpp_updated_by`

	return r.resolveLayer(ctx, jobID, actor, errSheetProductParam, errSQL, upsertSQL)
}

// ResolveLayer3Applicable resolves stg_import_applicable_param into
// cost_product_applicable_param, joining product master and mst_parameter. Rows
// referencing an unknown product or param are captured into stg_import_error; a
// non-integer display_order is stored as NULL. Mirrors BulkUpsertApplicable.
func (r *CostImportStagingRepository) ResolveLayer3Applicable(ctx context.Context, jobID int64, actor string) (int, error) {
	const errSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT s.job_id, '` + errSheetApplicableParam + `', s.row_num, s.legacy_oracle_sys_id,
       CASE
         WHEN cpm.cpm_product_sys_id IS NULL THEN 'produk tidak dikenal: ' || COALESCE(s.legacy_oracle_sys_id, '')
         ELSE 'param_code tidak dikenal: ' || COALESCE(s.param_code, '')
       END
FROM stg_import_applicable_param s
LEFT JOIN cost_product_master cpm
       ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
LEFT JOIN mst_parameter p
       ON p.param_code = s.param_code AND p.deleted_at IS NULL AND p.is_active = TRUE
WHERE s.job_id = $1 AND (cpm.cpm_product_sys_id IS NULL OR p.id IS NULL)`

	const upsertSQL = `
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order,
    capp_created_at, capp_created_by, capp_updated_at, capp_updated_by
)
SELECT
    cpm.cpm_product_sys_id, p.id,
    (NULLIF(s.is_required, '') IS NOT NULL AND lower(btrim(s.is_required)) IN ('true', '1', 'y', 'yes')),
    CASE WHEN btrim(s.display_order) ~ '^[0-9]+$' THEN btrim(s.display_order)::int ELSE NULL END,
    now(), $2, now(), $2
FROM stg_import_applicable_param s
JOIN cost_product_master cpm
      ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
JOIN mst_parameter p
      ON p.param_code = s.param_code AND p.deleted_at IS NULL AND p.is_active = TRUE
WHERE s.job_id = $1
ON CONFLICT (capp_product_sys_id, capp_param_id) DO UPDATE SET
    capp_is_required   = EXCLUDED.capp_is_required,
    capp_display_order = EXCLUDED.capp_display_order,
    capp_updated_at    = EXCLUDED.capp_updated_at,
    capp_updated_by    = EXCLUDED.capp_updated_by`

	return r.resolveLayer(ctx, jobID, actor, errSheetApplicableParam, errSQL, upsertSQL)
}

// ResolveLayer4RouteHead resolves stg_import_route_head into cost_route_head,
// joining product master by legacy id. Rows referencing an unknown product or an
// invalid routing_status are captured into stg_import_error. The status is
// normalized to upper-case (default DRAFT) to satisfy chk_crh_status, and LOCKED
// heads are left untouched on conflict. Mirrors BulkUpsertHeads.
func (r *CostImportStagingRepository) ResolveLayer4RouteHead(ctx context.Context, jobID int64, actor string) (int, error) {
	const errSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT s.job_id, '` + errSheetRouteHead + `', s.row_num, s.legacy_oracle_sys_id,
       CASE
         WHEN cpm.cpm_product_sys_id IS NULL THEN 'produk tidak dikenal: ' || COALESCE(s.legacy_oracle_sys_id, '')
         ELSE 'routing_status tidak valid (DRAFT/COMPLETE/LOCKED): ' || COALESCE(s.routing_status, '')
       END
FROM stg_import_route_head s
LEFT JOIN cost_product_master cpm
       ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
WHERE s.job_id = $1
  AND (
        cpm.cpm_product_sys_id IS NULL
     OR (NULLIF(btrim(s.routing_status), '') IS NOT NULL
         AND upper(btrim(s.routing_status)) NOT IN ('DRAFT', 'COMPLETE', 'LOCKED'))
      )`

	const upsertSQL = `
INSERT INTO cost_route_head (
    crh_product_sys_id, crh_routing_status, crh_notes,
    crh_created_at, crh_created_by, crh_updated_at, crh_updated_by
)
SELECT
    cpm.cpm_product_sys_id,
    CASE WHEN NULLIF(btrim(s.routing_status), '') IS NULL THEN 'DRAFT'
         ELSE upper(btrim(s.routing_status)) END,
    NULLIF(s.notes, ''),
    now(), $2, now(), $2
FROM stg_import_route_head s
JOIN cost_product_master cpm
      ON cpm.cpm_flex_02 = s.legacy_oracle_sys_id AND cpm.cpm_flex_02 <> ''
WHERE s.job_id = $1
  AND (NULLIF(btrim(s.routing_status), '') IS NULL
       OR upper(btrim(s.routing_status)) IN ('DRAFT', 'COMPLETE', 'LOCKED'))
ON CONFLICT (crh_product_sys_id) WHERE crh_deleted_at IS NULL AND crh_routing_status <> 'LOCKED'
DO UPDATE SET
    crh_notes      = CASE WHEN cost_route_head.crh_routing_status = 'LOCKED' THEN cost_route_head.crh_notes ELSE EXCLUDED.crh_notes END,
    crh_updated_at = CASE WHEN cost_route_head.crh_routing_status = 'LOCKED' THEN cost_route_head.crh_updated_at ELSE EXCLUDED.crh_updated_at END,
    crh_updated_by = CASE WHEN cost_route_head.crh_routing_status = 'LOCKED' THEN cost_route_head.crh_updated_by ELSE EXCLUDED.crh_updated_by END`

	return r.resolveLayer(ctx, jobID, actor, errSheetRouteHead, errSQL, upsertSQL)
}

// ResolveLayer5RouteSeq resolves stg_import_route_seq into cost_route_seq,
// joining the active (non-LOCKED) route head (by its product legacy id) and the
// node product master (by node legacy id). Rows whose head or node product is
// unknown, or whose level/seq fail to cast to int, are captured into
// stg_import_error. Mirrors BulkUpsertSeqs.
func (r *CostImportStagingRepository) ResolveLayer5RouteSeq(ctx context.Context, jobID int64, actor string) (int, error) {
	const errSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT s.job_id, '` + errSheetRouteSeq + `', s.row_num, s.route_head_legacy_product_id,
       CASE
         WHEN crh.crh_head_id IS NULL THEN 'route head aktif tidak ditemukan untuk produk: ' || COALESCE(s.route_head_legacy_product_id, '')
         WHEN node.cpm_product_sys_id IS NULL THEN 'node produk tidak dikenal: ' || COALESCE(s.node_product_legacy_id, '')
         ELSE 'route_level/route_seq tidak valid: ' || COALESCE(s.route_level, '') || '/' || COALESCE(s.route_seq, '')
       END
FROM stg_import_route_seq s
LEFT JOIN cost_product_master head_cpm
       ON head_cpm.cpm_flex_02 = s.route_head_legacy_product_id AND head_cpm.cpm_flex_02 <> ''
LEFT JOIN cost_route_head crh
       ON crh.crh_product_sys_id = head_cpm.cpm_product_sys_id
      AND crh.crh_deleted_at IS NULL AND crh.crh_routing_status <> 'LOCKED'
LEFT JOIN cost_product_master node
       ON node.cpm_flex_02 = s.node_product_legacy_id AND node.cpm_flex_02 <> ''
WHERE s.job_id = $1
  AND (
        crh.crh_head_id IS NULL
     OR node.cpm_product_sys_id IS NULL
     OR btrim(s.route_level) !~ '^[0-9]+$'
     OR btrim(s.route_seq)   !~ '^[0-9]+$'
      )`

	const upsertSQL = `
INSERT INTO cost_route_seq (
    crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
    crs_route_name, crs_route_item_code, crs_route_shade_code, crs_route_shade_name,
    crs_created_at, crs_created_by, crs_updated_at, crs_updated_by
)
SELECT
    crh.crh_head_id, node.cpm_product_sys_id, btrim(s.route_level)::int, btrim(s.route_seq)::int,
    NULLIF(s.route_name, ''), NULLIF(s.route_item_code, ''),
    NULLIF(s.route_shade_code, ''), NULLIF(s.route_shade_name, ''),
    now(), $2, now(), $2
FROM stg_import_route_seq s
JOIN cost_product_master head_cpm
      ON head_cpm.cpm_flex_02 = s.route_head_legacy_product_id AND head_cpm.cpm_flex_02 <> ''
JOIN cost_route_head crh
      ON crh.crh_product_sys_id = head_cpm.cpm_product_sys_id
     AND crh.crh_deleted_at IS NULL AND crh.crh_routing_status <> 'LOCKED'
JOIN cost_product_master node
      ON node.cpm_flex_02 = s.node_product_legacy_id AND node.cpm_flex_02 <> ''
WHERE s.job_id = $1
  AND btrim(s.route_level) ~ '^[0-9]+$'
  AND btrim(s.route_seq)   ~ '^[0-9]+$'
ON CONFLICT (crs_head_id, crs_route_level, crs_route_seq) DO UPDATE SET
    crs_product_sys_id   = EXCLUDED.crs_product_sys_id,
    crs_route_name       = EXCLUDED.crs_route_name,
    crs_route_item_code  = EXCLUDED.crs_route_item_code,
    crs_route_shade_code = EXCLUDED.crs_route_shade_code,
    crs_route_shade_name = EXCLUDED.crs_route_shade_name,
    crs_updated_at       = EXCLUDED.crs_updated_at,
    crs_updated_by       = EXCLUDED.crs_updated_by`

	return r.resolveLayer(ctx, jobID, actor, errSheetRouteSeq, errSQL, upsertSQL)
}

// ResolveLayer6RouteRM resolves stg_import_route_rm into cost_route_rm with a
// full replace: it deletes every existing RM belonging to the affected sequence
// set (the seqs touched by this job) and then re-inserts the staged RMs. Rows
// whose head/seq cannot be resolved, whose RM type/reference is inconsistent, or
// whose ratio fails to cast are captured into stg_import_error. Mirrors
// BulkReplaceRMs (delete-then-insert per seq).
func (r *CostImportStagingRepository) ResolveLayer6RouteRM(ctx context.Context, jobID int64, actor string) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("begin %s resolve tx: %w", errSheetRouteRM, err)
	}
	defer rollbackOnErr(tx)

	if _, err := tx.ExecContext(ctx, routeRMErrSQL, jobID); err != nil {
		return 0, fmt.Errorf("%s capture errors: %w", errSheetRouteRM, err)
	}

	if _, err := tx.ExecContext(ctx, routeRMDeleteSQL, jobID); err != nil {
		return 0, fmt.Errorf("%s delete existing rms: %w", errSheetRouteRM, err)
	}

	res, err := tx.ExecContext(ctx, routeRMInsertSQL, jobID, actor)
	if err != nil {
		return 0, fmt.Errorf("%s insert valid rows: %w", errSheetRouteRM, err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("%s rows affected: %w", errSheetRouteRM, err)
	}

	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("commit %s resolve: %w", errSheetRouteRM, err)
	}
	return clampRowsAffected(affected), nil
}

// routeRMErrSQL captures route-RM rows whose head/seq cannot be resolved, whose
// ratio is non-positive or non-numeric, or whose rm_type does not match exactly
// one populated reference column (PRODUCT->product, ITEM->item, GROUP->group).
const routeRMErrSQL = `
INSERT INTO stg_import_error (job_id, sheet, row_num, key_info, error_message)
SELECT s.job_id, '` + errSheetRouteRM + `', s.row_num, s.route_head_legacy_product_id,
       CASE
         WHEN crs.crs_seq_id IS NULL THEN 'route seq tidak ditemukan untuk produk ' || COALESCE(s.route_head_legacy_product_id, '') || ' level/seq ' || COALESCE(s.route_level, '') || '/' || COALESCE(s.route_seq, '')
         WHEN (CASE WHEN btrim(s.ratio) ~ '^[0-9]+(\.[0-9]+)?$' THEN btrim(s.ratio)::numeric ELSE 0 END) <= 0 THEN 'ratio tidak valid: ' || COALESCE(s.ratio, '')
         WHEN upper(btrim(s.rm_type)) = 'PRODUCT' AND rm.cpm_product_sys_id IS NULL THEN 'RM produk tidak dikenal: ' || COALESCE(s.rm_product_legacy_id, '')
         ELSE 'rm_type/referensi tidak konsisten: ' || COALESCE(s.rm_type, '')
       END
FROM stg_import_route_rm s
LEFT JOIN cost_product_master head_cpm
       ON head_cpm.cpm_flex_02 = s.route_head_legacy_product_id AND head_cpm.cpm_flex_02 <> ''
LEFT JOIN cost_route_head crh
       ON crh.crh_product_sys_id = head_cpm.cpm_product_sys_id
      AND crh.crh_deleted_at IS NULL AND crh.crh_routing_status <> 'LOCKED'
LEFT JOIN cost_route_seq crs
       ON crs.crs_head_id = crh.crh_head_id
      AND crs.crs_route_level = (CASE WHEN btrim(s.route_level) ~ '^[0-9]+$' THEN btrim(s.route_level)::int ELSE -1 END)
      AND crs.crs_route_seq   = (CASE WHEN btrim(s.route_seq)   ~ '^[0-9]+$' THEN btrim(s.route_seq)::int   ELSE -1 END)
      AND crs.crs_deleted_at IS NULL
LEFT JOIN cost_product_master rm
       ON rm.cpm_flex_02 = s.rm_product_legacy_id AND rm.cpm_flex_02 <> ''
WHERE s.job_id = $1
  AND (
        crs.crs_seq_id IS NULL
     OR (CASE WHEN btrim(s.ratio) ~ '^[0-9]+(\.[0-9]+)?$' THEN btrim(s.ratio)::numeric ELSE 0 END) <= 0
     OR upper(btrim(s.rm_type)) NOT IN ('PRODUCT', 'ITEM', 'GROUP')
     OR (upper(btrim(s.rm_type)) = 'PRODUCT' AND rm.cpm_product_sys_id IS NULL)
     OR (upper(btrim(s.rm_type)) = 'ITEM'    AND NULLIF(s.rm_item_code, '')  IS NULL)
     OR (upper(btrim(s.rm_type)) = 'GROUP'   AND NULLIF(s.rm_group_code, '') IS NULL)
      )`

// routeRMDeleteSQL removes every existing RM that belongs to a sequence touched
// by this job, so the subsequent insert performs a full replace per seq.
const routeRMDeleteSQL = `
DELETE FROM cost_route_rm
WHERE crm_seq_id IN (
    SELECT DISTINCT crs.crs_seq_id
    FROM stg_import_route_rm s
    JOIN cost_product_master head_cpm
          ON head_cpm.cpm_flex_02 = s.route_head_legacy_product_id AND head_cpm.cpm_flex_02 <> ''
    JOIN cost_route_head crh
          ON crh.crh_product_sys_id = head_cpm.cpm_product_sys_id
         AND crh.crh_deleted_at IS NULL AND crh.crh_routing_status <> 'LOCKED'
    JOIN cost_route_seq crs
          ON crs.crs_head_id = crh.crh_head_id
         AND crs.crs_route_level = (CASE WHEN btrim(s.route_level) ~ '^[0-9]+$' THEN btrim(s.route_level)::int ELSE -1 END)
         AND crs.crs_route_seq   = (CASE WHEN btrim(s.route_seq)   ~ '^[0-9]+$' THEN btrim(s.route_seq)::int   ELSE -1 END)
         AND crs.crs_deleted_at IS NULL
    WHERE s.job_id = $1
)`

// routeRMInsertSQL inserts the valid staged RMs, resolving the owning seq, the
// seq's product (as parent), and (for PRODUCT type) the RM product. The single
// populated reference column is selected per rm_type to satisfy chk_crm_one_ref
// and chk_crm_type_ref_match.
const routeRMInsertSQL = `
INSERT INTO cost_route_rm (
    crm_seq_id, crm_parent_product_sys_id, crm_rm_type,
    crm_rm_product_sys_id, crm_rm_item_code, crm_rm_group_code,
    crm_route_rm_ratio, crm_route_rm_name,
    crm_route_rm_shade_code, crm_route_rm_shade_name,
    crm_sub_type, crm_notes,
    crm_created_at, crm_created_by, crm_updated_at, crm_updated_by
)
SELECT
    crs.crs_seq_id, crs.crs_product_sys_id, upper(btrim(s.rm_type)),
    CASE WHEN upper(btrim(s.rm_type)) = 'PRODUCT' THEN rm.cpm_product_sys_id ELSE NULL END,
    CASE WHEN upper(btrim(s.rm_type)) = 'ITEM'    THEN NULLIF(s.rm_item_code, '')  ELSE NULL END,
    CASE WHEN upper(btrim(s.rm_type)) = 'GROUP'   THEN NULLIF(s.rm_group_code, '') ELSE NULL END,
    NULLIF(btrim(s.ratio), '')::numeric, NULLIF(s.rm_name, ''),
    NULLIF(s.rm_shade_code, ''), NULLIF(s.rm_shade_name, ''),
    NULLIF(s.sub_type, ''), NULLIF(s.notes, ''),
    now(), $2, now(), $2
FROM stg_import_route_rm s
JOIN cost_product_master head_cpm
      ON head_cpm.cpm_flex_02 = s.route_head_legacy_product_id AND head_cpm.cpm_flex_02 <> ''
JOIN cost_route_head crh
      ON crh.crh_product_sys_id = head_cpm.cpm_product_sys_id
     AND crh.crh_deleted_at IS NULL AND crh.crh_routing_status <> 'LOCKED'
JOIN cost_route_seq crs
      ON crs.crs_head_id = crh.crh_head_id
     AND crs.crs_route_level = (CASE WHEN btrim(s.route_level) ~ '^[0-9]+$' THEN btrim(s.route_level)::int ELSE -1 END)
     AND crs.crs_route_seq   = (CASE WHEN btrim(s.route_seq)   ~ '^[0-9]+$' THEN btrim(s.route_seq)::int   ELSE -1 END)
     AND crs.crs_deleted_at IS NULL
LEFT JOIN cost_product_master rm
      ON rm.cpm_flex_02 = s.rm_product_legacy_id AND rm.cpm_flex_02 <> ''
WHERE s.job_id = $1
  AND (CASE WHEN btrim(s.ratio) ~ '^[0-9]+(\.[0-9]+)?$' THEN btrim(s.ratio)::numeric ELSE 0 END) > 0
  AND upper(btrim(s.rm_type)) IN ('PRODUCT', 'ITEM', 'GROUP')
  AND (upper(btrim(s.rm_type)) <> 'PRODUCT' OR rm.cpm_product_sys_id IS NOT NULL)
  AND (upper(btrim(s.rm_type)) <> 'ITEM'    OR NULLIF(s.rm_item_code, '')  IS NOT NULL)
  AND (upper(btrim(s.rm_type)) <> 'GROUP'   OR NULLIF(s.rm_group_code, '') IS NOT NULL)`
