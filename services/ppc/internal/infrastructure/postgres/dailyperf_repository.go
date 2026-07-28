package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/dailyperf"
)

// DailyPerfRepository implements all daily-performance persistence ports (shift
// logs, area logs, downtime, waste, notes, efficiency snapshots) plus the read
// ports used by the efficiency engine, over a single *DB.
type DailyPerfRepository struct {
	db *DB
}

// NewDailyPerfRepository builds a daily-performance repository over the DB.
func NewDailyPerfRepository(db *DB) *DailyPerfRepository {
	return &DailyPerfRepository{db: db}
}

// ── MachineShiftLog ──────────────────────────────────────────────────────────

// Upsert inserts or updates a machine shift log on its natural key and assigns
// the generated id + denormalized machine number.
func (r *DailyPerfRepository) Upsert(ctx context.Context, log *dailyperf.MachineShiftLog) error {
	query := `
		INSERT INTO machine_shift_log (
			msl_machine_id, msl_date, msl_shift, msl_positions_total, msl_positions_running,
			msl_running_minutes, msl_status, msl_input_by, msl_input_at, msl_updated_at
		) VALUES ($1, $2, $3, $4::INT, $5::DECIMAL, $6::INT, $7, $8, $9, $10)
		ON CONFLICT (msl_machine_id, msl_date, msl_shift) DO UPDATE SET
			msl_positions_total = $4::INT, msl_positions_running = $5::DECIMAL,
			msl_running_minutes = $6::INT, msl_status = $7, msl_updated_at = $10
		RETURNING msl_id`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		log.MachineID(), log.Date(), log.Shift(), log.PositionsTotal(), log.PositionsRunning(),
		log.RunningMinutes(), log.Status(), log.InputBy(), log.InputAt(), log.UpdatedAt(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to upsert machine shift log: %w", err)
	}
	log.SetID(id)
	return nil
}

const machineShiftLogColumns = `msl_id, msl_machine_id, COALESCE(m.machine_no, ''),
	msl_date, msl_shift, COALESCE(msl_positions_total, 0), COALESCE(msl_positions_running, 0),
	COALESCE(msl_running_minutes, 0), msl_status, msl_input_by, msl_input_at, msl_updated_at`

// GetByKey loads a machine shift log by its natural key.
func (r *DailyPerfRepository) GetByKey(ctx context.Context, machineID int64, date time.Time, shift string) (*dailyperf.MachineShiftLog, error) {
	query := `SELECT ` + machineShiftLogColumns + `
		FROM machine_shift_log
		LEFT JOIN machine m ON m.machine_id = msl_machine_id
		WHERE msl_machine_id = $1 AND msl_date = $2 AND msl_shift = $3`
	return r.scanShiftLogRow(r.db.QueryRowContext(ctx, query, machineID, date, shift))
}

// GetByID loads a machine shift log by id.
func (r *DailyPerfRepository) GetByID(ctx context.Context, id int64) (*dailyperf.MachineShiftLog, error) {
	query := `SELECT ` + machineShiftLogColumns + `
		FROM machine_shift_log
		LEFT JOIN machine m ON m.machine_id = msl_machine_id
		WHERE msl_id = $1`
	return r.scanShiftLogRow(r.db.QueryRowContext(ctx, query, id))
}

func (r *DailyPerfRepository) scanShiftLogRow(row *sql.Row) (*dailyperf.MachineShiftLog, error) {
	var p dailyperf.ReconstructShiftLogParams
	if err := row.Scan(
		&p.ID, &p.MachineID, &p.MachineNo, &p.Date, &p.Shift, &p.PositionsTotal,
		&p.PositionsRunning, &p.RunningMinutes, &p.Status, &p.InputBy, &p.InputAt, &p.UpdatedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dailyperf.ErrShiftLogNotFound
		}
		return nil, fmt.Errorf("failed to scan machine shift log: %w", err)
	}
	return dailyperf.ReconstructMachineShiftLog(p), nil
}

// shiftLogSortColumns maps the API sort keys to safe column expressions.
var shiftLogSortColumns = map[string]string{
	"date":       "msl_date",
	"machine_no": "m.machine_no",
	"shift":      "msl_shift",
}

// List returns machine shift logs matching the filter plus the total count.
func (r *DailyPerfRepository) List(ctx context.Context, filter dailyperf.ShiftLogFilter) ([]*dailyperf.MachineShiftLog, int64, error) {
	where, args := buildShiftLogFilter(filter)

	var total int64
	countQuery := `SELECT COUNT(*) FROM machine_shift_log
		LEFT JOIN machine m ON m.machine_id = msl_machine_id` + where
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count machine shift logs: %w", err)
	}

	orderCol, ok := shiftLogSortColumns[filter.SortBy]
	if !ok {
		orderCol = "msl_date"
	}
	direction := sortDirection(filter.SortOrder)
	if filter.SortBy == "" {
		direction = sortDESC
	}

	limit := filter.PageSize
	if limit <= 0 {
		limit = 20
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	query := `SELECT ` + machineShiftLogColumns + `
		FROM machine_shift_log
		LEFT JOIN machine m ON m.machine_id = msl_machine_id` + where +
		fmt.Sprintf(` ORDER BY %s %s, msl_id DESC LIMIT $%d OFFSET $%d`, orderCol, direction, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list machine shift logs: %w", err)
	}
	defer closeRows(rows)

	var result []*dailyperf.MachineShiftLog
	for rows.Next() {
		log, scanErr := scanShiftLogRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, log)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating machine shift logs: %w", err)
	}
	return result, total, nil
}

