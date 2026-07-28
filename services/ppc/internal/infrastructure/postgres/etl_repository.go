package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ETLRepository provides PostgreSQL reads/writes for the Oracle ETL and the
// suggest chain: watermarks, WO matching, two-axis production-actual upserts,
// sales-order staging full-replace, and suggest reads.
type ETLRepository struct {
	db *DB
}

// NewETLRepository creates a new ETLRepository.
func NewETLRepository(db *DB) *ETLRepository {
	return &ETLRepository{db: db}
}

// ProductionActualUpsert is the two-axis (v1.2) upsert payload for one
// (wo, date, shift) production-actual row. QtyBobbin is the immutable bobbin
// baseline; it also seeds wpa_qty_actual unless the existing row is ADJUSTED.
type ProductionActualUpsert struct {
	WOID          int64
	Date          time.Time
	Shift         string
	Area          string
	TotalBobbins  int
	FullBobbins   int
	UnfullBobbins int
	NormalBobs    int
	DowngradeBobs int
	PendingBobs   int
	PackCekBobs   int
	QtyBobbin     float64
}

// SpgProductionActualUpsert is the SPG two-axis upsert payload for one
// (wo, date, shift) production-actual row. SPG uses a dual quantity basis:
// QtyDoffedKg (GROSS × weight) is the efficiency basis and seeds the immutable
// wpa_qty_bobbin; QtyTransferredKg (TRANSFERRED × weight) is the fulfillment
// basis. An operator-ADJUSTED wpa_qty_actual is preserved.
type SpgProductionActualUpsert struct {
	WOID             int64
	Date             time.Time
	Shift            string
	Area             string
	GrossBobbins     int
	TransferredBobs  int
	CutBobbins       int
	NotTransfer      int
	NormalBobsSpg    int
	DowngradeBobsSpg int
	NotCheckedBobs   int
	WeightPerBob     float64
	QtyDoffedKg      float64
	QtyTransferredKg float64
}

// SoStagingRow is one sales-order staging row for the full-replace ETL. Zero
// ContractDate / Deadline are written as SQL NULL.
type SoStagingRow struct {
	ContractNo    string
	ContractDate  time.Time
	ContractSysID int64
	CustomerCode  string
	CustomerName  string
	ItemCode      string
	GradeCode     string
	ShadeCode     string
	QtyOrdered    float64
	QtyDelivered  float64
	QtyRemaining  float64
	Deadline      time.Time
	MergeNo       string
	Term          string
	Rate          float64
	Currency      string
	BlockedStatus string
	OutstandingAR float64
}

// GetWatermark returns the last-run timestamp for an ETL source table. A missing
// row yields the Unix epoch (far past) so the first run pulls all history.
func (r *ETLRepository) GetWatermark(ctx context.Context, table string) (time.Time, error) {
	const query = `SELECT ewm_last_run FROM etl_watermark WHERE ewm_table_name = $1`
	var ts time.Time
	err := r.db.QueryRowContext(ctx, query, table).Scan(&ts)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Unix(0, 0).UTC(), nil
	}
	if err != nil {
		return time.Time{}, fmt.Errorf("get watermark %q: %w", table, err)
	}
	return ts, nil
}

// AdvanceWatermark upserts the last-run timestamp for an ETL source table.
func (r *ETLRepository) AdvanceWatermark(ctx context.Context, table string, ts time.Time) error {
	const query = `
		INSERT INTO etl_watermark (ewm_table_name, ewm_last_run, ewm_updated_at)
		VALUES ($1, $2, NOW())
		ON CONFLICT (ewm_table_name)
		DO UPDATE SET ewm_last_run = $2, ewm_updated_at = NOW()`
	if _, err := r.db.ExecContext(ctx, query, table, ts); err != nil {
		return fmt.Errorf("advance watermark %q: %w", table, err)
	}
	return nil
}

