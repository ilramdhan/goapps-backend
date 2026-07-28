package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// ── Parameters (1:N per param) ───────────────────────────────────────────────

// ReplaceParameters replaces all planned parameter rows for a WO in one tx.
func (r *WorkOrderRepository) ReplaceParameters(ctx context.Context, woID int64, params []*workorder.Parameter) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM wo_parameter WHERE wop_wo_id = $1`, woID); err != nil {
			return fmt.Errorf("failed to clear wo parameters: %w", err)
		}
		for _, p := range params {
			if err := insertParameterTx(ctx, tx, woID, p); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertParameterTx(ctx context.Context, tx *sql.Tx, woID int64, p *workorder.Parameter) error {
	query := `
		INSERT INTO wo_parameter (
			wop_wo_id, wop_param_id, wop_value_ppc_num, wop_value_ppc_text, wop_value_ppc_flag,
			wop_value_pc_num, wop_value_pc_text, wop_value_pc_flag, wop_is_dual
		) VALUES ($1, $2::UUID, $3::DECIMAL, $4, $5::BOOLEAN, $6::DECIMAL, $7, $8::BOOLEAN, $9)
		RETURNING wop_id`
	var id int64
	err := tx.QueryRowContext(ctx, query,
		woID, p.ParamID, floatPtrArg(p.ValuePPCNum), stringPtrToNull(p.ValuePPCText), boolPtrToNull(p.ValuePPCFlag),
		floatPtrArg(p.ValuePCNum), stringPtrToNull(p.ValuePCText), boolPtrToNull(p.ValuePCFlag), p.IsDual,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to insert wo parameter: %w", err)
	}
	p.ID = id
	return nil
}

// SetParameterPPCValue upserts the PPC value of one parameter.
func (r *WorkOrderRepository) SetParameterPPCValue(ctx context.Context, woID int64, p *workorder.Parameter) error {
	query := `
		UPDATE wo_parameter SET
			wop_value_ppc_num = $3::DECIMAL, wop_value_ppc_text = $4, wop_value_ppc_flag = $5::BOOLEAN
		WHERE wop_wo_id = $1 AND wop_param_id = $2::UUID`
	res, err := r.db.ExecContext(ctx, query,
		woID, p.ParamID, floatPtrArg(p.ValuePPCNum), stringPtrToNull(p.ValuePPCText), boolPtrToNull(p.ValuePPCFlag),
	)
	if err != nil {
		return fmt.Errorf("failed to set wo parameter ppc value: %w", err)
	}
	return checkAffected(res, workorder.ErrNotFound)
}

// SetParameterPCValue upserts the PC value of one parameter.
func (r *WorkOrderRepository) SetParameterPCValue(ctx context.Context, woID int64, p *workorder.Parameter) error {
	query := `
		UPDATE wo_parameter SET
			wop_value_pc_num = $3::DECIMAL, wop_value_pc_text = $4, wop_value_pc_flag = $5::BOOLEAN
		WHERE wop_wo_id = $1 AND wop_param_id = $2::UUID`
	res, err := r.db.ExecContext(ctx, query,
		woID, p.ParamID, floatPtrArg(p.ValuePCNum), stringPtrToNull(p.ValuePCText), boolPtrToNull(p.ValuePCFlag),
	)
	if err != nil {
		return fmt.Errorf("failed to set wo parameter pc value: %w", err)
	}
	return checkAffected(res, workorder.ErrNotFound)
}

// ListParameters lists a WO's planned parameters.
func (r *WorkOrderRepository) ListParameters(ctx context.Context, woID int64) ([]*workorder.Parameter, error) {
	query := `SELECT wop_id, wop_wo_id, wop_param_id,
		wop_value_ppc_num, wop_value_ppc_text, wop_value_ppc_flag,
		wop_value_pc_num, wop_value_pc_text, wop_value_pc_flag, wop_is_dual
		FROM wo_parameter WHERE wop_wo_id = $1 ORDER BY wop_id`
	rows, err := r.db.QueryContext(ctx, query, woID)
	if err != nil {
		return nil, fmt.Errorf("failed to list wo parameters: %w", err)
	}
	defer closeRows(rows)

	var result []*workorder.Parameter
	for rows.Next() {
		var p workorder.Parameter
		var ppcNum, pcNum sql.NullFloat64
		var ppcText, pcText sql.NullString
		var ppcFlag, pcFlag sql.NullBool
		if err := rows.Scan(
			&p.ID, &p.WOID, &p.ParamID,
			&ppcNum, &ppcText, &ppcFlag, &pcNum, &pcText, &pcFlag, &p.IsDual,
		); err != nil {
			return nil, fmt.Errorf("failed to scan wo parameter: %w", err)
		}
		p.ValuePPCNum = nullFloatPtr(ppcNum)
		p.ValuePPCText = nullStringPtr(ppcText)
		p.ValuePPCFlag = nullBoolPtr(ppcFlag)
		p.ValuePCNum = nullFloatPtr(pcNum)
		p.ValuePCText = nullStringPtr(pcText)
		p.ValuePCFlag = nullBoolPtr(pcFlag)
		result = append(result, &p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating wo parameters: %w", err)
	}
	return result, nil
}

// ── Execution (1:N per date+shift+param) ─────────────────────────────────────

// UpsertExecution upserts one actual parameter value.
func (r *WorkOrderRepository) UpsertExecution(ctx context.Context, exec *workorder.Execution) error {
	query := `
		INSERT INTO wo_execution (
			woe_wo_id, woe_date, woe_shift, woe_param_id, woe_value_num, woe_value_text, woe_value_flag, woe_input_by, woe_input_at
		) VALUES ($1, $2, $3, $4::UUID, $5::DECIMAL, $6, $7::BOOLEAN, $8, $9)
		ON CONFLICT (woe_wo_id, woe_date, woe_shift, woe_param_id) DO UPDATE SET
			woe_value_num = $5::DECIMAL, woe_value_text = $6, woe_value_flag = $7::BOOLEAN,
			woe_input_by = $8, woe_input_at = $9
		RETURNING woe_id`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		exec.WOID, exec.Date, exec.Shift, exec.ParamID, floatPtrArg(exec.ValueNum),
		stringPtrToNull(exec.ValueText), boolPtrToNull(exec.ValueFlag), exec.InputBy, exec.InputAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to upsert wo execution: %w", err)
	}
	exec.ID = id
	return nil
}

// ListExecutions lists a WO's actual parameter values.
func (r *WorkOrderRepository) ListExecutions(ctx context.Context, woID int64) ([]*workorder.Execution, error) {
	query := `SELECT woe_id, woe_wo_id, woe_date, woe_shift, woe_param_id,
		woe_value_num, woe_value_text, woe_value_flag, woe_input_by, woe_input_at
		FROM wo_execution WHERE woe_wo_id = $1 ORDER BY woe_date, woe_shift, woe_id`
	rows, err := r.db.QueryContext(ctx, query, woID)
	if err != nil {
		return nil, fmt.Errorf("failed to list wo executions: %w", err)
	}
	defer closeRows(rows)

	var result []*workorder.Execution
	for rows.Next() {
		var e workorder.Execution
		var num sql.NullFloat64
		var text sql.NullString
		var flag sql.NullBool
		if err := rows.Scan(
			&e.ID, &e.WOID, &e.Date, &e.Shift, &e.ParamID, &num, &text, &flag, &e.InputBy, &e.InputAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan wo execution: %w", err)
		}
		e.ValueNum = nullFloatPtr(num)
		e.ValueText = nullStringPtr(text)
		e.ValueFlag = nullBoolPtr(flag)
		result = append(result, &e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating wo executions: %w", err)
	}
	return result, nil
}

// ── RM allocation (1:N from route) ───────────────────────────────────────────

// ReplaceRmAllocations replaces all RM allocation lines for a WO in one tx.
func (r *WorkOrderRepository) ReplaceRmAllocations(ctx context.Context, woID int64, allocs []*workorder.RmAllocation) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM wo_rm_allocation WHERE wra_wo_id = $1`, woID); err != nil {
			return fmt.Errorf("failed to clear wo rm allocations: %w", err)
		}
		for _, a := range allocs {
			if err := insertRmAllocationTx(ctx, tx, woID, a); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertRmAllocationTx(ctx context.Context, tx *sql.Tx, woID int64, a *workorder.RmAllocation) error {
	query := `
		INSERT INTO wo_rm_allocation (
			wra_wo_id, wra_crm_rm_id, wra_rm_type, wra_lot_no, wra_rm_source, wra_fresh_box, wra_shade_code, wra_qty_allocated, wra_notes
		) VALUES ($1, $2, NULLIF($3, ''), $4, NULLIF($5, ''), NULLIF($6, ''), NULLIF($7, ''), $8, NULLIF($9, ''))
		RETURNING wra_id`
	var id int64
	err := tx.QueryRowContext(ctx, query,
		woID, a.CrmRmID, a.RmType, a.LotNo, a.RmSource, a.FreshBox, a.ShadeCode, a.QtyAllocated, a.Notes,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to insert wo rm allocation: %w", err)
	}
	a.ID = id
	return nil
}

// ListRmAllocations lists a WO's RM allocations.
func (r *WorkOrderRepository) ListRmAllocations(ctx context.Context, woID int64) ([]*workorder.RmAllocation, error) {
	query := `SELECT wra_id, wra_wo_id, wra_crm_rm_id, wra_rm_type, wra_lot_no, wra_rm_source, wra_fresh_box, wra_shade_code, wra_qty_allocated, wra_notes
		FROM wo_rm_allocation WHERE wra_wo_id = $1 ORDER BY wra_id`
	rows, err := r.db.QueryContext(ctx, query, woID)
	if err != nil {
		return nil, fmt.Errorf("failed to list wo rm allocations: %w", err)
	}
	defer closeRows(rows)

	var result []*workorder.RmAllocation
	for rows.Next() {
		var a workorder.RmAllocation
		var rmType, rmSource, freshBox, shade, notes sql.NullString
		if err := rows.Scan(&a.ID, &a.WOID, &a.CrmRmID, &rmType, &a.LotNo, &rmSource, &freshBox, &shade, &a.QtyAllocated, &notes); err != nil {
			return nil, fmt.Errorf("failed to scan wo rm allocation: %w", err)
		}
		a.RmType = nullString(rmType)
		a.RmSource = nullString(rmSource)
		a.FreshBox = nullString(freshBox)
		a.ShadeCode = nullString(shade)
		a.Notes = nullString(notes)
		result = append(result, &a)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating wo rm allocations: %w", err)
	}
	return result, nil
}

// ── Production actual (1:N, two-axis) ────────────────────────────────────────

const woProductionActualColumns = `wpa_id, wpa_wo_id, wpa_date, wpa_shift, wpa_area,
	COALESCE(wpa_total_bobbins,0), COALESCE(wpa_full_bobbins,0), COALESCE(wpa_unfull_bobbins,0),
	COALESCE(wpa_normal_bobs,0), COALESCE(wpa_downgrade_bobs,0), COALESCE(wpa_pending_bobs,0), COALESCE(wpa_pack_cek_bobs,0),
	COALESCE(wpa_gross_bobbins,0), COALESCE(wpa_transferred_bobs,0), COALESCE(wpa_cut_bobbins,0), COALESCE(wpa_not_transfer,0),
	COALESCE(wpa_normal_bobs_spg,0), COALESCE(wpa_downgrade_bobs_spg,0), COALESCE(wpa_not_checked_bobs,0), COALESCE(wpa_weight_per_bob,0),
	COALESCE(wpa_qty_bobbin,0), COALESCE(wpa_qty_actual,0), COALESCE(wpa_qty_source,'BOBBIN'), COALESCE(wpa_manual_reason,''),
	COALESCE(wpa_qty_doffed_kg,0), COALESCE(wpa_qty_transferred_kg,0),
	COALESCE(wpa_breaks_shift1,0), COALESCE(wpa_breaks_shift2,0), COALESCE(wpa_breaks_shift3,0),
	COALESCE(wpa_doff_full_count,0), COALESCE(wpa_doff_manual_count,0), COALESCE(wpa_co_failure_count,0),
	COALESCE(wpa_sync_status,''), wpa_synced_at, wpa_last_edited_by, wpa_last_edited_at`

// GetProductionActuals lists production-actual rows for a WO (optionally scoped).
func (r *WorkOrderRepository) GetProductionActuals(ctx context.Context, woID int64, date *time.Time, shift string) ([]*workorder.ProductionActual, error) {
	query := `SELECT ` + woProductionActualColumns + ` FROM wo_production_actual WHERE wpa_wo_id = $1`
	args := []interface{}{woID}
	idx := 2
	if date != nil {
		query += fmt.Sprintf(` AND wpa_date = $%d`, idx)
		args = append(args, *date)
		idx++
	}
	if shift != "" {
		query += fmt.Sprintf(` AND wpa_shift = $%d`, idx)
		args = append(args, shift)
	}
	query += ` ORDER BY wpa_date, wpa_shift`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list production actuals: %w", err)
	}
	defer closeRows(rows)

	var result []*workorder.ProductionActual
	for rows.Next() {
		pa, scanErr := scanProductionActual(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, pa)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating production actuals: %w", err)
	}
	return result, nil
}

