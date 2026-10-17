package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// CostProductMasterRepository also persists per-product RM denier overrides (000578).
var _ costproductmaster.RMDenierOverrideRepository = (*CostProductMasterRepository)(nil)

const rmDenierOverrideSelect = `
	SELECT o.cpdo_id, o.cpdo_product_sys_id, o.cpdo_rm_type,
	       COALESCE(o.cpdo_rm_product_sys_id, 0),
	       COALESCE(rp.cpm_product_code, ''), COALESCE(rp.cpm_product_name, ''),
	       COALESCE(o.cpdo_rm_group_code, ''), COALESCE(g.group_name, ''),
	       o.cpdo_denier::float8, o.cpdo_reason,
	       o.cpdo_created_at, o.cpdo_created_by, o.cpdo_updated_at, o.cpdo_updated_by
	FROM cost_product_rm_denier_override o
	LEFT JOIN cost_product_master rp ON rp.cpm_product_sys_id = o.cpdo_rm_product_sys_id
	LEFT JOIN cst_rm_group_head   g  ON g.group_code = o.cpdo_rm_group_code AND g.deleted_at IS NULL`

// overrideQuerier is the read subset shared by *sql.DB and *sql.Tx.
type overrideQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

// ListRMDenierOverrides returns the product's overrides, PRODUCT lines first then
// GROUP lines, each ordered by RM code.
func (r *CostProductMasterRepository) ListRMDenierOverrides(
	ctx context.Context, productSysID int64,
) ([]*costproductmaster.RMDenierOverride, error) {
	return queryRMDenierOverrides(ctx, r.db, rmDenierOverrideSelect+`
		WHERE o.cpdo_product_sys_id = $1
		ORDER BY o.cpdo_rm_type DESC, COALESCE(rp.cpm_product_code, o.cpdo_rm_group_code), o.cpdo_id`,
		productSysID)
}

func queryRMDenierOverrides(
	ctx context.Context, q overrideQuerier, sqlText string, args ...any,
) ([]*costproductmaster.RMDenierOverride, error) {
	rows, err := q.QueryContext(ctx, sqlText, args...)
	if err != nil {
		return nil, fmt.Errorf("list RM denier overrides: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			_ = cErr
		}
	}()
	out := []*costproductmaster.RMDenierOverride{}
	for rows.Next() {
		var rec costproductmaster.RMDenierOverrideRecord
		if err := rows.Scan(
			&rec.ID, &rec.ProductSysID, &rec.RMType,
			&rec.RMProductSysID, &rec.RMProductCode, &rec.RMProductName,
			&rec.RMGroupCode, &rec.RMGroupName,
			&rec.Denier, &rec.Reason,
			&rec.CreatedAt, &rec.CreatedBy, &rec.UpdatedAt, &rec.UpdatedBy,
		); err != nil {
			return nil, fmt.Errorf("scan RM denier override: %w", err)
		}
		out = append(out, costproductmaster.ReconstructRMDenierOverride(rec))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RM denier overrides: %w", err)
	}
	return out, nil
}

// ListDirectDuplicates returns the direct copies of productSysID, ordered by code.
func (r *CostProductMasterRepository) ListDirectDuplicates(
	ctx context.Context, productSysID int64,
) ([]costproductmaster.DuplicateRef, error) {
	return listDirectDuplicates(ctx, r.db, productSysID)
}