func buildShiftLogFilter(filter dailyperf.ShiftLogFilter) (string, []interface{}) {
	where := " WHERE 1=1"
	var args []interface{}
	idx := 1
	if filter.MachineID != nil {
		where += fmt.Sprintf(" AND msl_machine_id = $%d", idx)
		args = append(args, *filter.MachineID)
		idx++
	}
	if filter.Area != "" {
		where += fmt.Sprintf(" AND m.machine_area = $%d", idx)
		args = append(args, filter.Area)
		idx++
	}
	if filter.Date != nil {
		where += fmt.Sprintf(" AND msl_date = $%d", idx)
		args = append(args, *filter.Date)
		idx++
	}
	if filter.Shift != "" {
		where += fmt.Sprintf(" AND msl_shift = $%d", idx)
		args = append(args, filter.Shift)
		idx++
	}
	if filter.Status != "" {
		where += fmt.Sprintf(" AND msl_status = $%d", idx)
		args = append(args, filter.Status)
	}
	return where, args
}

func scanShiftLogRows(rows *sql.Rows) (*dailyperf.MachineShiftLog, error) {
	var p dailyperf.ReconstructShiftLogParams
	if err := rows.Scan(
		&p.ID, &p.MachineID, &p.MachineNo, &p.Date, &p.Shift, &p.PositionsTotal,
		&p.PositionsRunning, &p.RunningMinutes, &p.Status, &p.InputBy, &p.InputAt, &p.UpdatedAt,
	); err != nil {
		return nil, fmt.Errorf("failed to scan machine shift log: %w", err)
	}
	return dailyperf.ReconstructMachineShiftLog(p), nil
}

// ── AreaShiftLog ─────────────────────────────────────────────────────────────

// UpsertArea inserts or updates an area shift log on its natural key.
func (r *DailyPerfRepository) UpsertArea(ctx context.Context, log *dailyperf.AreaShiftLog) error {
	query := `
		INSERT INTO area_shift_log (asl_area, asl_date, asl_shift, asl_ot_hours, asl_notes, asl_input_by, asl_input_at)
		VALUES ($1, $2, $3, $4::DECIMAL, NULLIF($5, ''), $6, $7)
		ON CONFLICT (asl_area, asl_date, asl_shift) DO UPDATE SET
			asl_ot_hours = $4::DECIMAL, asl_notes = NULLIF($5, ''), asl_input_by = $6, asl_input_at = $7
		RETURNING asl_id`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		log.AreaCode(), log.Date(), stringPtrToNull(log.Shift()), floatPtrArg(log.OtHours()),
		log.Notes(), log.InputBy(), log.InputAt(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to upsert area shift log: %w", err)
	}
	log.SetID(id)
	return nil
}

// ── DowntimeEvent ────────────────────────────────────────────────────────────

// ReplaceDowntimeForShiftLog replaces all downtime events for a shift log in one tx.
func (r *DailyPerfRepository) ReplaceDowntimeForShiftLog(ctx context.Context, shiftLogID int64, events []*dailyperf.DowntimeEvent) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM downtime_event WHERE de_shift_log_id = $1`, shiftLogID); err != nil {
			return fmt.Errorf("failed to clear downtime events: %w", err)
		}
		for _, e := range events {
			if err := insertDowntimeTx(ctx, tx, shiftLogID, e); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertDowntimeTx(ctx context.Context, tx *sql.Tx, shiftLogID int64, e *dailyperf.DowntimeEvent) error {
	query := `
		INSERT INTO downtime_event (
			de_machine_id, de_wo_id, de_shift_log_id, de_ce_id, de_date, de_shift,
			de_position_no, de_reason_id, de_start_at, de_end_at, de_duration_min,
			de_lost_kg, de_notes, de_input_by, de_input_at
		) VALUES ($1, $2::BIGINT, $3, $4::BIGINT, $5, $6, $7, $8, $9::TIMESTAMPTZ, $10::TIMESTAMPTZ,
			$11::INT, $12::DECIMAL, $13, $14, $15)
		RETURNING de_id`
	var id int64
	err := tx.QueryRowContext(ctx, query,
		e.MachineID, int64PtrArg(e.WoID), shiftLogID, int64PtrArg(e.CeID), e.Date, e.Shift,
		stringPtrToNull(e.PositionNo), e.ReasonID, timePtrArg(e.StartAt), timePtrArg(e.EndAt),
		int32PtrArg(e.DurationMin), floatPtrArg(e.LostKg), stringPtrToNull(e.Notes), e.InputBy, e.InputAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to insert downtime event: %w", err)
	}
	e.ID = id
	return nil
}

// ── WasteActual ──────────────────────────────────────────────────────────────

// ReplaceWasteForShiftLog replaces all waste rows for a shift log in one tx.
func (r *DailyPerfRepository) ReplaceWasteForShiftLog(ctx context.Context, shiftLogID int64, rows []*dailyperf.WasteActual) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM waste_actual WHERE wa_shift_log_id = $1`, shiftLogID); err != nil {
			return fmt.Errorf("failed to clear waste rows: %w", err)
		}
		for _, w := range rows {
			if err := insertWasteTx(ctx, tx, shiftLogID, w); err != nil {
				return err
			}
		}
		return nil
	})
}

