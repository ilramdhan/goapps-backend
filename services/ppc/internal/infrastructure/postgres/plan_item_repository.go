package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// PlanItemRepository implements planitem.Repository using PostgreSQL.
type PlanItemRepository struct {
	db *DB
}

// NewPlanItemRepository creates a new PlanItemRepository.
func NewPlanItemRepository(db *DB) *PlanItemRepository {
	return &PlanItemRepository{db: db}
}

var _ planitem.Repository = (*PlanItemRepository)(nil)

const planItemColumns = `ppi_id, ppi_cpm_product_sys_id, ppi_type, ppi_demand_id, ppi_parent_item_id,
	ppi_qty_target, ppi_deadline, ppi_rm_source, ppi_sequence, ppi_status, ppi_machine_group_id,
	ppi_preferred_machine_id, ppi_month, ppi_notes, ppi_created_by, ppi_created_at, ppi_updated_at,
	ppi_planned_start_date, ppi_planned_duration_days, ppi_duration_source,
	ppi_shade_code, ppi_shade_name, ppi_carry_from_item_id, ppi_carry_action`

// planItemColumnsQualified is planItemColumns with every column prefixed by the
// `ppi` table alias, for queries that join or correlate. Same column order, so
// planItemDTO.dest() scans either one unchanged.
const planItemColumnsQualified = `ppi.ppi_id, ppi.ppi_cpm_product_sys_id, ppi.ppi_type, ppi.ppi_demand_id,
	ppi.ppi_parent_item_id, ppi.ppi_qty_target, ppi.ppi_deadline, ppi.ppi_rm_source, ppi.ppi_sequence,
	ppi.ppi_status, ppi.ppi_machine_group_id, ppi.ppi_preferred_machine_id, ppi.ppi_month, ppi.ppi_notes,
	ppi.ppi_created_by, ppi.ppi_created_at, ppi.ppi_updated_at, ppi.ppi_planned_start_date,
	ppi.ppi_planned_duration_days, ppi.ppi_duration_source, ppi.ppi_shade_code, ppi.ppi_shade_name,
	ppi.ppi_carry_from_item_id, ppi.ppi_carry_action`

const planItemInsert = `
	INSERT INTO production_plan_item (
		ppi_cpm_product_sys_id, ppi_type, ppi_demand_id, ppi_parent_item_id, ppi_qty_target,
		ppi_deadline, ppi_rm_source, ppi_sequence, ppi_status, ppi_machine_group_id,
		ppi_preferred_machine_id, ppi_month, ppi_notes, ppi_created_by, ppi_created_at, ppi_updated_at,
		ppi_planned_start_date, ppi_planned_duration_days, ppi_duration_source,
		ppi_shade_code, ppi_shade_name, ppi_carry_from_item_id, ppi_carry_action
	) VALUES (
		$1, $2, $3::BIGINT, $4::BIGINT, $5, $6, NULLIF($7, '')::VARCHAR, $8, $9, $10,
		$11::BIGINT, $12, NULLIF($13, '')::TEXT, $14, $15, $16,
		$17::DATE, $18::INT, $19,
		NULLIF($20, '')::VARCHAR, NULLIF($21, '')::VARCHAR,
		$22::BIGINT, NULLIF($23, '')::VARCHAR
	) RETURNING ppi_id`

// planItemInsertArgs flattens an entity into the planItemInsert placeholders.
func planItemInsertArgs(entity *planitem.PlanItem) []interface{} {
	return []interface{}{
		entity.CpmProductSysID(), entity.Type(), int64PtrArg(entity.DemandID()), int64PtrArg(entity.ParentItemID()), entity.QtyTarget(),
		entity.Deadline(), entity.RMSource(), entity.Sequence(), entity.Status(), entity.MachineGroupID(),
		int64PtrArg(entity.PreferredMachineID()), entity.Month(), entity.Notes(), entity.CreatedBy(), entity.CreatedAt(), entity.UpdatedAt(),
		timePtrArg(entity.PlannedStartDate()), int32PtrArg(entity.PlannedDurationDays()), entity.DurationSource(),
		entity.ShadeCode(), entity.ShadeName(),
		int64PtrArg(entity.CarryFromItemID()), entity.CarryAction(),
	}
}