func listDirectDuplicates(ctx context.Context, q overrideQuerier, productSysID int64) ([]costproductmaster.DuplicateRef, error) {
	rows, err := q.QueryContext(ctx, `
		SELECT cpm_product_sys_id, cpm_product_code, cpm_product_name
		FROM cost_product_master
		WHERE cpm_duplicated_from_sys_id = $1
		ORDER BY cpm_product_code, cpm_product_sys_id`, productSysID)
	if err != nil {
		return nil, fmt.Errorf("list direct duplicates of %d: %w", productSysID, err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			_ = cErr
		}
	}()
	out := []costproductmaster.DuplicateRef{}
	for rows.Next() {
		var d costproductmaster.DuplicateRef
		if err := rows.Scan(&d.ProductSysID, &d.ProductCode, &d.ProductName); err != nil {
			return nil, fmt.Errorf("scan duplicate ref: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate duplicate refs: %w", err)
	}
	return out, nil
}

// overrideTargets returns the owner plus, when applyToDuplicates, its direct copies.
// The owner's row stays first. For a PRODUCT override a copy that IS the RM product
// is skipped (a product cannot override its own denier).
func overrideTargets(
	ctx context.Context, tx *sql.Tx, owner int64, applyToDuplicates bool, rmProductSysID int64,
) (targets, copies []int64, err error) {
	targets = []int64{owner}
	copies = []int64{}
	if !applyToDuplicates {
		return targets, copies, nil
	}
	dups, err := listDirectDuplicates(ctx, tx, owner)
	if err != nil {
		return nil, nil, err
	}
	for _, d := range dups {
		if rmProductSysID != 0 && d.ProductSysID == rmProductSysID {
			continue
		}
		targets = append(targets, d.ProductSysID)
		copies = append(copies, d.ProductSysID)
	}
	return targets, copies, nil
}

// The two upserts differ only in the ON CONFLICT target (one partial unique index per
// RM type); they are kept as constants so no SQL is built at runtime.
const (
	upsertRMDenierOverrideInsert = `
		INSERT INTO cost_product_rm_denier_override (
			cpdo_product_sys_id, cpdo_rm_type, cpdo_rm_product_sys_id, cpdo_rm_group_code,
			cpdo_denier, cpdo_reason, cpdo_created_by, cpdo_updated_by
		)
		SELECT t.id, $2, NULLIF($3, 0)::bigint, NULLIF($4, ''), $5, $6, $7, $7
		FROM unnest($1::bigint[]) AS t(id)`
	upsertRMDenierOverrideSet = `
		DO UPDATE SET cpdo_denier     = EXCLUDED.cpdo_denier,
		              cpdo_reason     = EXCLUDED.cpdo_reason,
		              cpdo_updated_at = NOW(),
		              cpdo_updated_by = EXCLUDED.cpdo_updated_by`
	upsertRMDenierOverrideProductSQL = upsertRMDenierOverrideInsert + `
		ON CONFLICT (cpdo_product_sys_id, cpdo_rm_product_sys_id) WHERE cpdo_rm_type = 'PRODUCT'` +
		upsertRMDenierOverrideSet
	upsertRMDenierOverrideGroupSQL = upsertRMDenierOverrideInsert + `
		ON CONFLICT (cpdo_product_sys_id, cpdo_rm_group_code) WHERE cpdo_rm_type = 'GROUP'` +
		upsertRMDenierOverrideSet
)

// UpsertRMDenierOverride writes o for its owner and, with applyToDuplicates, for every
// direct copy, all in one transaction.
func (r *CostProductMasterRepository) UpsertRMDenierOverride(
	ctx context.Context, o *costproductmaster.RMDenierOverride, applyToDuplicates bool, actor string,
) (*costproductmaster.RMDenierOverride, []int64, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, nil, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	if err := checkOverrideRefs(ctx, tx, o.ProductSysID(), o.RMType(), o.RMProductSysID(), o.RMGroupCode()); err != nil {
		return nil, nil, err
	}
	targets, copies, err := overrideTargets(ctx, tx, o.ProductSysID(), applyToDuplicates, o.RMProductSysID())
	if err != nil {
		return nil, nil, err
	}

	upsertSQL := upsertRMDenierOverrideProductSQL
	if o.RMType() == costproductmaster.RMTypeGroup {
		upsertSQL = upsertRMDenierOverrideGroupSQL
	}
	if _, err := tx.ExecContext(ctx, upsertSQL,
		pq.Array(targets), o.RMType(), o.RMProductSysID(), o.RMGroupCode(),
		o.Denier(), o.Reason(), actor,
	); err != nil {
		return nil, nil, fmt.Errorf("upsert RM denier override: %w", err)
	}

	saved, err := queryRMDenierOverrides(ctx, tx, rmDenierOverrideSelect+`
		WHERE o.cpdo_product_sys_id = $1 AND o.cpdo_rm_type = $2
		  AND COALESCE(o.cpdo_rm_product_sys_id, 0) = $3
		  AND COALESCE(o.cpdo_rm_group_code, '') = $4`,
		o.ProductSysID(), o.RMType(), o.RMProductSysID(), o.RMGroupCode())
	if err != nil {
		return nil, nil, err
	}
	if len(saved) != 1 {
		return nil, nil, fmt.Errorf("read back RM denier override: got %d rows", len(saved))
	}

	if err := tx.Commit(); err != nil {
		return nil, nil, fmt.Errorf("commit RM denier override upsert: %w", err)
	}
	committed = true
	return saved[0], copies, nil
}

// checkOverrideRefs verifies the owner product and the overridden RM exist, mapping a
// missing row to the matching domain sentinel instead of a raw FK violation.
func checkOverrideRefs(ctx context.Context, tx *sql.Tx, owner int64, rmType string, rmProductSysID int64, rmGroupCode string) error {
	var ok bool
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM cost_product_master WHERE cpm_product_sys_id = $1)`, owner,
	).Scan(&ok); err != nil {
		return fmt.Errorf("check override owner %d: %w", owner, err)
	}
	if !ok {
		return costproductmaster.ErrNotFound
	}
	if rmType == costproductmaster.RMTypeProduct {
		if err := tx.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM cost_product_master WHERE cpm_product_sys_id = $1)`, rmProductSysID,
		).Scan(&ok); err != nil {
			return fmt.Errorf("check override RM product %d: %w", rmProductSysID, err)
		}
		if !ok {
			return costproductmaster.ErrOverrideRMProductNotFound
		}
		return nil
	}
	if err := tx.QueryRowContext(ctx,
		`SELECT EXISTS (SELECT 1 FROM cst_rm_group_head WHERE group_code = $1 AND deleted_at IS NULL)`, rmGroupCode,
	).Scan(&ok); err != nil {
		return fmt.Errorf("check override RM group %s: %w", rmGroupCode, err)
	}
	if !ok {
		return costproductmaster.ErrOverrideRMGroupNotFound
	}
	return nil
}