func insertWasteTx(ctx context.Context, tx *sql.Tx, shiftLogID int64, w *dailyperf.WasteActual) error {
	// wa_area is required NOT NULL; derive it from the machine when the caller did
	// not set it (waste is recorded through the shift entry, which is machine-scoped).
	query := `
		INSERT INTO waste_actual (
			wa_area, wa_machine_id, wa_wo_id, wa_shift_log_id, wa_date, wa_shift,
			wa_category_id, wa_qty_kg, wa_is_upset, wa_notes, wa_input_by, wa_input_at
		) VALUES (
			COALESCE(NULLIF($1, ''), (SELECT machine_area FROM machine WHERE machine_id = $2)),
			$2::BIGINT, $3::BIGINT, $4, $5, $6, $7, $8::DECIMAL, $9, $10, $11, $12)
		RETURNING wa_id`
	var id int64
	err := tx.QueryRowContext(ctx, query,
		w.Area, int64PtrArg(w.MachineID), int64PtrArg(w.WoID), shiftLogID, w.Date, w.Shift,
		w.CategoryID, w.QtyKg, w.IsUpset, stringPtrToNull(w.Notes), w.InputBy, w.InputAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to insert waste actual: %w", err)
	}
	w.ID = id
	return nil
}

// ── ShiftLogNote ─────────────────────────────────────────────────────────────

// CreateNote persists a new shift-log note and assigns its generated id.
func (r *DailyPerfRepository) CreateNote(ctx context.Context, note *dailyperf.ShiftLogNote) error {
	query := `
		INSERT INTO shift_log_note (sln_machine_id, sln_date, sln_shift, sln_type, sln_note, sln_wo_id, sln_input_by, sln_input_at)
		VALUES ($1, $2, $3, $4, $5, $6::BIGINT, $7, $8)
		RETURNING sln_id`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		note.MachineID(), note.Date(), note.Shift(), note.NoteType(), note.Note(),
		int64PtrArg(note.WoID()), note.InputBy(), note.InputAt(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to create shift log note: %w", err)
	}
	note.SetID(id)
	return nil
}

// UpdateNote mutates an existing shift-log note (type + body).
func (r *DailyPerfRepository) UpdateNote(ctx context.Context, note *dailyperf.ShiftLogNote) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE shift_log_note SET sln_type = $2, sln_note = $3 WHERE sln_id = $1`,
		note.ID(), note.NoteType(), note.Note())
	if err != nil {
		return fmt.Errorf("failed to update shift log note: %w", err)
	}
	return checkAffected(res, dailyperf.ErrNoteNotFound)
}

// DeleteNote removes a shift-log note by id.
func (r *DailyPerfRepository) DeleteNote(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM shift_log_note WHERE sln_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete shift log note: %w", err)
	}
	return checkAffected(res, dailyperf.ErrNoteNotFound)
}

const shiftLogNoteColumns = `sln_id, sln_machine_id, COALESCE(m.machine_no, ''), sln_date, sln_shift,
	sln_type, sln_note, sln_wo_id, sln_input_by, sln_input_at`

// GetNoteByID loads a shift-log note by id.
func (r *DailyPerfRepository) GetNoteByID(ctx context.Context, id int64) (*dailyperf.ShiftLogNote, error) {
	query := `SELECT ` + shiftLogNoteColumns + `
		FROM shift_log_note
		LEFT JOIN machine m ON m.machine_id = sln_machine_id
		WHERE sln_id = $1`
	row := r.db.QueryRowContext(ctx, query, id)
	note, err := scanShiftLogNoteRow(row)
	if err != nil {
		return nil, err
	}
	return note, nil
}

func scanShiftLogNoteRow(row *sql.Row) (*dailyperf.ShiftLogNote, error) {
	var p dailyperf.ReconstructShiftLogNoteParams
	var woID sql.NullInt64
	if err := row.Scan(
		&p.ID, &p.MachineID, &p.MachineNo, &p.Date, &p.Shift, &p.NoteType, &p.Note, &woID, &p.InputBy, &p.InputAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, dailyperf.ErrNoteNotFound
		}
		return nil, fmt.Errorf("failed to scan shift log note: %w", err)
	}
	p.WoID = nullInt64Ptr(woID)
	return dailyperf.ReconstructShiftLogNote(p), nil
}

var shiftLogNoteSortColumns = map[string]string{
	"date":       "sln_date",
	"type":       "sln_type",
	"created_at": "sln_input_at",
}

// ListNotes returns a filtered, paginated page of shift-log notes plus the total.
func (r *DailyPerfRepository) ListNotes(ctx context.Context, filter dailyperf.ShiftLogNoteFilter) ([]*dailyperf.ShiftLogNote, int64, error) {
	where, args := buildNoteFilter(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM shift_log_note`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count shift log notes: %w", err)
	}

	orderCol, ok := shiftLogNoteSortColumns[filter.SortBy]
	if !ok {
		orderCol = "sln_input_at"
	}
	direction := sortDirection(filter.SortOrder)
	if filter.SortBy == "" {
		direction = sortDESC
	}

	limit := filter.PageSize
	if limit <= 0 {
		limit = 20
	}
	page := filter.Page
	if page <= 0 {
		page = 1
	}
	offset := (page - 1) * limit

	query := `SELECT ` + shiftLogNoteColumns + `
		FROM shift_log_note
		LEFT JOIN machine m ON m.machine_id = sln_machine_id` + where +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, direction, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list shift log notes: %w", err)
	}
	defer closeRows(rows)

	var result []*dailyperf.ShiftLogNote
	for rows.Next() {
		note, scanErr := scanShiftLogNoteRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, note)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating shift log notes: %w", err)
	}
	return result, total, nil
}