// MatchWO resolves a (lotNo, machineNo) pair to a work order. The lot number is
// UNIQUE and drives bobbin matching; the machine number is cross-checked via
// join. No match returns ok=false (not an error) so the ETL logs SYNC_FAILED.
func (r *ETLRepository) MatchWO(ctx context.Context, lotNo, machineNo string) (woID int64, area string, ok bool, err error) {
	const query = `
		SELECT wo.wo_id, wo.wo_area
		FROM work_order wo
		JOIN machine m ON m.machine_id = wo.wo_machine_id
		WHERE wo.wo_lot_no = $1 AND m.machine_no = $2`
	err = r.db.QueryRowContext(ctx, query, lotNo, machineNo).Scan(&woID, &area)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("match WO (lot=%s machine=%s): %w", lotNo, machineNo, err)
	}
	return woID, area, true, nil
}

// LotStdWeights reads the standard full/unfull bobbin weights for a lot. A
// missing lot returns ok=false (callers treat qty_bobbin as 0).
func (r *ETLRepository) LotStdWeights(ctx context.Context, lotNo string) (full, unfull float64, ok bool, err error) {
	const query = `SELECT lm_std_weight_full, lm_std_weight_unfull FROM lot_master WHERE lm_lot_no = $1`
	err = r.db.QueryRowContext(ctx, query, lotNo).Scan(&full, &unfull)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("lot std weights (lot=%s): %w", lotNo, err)
	}
	return full, unfull, true, nil
}

// LotStdWeightsByWO reads the standard bobbin weights for the lot attached to a
// WO (suggest P2/P4). A missing WO or lot returns ok=false.
func (r *ETLRepository) LotStdWeightsByWO(ctx context.Context, woID int64) (full, unfull float64, ok bool, err error) {
	const query = `
		SELECT lm.lm_std_weight_full, lm.lm_std_weight_unfull
		FROM work_order wo
		JOIN lot_master lm ON lm.lm_lot_no = wo.wo_lot_no
		WHERE wo.wo_id = $1`
	err = r.db.QueryRowContext(ctx, query, woID).Scan(&full, &unfull)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, fmt.Errorf("lot std weights by wo (wo=%d): %w", woID, err)
	}
	return full, unfull, true, nil
}

// UpsertProductionActual writes the two-axis production-actual row. It always
// sets the bobbin columns and wpa_qty_bobbin. wpa_qty_actual is seeded from the
// bobbin baseline unless the existing row's source is ADJUSTED, in which case the
// operator-adjusted qty_actual and source are preserved by the SQL CASE.
func (r *ETLRepository) UpsertProductionActual(ctx context.Context, in ProductionActualUpsert) error {
	const query = `
		INSERT INTO wo_production_actual (
			wpa_wo_id, wpa_date, wpa_shift, wpa_area,
			wpa_total_bobbins, wpa_full_bobbins, wpa_unfull_bobbins, wpa_normal_bobs,
			wpa_downgrade_bobs, wpa_pending_bobs, wpa_pack_cek_bobs,
			wpa_qty_bobbin, wpa_qty_actual, wpa_qty_source, wpa_sync_status, wpa_synced_at)
		VALUES ($1, $2::DATE, $3, $4,
			$5::INT, $6::INT, $7::INT, $8::INT, $9::INT, $10::INT, $11::INT,
			$12::DECIMAL, $12::DECIMAL, 'BOBBIN', 'OK', NOW())
		ON CONFLICT (wpa_wo_id, wpa_date, wpa_shift) DO UPDATE SET
			wpa_total_bobbins = EXCLUDED.wpa_total_bobbins,
			wpa_full_bobbins = EXCLUDED.wpa_full_bobbins,
			wpa_unfull_bobbins = EXCLUDED.wpa_unfull_bobbins,
			wpa_normal_bobs = EXCLUDED.wpa_normal_bobs,
			wpa_downgrade_bobs = EXCLUDED.wpa_downgrade_bobs,
			wpa_pending_bobs = EXCLUDED.wpa_pending_bobs,
			wpa_pack_cek_bobs = EXCLUDED.wpa_pack_cek_bobs,
			wpa_qty_bobbin = EXCLUDED.wpa_qty_bobbin,
			wpa_area = EXCLUDED.wpa_area,
			wpa_sync_status = 'OK',
			wpa_synced_at = NOW(),
			wpa_qty_actual = CASE
				WHEN wo_production_actual.wpa_qty_source = 'ADJUSTED'
				THEN wo_production_actual.wpa_qty_actual
				ELSE EXCLUDED.wpa_qty_bobbin END,
			wpa_qty_source = CASE
				WHEN wo_production_actual.wpa_qty_source = 'ADJUSTED'
				THEN 'ADJUSTED'
				ELSE 'BOBBIN' END`
	_, err := r.db.ExecContext(ctx, query,
		in.WOID, in.Date, in.Shift, in.Area,
		in.TotalBobbins, in.FullBobbins, in.UnfullBobbins, in.NormalBobs,
		in.DowngradeBobs, in.PendingBobs, in.PackCekBobs, in.QtyBobbin,
	)
	if err != nil {
		return fmt.Errorf("upsert production actual (wo=%d): %w", in.WOID, err)
	}
	return nil
}