// DeleteRMDenierOverride hard-deletes the override for the owner and, with
// ApplyToDuplicates, the same RM identity on every direct copy, in one transaction.
func (r *CostProductMasterRepository) DeleteRMDenierOverride(
	ctx context.Context, in costproductmaster.DeleteRMDenierOverrideInput,
) (int, []int64, error) {
	rmType, rmProductSysID, rmGroupCode, err := costproductmaster.ValidateRMIdentity(
		in.ProductSysID, in.RMType, in.RMProductSysID, in.RMGroupCode)
	if err != nil {
		return 0, nil, err
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, nil, fmt.Errorf("begin tx: %w", err)
	}
	committed := false
	defer func() {
		if committed {
			return
		}
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
			_ = rbErr
		}
	}()

	targets, _, err := overrideTargets(ctx, tx, in.ProductSysID, in.ApplyToDuplicates, rmProductSysID)
	if err != nil {
		return 0, nil, err
	}
	rows, err := tx.QueryContext(ctx, `
		DELETE FROM cost_product_rm_denier_override
		WHERE cpdo_product_sys_id = ANY($1::bigint[])
		  AND cpdo_rm_type = $2
		  AND COALESCE(cpdo_rm_product_sys_id, 0) = $3
		  AND COALESCE(cpdo_rm_group_code, '') = $4
		RETURNING cpdo_product_sys_id`,
		pq.Array(targets), rmType, rmProductSysID, rmGroupCode)
	if err != nil {
		return 0, nil, fmt.Errorf("delete RM denier override: %w", err)
	}
	deleted := 0
	applied := []int64{}
	for rows.Next() {
		var pid int64
		if sErr := rows.Scan(&pid); sErr != nil {
			if cErr := rows.Close(); cErr != nil {
				_ = cErr
			}
			return 0, nil, fmt.Errorf("scan deleted RM denier override: %w", sErr)
		}
		deleted++
		if pid != in.ProductSysID {
			applied = append(applied, pid)
		}
	}
	if err := rows.Err(); err != nil {
		return 0, nil, fmt.Errorf("iterate deleted RM denier overrides: %w", err)
	}
	if cErr := rows.Close(); cErr != nil {
		return 0, nil, fmt.Errorf("close deleted RM denier overrides: %w", cErr)
	}

	if err := tx.Commit(); err != nil {
		return 0, nil, fmt.Errorf("commit RM denier override delete: %w", err)
	}
	committed = true
	return deleted, applied, nil
}