func buildNoteFilter(filter dailyperf.ShiftLogNoteFilter) (string, []interface{}) {
	where := " WHERE 1=1"
	var args []interface{}
	idx := 1
	if filter.MachineID != nil {
		where += fmt.Sprintf(" AND sln_machine_id = $%d", idx)
		args = append(args, *filter.MachineID)
		idx++
	}
	if filter.Date != nil {
		where += fmt.Sprintf(" AND sln_date = $%d", idx)
		args = append(args, *filter.Date)
		idx++
	}
	if filter.Shift != "" {
		where += fmt.Sprintf(" AND sln_shift = $%d", idx)
		args = append(args, filter.Shift)
		idx++
	}
	if filter.NoteType != "" {
		where += fmt.Sprintf(" AND sln_type = $%d", idx)
		args = append(args, filter.NoteType)
	}
	return where, args
}

func scanShiftLogNoteRows(rows *sql.Rows) (*dailyperf.ShiftLogNote, error) {
	var p dailyperf.ReconstructShiftLogNoteParams
	var woID sql.NullInt64
	if err := rows.Scan(
		&p.ID, &p.MachineID, &p.MachineNo, &p.Date, &p.Shift, &p.NoteType, &p.Note, &woID, &p.InputBy, &p.InputAt,
	); err != nil {
		return nil, fmt.Errorf("failed to scan shift log note: %w", err)
	}
	p.WoID = nullInt64Ptr(woID)
	return dailyperf.ReconstructShiftLogNote(p), nil
}

// ── EfficiencySnapshot ───────────────────────────────────────────────────────

// UpsertSnapshot inserts or updates an efficiency snapshot on its unique scope key.
func (r *DailyPerfRepository) UpsertSnapshot(ctx context.Context, snap *dailyperf.EfficiencySnapshot) error {
	query := `
		INSERT INTO efficiency_snapshot (
			es_area, es_scope, es_machine_id, es_wo_id, es_date, es_shift, es_segment, es_is_excluding,
			es_qty_theoretical_100, es_qty_theoretical_rng, es_qty_loss, es_qty_waste, es_qty_actual,
			es_eff_production_pct, es_eff_running_pct, es_eff_plant_pct, es_yield_pct, es_waste_pct,
			es_breaks_count, es_breaks_per_ton, es_calc_at
		) VALUES (
			$1, $2, $3::BIGINT, $4::BIGINT, $5, $6, $7, $8,
			$9::DECIMAL, $10::DECIMAL, $11::DECIMAL, $12::DECIMAL, $13::DECIMAL,
			$14::DECIMAL, $15::DECIMAL, $16::DECIMAL, $17::DECIMAL, $18::DECIMAL,
			$19::INT, $20::DECIMAL, $21)
		ON CONFLICT (es_area, es_scope, es_machine_id, es_wo_id, es_date, es_shift, es_segment, es_is_excluding)
		DO UPDATE SET
			es_qty_theoretical_100 = $9::DECIMAL, es_qty_theoretical_rng = $10::DECIMAL,
			es_qty_loss = $11::DECIMAL, es_qty_waste = $12::DECIMAL, es_qty_actual = $13::DECIMAL,
			es_eff_production_pct = $14::DECIMAL, es_eff_running_pct = $15::DECIMAL,
			es_eff_plant_pct = $16::DECIMAL, es_yield_pct = $17::DECIMAL, es_waste_pct = $18::DECIMAL,
			es_breaks_count = $19::INT, es_breaks_per_ton = $20::DECIMAL, es_calc_at = $21
		RETURNING es_id`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		snap.Area, snap.Scope, int64PtrArg(snap.MachineID), int64PtrArg(snap.WoID), snap.Date,
		stringPtrToNull(snap.Shift), stringPtrToNull(snap.Segment), snap.IsExcluding,
		snap.QtyTheoretical100, snap.QtyTheoreticalRng, snap.QtyLoss, snap.QtyWaste, snap.QtyActual,
		snap.EffProductionPct, snap.EffRunningPct, snap.EffPlantPct, snap.YieldPct, snap.WastePct,
		snap.BreaksCount, snap.BreaksPerTon, snap.CalcAt,
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to upsert efficiency snapshot: %w", err)
	}
	snap.ID = id
	return nil
}