// AdjustActual sets qty_actual (ADJUSTED) for a (wo,date,shift) row and appends a
// wo_actual_log entry in one transaction, then returns the updated row.
func (r *WorkOrderRepository) AdjustActual(ctx context.Context, woID int64, date time.Time, shift string, qtyActual float64, reason string, editedBy int64) (*workorder.ProductionActual, error) {
	var updated *workorder.ProductionActual
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		var beforeQty sql.NullFloat64
		var beforeSrc sql.NullString
		err := tx.QueryRowContext(ctx,
			`SELECT wpa_qty_actual, wpa_qty_source FROM wo_production_actual
			 WHERE wpa_wo_id = $1 AND wpa_date = $2 AND wpa_shift = $3 FOR UPDATE`,
			woID, date, shift).Scan(&beforeQty, &beforeSrc)
		if errors.Is(err, sql.ErrNoRows) {
			return workorder.ErrActualNotFound
		}
		if err != nil {
			return fmt.Errorf("failed to load production actual: %w", err)
		}

		now := time.Now()
		if _, err := tx.ExecContext(ctx,
			`UPDATE wo_production_actual SET
				wpa_qty_actual = $4, wpa_qty_source = 'ADJUSTED', wpa_manual_reason = $5,
				wpa_last_edited_by = $6, wpa_last_edited_at = $7
			 WHERE wpa_wo_id = $1 AND wpa_date = $2 AND wpa_shift = $3`,
			woID, date, shift, qtyActual, reason, editedBy, now); err != nil {
			return fmt.Errorf("failed to adjust production actual: %w", err)
		}

		if _, err := tx.ExecContext(ctx,
			`INSERT INTO wo_actual_log (wal_wo_id, wal_qty_before, wal_qty_after, wal_source_before, wal_source_after, wal_reason, wal_edited_by, wal_edited_at)
			 VALUES ($1, $2::DECIMAL, $3, $4, 'ADJUSTED', $5, $6, $7)`,
			woID, nullFloatArg(beforeQty), qtyActual, nullStringArg(beforeSrc), reason, editedBy, now); err != nil {
			return fmt.Errorf("failed to append wo actual log: %w", err)
		}

		row, err := scanSingleProductionActual(ctx, tx, woID, date, shift)
		if err != nil {
			return err
		}
		updated = row
		return nil
	})
	if err != nil {
		return nil, err
	}
	return updated, nil
}

