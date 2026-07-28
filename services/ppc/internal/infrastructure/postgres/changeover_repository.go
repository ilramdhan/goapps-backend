package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/changeover"
)

// ChangeoverRepository is the PostgreSQL implementation of changeover.Repository.
// changeover_event holds the estimate/actual; changeover_component holds the
// per-code breakdown (BASE + C1..C7).
type ChangeoverRepository struct {
	db *DB
}

// NewChangeoverRepository creates a new ChangeoverRepository.
func NewChangeoverRepository(db *DB) *ChangeoverRepository {
	return &ChangeoverRepository{db: db}
}

// Create inserts a changeover event and its components in a single transaction,
// setting the generated event id on the aggregate.
func (r *ChangeoverRepository) Create(ctx context.Context, event *changeover.Event) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		const eventInsert = `
			INSERT INTO changeover_event (
				ce_from_wo_id, ce_to_wo_id, ce_machine_id,
				ce_duration_estimated, ce_waste_estimated, ce_group,
				ce_status, ce_notes)
			VALUES ($1, $2, $3, $4::INT, $5::DECIMAL, $6, $7, $8)
			RETURNING ce_id`
		var eventID int64
		err := tx.QueryRowContext(ctx, eventInsert,
			event.FromWOID(), event.ToWOID(), event.MachineID(),
			event.DurationEstimated(), event.WasteEstimated(), event.Group(),
			event.Status(), event.Notes(),
		).Scan(&eventID)
		if err != nil {
			return fmt.Errorf("insert changeover_event: %w", err)
		}
		event.SetID(eventID)

		for _, comp := range event.Components() {
			if err := insertChangeoverComponent(ctx, tx, eventID, comp); err != nil {
				return err
			}
		}
		return nil
	})
}

// insertChangeoverComponent inserts one component line for an event.
func insertChangeoverComponent(ctx context.Context, tx *sql.Tx, eventID int64, comp changeover.Component) error {
	const query = `
		INSERT INTO changeover_component (
			cc_event_id, cc_component_code, cc_duration_applied,
			cc_waste_applied, cc_is_auto_detected, cc_override_by, cc_override_at)
		VALUES ($1, $2, $3::INT, $4::DECIMAL, $5, $6::BIGINT, $7)`
	if _, err := tx.ExecContext(ctx, query,
		eventID, comp.Code(), comp.DurationMin(), comp.WasteKg(),
		comp.IsAutoDetected(), int64PtrArg(comp.OverrideBy()), timePtrArg(comp.OverrideAt()),
	); err != nil {
		return fmt.Errorf("insert changeover_component %s: %w", comp.Code(), err)
	}
	return nil
}

const changeoverEventColumns = `
	e.ce_id, e.ce_from_wo_id, e.ce_to_wo_id, e.ce_machine_id,
	e.ce_duration_estimated, e.ce_waste_estimated, e.ce_group,
	e.ce_duration_actual, e.ce_waste_actual, e.ce_status,
	e.ce_started_at, e.ce_completed_at, e.ce_notes`

// GetByID loads a changeover event and its components.
func (r *ChangeoverRepository) GetByID(ctx context.Context, id int64) (*changeover.Event, error) {
	query := `SELECT ` + changeoverEventColumns + ` FROM changeover_event e WHERE e.ce_id = $1`
	row := r.db.QueryRowContext(ctx, query, id)
	event, err := scanChangeoverEvent(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, changeover.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("get changeover_event %d: %w", id, err)
	}
	comps, err := r.loadComponents(ctx, id)
	if err != nil {
		return nil, err
	}
	return rebuildEvent(event, comps), nil
}

// List returns changeover events matching the filter plus the total count.
// Components are not loaded in list mode (kept lightweight for the Gantt/list).
func (r *ChangeoverRepository) List(ctx context.Context, filter changeover.Filter) ([]*changeover.Event, int64, error) {
	where, args := buildChangeoverFilter(filter)
	countQuery := `SELECT COUNT(*) FROM changeover_event e` + where
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count changeover_event: %w", err)
	}

	limit, offset := pageBounds(filter.Page, filter.PageSize)
	listQuery := `SELECT ` + changeoverEventColumns + ` FROM changeover_event e` + where +
		fmt.Sprintf(` ORDER BY e.ce_id DESC LIMIT %d OFFSET %d`, limit, offset)
	rows, err := r.db.QueryContext(ctx, listQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list changeover_event: %w", err)
	}
	defer closeRows(rows)

	var events []*changeover.Event
	for rows.Next() {
		event, scanErr := scanChangeoverEvent(rows)
		if scanErr != nil {
			return nil, 0, fmt.Errorf("scan changeover_event: %w", scanErr)
		}
		events = append(events, rebuildEvent(event, nil))
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate changeover_event: %w", err)
	}
	return events, total, nil
}