// snapshotColumns is the shared column list for reading efficiency_snapshot rows.
const snapshotColumns = `
	es_id, es_area, es_scope, es_machine_id, es_wo_id, es_date, es_shift, es_segment,
	es_is_excluding, es_qty_theoretical_100, es_qty_theoretical_rng, es_qty_loss,
	es_qty_waste, es_qty_actual, es_eff_production_pct, es_eff_running_pct,
	es_eff_plant_pct, es_yield_pct, es_waste_pct, es_breaks_count, es_breaks_per_ton, es_calc_at`

// ListSnapshots returns efficiency snapshots matching the filter plus the total
// count. Used by the dashboard list and the Excel export.
func (r *DailyPerfRepository) ListSnapshots(ctx context.Context, filter dailyperf.SnapshotFilter) ([]*dailyperf.EfficiencySnapshot, int64, error) {
	where, args := buildSnapshotFilter(filter)
	countQuery := `SELECT COUNT(*) FROM efficiency_snapshot` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count efficiency_snapshot: %w", err)
	}

	orderBy := snapshotOrderBy(filter.SortBy, filter.SortOrder)
	limit, offset := pageBounds(filter.Page, filter.PageSize)
	listQuery := `SELECT ` + snapshotColumns + ` FROM efficiency_snapshot` + where +
		fmt.Sprintf(` ORDER BY %s LIMIT %d OFFSET %d`, orderBy, limit, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list efficiency_snapshot: %w", err)
	}
	defer closeRows(rows)

	var result []*dailyperf.EfficiencySnapshot
	for rows.Next() {
		snap, scanErr := scanSnapshot(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan efficiency_snapshot: %w", scanErr)
		}
		result = append(result, snap)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate efficiency_snapshot: %w", err)
	}
	return result, total, nil
}

// scanSnapshot scans a flat efficiency_snapshot row into a domain snapshot.
func scanSnapshot(s rowScanner) (*dailyperf.EfficiencySnapshot, error) {
	var (
		snap                dailyperf.EfficiencySnapshot
		machineID, woID     sql.NullInt64
		shift, segment      sql.NullString
		theo100, theoRng    sql.NullFloat64
		loss, waste, actual sql.NullFloat64
		effProd, effRun     sql.NullFloat64
		effPlant, yield     sql.NullFloat64
		wastePct, perTon    sql.NullFloat64
		breaks              sql.NullInt64
	)
	if err := s.Scan(
		&snap.ID, &snap.Area, &snap.Scope, &machineID, &woID, &snap.Date, &shift, &segment,
		&snap.IsExcluding, &theo100, &theoRng, &loss, &waste, &actual,
		&effProd, &effRun, &effPlant, &yield, &wastePct, &breaks, &perTon, &snap.CalcAt,
	); err != nil {
		return nil, err
	}
	snap.MachineID = nullInt64Ptr(machineID)
	snap.WoID = nullInt64Ptr(woID)
	snap.Shift = nullStringPtr(shift)
	snap.Segment = nullStringPtr(segment)
	snap.QtyTheoretical100 = theo100.Float64
	snap.QtyTheoreticalRng = theoRng.Float64
	snap.QtyLoss = loss.Float64
	snap.QtyWaste = waste.Float64
	snap.QtyActual = actual.Float64
	snap.EffProductionPct = effProd.Float64
	snap.EffRunningPct = effRun.Float64
	snap.EffPlantPct = effPlant.Float64
	snap.YieldPct = yield.Float64
	snap.WastePct = wastePct.Float64
	snap.BreaksCount = safeInt64ToInt32(breaks.Int64)
	snap.BreaksPerTon = perTon.Float64
	return &snap, nil
}

