package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbdozing"
)

// MBDozingImpactRepository implements mbdozing.ImpactRepository.
//
// READ-ONLY (user decision K-18): this repository issues SELECT statements only.
// It deliberately exposes no write method — freezing a new dozing value onto
// products belongs to P8.
type MBDozingImpactRepository struct{ db *DB }

// NewMBDozingImpactRepository constructs the repository.
func NewMBDozingImpactRepository(db *DB) *MBDozingImpactRepository {
	return &MBDozingImpactRepository{db: db}
}

var _ mbdozing.ImpactRepository = (*MBDozingImpactRepository)(nil)

// defaultImpactLimit is used when the caller passes a non-positive limit.
const defaultImpactLimit = 20

// maxImpactLimit caps the number of rows a single preview may return, matching
// the proto-level bound on PreviewDozingImpactRequest.limit.
const maxImpactLimit = 200

// impactRowsQuery selects the affected products.
//
// A product is "affected" when it carries an MB_SP_CODE parameter value equal to
// the spin's ORION item code — that is the parameter bound to the MB_SPIN lookup
// master (migration 000407, PART 2). The frozen dozing is read from the sibling
// MB_SP_DOZING parameter value on the same product and stays NULL when absent;
// it is never defaulted.
const impactRowsQuery = `
SELECT m.cpm_product_sys_id,
       m.cpm_product_code,
       m.cpm_product_name,
       m.cpm_is_locked,
       d.cpp_value_numeric::float8 AS frozen_dozing
FROM cost_product_parameter c
JOIN mst_parameter p
       ON p.id = c.cpp_param_id
      AND p.param_code = 'MB_SP_CODE'
      AND p.deleted_at IS NULL
JOIN cost_product_master m
       ON m.cpm_product_sys_id = c.cpp_product_sys_id
LEFT JOIN mst_parameter pd
       ON pd.param_code = 'MB_SP_DOZING'
      AND pd.deleted_at IS NULL
LEFT JOIN cost_product_parameter d
       ON d.cpp_product_sys_id = m.cpm_product_sys_id
      AND d.cpp_param_id = pd.id
WHERE c.cpp_value_text = $1
ORDER BY m.cpm_is_locked DESC, m.cpm_product_code
LIMIT $2`

// impactTotalsQuery counts every affected product, ignoring the row limit.
const impactTotalsQuery = `
SELECT COUNT(*)                                             AS total_affected,
       COUNT(*) FILTER (WHERE m.cpm_is_locked)              AS total_locked
FROM cost_product_parameter c
JOIN mst_parameter p
       ON p.id = c.cpp_param_id
      AND p.param_code = 'MB_SP_CODE'
      AND p.deleted_at IS NULL
JOIN cost_product_master m
       ON m.cpm_product_sys_id = c.cpp_product_sys_id
WHERE c.cpp_value_text = $1`

// ImpactBySpin returns up to limit affected products plus the un-truncated totals.
func (r *MBDozingImpactRepository) ImpactBySpin(ctx context.Context, orionItemCode string, limit int) ([]mbdozing.ImpactRow, mbdozing.Totals, error) {
	if limit <= 0 {
		limit = defaultImpactLimit
	}
	if limit > maxImpactLimit {
		limit = maxImpactLimit
	}

	var totals mbdozing.Totals
	if err := r.db.QueryRowContext(ctx, impactTotalsQuery, orionItemCode).
		Scan(&totals.TotalAffected, &totals.TotalLocked); err != nil {
		return nil, mbdozing.Totals{}, fmt.Errorf("mb_dozing_impact_repository: totals: %w", err)
	}

	rows, err := r.db.QueryContext(ctx, impactRowsQuery, orionItemCode, limit)
	if err != nil {
		return nil, mbdozing.Totals{}, fmt.Errorf("mb_dozing_impact_repository: rows: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	out := make([]mbdozing.ImpactRow, 0, limit)
	for rows.Next() {
		var row mbdozing.ImpactRow
		var frozen sql.NullFloat64
		if err := rows.Scan(&row.ProductSysID, &row.ProductCode, &row.ProductName, &row.IsLocked, &frozen); err != nil {
			return nil, mbdozing.Totals{}, fmt.Errorf("mb_dozing_impact_repository: scan: %w", err)
		}
		if frozen.Valid {
			v := frozen.Float64
			row.FrozenDozing = &v
		}
		out = append(out, row)
	}
	if err := rows.Err(); err != nil {
		return nil, mbdozing.Totals{}, fmt.Errorf("mb_dozing_impact_repository: iterate: %w", err)
	}

	return out, totals, nil
}
