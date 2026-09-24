package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

var _ costproducttype.OilConfigRepository = (*CostProductTypeRepository)(nil)

// GetOilConfig returns the oil class and mapped oil groups of a product type.
// Mapped groups are listed even when a group was later un-flagged/deleted, so
// the editor can show and remove stale rows.
func (r *CostProductTypeRepository) GetOilConfig(ctx context.Context, typeID int32) (*costproducttype.OilConfig, error) {
	var class string
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(cpt_oil_class, '') FROM cost_product_type WHERE cpt_type_id = $1`, typeID,
	).Scan(&class)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, costproducttype.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get cpt_oil_class: %w", err)
	}

	const q = `
SELECT gh.group_code, gh.group_name, og.cptog_is_default
FROM cost_product_type_oil_group og
JOIN cst_rm_group_head gh ON gh.group_head_id = og.cptog_group_head_id
WHERE og.cptog_type_id = $1
ORDER BY og.cptog_is_default DESC, gh.group_code`
	rows, err := r.db.QueryContext(ctx, q, typeID)
	if err != nil {
		return nil, fmt.Errorf("list cost_product_type_oil_group: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	cfg := &costproducttype.OilConfig{TypeID: typeID, OilClass: class, Groups: []costproducttype.OilGroupEntry{}}
	for rows.Next() {
		var g costproducttype.OilGroupEntry
		if err := rows.Scan(&g.GroupCode, &g.GroupName, &g.IsDefault); err != nil {
			return nil, fmt.Errorf("scan cost_product_type_oil_group: %w", err)
		}
		cfg.Groups = append(cfg.Groups, g)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost_product_type_oil_group: %w", err)
	}
	return cfg, nil
}

// ResolveOilGroups returns the active oil RM groups (v_rm_group_oil, not
// deleted) among codes, keyed by group code.
func (r *CostProductTypeRepository) ResolveOilGroups(ctx context.Context, codes []string) (map[string]costproducttype.ResolvedOilGroup, error) {
	out := make(map[string]costproducttype.ResolvedOilGroup, len(codes))
	if len(codes) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx,
		`SELECT group_head_id::text, group_code, group_name
		 FROM v_rm_group_oil
		 WHERE deleted_at IS NULL AND group_code = ANY($1)`, pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("resolve oil groups: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var g costproducttype.ResolvedOilGroup
		if err := rows.Scan(&g.GroupHeadID, &g.GroupCode, &g.GroupName); err != nil {
			return nil, fmt.Errorf("scan oil group: %w", err)
		}
		out[g.GroupCode] = g
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate oil groups: %w", err)
	}
	return out, nil
}

// ReplaceOilConfig sets cpt_oil_class and replaces every mapped oil group of the
// type in one transaction (delete + insert, so the partial unique default index
// never sees two defaults).
func (r *CostProductTypeRepository) ReplaceOilConfig(
	ctx context.Context, typeID int32, oilClass string, groups []costproducttype.ReplaceOilGroup, actor string,
) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE cost_product_type SET cpt_oil_class = NULLIF($2, ''), cpt_updated_at = NOW() WHERE cpt_type_id = $1`,
			typeID, oilClass)
		if err != nil {
			return fmt.Errorf("update cpt_oil_class: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			return costproducttype.ErrNotFound
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM cost_product_type_oil_group WHERE cptog_type_id = $1`, typeID); err != nil {
			return fmt.Errorf("clear cost_product_type_oil_group: %w", err)
		}
		const ins = `
INSERT INTO cost_product_type_oil_group (cptog_type_id, cptog_group_head_id, cptog_is_default, cptog_created_by)
VALUES ($1, $2::uuid, $3, $4)`
		for _, g := range groups {
			if _, err := tx.ExecContext(ctx, ins, typeID, g.GroupHeadID, g.IsDefault, actor); err != nil {
				return fmt.Errorf("insert cost_product_type_oil_group: %w", err)
			}
		}
		return nil
	})
}
