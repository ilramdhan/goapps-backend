// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"fmt"
)

// CstMBCostRepository implements persistence for the cst_mb_cost periodic active-cost cache —
// the only table downstream MB-cost consumers (POY, etc.) ever read from.
type CstMBCostRepository struct {
	db *DB
}

// NewCstMBCostRepository creates a new CstMBCostRepository instance.
func NewCstMBCostRepository(db *DB) *CstMBCostRepository {
	return &CstMBCostRepository{db: db}
}

// Upsert writes one cst_mb_cost row per (mbh_id, period, cost_type), called only from
// Push-to-Head execute — this table is never written from any other code path.
func (r *CstMBCostRepository) Upsert(ctx context.Context, tx *sql.Tx, mbhID, period, costType, costValue string, sourceCpcID int64, pushedBy string) error {
	const q = `
		INSERT INTO cst_mb_cost
			(mbc_mbh_id, mbc_period, mbc_cost_type, mbc_cost_value, mbc_source_cpc_id, mbc_pushed_by)
		VALUES ($1, $2, $3, $4, NULLIF($5, 0), $6)
		ON CONFLICT (mbc_mbh_id, mbc_period, mbc_cost_type)
		DO UPDATE SET mbc_cost_value = EXCLUDED.mbc_cost_value,
		              mbc_source_cpc_id = EXCLUDED.mbc_source_cpc_id,
		              mbc_pushed_at = NOW(),
		              mbc_pushed_by = EXCLUDED.mbc_pushed_by,
		              mbc_updated_at = NOW()`
	_, err := tx.ExecContext(ctx, q, mbhID, period, costType, costValue, sourceCpcID, pushedBy)
	if err != nil {
		return fmt.Errorf("cst_mb_cost_repository: upsert: %w", err)
	}
	return nil
}

// ListStalePushedMBHIDs returns the distinct mbh_ids whose already-pushed cost for period has
// gone stale: an active cst_mb_cost row exists, but its source cst_product_cost row is either
// unlinked (mbc_source_cpc_id IS NULL, e.g. the FK's ON DELETE SET NULL fired) or SUPERSEDED,
// while a newer non-superseded row exists for the same product/period/cost type. That happens
// when MB Batch re-runs after a push — the pushed value is stale and silently so, hence this
// read-only visibility query. Never used to gate a write; Preview only labels the head.
func (r *CstMBCostRepository) ListStalePushedMBHIDs(ctx context.Context, period string) ([]string, error) {
	const q = `
		SELECT DISTINCT mbc.mbc_mbh_id::text
		FROM cst_mb_cost mbc
		JOIN mst_mb_head mbh
		  ON mbh.mbh_id = mbc.mbc_mbh_id
		 AND mbh.mbh_entry_status = 'VALIDATED'
		 AND mbh.deleted_at IS NULL
		LEFT JOIN cst_product_cost src ON src.cpc_cost_id = mbc.mbc_source_cpc_id
		WHERE mbc.mbc_period = $1
		  AND mbc.mbc_is_active = TRUE
		  AND (mbc.mbc_source_cpc_id IS NULL OR src.cpc_status = 'SUPERSEDED')
		  AND EXISTS (
		        SELECT 1 FROM cst_product_cost cur
		        WHERE cur.cpc_product_sys_id = mbh.mbh_cost_product_id
		          AND cur.cpc_period = mbc.mbc_period
		          AND cur.cpc_calculation_type = mbc.mbc_cost_type
		          AND cur.cpc_status != 'SUPERSEDED'
		      )`
	rows, err := r.db.QueryContext(ctx, q, period)
	if err != nil {
		return nil, fmt.Errorf("cst_mb_cost_repository: list stale pushed mbh ids: %w", err)
	}
	defer closeRows(rows)

	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("cst_mb_cost_repository: scan stale pushed mbh id: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("cst_mb_cost_repository: iterate stale pushed mbh ids: %w", err)
	}
	return out, nil
}

// LatestByType returns the most recent active cost_value for mbhID + costType, used by
// Plan 04's LoadMBCosts calc-engine loader — the sole read path for MB cost consumers.
func (r *CstMBCostRepository) LatestByType(ctx context.Context, mbhID, costType string) (string, error) {
	const q = `
		SELECT mbc_cost_value FROM cst_mb_cost
		WHERE mbc_mbh_id = $1 AND mbc_cost_type = $2 AND mbc_is_active = TRUE
		ORDER BY mbc_period DESC LIMIT 1`
	var value string
	err := r.db.QueryRowContext(ctx, q, mbhID, costType).Scan(&value)
	if err != nil {
		return "", fmt.Errorf("cst_mb_cost_repository: latest by type: %w", err)
	}
	return value, nil
}