// Create persists a new plan item and assigns its generated ID.
func (r *PlanItemRepository) Create(ctx context.Context, entity *planitem.PlanItem) error {
	var id int64
	if err := r.db.QueryRowContext(ctx, planItemInsert, planItemInsertArgs(entity)...).Scan(&id); err != nil {
		return fmt.Errorf("failed to create plan item: %w", err)
	}
	rehydratePlanItem(entity, id)
	return nil
}

// CreateBatch persists a whole cascade chain in one transaction, in slice
// order. Each item carrying a pending parent index has its parent stamped from
// the ID assigned to that earlier item, so the chain is written before any of
// its IDs exist. Any failure rolls the entire batch back: a half-written
// cascade would under-plan production silently.
func (r *PlanItemRepository) CreateBatch(ctx context.Context, items []*planitem.PlanItem) error {
	if len(items) == 0 {
		return nil
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		ids := make([]int64, 0, len(items))
		for i, entity := range items {
			if idx := entity.PendingParentIndex(); idx != nil {
				if *idx >= len(ids) {
					return fmt.Errorf("failed to create plan item batch: item %d references parent %d not yet written", i, *idx)
				}
				entity.ResolvePendingParent(ids[*idx])
			}
			var id int64
			if err := tx.QueryRowContext(ctx, planItemInsert, planItemInsertArgs(entity)...).Scan(&id); err != nil {
				return fmt.Errorf("failed to create plan item %d of %d: %w", i+1, len(items), err)
			}
			rehydratePlanItem(entity, id)
			ids = append(ids, id)
		}
		return nil
	})
}

// GetByID retrieves a plan item by its ID.
func (r *PlanItemRepository) GetByID(ctx context.Context, id int64) (*planitem.PlanItem, error) {
	query := `SELECT ` + planItemColumns + ` FROM production_plan_item WHERE ppi_id = $1`
	var dto planItemDTO
	if err := r.db.QueryRowContext(ctx, query, id).Scan(dto.dest()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, planitem.ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan plan item: %w", err)
	}
	return dto.toEntity(), nil
}

// List retrieves plan items with filtering and pagination.
func (r *PlanItemRepository) List(ctx context.Context, filter planitem.ListFilter) ([]*planitem.PlanItem, int64, error) {
	filter.Validate()

	base := `FROM production_plan_item WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if filter.Month != "" {
		base += fmt.Sprintf(` AND ppi_month = $%d`, idx)
		args = append(args, filter.Month)
		idx++
	}
	if filter.Type != "" {
		base += fmt.Sprintf(` AND ppi_type = $%d`, idx)
		args = append(args, filter.Type)
		idx++
	}
	if filter.Status != "" {
		base += fmt.Sprintf(` AND ppi_status = $%d`, idx)
		args = append(args, filter.Status)
		idx++
	}
	if filter.MachineGroupID != nil {
		base += fmt.Sprintf(` AND ppi_machine_group_id = $%d`, idx)
		args = append(args, *filter.MachineGroupID)
		idx++
	}
	if filter.DemandID != nil {
		base += fmt.Sprintf(` AND ppi_demand_id = $%d`, idx)
		args = append(args, *filter.DemandID)
		idx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND ppi_notes ILIKE $%d`, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count plan items: %w", err)
	}

	sortColumnMap := map[string]string{
		"deadline":     "ppi_deadline",
		"product_code": "ppi_cpm_product_sys_id",
		"sequence":     "ppi_sequence",
		"status":       "ppi_status",
		"created_at":   "ppi_created_at",
	}
	orderCol := "ppi_sequence"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + planItemColumns + ` ` + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, sortDirection(filter.SortOrder), idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list plan items: %w", err)
	}
	defer closeRows(rows)

	result, err := scanPlanItemRows(rows)
	if err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// Update persists changes to a plan item and appends log entries in one tx.