// UpsertSpgProductionActual writes the SPG two-axis production-actual row. It
// always sets the SPG bobbin columns, the dual doffed/transferred quantities,
// and seeds wpa_qty_bobbin from the doffed (efficiency) basis. wpa_qty_actual is
// seeded from the doffed basis unless the existing row's source is ADJUSTED, in
// which case the operator-adjusted qty_actual and source are preserved.
func (r *ETLRepository) UpsertSpgProductionActual(ctx context.Context, in SpgProductionActualUpsert) error {
	const query = `
		INSERT INTO wo_production_actual (
			wpa_wo_id, wpa_date, wpa_shift, wpa_area,
			wpa_gross_bobbins, wpa_transferred_bobs, wpa_cut_bobbins, wpa_not_transfer,
			wpa_normal_bobs_spg, wpa_downgrade_bobs_spg, wpa_not_checked_bobs, wpa_weight_per_bob,
			wpa_qty_doffed_kg, wpa_qty_transferred_kg,
			wpa_qty_bobbin, wpa_qty_actual, wpa_qty_source, wpa_sync_status, wpa_synced_at)
		VALUES ($1, $2::DATE, $3, $4,
			$5::INT, $6::INT, $7::INT, $8::INT,
			$9::INT, $10::INT, $11::INT, $12::DECIMAL,
			$13::DECIMAL, $14::DECIMAL,
			$13::DECIMAL, $13::DECIMAL, 'BOBBIN', 'OK', NOW())
		ON CONFLICT (wpa_wo_id, wpa_date, wpa_shift) DO UPDATE SET
			wpa_gross_bobbins = EXCLUDED.wpa_gross_bobbins,
			wpa_transferred_bobs = EXCLUDED.wpa_transferred_bobs,
			wpa_cut_bobbins = EXCLUDED.wpa_cut_bobbins,
			wpa_not_transfer = EXCLUDED.wpa_not_transfer,
			wpa_normal_bobs_spg = EXCLUDED.wpa_normal_bobs_spg,
			wpa_downgrade_bobs_spg = EXCLUDED.wpa_downgrade_bobs_spg,
			wpa_not_checked_bobs = EXCLUDED.wpa_not_checked_bobs,
			wpa_weight_per_bob = EXCLUDED.wpa_weight_per_bob,
			wpa_qty_doffed_kg = EXCLUDED.wpa_qty_doffed_kg,
			wpa_qty_transferred_kg = EXCLUDED.wpa_qty_transferred_kg,
			wpa_qty_bobbin = EXCLUDED.wpa_qty_bobbin,
			wpa_area = EXCLUDED.wpa_area,
			wpa_sync_status = 'OK',
			wpa_synced_at = NOW(),
			wpa_qty_actual = CASE
				WHEN wo_production_actual.wpa_qty_source = 'ADJUSTED'
				THEN wo_production_actual.wpa_qty_actual
				ELSE EXCLUDED.wpa_qty_bobbin END,
			wpa_qty_source = CASE
				WHEN wo_production_actual.wpa_qty_source = 'ADJUSTED'
				THEN 'ADJUSTED'
				ELSE 'BOBBIN' END`
	_, err := r.db.ExecContext(ctx, query,
		in.WOID, in.Date, in.Shift, in.Area,
		in.GrossBobbins, in.TransferredBobs, in.CutBobbins, in.NotTransfer,
		in.NormalBobsSpg, in.DowngradeBobsSpg, in.NotCheckedBobs, in.WeightPerBob,
		in.QtyDoffedKg, in.QtyTransferredKg,
	)
	if err != nil {
		return fmt.Errorf("upsert spg production actual (wo=%d): %w", in.WOID, err)
	}
	return nil
}