// buildSnapshotFilter builds the WHERE clause and args for the snapshot list.
func buildSnapshotFilter(filter dailyperf.SnapshotFilter) (string, []any) {
	var conds []string
	var args []any
	idx := 1
	if filter.Area != "" {
		conds = append(conds, fmt.Sprintf("es_area = $%d", idx))
		args = append(args, filter.Area)
		idx++
	}
	if filter.Scope != "" {
		conds = append(conds, fmt.Sprintf("es_scope = $%d", idx))
		args = append(args, filter.Scope)
		idx++
	}
	if filter.MachineID != nil {
		conds = append(conds, fmt.Sprintf("es_machine_id = $%d", idx))
		args = append(args, *filter.MachineID)
		idx++
	}
	if filter.DateFrom != nil {
		conds = append(conds, fmt.Sprintf("es_date >= $%d", idx))
		args = append(args, *filter.DateFrom)
		idx++
	}
	if filter.DateTo != nil {
		conds = append(conds, fmt.Sprintf("es_date <= $%d", idx))
		args = append(args, *filter.DateTo)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// snapshotOrderBy maps a frontend sort field to a safe ORDER BY clause.
func snapshotOrderBy(sortBy, sortOrder string) string {
	col := "es_date"
	switch sortBy {
	case "area":
		col = "es_area"
	case "scope":
		col = "es_scope"
	case "eff_production_pct":
		col = "es_eff_production_pct"
	}
	dir := "DESC"
	if strings.EqualFold(sortOrder, "asc") {
		dir = "ASC"
	}
	return col + " " + dir + ", es_id ASC"
}

// DeleteScope removes snapshots for an (area, date), optionally scoped to a machine.
func (r *DailyPerfRepository) DeleteScope(ctx context.Context, areaCode string, date time.Time, machineID *int64) error {
	query := `DELETE FROM efficiency_snapshot WHERE es_area = $1 AND es_date = $2
		AND ($3::BIGINT IS NULL OR es_machine_id = $3::BIGINT)`
	if _, err := r.db.ExecContext(ctx, query, areaCode, date, int64PtrArg(machineID)); err != nil {
		return fmt.Errorf("failed to delete efficiency snapshots: %w", err)
	}
	return nil
}

// ── Read ports ───────────────────────────────────────────────────────────────

// ProductionActuals returns production-actual rows for an area on a date. Breaks
// are selected per shift from the shift-specific break column.
func (r *DailyPerfRepository) ProductionActuals(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]dailyperf.ProductionActual, error) {
	query := `
		SELECT wpa.wpa_wo_id, wo.wo_machine_id, wpa.wpa_date, wpa.wpa_shift,
			COALESCE(wpa.wpa_qty_actual, 0),
			CASE wpa.wpa_shift
				WHEN '1' THEN COALESCE(wpa.wpa_breaks_shift1, 0)
				WHEN '2' THEN COALESCE(wpa.wpa_breaks_shift2, 0)
				WHEN '3' THEN COALESCE(wpa.wpa_breaks_shift3, 0)
				ELSE 0 END,
			COALESCE(wo.wo_prod_category, 'NORMAL')
		FROM wo_production_actual wpa
		JOIN work_order wo ON wo.wo_id = wpa.wpa_wo_id
		WHERE wpa.wpa_area = $1 AND wpa.wpa_date = $2
			AND ($3::BIGINT IS NULL OR wo.wo_machine_id = $3::BIGINT)
			AND ($4::TEXT IS NULL OR wpa.wpa_shift = $4::TEXT)`
	rows, err := r.db.QueryContext(ctx, query, areaCode, date, int64PtrArg(machineID), stringPtrArg(shift))
	if err != nil {
		return nil, fmt.Errorf("failed to read production actuals: %w", err)
	}
	defer closeRows(rows)

	var result []dailyperf.ProductionActual
	for rows.Next() {
		var p dailyperf.ProductionActual
		if err := rows.Scan(&p.WoID, &p.MachineID, &p.Date, &p.Shift, &p.QtyActual, &p.Breaks, &p.ProdCategory); err != nil {
			return nil, fmt.Errorf("failed to scan production actual: %w", err)
		}
		result = append(result, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating production actuals: %w", err)
	}
	return result, nil
}

// DowntimeAggregates returns per-(machine,shift) downtime rollups, splitting out
// the portion whose reason is excluded from efficiency.
func (r *DailyPerfRepository) DowntimeAggregates(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]dailyperf.DowntimeAggregate, error) {
	query := `
		SELECT de.de_machine_id, COALESCE(de.de_shift, ''),
			COALESCE(SUM(de.de_duration_min), 0),
			COALESCE(SUM(de.de_lost_kg), 0),
			COALESCE(SUM(CASE WHEN drm.drm_is_exclude_from_eff THEN de.de_duration_min ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN drm.drm_is_exclude_from_eff THEN de.de_lost_kg ELSE 0 END), 0)
		FROM downtime_event de
		JOIN machine m ON m.machine_id = de.de_machine_id
		JOIN downtime_reason_master drm ON drm.drm_id = de.de_reason_id
		WHERE m.machine_area = $1 AND de.de_date = $2
			AND ($3::BIGINT IS NULL OR de.de_machine_id = $3::BIGINT)
			AND ($4::TEXT IS NULL OR de.de_shift = $4::TEXT)
		GROUP BY de.de_machine_id, de.de_shift`
	rows, err := r.db.QueryContext(ctx, query, areaCode, date, int64PtrArg(machineID), stringPtrArg(shift))
	if err != nil {
		return nil, fmt.Errorf("failed to read downtime aggregates: %w", err)
	}
	defer closeRows(rows)

	var result []dailyperf.DowntimeAggregate
	for rows.Next() {
		var d dailyperf.DowntimeAggregate
		if err := rows.Scan(&d.MachineID, &d.Shift, &d.DurationMin, &d.LostKg, &d.ExcludedDurationMin, &d.ExcludedLostKg); err != nil {
			return nil, fmt.Errorf("failed to scan downtime aggregate: %w", err)
		}
		result = append(result, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating downtime aggregates: %w", err)
	}
	return result, nil
}

// WasteAggregates returns per-(machine,shift) waste rollups for an area on a date.
func (r *DailyPerfRepository) WasteAggregates(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]dailyperf.WasteAggregate, error) {
	query := `
		SELECT COALESCE(wa_machine_id, 0), COALESCE(wa_shift, ''), COALESCE(SUM(wa_qty_kg), 0)
		FROM waste_actual
		WHERE wa_area = $1 AND wa_date = $2
			AND ($3::BIGINT IS NULL OR wa_machine_id = $3::BIGINT)
			AND ($4::TEXT IS NULL OR wa_shift = $4::TEXT)
		GROUP BY wa_machine_id, wa_shift`
	rows, err := r.db.QueryContext(ctx, query, areaCode, date, int64PtrArg(machineID), stringPtrArg(shift))
	if err != nil {
		return nil, fmt.Errorf("failed to read waste aggregates: %w", err)
	}
	defer closeRows(rows)

	var result []dailyperf.WasteAggregate
	for rows.Next() {
		var w dailyperf.WasteAggregate
		if err := rows.Scan(&w.MachineID, &w.Shift, &w.QtyKg); err != nil {
			return nil, fmt.Errorf("failed to scan waste aggregate: %w", err)
		}
		result = append(result, w)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating waste aggregates: %w", err)
	}
	return result, nil
}

// ShiftLogsForArea lists the machine shift logs for an area on a date.
func (r *DailyPerfRepository) ShiftLogsForArea(ctx context.Context, areaCode string, date time.Time, machineID *int64, shift *string) ([]*dailyperf.MachineShiftLog, error) {
	query := `SELECT ` + machineShiftLogColumns + `
		FROM machine_shift_log
		JOIN machine m ON m.machine_id = msl_machine_id
		WHERE m.machine_area = $1 AND msl_date = $2
			AND ($3::BIGINT IS NULL OR msl_machine_id = $3::BIGINT)
			AND ($4::TEXT IS NULL OR msl_shift = $4::TEXT)`
	rows, err := r.db.QueryContext(ctx, query, areaCode, date, int64PtrArg(machineID), stringPtrArg(shift))
	if err != nil {
		return nil, fmt.Errorf("failed to read shift logs for area: %w", err)
	}
	defer closeRows(rows)

	var result []*dailyperf.MachineShiftLog
	for rows.Next() {
		var p dailyperf.ReconstructShiftLogParams
		if err := rows.Scan(
			&p.ID, &p.MachineID, &p.MachineNo, &p.Date, &p.Shift, &p.PositionsTotal,
			&p.PositionsRunning, &p.RunningMinutes, &p.Status, &p.InputBy, &p.InputAt, &p.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan shift log: %w", err)
		}
		result = append(result, dailyperf.ReconstructMachineShiftLog(p))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating shift logs for area: %w", err)
	}
	return result, nil
}

// ── WOResolveContext + MachineNoLookup ───────────────────────────────────────

// ResolveContext resolves a WO's finance product sys id, optional ref WO, and
// machine for the well-known parameter resolution.
func (r *DailyPerfRepository) ResolveContext(ctx context.Context, woID int64) (int64, *int64, int64, error) {
	var productSysID, machineID int64
	var refWoID sql.NullInt64
	query := `
		SELECT COALESCE(ppi.ppi_cpm_product_sys_id, 0), wo.wo_ref_wo_id, wo.wo_machine_id
		FROM work_order wo
		LEFT JOIN production_plan_item ppi ON ppi.ppi_id = wo.wo_plan_item_id
		WHERE wo.wo_id = $1`
	err := r.db.QueryRowContext(ctx, query, woID).Scan(&productSysID, &refWoID, &machineID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil, 0, nil
	}
	if err != nil {
		return 0, nil, 0, fmt.Errorf("failed to resolve WO context: %w", err)
	}
	return productSysID, nullInt64Ptr(refWoID), machineID, nil
}

// MachineNo returns a machine's number, or "" when not found.
func (r *DailyPerfRepository) MachineNo(ctx context.Context, machineID int64) (string, error) {
	var no sql.NullString
	err := r.db.QueryRowContext(ctx, `SELECT machine_no FROM machine WHERE machine_id = $1`, machineID).Scan(&no)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("failed to look up machine no: %w", err)
	}
	return nullString(no), nil
}