func (r *PlanItemRepository) Update(ctx context.Context, entity *planitem.PlanItem, changes []planitem.LogEntry) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		query := `
			UPDATE production_plan_item SET
				ppi_qty_target = $2, ppi_deadline = $3, ppi_rm_source = NULLIF($4, '')::VARCHAR,
				ppi_sequence = $5, ppi_status = $6, ppi_machine_group_id = $7,
				ppi_preferred_machine_id = $8::BIGINT, ppi_notes = NULLIF($9, '')::TEXT, ppi_updated_at = $10,
				ppi_month = $11, ppi_planned_start_date = $12::DATE,
				ppi_planned_duration_days = $13::INT, ppi_duration_source = $14
			WHERE ppi_id = $1`
		res, err := tx.ExecContext(ctx, query,
			entity.ID(), entity.QtyTarget(), entity.Deadline(), entity.RMSource(),
			entity.Sequence(), entity.Status(), entity.MachineGroupID(),
			int64PtrArg(entity.PreferredMachineID()), entity.Notes(), entity.UpdatedAt(),
			entity.Month(), timePtrArg(entity.PlannedStartDate()),
			int32PtrArg(entity.PlannedDurationDays()), entity.DurationSource(),
		)
		if err != nil {
			return fmt.Errorf("failed to update plan item: %w", err)
		}
		if err := checkAffected(res, planitem.ErrNotFound); err != nil {
			return err
		}
		return insertPlanLogs(ctx, tx, entity.ID(), changes)
	})
}

func insertPlanLogs(ctx context.Context, tx *sql.Tx, planItemID int64, changes []planitem.LogEntry) error {
	const q = `INSERT INTO production_plan_log (
		ppl_plan_item_id, ppl_field_changed, ppl_value_before, ppl_value_after, ppl_changed_by, ppl_reason
	) VALUES ($1, $2, NULLIF($3, '')::TEXT, NULLIF($4, '')::TEXT, $5, NULLIF($6, '')::TEXT)`
	for _, c := range changes {
		if _, err := tx.ExecContext(ctx, q, planItemID, c.Field, c.Before, c.After, c.ChangedBy, c.Reason); err != nil {
			return fmt.Errorf("failed to insert plan log: %w", err)
		}
	}
	return nil
}

// Delete removes a plan item by its ID.
func (r *PlanItemRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM production_plan_item WHERE ppi_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete plan item: %w", err)
	}
	return checkAffected(res, planitem.ErrNotFound)
}

// ganttColumns selects the core plan-item columns (qualified to ppi) plus the
// Gantt-only decoration columns joined from machine_group/work_order/machine.
const ganttColumns = `ppi.ppi_id, ppi.ppi_cpm_product_sys_id, ppi.ppi_type, ppi.ppi_demand_id, ppi.ppi_parent_item_id,
	ppi.ppi_qty_target, ppi.ppi_deadline, ppi.ppi_rm_source, ppi.ppi_sequence, ppi.ppi_status, ppi.ppi_machine_group_id,
	ppi.ppi_preferred_machine_id, ppi.ppi_month, ppi.ppi_notes, ppi.ppi_created_by, ppi.ppi_created_at, ppi.ppi_updated_at,
	ppi.ppi_planned_start_date, ppi.ppi_planned_duration_days, ppi.ppi_duration_source,
	ppi.ppi_shade_code, ppi.ppi_shade_name, ppi.ppi_carry_from_item_id, ppi.ppi_carry_action,
	COALESCE(mg.group_area, ''), COALESCE(m.machine_no, ''), COALESCE(wo.wo_id, 0), COALESCE(wo.wo_lot_no, ''),
	EXISTS (SELECT 1 FROM changeover_event ce WHERE ce.ce_to_wo_id = wo.wo_id)`