// UpdateActual persists the actual duration/waste, status and timestamps of an
// existing event (used by Start / Complete flows).
func (r *ChangeoverRepository) UpdateActual(ctx context.Context, event *changeover.Event) error {
	const query = `
		UPDATE changeover_event SET
			ce_duration_actual = $2::INT,
			ce_waste_actual = $3::DECIMAL,
			ce_status = $4,
			ce_started_at = $5,
			ce_completed_at = $6,
			ce_notes = $7
		WHERE ce_id = $1`
	res, err := r.db.ExecContext(ctx, query,
		event.ID(),
		int32PtrArg(event.DurationActual()),
		floatPtrArg(event.WasteActual()),
		event.Status(),
		timePtrArg(event.StartedAt()),
		timePtrArg(event.CompletedAt()),
		event.Notes(),
	)
	if err != nil {
		return fmt.Errorf("update changeover_event %d: %w", event.ID(), err)
	}
	return checkAffected(res, changeover.ErrNotFound)
}

// loadComponents reads all component lines for an event ordered by code.
func (r *ChangeoverRepository) loadComponents(ctx context.Context, eventID int64) ([]changeover.Component, error) {
	const query = `
		SELECT cc_id, cc_event_id, cc_component_code, cc_duration_applied,
			cc_waste_applied, cc_is_auto_detected, cc_override_by, cc_override_at
		FROM changeover_component WHERE cc_event_id = $1 ORDER BY cc_component_code`
	rows, err := r.db.QueryContext(ctx, query, eventID)
	if err != nil {
		return nil, fmt.Errorf("list changeover_component (event=%d): %w", eventID, err)
	}
	defer closeRows(rows)

	var comps []changeover.Component
	for rows.Next() {
		var (
			id, evID        int64
			code            string
			durationApplied int32
			wasteApplied    float64
			isAuto          bool
			overrideBy      sql.NullInt64
			overrideAt      sql.NullTime
		)
		if err := rows.Scan(&id, &evID, &code, &durationApplied, &wasteApplied,
			&isAuto, &overrideBy, &overrideAt); err != nil {
			return nil, fmt.Errorf("scan changeover_component: %w", err)
		}
		comps = append(comps, changeover.ReconstructComponent(
			id, evID, strings.TrimSpace(code), durationApplied, wasteApplied,
			isAuto, nullInt64Ptr(overrideBy), nullTimePtr(overrideAt),
		))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate changeover_component: %w", err)
	}
	return comps, nil
}

// scannedEvent holds the flat columns of a changeover_event row before rebuild.
type scannedEvent struct {
	id                int64
	fromWOID          int64
	toWOID            int64
	machineID         int64
	durationEstimated sql.NullInt64
	wasteEstimated    sql.NullFloat64
	group             sql.NullString
	durationActual    sql.NullInt64
	wasteActual       sql.NullFloat64
	status            sql.NullString
	startedAt         sql.NullTime
	completedAt       sql.NullTime
	notes             sql.NullString
}

// rowScanner abstracts *sql.Row and *sql.Rows for shared scanning.
type rowScanner interface {
	Scan(dest ...any) error
}

// scanChangeoverEvent scans a flat changeover_event row.
func scanChangeoverEvent(s rowScanner) (scannedEvent, error) {
	var e scannedEvent
	err := s.Scan(
		&e.id, &e.fromWOID, &e.toWOID, &e.machineID,
		&e.durationEstimated, &e.wasteEstimated, &e.group,
		&e.durationActual, &e.wasteActual, &e.status,
		&e.startedAt, &e.completedAt, &e.notes,
	)
	return e, err
}

// rebuildEvent maps a scanned row plus components into a domain aggregate.
func rebuildEvent(e scannedEvent, comps []changeover.Component) *changeover.Event {
	var durActual *int32
	if e.durationActual.Valid {
		v := safeInt64ToInt32(e.durationActual.Int64)
		durActual = &v
	}
	return changeover.ReconstructEvent(
		e.id, e.fromWOID, e.toWOID, e.machineID,
		safeInt64ToInt32(e.durationEstimated.Int64), e.wasteEstimated.Float64,
		nullString(e.group), durActual, nullFloatPtr(e.wasteActual),
		nullString(e.status), nullTimePtr(e.startedAt), nullTimePtr(e.completedAt),
		nullString(e.notes), comps,
	)
}

