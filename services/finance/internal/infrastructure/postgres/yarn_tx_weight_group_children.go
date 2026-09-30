package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// Read-side helpers of YarnTxWeightGroupRepository: batch-load the product
// type mappings and live rules of a page of groups (split out to keep the
// repository file under the 400-line guideline).

// attachChildren loads product types and live rules for all groups in two queries.
func (r *YarnTxWeightGroupRepository) attachChildren(ctx context.Context, params []*yarntxweightgroup.ReconstructParams) error {
	if len(params) == 0 {
		return nil
	}
	byID := make(map[uuid.UUID]*yarntxweightgroup.ReconstructParams, len(params))
	ids := make([]string, len(params))
	for i, p := range params {
		byID[p.ID] = p
		ids[i] = p.ID.String()
	}
	if err := r.loadGroupTypes(ctx, ids, byID); err != nil {
		return err
	}
	return r.loadGroupRules(ctx, ids, byID)
}

func (r *YarnTxWeightGroupRepository) loadGroupTypes(ctx context.Context, ids []string, byID map[uuid.UUID]*yarntxweightgroup.ReconstructParams) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT gt.ytwg_id, gt.product_type_id, COALESCE(pt.cpt_type_code, ''), COALESCE(pt.cpt_type_name, '')
		FROM mst_yarn_tx_weight_group_type gt
		LEFT JOIN cost_product_type pt ON pt.cpt_type_id = gt.product_type_id
		WHERE gt.ytwg_id = ANY($1::uuid[])
		ORDER BY pt.cpt_type_code`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("load yarn tx weight group types: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			gid uuid.UUID
			ref yarntxweightgroup.ProductTypeRef
		)
		if err := rows.Scan(&gid, &ref.ID, &ref.Code, &ref.Name); err != nil {
			return fmt.Errorf("scan yarn tx weight group type: %w", err)
		}
		if p, ok := byID[gid]; ok {
			p.ProductTypes = append(p.ProductTypes, ref)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate yarn tx weight group types: %w", err)
	}
	return nil
}

func (r *YarnTxWeightGroupRepository) loadGroupRules(ctx context.Context, ids []string, byID map[uuid.UUID]*yarntxweightgroup.ReconstructParams) error {
	rows, err := r.db.QueryContext(ctx, `
		SELECT w.ytw_group_id, w.ytw_grade, w.ytw_mode, w.ytw_value, w.ytw_description
		FROM mst_yarn_tx_weight w
		WHERE w.ytw_group_id = ANY($1::uuid[]) AND w.deleted_at IS NULL
		ORDER BY array_position(ARRAY['AE','A9','A','B','C']::varchar[], w.ytw_grade)`, pq.Array(ids))
	if err != nil {
		return fmt.Errorf("load yarn tx weight group rules: %w", err)
	}
	defer closeRows(rows)
	for rows.Next() {
		var (
			gid         uuid.UUID
			grade, mode string
			value       float64
			desc        sql.NullString
		)
		if err := rows.Scan(&gid, &grade, &mode, &value, &desc); err != nil {
			return fmt.Errorf("scan yarn tx weight group rule: %w", err)
		}
		if p, ok := byID[gid]; ok {
			p.Rules = append(p.Rules, yarntxweightgroup.Rule{
				Grade: yarntxweight.Grade(grade), Mode: yarntxweight.Mode(mode), Value: value, Description: desc.String,
			})
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate yarn tx weight group rules: %w", err)
	}
	return nil
}