// ListForGantt retrieves plan items for a month/area window for the Gantt
// view, LEFT JOINed to machine_group (area), the linked work_order (machine,
// WO id, lot) and machine (machine_no), plus a changeover_event existence
// check keyed on the linked WO being a changeover target.
func (r *PlanItemRepository) ListForGantt(ctx context.Context, filter planitem.GanttFilter) ([]*planitem.GanttRow, error) {
	args := []interface{}{filter.Month}
	idx := 2

	// The area predicate belongs in the LEFT JOIN ON clause: in the WHERE clause
	// it degrades the LEFT JOIN to an INNER JOIN and silently drops plan items
	// that have no machine group yet (or whose group has a blank group_area).
	groupJoinCond := `mg.group_id = ppi.ppi_machine_group_id`
	groupAreaWhere := ``
	if filter.Area != "" {
		groupJoinCond += fmt.Sprintf(` AND mg.group_area = $%d`, idx)
		args = append(args, filter.Area)
		idx++
		// Keep plan items that are not yet assigned to a machine group, but drop
		// the ones whose group exists and belongs to a different area.
		groupAreaWhere = ` AND (ppi.ppi_machine_group_id IS NULL OR mg.group_id IS NOT NULL)`
	}

	base := `FROM production_plan_item ppi
		LEFT JOIN machine_group mg ON ` + groupJoinCond + `
		LEFT JOIN work_order wo ON wo.wo_plan_item_id = ppi.ppi_id
		LEFT JOIN machine m ON m.machine_id = wo.wo_machine_id
		WHERE ppi.ppi_month = $1` + groupAreaWhere
	if filter.MachineGroupID != nil {
		base += fmt.Sprintf(` AND ppi.ppi_machine_group_id = $%d`, idx)
		args = append(args, *filter.MachineGroupID)
		idx++
	}
	if filter.FromDate != nil {
		base += fmt.Sprintf(` AND ppi.ppi_deadline >= $%d`, idx)
		args = append(args, *filter.FromDate)
		idx++
	}
	if filter.ToDate != nil {
		base += fmt.Sprintf(` AND ppi.ppi_deadline <= $%d`, idx)
		args = append(args, *filter.ToDate)
	}
	base += ` ORDER BY ppi.ppi_machine_group_id, ppi.ppi_sequence, ppi.ppi_deadline`

	query := `SELECT ` + ganttColumns + ` ` + base
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list plan items for gantt: %w", err)
	}
	defer closeRows(rows)
	return scanGanttRows(rows)
}