// buildChangeoverFilter builds the WHERE clause and args for the list query.
func buildChangeoverFilter(filter changeover.Filter) (string, []any) {
	var conds []string
	var args []any
	idx := 1
	if filter.MachineID != nil {
		conds = append(conds, fmt.Sprintf("e.ce_machine_id = $%d", idx))
		args = append(args, *filter.MachineID)
		idx++
	}
	if filter.Status != "" {
		conds = append(conds, fmt.Sprintf("e.ce_status = $%d", idx))
		args = append(args, filter.Status)
		idx++
	}
	if filter.DateFrom != nil {
		conds = append(conds, fmt.Sprintf("e.ce_started_at >= $%d", idx))
		args = append(args, *filter.DateFrom)
		idx++
	}
	if filter.DateTo != nil {
		conds = append(conds, fmt.Sprintf("e.ce_started_at <= $%d", idx))
		args = append(args, *filter.DateTo)
	}
	if len(conds) == 0 {
		return "", args
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// pageBounds normalizes page/pageSize into a SQL LIMIT/OFFSET pair.
func pageBounds(page, pageSize int32) (limit, offset int32) {
	if pageSize <= 0 {
		pageSize = 20
	}
	if page <= 0 {
		page = 1
	}
	return pageSize, (page - 1) * pageSize
}

// safeInt64ToInt32 clamps an int64 to the int32 range.
func safeInt64ToInt32(v int64) int32 {
	const maxInt32 = int64(1)<<31 - 1
	const minInt32 = -(int64(1) << 31)
	if v > maxInt32 {
		return int32(maxInt32)
	}
	if v < minInt32 {
		return int32(minInt32)
	}
	return int32(v) //nolint:gosec // clamped to int32 range above
}

// ChangeoverWOSpecSource resolves a WO's changeover-relevant spec + machine from
// the work_order spec snapshot (set at approve) and product_ppc_config.
type ChangeoverWOSpecSource struct {
	db *DB
}

// NewChangeoverWOSpecSource builds the WO spec source for changeover detection.
func NewChangeoverWOSpecSource(db *DB) *ChangeoverWOSpecSource {
	return &ChangeoverWOSpecSource{db: db}
}

// MachineForWO returns the machine id of a work order.
func (s *ChangeoverWOSpecSource) MachineForWO(ctx context.Context, woID int64) (int64, bool, error) {
	const query = `SELECT wo_machine_id FROM work_order WHERE wo_id = $1`
	var machineID int64
	err := s.db.QueryRowContext(ctx, query, woID).Scan(&machineID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, fmt.Errorf("machine for wo %d: %w", woID, err)
	}
	return machineID, true, nil
}

// SpecForWO resolves the changeover-relevant spec of a work order from its
// spec_snapshot JSONB (denier/color/shade/filament/twist) plus lot and product
// id. Missing snapshot fields default to zero/empty (treated as "unknown").
func (s *ChangeoverWOSpecSource) SpecForWO(ctx context.Context, woID int64) (changeover.Spec, bool, error) {
	const query = `
		SELECT wo.wo_lot_no, wo.wo_spec_snapshot, ppi.ppi_cpm_product_sys_id
		FROM work_order wo
		JOIN production_plan_item ppi ON ppi.ppi_id = wo.wo_plan_item_id
		WHERE wo.wo_id = $1`
	var (
		lotNo    string
		specJSON []byte
		prodID   sql.NullInt64
	)
	err := s.db.QueryRowContext(ctx, query, woID).Scan(&lotNo, &specJSON, &prodID)
	if errors.Is(err, sql.ErrNoRows) {
		return changeover.Spec{}, false, nil
	}
	if err != nil {
		return changeover.Spec{}, false, fmt.Errorf("spec for wo %d: %w", woID, err)
	}
	snapshot, err := unmarshalSnapshot(specJSON)
	if err != nil {
		return changeover.Spec{}, false, fmt.Errorf("decode spec snapshot for wo %d: %w", woID, err)
	}
	spec := specFromSnapshot(snapshot)
	spec.LotNo = strings.TrimSpace(lotNo)
	spec.ProductSysID = prodID.Int64
	return spec, true, nil
}

// specFromSnapshot maps loosely-typed snapshot fields to a changeover.Spec.
func specFromSnapshot(m map[string]any) changeover.Spec {
	return changeover.Spec{
		Denier:        snapshotFloat(m, "denier"),
		ColorFamily:   snapshotString(m, "color_family"),
		ShadeDarkness: int(snapshotFloat(m, "shade_darkness")),
		FilamentCount: int(snapshotFloat(m, "filament_count")),
		TwistDir:      snapshotString(m, "twist_dir"),
	}
}

// snapshotString reads a string-ish value from a JSONB snapshot map. Numbers are
// coerced via their string form; missing keys yield "".
func snapshotString(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok || v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// snapshotFloat reads a numeric value from a JSONB snapshot map. JSON numbers
// decode to float64; numeric strings are parsed. Missing/invalid keys yield 0.
func snapshotFloat(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok || v == nil {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(n), 64)
		if err != nil {
			return 0
		}
		return f
	default:
		return 0
	}
}
