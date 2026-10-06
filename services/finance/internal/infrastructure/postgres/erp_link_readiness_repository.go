package postgres

// erp_link_readiness_repository.go is the read-only PostgreSQL source of the
// ERP link-readiness report (plan P3-T9, recon §9a–§9f). Every statement is a
// SELECT over cost_product_master / cost_product_type / cost_erp_item /
// cst_product_cost. The D-LINK key is cpm_erp_item_code + cpm_shade_code on
// active AX products; cpm_erp_grade_code_1/2 are never read.

import (
	"context"
	"fmt"

	"github.com/lib/pq"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
)

// ErpLinkReadinessRepository implements erpintegration.LinkReadinessSource.
type ErpLinkReadinessRepository struct{ db *DB }

var _ app.LinkReadinessSource = (*ErpLinkReadinessRepository)(nil)

// NewErpLinkReadinessRepository builds the repository.
func NewErpLinkReadinessRepository(db *DB) *ErpLinkReadinessRepository {
	return &ErpLinkReadinessRepository{db: db}
}

// erpLinkReadinessLinkedSQL lists active linked AX products with their type,
// attributes, replica presence and non-SUPERSEDED ACTUAL row count for $1.
const erpLinkReadinessLinkedSQL = `
	SELECT cpm.cpm_product_sys_id, cpm.cpm_product_code, COALESCE(cpm.cpm_product_name,''),
		TRIM(cpm.cpm_erp_item_code), COALESCE(TRIM(cpm.cpm_shade_code),''), COALESCE(cpt.cpt_type_code,''),
		COALESCE(TRIM(cpm.cpm_erp_fg_type),''), COALESCE(TRIM(cpm.cpm_erp_chp_item_code),''),
		COALESCE(TRIM(cpm.cpm_erp_ms_batch_item),''), COALESCE(TRIM(cpm.cpm_erp_item_type),''),
		COALESCE(cpm.cpm_erp_prd_per_day::text,''),
		EXISTS (SELECT 1 FROM cost_erp_item cei
			WHERE UPPER(TRIM(cei.cei_item_code)) = UPPER(TRIM(cpm.cpm_erp_item_code))),
		(SELECT COUNT(*) FROM cst_product_cost cpc
			WHERE cpc.cpc_product_sys_id = cpm.cpm_product_sys_id
			  AND cpc.cpc_period = $1
			  AND cpc.cpc_calculation_type = 'ACTUAL'
			  AND cpc.cpc_status <> 'SUPERSEDED')
	FROM cost_product_master cpm
	LEFT JOIN cost_product_type cpt ON cpt.cpt_type_id = cpm.cpm_product_type_id
	WHERE cpm.cpm_is_active
	  AND NULLIF(TRIM(cpm.cpm_erp_item_code),'') IS NOT NULL
	  AND UPPER(COALESCE(NULLIF(TRIM(cpm.cpm_grade_code),''),'AX')) = 'AX'
	ORDER BY cpm.cpm_product_sys_id`

// ListLinkedAxProducts implements erpintegration.LinkReadinessSource.
func (r *ErpLinkReadinessRepository) ListLinkedAxProducts(ctx context.Context, period string) (out []app.LinkedAxProduct, err error) {
	rows, err := r.db.QueryContext(ctx, erpLinkReadinessLinkedSQL, period)
	if err != nil {
		return nil, fmt.Errorf("list linked AX products: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close linked AX products: %w", cerr)
		}
	}()
	for rows.Next() {
		var p app.LinkedAxProduct
		a := &p.Attrs
		if err := rows.Scan(&p.SysID, &p.ProductCode, &p.ProductName, &p.ItemCode, &p.ShadeCode, &p.TypeCode,
			&a.FgType, &a.ChpItemCode, &a.MsBatchItem, &a.ItemType, &a.PrdPerDay,
			&p.InReplica, &p.ActualRows); err != nil {
			return nil, fmt.Errorf("scan linked AX product: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate linked AX products: %w", err)
	}
	return out, nil
}

// erpLinkReadinessUnlinkedSQL lists active AX products with no ERP item code
// whose normalized shade is in $1.
const erpLinkReadinessUnlinkedSQL = `
	SELECT cpm.cpm_product_sys_id, cpm.cpm_product_code, COALESCE(cpm.cpm_product_name,''),
		COALESCE(TRIM(cpm.cpm_shade_code),''), COALESCE(cpt.cpt_type_code,'')
	FROM cost_product_master cpm
	LEFT JOIN cost_product_type cpt ON cpt.cpt_type_id = cpm.cpm_product_type_id
	WHERE cpm.cpm_is_active
	  AND NULLIF(TRIM(cpm.cpm_erp_item_code),'') IS NULL
	  AND UPPER(COALESCE(NULLIF(TRIM(cpm.cpm_grade_code),''),'AX')) = 'AX'
	  AND UPPER(TRIM(COALESCE(cpm.cpm_shade_code,''))) = ANY($1::text[])
	ORDER BY cpm.cpm_product_sys_id`

// ListUnlinkedAxProducts implements erpintegration.LinkReadinessSource.
func (r *ErpLinkReadinessRepository) ListUnlinkedAxProducts(ctx context.Context, shadeKeys []string) (out []app.UnlinkedAxProduct, err error) {
	if len(shadeKeys) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, erpLinkReadinessUnlinkedSQL, pq.Array(shadeKeys))
	if err != nil {
		return nil, fmt.Errorf("list unlinked AX products: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close unlinked AX products: %w", cerr)
		}
	}()
	for rows.Next() {
		var p app.UnlinkedAxProduct
		if err := rows.Scan(&p.SysID, &p.ProductCode, &p.ProductName, &p.ShadeCode, &p.TypeCode); err != nil {
			return nil, fmt.Errorf("scan unlinked AX product: %w", err)
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unlinked AX products: %w", err)
	}
	return out, nil
}

// CountReplicaItems implements erpintegration.LinkReadinessSource.
func (r *ErpLinkReadinessRepository) CountReplicaItems(ctx context.Context) (int64, error) {
	var n int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_erp_item`).Scan(&n); err != nil {
		return 0, fmt.Errorf("count replica items: %w", err)
	}
	return n, nil
}