// MarkSyncFailed flags a matched-but-failed production-actual row as SYNC_FAILED.
// Unmatched Oracle rows have no target row and are only logged by the caller.
func (r *ETLRepository) MarkSyncFailed(ctx context.Context, woID int64, date time.Time, shift string) error {
	const query = `
		UPDATE wo_production_actual
		SET wpa_sync_status = 'SYNC_FAILED', wpa_synced_at = NOW()
		WHERE wpa_wo_id = $1 AND wpa_date = $2::DATE AND wpa_shift = $3`
	if _, err := r.db.ExecContext(ctx, query, woID, date, shift); err != nil {
		return fmt.Errorf("mark sync failed (wo=%d): %w", woID, err)
	}
	return nil
}

// soStagingKey builds the natural key for preserving pull state across replaces.
func soStagingKey(contractSysID int64, itemCode, gradeCode, shadeCode string) string {
	return fmt.Sprintf("%d|%s|%s|%s", contractSysID, itemCode, gradeCode, shadeCode)
}

// ReplaceSalesOrderStaging replaces the unpulled sales-order backlog in a single
// transaction. Rows already pulled into production_demand (sos_pulled_to_demand_id
// set) are never deleted or reinserted -- production_demand.pd_sos_ref has an FK to
// sos_id, so deleting a pulled row would violate that constraint. Pulled rows still
// present in the new Oracle pull are refreshed in place (same sos_id); rows no
// longer present are left untouched, since their sos_id is still referenced.
func (r *ETLRepository) ReplaceSalesOrderStaging(ctx context.Context, rows []SoStagingRow) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		pulled, err := loadPulledMap(ctx, tx)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `DELETE FROM sales_order_staging WHERE sos_pulled_to_demand_id IS NULL`); err != nil {
			return fmt.Errorf("clear sales_order_staging: %w", err)
		}
		for i := range rows {
			key := soStagingKey(rows[i].ContractSysID, rows[i].ItemCode, rows[i].GradeCode, rows[i].ShadeCode)
			if _, isPulled := pulled[key]; isPulled {
				if err := updateSoStagingRow(ctx, tx, rows[i]); err != nil {
					return err
				}
				continue
			}
			if err := insertSoStagingRow(ctx, tx, rows[i]); err != nil {
				return err
			}
		}
		return nil
	})
}

// loadPulledMap captures the natural-key -> pulled-demand-id map for rows already
// pulled into demand, so the pull state survives the full replace.
func loadPulledMap(ctx context.Context, tx *sql.Tx) (map[string]int64, error) {
	const query = `
		SELECT sos_contract_sys_id, sos_item_code, sos_grade_code, sos_shade_code, sos_pulled_to_demand_id
		FROM sales_order_staging
		WHERE sos_pulled_to_demand_id IS NOT NULL`
	rows, err := tx.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("load pulled map: %w", err)
	}
	defer closeRows(rows)

	pulled := make(map[string]int64)
	for rows.Next() {
		var (
			contractSysID              sql.NullInt64
			itemCode, gradeCode, shade sql.NullString
			demandID                   sql.NullInt64
		)
		if err := rows.Scan(&contractSysID, &itemCode, &gradeCode, &shade, &demandID); err != nil {
			return nil, fmt.Errorf("scan pulled map row: %w", err)
		}
		if !demandID.Valid {
			continue
		}
		key := soStagingKey(contractSysID.Int64, nullString(itemCode), nullString(gradeCode), nullString(shade))
		pulled[key] = demandID.Int64
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate pulled map rows: %w", err)
	}
	return pulled, nil
}

