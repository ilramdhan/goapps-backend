package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbactivesync"
)

// MBActiveSyncReportReader implements mbactivesync.ReportReader. READ ONLY: every
// read runs inside one read-only REPEATABLE READ transaction.
type MBActiveSyncReportReader struct{ db *DB }

// NewMBActiveSyncReportReader creates the reader.
func NewMBActiveSyncReportReader(db *DB) *MBActiveSyncReportReader {
	return &MBActiveSyncReportReader{db: db}
}

var _ mbactivesync.ReportReader = (*MBActiveSyncReportReader)(nil)

// mbasReportProductsQuery: active products that carry an MB_SP_CODE / MB_SP_DOZING value
// or a non-blank shade (the FG-aware lookup population), with the stored values.
const mbasReportProductsQuery = `
	SELECT m.cpm_product_sys_id, COALESCE(m.cpm_product_code, ''), COALESCE(m.cpm_product_name, ''),
	       COALESCE(t.cpt_type_code, ''), COALESCE(m.cpm_shade_code, ''), COALESCE(m.cpm_is_locked, FALSE),
	       (sp.cpp_value_id IS NOT NULL), COALESCE(sp.cpp_value_text, ''), sp.cpp_value_mb_spin_id,
	       dz.cpp_value_numeric
	FROM cost_product_master m
	LEFT JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
	LEFT JOIN mst_parameter psp ON psp.param_code = 'MB_SP_CODE' AND psp.deleted_at IS NULL
	LEFT JOIN cost_product_parameter sp ON sp.cpp_product_sys_id = m.cpm_product_sys_id AND sp.cpp_param_id = psp.id
	LEFT JOIN mst_parameter pdz ON pdz.param_code = 'MB_SP_DOZING' AND pdz.deleted_at IS NULL
	LEFT JOIN cost_product_parameter dz ON dz.cpp_product_sys_id = m.cpm_product_sys_id AND dz.cpp_param_id = pdz.id
	WHERE m.cpm_is_active
	  AND COALESCE(t.cpt_type_code, '') <> 'MB'
	  AND (sp.cpp_value_id IS NOT NULL OR dz.cpp_value_id IS NOT NULL OR COALESCE(TRIM(m.cpm_shade_code), '') <> '')
	ORDER BY m.cpm_product_sys_id`

const mbasReportBatch = 500

// LoadReportProducts implements mbactivesync.ReportReader.
func (r *MBActiveSyncReportReader) LoadReportProducts(ctx context.Context) ([]mbactivesync.ReportProduct, error) {
	tx, err := r.db.BeginTx(ctx, snapshotTxOptions())
	if err != nil {
		return nil, fmt.Errorf("begin read-only report tx: %w", err)
	}
	defer rollbackSnapshotTx(tx)

	out, err := mbasQueryReportProducts(ctx, tx)
	if err != nil {
		return nil, err
	}
	for start := 0; start < len(out); start += mbasReportBatch {
		end := min(start+mbasReportBatch, len(out))
		ids := make([]int64, 0, end-start)
		for i := start; i < end; i++ {
			ids = append(ids, out[i].SysID)
		}
		cands, cErr := mbasFgCandidatesOn(ctx, tx, ids)
		if cErr != nil {
			return nil, cErr
		}
		for i := start; i < end; i++ {
			out[i].FGCandidates = cands[out[i].SysID]
		}
	}
	return out, nil
}

func mbasQueryReportProducts(ctx context.Context, q supersedeQuerier) ([]mbactivesync.ReportProduct, error) {
	rows, err := q.QueryContext(ctx, mbasReportProductsQuery)
	if err != nil {
		return nil, fmt.Errorf("report products: %w", err)
	}
	defer closeRows(rows)
	var out []mbactivesync.ReportProduct
	for rows.Next() {
		var (
			p      mbactivesync.ReportProduct
			spinID uuid.NullUUID
			doz    sql.NullFloat64
		)
		if err := rows.Scan(&p.SysID, &p.Code, &p.Name, &p.TypeCode, &p.Shade, &p.Locked,
			&p.HasSPCode, &p.SPText, &spinID, &doz); err != nil {
			return nil, fmt.Errorf("scan report product: %w", err)
		}
		if spinID.Valid {
			v := spinID.UUID
			p.SPSpinID = &v
		}
		if doz.Valid {
			v := doz.Float64
			p.Dozing = &v
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report products: %w", err)
	}
	return out, nil
}

// mbasFgCandidatesOn runs the FgCandidateReader query (fgCandidatesSQL) on q, keeping
// the first occurrence per (product, candidate).
func mbasFgCandidatesOn(ctx context.Context, q supersedeQuerier, ids []int64) (map[int64][]int64, error) {
	out := make(map[int64][]int64, len(ids))
	rows, err := q.QueryContext(ctx, fgCandidatesSQL, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("report fg candidates: %w", err)
	}
	defer closeRows(rows)
	seen := make(map[[2]int64]struct{})
	for rows.Next() {
		var pid, cand, level, seq, rmID int64
		if err := rows.Scan(&pid, &cand, &level, &seq, &rmID); err != nil {
			return nil, fmt.Errorf("scan report fg candidate: %w", err)
		}
		k := [2]int64{pid, cand}
		if _, dup := seen[k]; dup {
			continue
		}
		seen[k] = struct{}{}
		out[pid] = append(out[pid], cand)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate report fg candidates: %w", err)
	}
	return out, nil
}