func scanGanttRows(rows *sql.Rows) ([]*planitem.GanttRow, error) {
	var result []*planitem.GanttRow
	for rows.Next() {
		var dto planItemDTO
		var area, machineNo, lotNo string
		var woID int64
		var isChangeover bool
		dest := append(dto.dest(), &area, &machineNo, &woID, &lotNo, &isChangeover)
		if err := rows.Scan(dest...); err != nil {
			return nil, fmt.Errorf("failed to scan gantt row: %w", err)
		}
		result = append(result, &planitem.GanttRow{
			Item:         dto.toEntity(),
			Area:         area,
			MachineNo:    machineNo,
			WoID:         woID,
			LotNo:        lotNo,
			IsChangeover: isChangeover,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating gantt rows: %w", err)
	}
	return result, nil
}

func scanPlanItemRows(rows *sql.Rows) ([]*planitem.PlanItem, error) {
	var result []*planitem.PlanItem
	for rows.Next() {
		var dto planItemDTO
		if err := rows.Scan(dto.dest()...); err != nil {
			return nil, fmt.Errorf("failed to scan plan item row: %w", err)
		}
		result = append(result, dto.toEntity())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating plan item rows: %w", err)
	}
	return result, nil
}

type planItemDTO struct {
	ID                 int64
	CpmProductSysID    int64
	Type               string
	DemandID           sql.NullInt64
	ParentItemID       sql.NullInt64
	QtyTarget          float64
	Deadline           time.Time
	RMSource           sql.NullString
	Sequence           int32
	Status             string
	MachineGroupID     int64
	PreferredMachineID sql.NullInt64
	Month              string
	Notes              sql.NullString
	CreatedBy          int64
	CreatedAt          time.Time
	UpdatedAt          time.Time
	PlannedStartDate   sql.NullTime
	PlannedDuration    sql.NullInt32
	DurationSource     string
	ShadeCode          sql.NullString
	ShadeName          sql.NullString
	CarryFromItemID    sql.NullInt64
	CarryAction        sql.NullString
}

func (d *planItemDTO) dest() []interface{} {
	return []interface{}{
		&d.ID, &d.CpmProductSysID, &d.Type, &d.DemandID, &d.ParentItemID,
		&d.QtyTarget, &d.Deadline, &d.RMSource, &d.Sequence, &d.Status, &d.MachineGroupID,
		&d.PreferredMachineID, &d.Month, &d.Notes, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
		&d.PlannedStartDate, &d.PlannedDuration, &d.DurationSource,
		&d.ShadeCode, &d.ShadeName, &d.CarryFromItemID, &d.CarryAction,
	}
}

func (d *planItemDTO) toEntity() *planitem.PlanItem {
	return planitem.Reconstruct(planitem.ReconstructParams{
		ID:                 d.ID,
		CpmProductSysID:    d.CpmProductSysID,
		Type:               d.Type,
		DemandID:           nullInt64Ptr(d.DemandID),
		ParentItemID:       nullInt64Ptr(d.ParentItemID),
		QtyTarget:          d.QtyTarget,
		Deadline:           d.Deadline,
		RMSource:           nullString(d.RMSource),
		Sequence:           d.Sequence,
		Status:             d.Status,
		MachineGroupID:     d.MachineGroupID,
		PreferredMachineID: nullInt64Ptr(d.PreferredMachineID),
		Month:              d.Month,
		Notes:              nullString(d.Notes),
		CreatedBy:          d.CreatedBy,
		CreatedAt:          d.CreatedAt,
		UpdatedAt:          d.UpdatedAt,
		PlannedStartDate:   nullTimePtr(d.PlannedStartDate),
		PlannedDuration:    nullInt32Ptr(d.PlannedDuration),
		DurationSource:     d.DurationSource,
		ShadeCode:          nullString(d.ShadeCode),
		ShadeName:          nullString(d.ShadeName),
		CarryFromItemID:    nullInt64Ptr(d.CarryFromItemID),
		CarryAction:        nullString(d.CarryAction),
	})
}

func rehydratePlanItem(entity *planitem.PlanItem, id int64) {
	*entity = *planitem.Reconstruct(planitem.ReconstructParams{
		ID:                 id,
		CpmProductSysID:    entity.CpmProductSysID(),
		Type:               entity.Type(),
		DemandID:           entity.DemandID(),
		ParentItemID:       entity.ParentItemID(),
		QtyTarget:          entity.QtyTarget(),
		Deadline:           entity.Deadline(),
		RMSource:           entity.RMSource(),
		Sequence:           entity.Sequence(),
		Status:             entity.Status(),
		MachineGroupID:     entity.MachineGroupID(),
		PreferredMachineID: entity.PreferredMachineID(),
		Month:              entity.Month(),
		Notes:              entity.Notes(),
		CreatedBy:          entity.CreatedBy(),
		CreatedAt:          entity.CreatedAt(),
		UpdatedAt:          entity.UpdatedAt(),
		PlannedStartDate:   entity.PlannedStartDate(),
		PlannedDuration:    entity.PlannedDurationDays(),
		DurationSource:     entity.DurationSource(),
		ShadeCode:          entity.ShadeCode(),
		ShadeName:          entity.ShadeName(),
		CarryFromItemID:    entity.CarryFromItemID(),
		CarryAction:        entity.CarryAction(),
	})
}