// int32PtrArg converts an optional int32 to a driver argument (NULL when nil).
func int32PtrArg(v *int32) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// stringPtrArg converts an optional string to a driver argument (NULL when nil).
func stringPtrArg(v *string) interface{} {
	if v == nil {
		return nil
	}
	return *v
}

// ── Interface adapters ───────────────────────────────────────────────────────
//
// The single DailyPerfRepository directly satisfies MachineShiftLogRepository,
// the efficiency read ports, WOResolveContext, and MachineNoLookup. The remaining
// domain ports share method names (Upsert / ReplaceForShiftLog / GetByID), so each
// gets a thin adapter that delegates to the uniquely-named repository methods.

// AreaShiftLogRepo adapts DailyPerfRepository to dailyperf.AreaShiftLogRepository.
type AreaShiftLogRepo struct{ r *DailyPerfRepository }

// NewAreaShiftLogRepo builds the area-shift-log adapter.
func NewAreaShiftLogRepo(r *DailyPerfRepository) *AreaShiftLogRepo { return &AreaShiftLogRepo{r: r} }

// Upsert inserts or updates an area shift log.
func (a *AreaShiftLogRepo) Upsert(ctx context.Context, log *dailyperf.AreaShiftLog) error {
	return a.r.UpsertArea(ctx, log)
}

