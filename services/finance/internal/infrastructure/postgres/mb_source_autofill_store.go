package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbsourceautofill"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbsource"
)

// MBSourceAutoFillStore implements mbsourceautofill.Store.
type MBSourceAutoFillStore struct{ db *DB }

// NewMBSourceAutoFillStore creates the store.
func NewMBSourceAutoFillStore(db *DB) *MBSourceAutoFillStore { return &MBSourceAutoFillStore{db: db} }

var _ mbsourceautofill.Store = (*MBSourceAutoFillStore)(nil)

// ListTargets returns unlocked, non-MB products with a shade whose MB_SP_CODE has no value.
func (s *MBSourceAutoFillStore) ListTargets(ctx context.Context, ids []int64) ([]mbsourceautofill.Target, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	const q = `
SELECT m.cpm_product_sys_id, m.cpm_shade_code
FROM cost_product_master m
JOIN cost_product_type t ON t.cpt_type_id = m.cpm_product_type_id
WHERE m.cpm_product_sys_id = ANY($1)
  AND NOT m.cpm_is_locked
  AND t.cpt_type_code <> 'MB'
  AND COALESCE(TRIM(m.cpm_shade_code), '') <> ''
  AND NOT EXISTS (
    SELECT 1 FROM cost_product_parameter c
    JOIN mst_parameter p ON p.id = c.cpp_param_id AND p.param_code = $2 AND p.deleted_at IS NULL
    WHERE c.cpp_product_sys_id = m.cpm_product_sys_id
      AND COALESCE(c.cpp_value_text, '') <> '')`
	rows, err := s.db.QueryContext(ctx, q, pq.Array(ids), mbsource.ParamSPCode)
	if err != nil {
		return nil, fmt.Errorf("list mb source targets: %w", err)
	}
	defer closeRows(rows)
	var out []mbsourceautofill.Target
	for rows.Next() {
		var t mbsourceautofill.Target
		if err := rows.Scan(&t.ProductSysID, &t.Shade); err != nil {
			return nil, fmt.Errorf("scan mb source target: %w", err)
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

// Apply attaches the MB_SP_CODE group to the product and writes the empty values, in one tx.
func (s *MBSourceAutoFillStore) Apply(ctx context.Context, ps mbsourceautofill.ParamSet, plan mbsourceautofill.Plan, actor string) (err error) {
	if actor == "" {
		actor = mbsource.FilledBy
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() {
		if err != nil {
			if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) {
				_ = rbErr
			}
		}
	}()

	const attach = `INSERT INTO cost_product_applicable_param (capp_product_sys_id, capp_param_id, capp_is_required, capp_created_by)
	                VALUES ($1, $2, FALSE, $3) ON CONFLICT (capp_product_sys_id, capp_param_id) DO NOTHING`
	// Value write: new row, or fill an existing row ONLY when it is still empty text.
	const put = `INSERT INTO cost_product_parameter
	   (cpp_product_sys_id, cpp_param_id, cpp_value_numeric, cpp_value_text, cpp_value_mb_spin_id,
	    cpp_filled_by, cpp_created_by)
	 VALUES ($1, $2, $3, $4, $5, $6, $6)
	 ON CONFLICT (cpp_product_sys_id, cpp_param_id) DO UPDATE SET
	    cpp_value_numeric = EXCLUDED.cpp_value_numeric, cpp_value_text = EXCLUDED.cpp_value_text,
	    cpp_value_mb_spin_id = EXCLUDED.cpp_value_mb_spin_id,
	    cpp_filled_at = NOW(), cpp_filled_by = EXCLUDED.cpp_filled_by,
	    cpp_updated_at = NOW(), cpp_updated_by = EXCLUDED.cpp_filled_by
	 WHERE cost_product_parameter.cpp_value_numeric IS NULL
	   AND cost_product_parameter.cpp_value_flag IS NULL
	   AND COALESCE(cost_product_parameter.cpp_value_text, '') = ''`

	attachIDs := []uuid.UUID{ps.TriggerID}
	for _, id := range ps.Children {
		attachIDs = append(attachIDs, id)
	}
	for _, id := range attachIDs {
		if _, err = tx.ExecContext(ctx, attach, plan.ProductSysID, id, actor); err != nil {
			return fmt.Errorf("attach capp: %w", err)
		}
	}
	if err = writeMBSourceValues(ctx, tx, put, ps, plan); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// writeMBSourceValues writes MB_SP_CODE (+ spin companion id) and each non-empty child.
func writeMBSourceValues(ctx context.Context, tx *sql.Tx, put string, ps mbsourceautofill.ParamSet, plan mbsourceautofill.Plan) error {
	res := plan.Resolution
	var spinID interface{}
	if res.SpinID != nil {
		spinID = *res.SpinID
	}
	if _, err := tx.ExecContext(ctx, put, plan.ProductSysID, ps.TriggerID, nil, res.SPCode, spinID, mbsource.FilledBy); err != nil {
		return fmt.Errorf("write %s: %w", mbsource.ParamSPCode, err)
	}
	for code, v := range res.Children {
		pid, ok := ps.Children[code]
		if !ok {
			continue
		}
		var num, txt interface{}
		switch {
		case v.Num != nil:
			num = *v.Num
		case v.Text != nil && *v.Text != "":
			txt = *v.Text
		default:
			continue
		}
		if _, err := tx.ExecContext(ctx, put, plan.ProductSysID, pid, num, txt, nil, mbsource.FilledBy); err != nil {
			return fmt.Errorf("write %s: %w", code, err)
		}
	}
	return nil
}