func scanSingleProductionActual(ctx context.Context, tx *sql.Tx, woID int64, date time.Time, shift string) (*workorder.ProductionActual, error) {
	query := `SELECT ` + woProductionActualColumns + ` FROM wo_production_actual
		WHERE wpa_wo_id = $1 AND wpa_date = $2 AND wpa_shift = $3`
	rows, err := tx.QueryContext(ctx, query, woID, date, shift)
	if err != nil {
		return nil, fmt.Errorf("failed to reload production actual: %w", err)
	}
	defer closeRows(rows)
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, fmt.Errorf("error reloading production actual: %w", err)
		}
		return nil, workorder.ErrActualNotFound
	}
	return scanProductionActual(rows)
}

func scanProductionActual(rows *sql.Rows) (*workorder.ProductionActual, error) {
	var pa workorder.ProductionActual
	var syncedAt, lastEditedAt sql.NullTime
	var lastEditedBy sql.NullInt64
	if err := rows.Scan(
		&pa.ID, &pa.WOID, &pa.Date, &pa.Shift, &pa.Area,
		&pa.TotalBobbins, &pa.FullBobbins, &pa.UnfullBobbins,
		&pa.NormalBobs, &pa.DowngradeBobs, &pa.PendingBobs, &pa.PackCekBobs,
		&pa.GrossBobbins, &pa.TransferredBobs, &pa.CutBobbins, &pa.NotTransfer,
		&pa.NormalBobsSpg, &pa.DowngradeBobsSpg, &pa.NotCheckedBobs, &pa.WeightPerBob,
		&pa.QtyBobbin, &pa.QtyActual, &pa.QtySource, &pa.AdjustReason,
		&pa.QtyDoffedKg, &pa.QtyTransferredKg,
		&pa.BreaksShift1, &pa.BreaksShift2, &pa.BreaksShift3,
		&pa.DoffFullCount, &pa.DoffManualCount, &pa.CoFailureCount,
		&pa.SyncStatus, &syncedAt, &lastEditedBy, &lastEditedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to scan production actual: %w", err)
	}
	pa.SyncedAt = nullTimePtr(syncedAt)
	pa.LastEditedBy = nullInt64Ptr(lastEditedBy)
	pa.LastEditedAt = nullTimePtr(lastEditedAt)
	return &pa, nil
}

// nullFloatArg converts a scanned sql.NullFloat64 to a driver arg (NULL when invalid).
func nullFloatArg(v sql.NullFloat64) interface{} {
	if !v.Valid {
		return nil
	}
	return v.Float64
}

// nullStringArg converts a scanned sql.NullString to a driver arg (NULL when invalid).
func nullStringArg(v sql.NullString) interface{} {
	if !v.Valid {
		return nil
	}
	return v.String
}