// DowntimeRepo adapts DailyPerfRepository to dailyperf.DowntimeEventRepository.
type DowntimeRepo struct{ r *DailyPerfRepository }

// NewDowntimeRepo builds the downtime-event adapter.
func NewDowntimeRepo(r *DailyPerfRepository) *DowntimeRepo { return &DowntimeRepo{r: r} }

// ReplaceForShiftLog replaces the downtime events for a shift log.
func (d *DowntimeRepo) ReplaceForShiftLog(ctx context.Context, shiftLogID int64, events []*dailyperf.DowntimeEvent) error {
	return d.r.ReplaceDowntimeForShiftLog(ctx, shiftLogID, events)
}

// WasteRepo adapts DailyPerfRepository to dailyperf.WasteActualRepository.
type WasteRepo struct{ r *DailyPerfRepository }

// NewWasteRepo builds the waste-actual adapter.
func NewWasteRepo(r *DailyPerfRepository) *WasteRepo { return &WasteRepo{r: r} }

// ReplaceForShiftLog replaces the waste rows for a shift log.
func (w *WasteRepo) ReplaceForShiftLog(ctx context.Context, shiftLogID int64, rows []*dailyperf.WasteActual) error {
	return w.r.ReplaceWasteForShiftLog(ctx, shiftLogID, rows)
}

// NoteRepo adapts DailyPerfRepository to dailyperf.ShiftLogNoteRepository.
type NoteRepo struct{ r *DailyPerfRepository }

// NewNoteRepo builds the shift-log-note adapter.
func NewNoteRepo(r *DailyPerfRepository) *NoteRepo { return &NoteRepo{r: r} }

// Create persists a new shift-log note.
func (n *NoteRepo) Create(ctx context.Context, note *dailyperf.ShiftLogNote) error {
	return n.r.CreateNote(ctx, note)
}

// Update mutates a shift-log note.
func (n *NoteRepo) Update(ctx context.Context, note *dailyperf.ShiftLogNote) error {
	return n.r.UpdateNote(ctx, note)
}

// Delete removes a shift-log note.
func (n *NoteRepo) Delete(ctx context.Context, id int64) error { return n.r.DeleteNote(ctx, id) }

// GetByID loads a shift-log note by id.
func (n *NoteRepo) GetByID(ctx context.Context, id int64) (*dailyperf.ShiftLogNote, error) {
	return n.r.GetNoteByID(ctx, id)
}

// List returns a filtered, paginated page of shift-log notes.
func (n *NoteRepo) List(ctx context.Context, filter dailyperf.ShiftLogNoteFilter) ([]*dailyperf.ShiftLogNote, int64, error) {
	return n.r.ListNotes(ctx, filter)
}

// SnapshotRepo adapts DailyPerfRepository to dailyperf.EfficiencySnapshotRepository.
type SnapshotRepo struct{ r *DailyPerfRepository }

// NewSnapshotRepo builds the efficiency-snapshot adapter.
func NewSnapshotRepo(r *DailyPerfRepository) *SnapshotRepo { return &SnapshotRepo{r: r} }

// Upsert inserts or updates an efficiency snapshot.
func (s *SnapshotRepo) Upsert(ctx context.Context, snap *dailyperf.EfficiencySnapshot) error {
	return s.r.UpsertSnapshot(ctx, snap)
}

// DeleteScope removes snapshots for a scope.
func (s *SnapshotRepo) DeleteScope(ctx context.Context, areaCode string, date time.Time, machineID *int64) error {
	return s.r.DeleteScope(ctx, areaCode, date, machineID)
}

// Compile-time interface assertions.
var (
	_ dailyperf.MachineShiftLogRepository    = (*DailyPerfRepository)(nil)
	_ dailyperf.ProductionActualReader       = (*DailyPerfRepository)(nil)
	_ dailyperf.DowntimeReader               = (*DailyPerfRepository)(nil)
	_ dailyperf.WasteReader                  = (*DailyPerfRepository)(nil)
	_ dailyperf.ShiftLogReader               = (*DailyPerfRepository)(nil)
	_ dailyperf.MachineNoLookup              = (*DailyPerfRepository)(nil)
	_ dailyperf.AreaShiftLogRepository       = (*AreaShiftLogRepo)(nil)
	_ dailyperf.DowntimeEventRepository      = (*DowntimeRepo)(nil)
	_ dailyperf.WasteActualRepository        = (*WasteRepo)(nil)
	_ dailyperf.ShiftLogNoteRepository       = (*NoteRepo)(nil)
	_ dailyperf.EfficiencySnapshotRepository = (*SnapshotRepo)(nil)
)