// insertSoStagingRow inserts one new (unpulled) staging row. Explicit casts avoid
// SQLSTATE 42P08 on NULLs.
func insertSoStagingRow(ctx context.Context, tx *sql.Tx, row SoStagingRow) error {
	const query = `
		INSERT INTO sales_order_staging (
			sos_contract_no, sos_contract_date, sos_contract_sys_id,
			sos_customer_code, sos_customer_name, sos_item_code,
			sos_grade_code, sos_shade_code,
			sos_qty_ordered, sos_qty_delivered, sos_qty_remaining,
			sos_deadline, sos_merge_no, sos_term, sos_rate,
			sos_currency, sos_blocked_status, sos_outstanding_ar,
			sos_etl_synced_at)
		VALUES (
			$1, $2::DATE, $3::BIGINT,
			$4, $5, $6,
			$7, $8,
			$9::DECIMAL, $10::DECIMAL, $11::DECIMAL,
			$12::DATE, $13, $14, $15::DECIMAL,
			$16, $17, $18::DECIMAL,
			NOW())`
	_, err := tx.ExecContext(ctx, query,
		row.ContractNo, nullableDate(row.ContractDate), row.ContractSysID,
		row.CustomerCode, row.CustomerName, row.ItemCode,
		row.GradeCode, row.ShadeCode,
		row.QtyOrdered, row.QtyDelivered, row.QtyRemaining,
		nullableDate(row.Deadline), row.MergeNo, row.Term, row.Rate,
		row.Currency, row.BlockedStatus, row.OutstandingAR,
	)
	if err != nil {
		return fmt.Errorf("insert sales_order_staging row: %w", err)
	}
	return nil
}

// updateSoStagingRow refreshes an already-pulled staging row in place, keyed by
// its natural key. sos_id and sos_pulled_to_demand_id are left untouched so the
// production_demand FK stays valid.
func updateSoStagingRow(ctx context.Context, tx *sql.Tx, row SoStagingRow) error {
	const query = `
		UPDATE sales_order_staging SET
			sos_contract_no = $1, sos_contract_date = $2::DATE,
			sos_customer_code = $3, sos_customer_name = $4,
			sos_qty_ordered = $5::DECIMAL, sos_qty_delivered = $6::DECIMAL, sos_qty_remaining = $7::DECIMAL,
			sos_deadline = $8::DATE, sos_merge_no = $9, sos_term = $10, sos_rate = $11::DECIMAL,
			sos_currency = $12, sos_blocked_status = $13, sos_outstanding_ar = $14::DECIMAL,
			sos_etl_synced_at = NOW()
		WHERE sos_contract_sys_id = $15 AND sos_item_code = $16 AND sos_grade_code = $17 AND sos_shade_code = $18`
	_, err := tx.ExecContext(ctx, query,
		row.ContractNo, nullableDate(row.ContractDate),
		row.CustomerCode, row.CustomerName,
		row.QtyOrdered, row.QtyDelivered, row.QtyRemaining,
		nullableDate(row.Deadline), row.MergeNo, row.Term, row.Rate,
		row.Currency, row.BlockedStatus, row.OutstandingAR,
		row.ContractSysID, row.ItemCode, row.GradeCode, row.ShadeCode,
	)
	if err != nil {
		return fmt.Errorf("update sales_order_staging row: %w", err)
	}
	return nil
}

// nullableDate returns a driver argument that is SQL NULL for a zero time.
func nullableDate(t time.Time) interface{} {
	if t.IsZero() {
		return nil
	}
	return t
}

// GradeActualUpsert is the packing grade-actual upsert payload for one
// (wo, lot, grade) row, sourced from Oracle PPC_GRADE_ACTUAL.
type GradeActualUpsert struct {
	WOID            int64
	LotNo           string
	Grade           string
	Dept            string
	TotalQtyKg      float64
	BobbinCount     int
	LastPackingDate time.Time
}

// MatchWOByLot resolves an original lot number to a work order. wo_lot_no is
// UNIQUE (PPC-generated) and drives grade-actual matching. No match returns
// ok=false (not an error) so the ETL logs SYNC_FAILED.
func (r *ETLRepository) MatchWOByLot(ctx context.Context, lotNo string) (woID int64, ok bool, err error) {
	const query = `SELECT wo_id FROM work_order WHERE wo_lot_no = $1`
	err = r.db.QueryRowContext(ctx, query, lotNo).Scan(&woID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("match WO by lot (lot=%s): %w", lotNo, err)
	}
	return woID, true, nil
}

// UpsertGradeActual writes one packing grade-actual row, keyed by
// (wga_wo_id, wga_lot_no, wga_grade). Re-runs overwrite the quantities and
// refresh wga_synced_at. A zero LastPackingDate is written as SQL NULL.
func (r *ETLRepository) UpsertGradeActual(ctx context.Context, in GradeActualUpsert) error {
	const query = `
		INSERT INTO wo_grade_actual (
			wga_wo_id, wga_lot_no, wga_grade, wga_dept,
			wga_total_qty_kg, wga_bobbin_count, wga_last_packing_date, wga_synced_at)
		VALUES ($1, $2, $3, $4, $5::DECIMAL, $6::INT, $7::DATE, NOW())
		ON CONFLICT (wga_wo_id, wga_lot_no, wga_grade) DO UPDATE SET
			wga_dept = EXCLUDED.wga_dept,
			wga_total_qty_kg = EXCLUDED.wga_total_qty_kg,
			wga_bobbin_count = EXCLUDED.wga_bobbin_count,
			wga_last_packing_date = EXCLUDED.wga_last_packing_date,
			wga_synced_at = NOW()`
	_, err := r.db.ExecContext(ctx, query,
		in.WOID, in.LotNo, in.Grade, nullableString(&in.Dept),
		in.TotalQtyKg, in.BobbinCount, nullableDate(in.LastPackingDate),
	)
	if err != nil {
		return fmt.Errorf("upsert grade actual (wo=%d lot=%s grade=%s): %w", in.WOID, in.LotNo, in.Grade, err)
	}
	return nil
}

// GradeActualRow is a read projection of one wo_grade_actual row for listing.
type GradeActualRow struct {
	ID              int64
	WOID            int64
	LotNo           string
	Grade           string
	Dept            string
	TotalQtyKg      float64
	BobbinCount     int32
	LastPackingDate *time.Time
	SyncedAt        time.Time
}

// GradeActualFilter selects and paginates wo_grade_actual rows.
type GradeActualFilter struct {
	Page      int32
	PageSize  int32
	WOID      *int64
	Grade     string
	Dept      string
	SortBy    string
	SortOrder string
}

// ListGradeActuals returns wo_grade_actual rows matching the filter plus the
// total count.
func (r *ETLRepository) ListGradeActuals(ctx context.Context, filter GradeActualFilter) ([]GradeActualRow, int64, error) {
	where, args := buildGradeActualFilter(filter)
	countQuery := `SELECT COUNT(*) FROM wo_grade_actual` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count wo_grade_actual: %w", err)
	}

	orderBy := gradeActualOrderBy(filter.SortBy, filter.SortOrder)
	limit, offset := gradeActualPageBounds(filter.Page, filter.PageSize)
	listQuery := `
		SELECT wga_id, wga_wo_id, wga_lot_no, wga_grade, wga_dept,
			wga_total_qty_kg, wga_bobbin_count, wga_last_packing_date, wga_synced_at
		FROM wo_grade_actual` + where +
		fmt.Sprintf(` ORDER BY %s LIMIT %d OFFSET %d`, orderBy, limit, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list wo_grade_actual: %w", err)
	}
	defer closeRows(rows)

	var result []GradeActualRow
	for rows.Next() {
		var (
			row     GradeActualRow
			dept    sql.NullString
			qty     sql.NullFloat64
			bobbins sql.NullInt64
			packing sql.NullTime
		)
		if err := rows.Scan(&row.ID, &row.WOID, &row.LotNo, &row.Grade, &dept,
			&qty, &bobbins, &packing, &row.SyncedAt); err != nil {
			return nil, 0, fmt.Errorf("scan wo_grade_actual: %w", err)
		}
		row.Dept = nullString(dept)
		row.TotalQtyKg = qty.Float64
		row.BobbinCount = safeInt64ToInt32(bobbins.Int64)
		row.LastPackingDate = nullTimePtr(packing)
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate wo_grade_actual: %w", err)
	}
	return result, total, nil
}

// buildGradeActualFilter builds the WHERE clause and args for the list query.
func buildGradeActualFilter(filter GradeActualFilter) (string, []any) {
	var conds []string
	var args []any
	idx := 1
	if filter.WOID != nil {
		conds = append(conds, fmt.Sprintf("wga_wo_id = $%d", idx))
		args = append(args, *filter.WOID)
		idx++
	}
	if filter.Grade != "" {
		conds = append(conds, fmt.Sprintf("wga_grade = $%d", idx))
		args = append(args, filter.Grade)
		idx++
	}
	if filter.Dept != "" {
		conds = append(conds, fmt.Sprintf("wga_dept = $%d", idx))
		args = append(args, filter.Dept)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// gradeActualOrderBy maps a frontend sort field to a safe ORDER BY clause.
func gradeActualOrderBy(sortBy, sortOrder string) string {
	col := "wga_wo_id"
	switch sortBy {
	case "grade":
		col = "wga_grade"
	case "lot_no":
		col = "wga_lot_no"
	case "last_packing_date":
		col = "wga_last_packing_date"
	}
	dir := "ASC"
	if strings.EqualFold(sortOrder, "desc") {
		dir = "DESC"
	}
	return col + " " + dir + ", wga_id ASC"
}

// gradeActualPageBounds normalizes page/pageSize into a SQL LIMIT/OFFSET pair.
func gradeActualPageBounds(page, pageSize int32) (limit, offset int32) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	return pageSize, (page - 1) * pageSize
}

// GradeActualQtyKg sums packing-done grade actuals for a WO (suggest P1). has is
// true when at least one grade row exists.
func (r *ETLRepository) GradeActualQtyKg(ctx context.Context, woID int64) (qty float64, has bool, err error) {
	const query = `SELECT COALESCE(SUM(wga_total_qty_kg), 0), COUNT(*) FROM wo_grade_actual WHERE wga_wo_id = $1`
	var count int64
	if err = r.db.QueryRowContext(ctx, query, woID).Scan(&qty, &count); err != nil {
		return 0, false, fmt.Errorf("grade actual qty (wo=%d): %w", woID, err)
	}
	return qty, count > 0, nil
}

// ProductionActualBobbins reads the bobbin counts and per-bobbin weight for a
// (wo, date, shift) production-actual row (suggest P2-P4). has is false when no
// row exists.
func (r *ETLRepository) ProductionActualBobbins(ctx context.Context, woID int64, date time.Time, shift string) (
	normalBobs, fullBobbins, unfullBobbins, transferredBobs, totalBobbins int, weightPerBob float64, has bool, err error,
) {
	const query = `
		SELECT COALESCE(wpa_normal_bobs, 0), COALESCE(wpa_full_bobbins, 0),
			COALESCE(wpa_unfull_bobbins, 0), COALESCE(wpa_transferred_bobs, 0),
			COALESCE(wpa_total_bobbins, 0), COALESCE(wpa_weight_per_bob, 0)
		FROM wo_production_actual
		WHERE wpa_wo_id = $1 AND wpa_date = $2::DATE AND wpa_shift = $3`
	err = r.db.QueryRowContext(ctx, query, woID, date, shift).Scan(
		&normalBobs, &fullBobbins, &unfullBobbins, &transferredBobs, &totalBobbins, &weightPerBob,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, 0, 0, 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, 0, 0, 0, 0, false, fmt.Errorf("production actual bobbins (wo=%d): %w", woID, err)
	}
	return normalBobs, fullBobbins, unfullBobbins, transferredBobs, totalBobbins, weightPerBob, true, nil
}

// ProductionActualOverride reads the current qty_actual and source for a
// (wo, date, shift) row. When source is ADJUSTED the operator override wins over
// the suggest chain. has is false when no row exists.
func (r *ETLRepository) ProductionActualOverride(ctx context.Context, woID int64, date time.Time, shift string) (
	qtyActual float64, source string, has bool, err error,
) {
	const query = `
		SELECT COALESCE(wpa_qty_actual, 0), COALESCE(wpa_qty_source, '')
		FROM wo_production_actual
		WHERE wpa_wo_id = $1 AND wpa_date = $2::DATE AND wpa_shift = $3`
	err = r.db.QueryRowContext(ctx, query, woID, date, shift).Scan(&qtyActual, &source)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, fmt.Errorf("production actual override (wo=%d): %w", woID, err)
	}
	return qtyActual, source, true, nil
}
