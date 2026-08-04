package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// WorkOrderRepository implements workorder.Repository using PostgreSQL (v1.2).
type WorkOrderRepository struct {
	db *DB
}

// NewWorkOrderRepository creates a new WorkOrderRepository.
func NewWorkOrderRepository(db *DB) *WorkOrderRepository {
	return &WorkOrderRepository{db: db}
}

var _ workorder.Repository = (*WorkOrderRepository)(nil)

const woColumns = `wo_id, wo_no, wo_lot_no, wo_area, wo_machine_id, wo_crh_head_id, wo_crh_version,
	wo_plan_item_id, wo_demand_id, wo_ref_wo_id, wo_ref_type, wo_qty_target, wo_grade_requirement,
	wo_deadline, wo_prod_category, wo_spec_snapshot, wo_packing_snapshot, wo_revision_no, wo_revision_reason,
	wo_status, wo_auto_approve_disabled, wo_pc_approved_at, wo_pc_approved_by, wo_pm_approved_at, wo_pm_approved_by,
	wo_plan_change_flag, wo_plan_change_note, wo_created_by, wo_created_at, wo_updated_at`

// Create persists a new WO header and assigns its generated ID.
//
// The header, its plan-item links and any parameters attached to the entity go
// in together: a WO whose links were lost would silently look like an un-merged
// single-item order, and one whose parameters were lost gives the PC operator an
// empty sheet with nothing to say why.
func (r *WorkOrderRepository) Create(ctx context.Context, entity *workorder.WorkOrder) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		return insertWorkOrderTx(ctx, tx, entity)
	})
}

// insertWorkOrderTx inserts a WO header plus its plan-item links and parameters
// inside an existing transaction. The lot provisioner reuses it so that minting
// a lot number, registering the lot, and creating the WO all commit or roll back
// together.
func insertWorkOrderTx(ctx context.Context, tx *sql.Tx, entity *workorder.WorkOrder) error {
	specJSON, err := marshalSnapshot(entity.SpecSnapshot())
	if err != nil {
		return err
	}
	packingJSON, err := marshalSnapshot(entity.PackingSnapshot())
	if err != nil {
		return err
	}
	query := `
		INSERT INTO work_order (
			wo_no, wo_lot_no, wo_area, wo_machine_id, wo_crh_head_id, wo_crh_version,
			wo_plan_item_id, wo_demand_id, wo_ref_wo_id, wo_ref_type, wo_qty_target, wo_grade_requirement,
			wo_deadline, wo_prod_category, wo_spec_snapshot, wo_packing_snapshot, wo_revision_no, wo_revision_reason,
			wo_status, wo_auto_approve_disabled, wo_plan_change_flag, wo_created_by, wo_created_at, wo_updated_at
		) VALUES (
			$1, $2, $3, $4, $5, $6,
			$7, $8::BIGINT, $9::BIGINT, NULLIF($10, ''), $11, NULLIF($12, ''),
			$13, $14, $15::JSONB, $16::JSONB, $17, NULLIF($18, ''),
			$19, $20, $21, $22, $23, $24
		) RETURNING wo_id`
	var id int64
	err = tx.QueryRowContext(ctx, query,
		entity.WoNo(), entity.LotNo(), entity.AreaCode(), entity.MachineID(), entity.CrhHeadID(), entity.CrhVersion(),
		entity.PlanItemID(), int64PtrArg(entity.DemandID()), int64PtrArg(entity.RefWoID()), entity.RefType(), entity.QtyTarget(), entity.GradeRequirement(),
		entity.Deadline(), entity.ProdCategory(), specJSON, packingJSON, entity.RevisionNo(), entity.RevisionReason(),
		entity.Status(), entity.AutoApproveDisabled(), entity.PlanChangeFlag(), entity.CreatedBy(), entity.CreatedAt(), entity.UpdatedAt(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("failed to create work order: duplicate wo_no/lot_no: %w", err)
		}
		return fmt.Errorf("failed to create work order: %w", err)
	}
	entity.SetID(id)
	if err := replacePlanItemLinksTx(ctx, tx, id, entity.PlanItemLinks()); err != nil {
		return err
	}
	return insertParametersTx(ctx, tx, id, entity.Parameters())
}

// replacePlanItemLinksTx rewrites the wo_plan_item_link rows of one WO inside
// an existing transaction. Delete-then-insert keeps the create and update paths
// symmetric: both end with exactly the links the entity carries.
//
// uq_wpl_wo_plan_item stops the same plan item joining the same WO twice;
// uq_wpl_plan_item (migration 000033) stops it joining TWO work orders. Either
// violation surfaces as the domain's already-linked error rather than a raw
// driver error.
func replacePlanItemLinksTx(ctx context.Context, tx *sql.Tx, woID int64, links []workorder.PlanItemLink) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM wo_plan_item_link WHERE wpl_wo_id = $1`, woID); err != nil {
		return fmt.Errorf("failed to clear work order plan item links: %w", err)
	}
	for _, l := range links {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO wo_plan_item_link (wpl_wo_id, wpl_plan_item_id, wpl_qty_contribution)
			 VALUES ($1, $2, $3)`,
			woID, l.PlanItemID, l.QtyContribution)
		if err != nil {
			if isUniqueViolation(err) {
				return workorder.ErrPlanItemAlreadyLinked
			}
			return fmt.Errorf("failed to link plan item to work order: %w", err)
		}
	}
	return nil
}

// ReplacePlanItemLinks rewrites the plan items a WO covers.
func (r *WorkOrderRepository) ReplacePlanItemLinks(ctx context.Context, woID int64, links []workorder.PlanItemLink) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		return replacePlanItemLinksTx(ctx, tx, woID, links)
	})
}

// ListPlanItemLinks lists the plan items a WO covers, anchor included.
func (r *WorkOrderRepository) ListPlanItemLinks(ctx context.Context, woID int64) ([]workorder.PlanItemLink, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT wpl_id, wpl_wo_id, wpl_plan_item_id, COALESCE(wpl_qty_contribution, 0)
		 FROM wo_plan_item_link WHERE wpl_wo_id = $1 ORDER BY wpl_id`, woID)
	if err != nil {
		return nil, fmt.Errorf("failed to list work order plan item links: %w", err)
	}
	defer closeRows(rows)

	links := []workorder.PlanItemLink{}
	for rows.Next() {
		var l workorder.PlanItemLink
		if err := rows.Scan(&l.ID, &l.WOID, &l.PlanItemID, &l.QtyContribution); err != nil {
			return nil, fmt.Errorf("failed to scan work order plan item link: %w", err)
		}
		links = append(links, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate work order plan item links: %w", err)
	}
	return links, nil
}

// GetByID retrieves a WO header with its parameters + RM allocations.
func (r *WorkOrderRepository) GetByID(ctx context.Context, id int64) (*workorder.WorkOrder, error) {
	query := `SELECT ` + woColumns + ` FROM work_order WHERE wo_id = $1`
	var dto woDTO
	if err := r.db.QueryRowContext(ctx, query, id).Scan(dto.dest()...); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, workorder.ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan work order: %w", err)
	}
	entity, err := dto.toEntity()
	if err != nil {
		return nil, err
	}
	params, err := r.ListParameters(ctx, id)
	if err != nil {
		return nil, err
	}
	entity.AttachParameters(params)
	allocs, err := r.ListRmAllocations(ctx, id)
	if err != nil {
		return nil, err
	}
	entity.AttachRmAllocations(allocs)
	links, err := r.ListPlanItemLinks(ctx, id)
	if err != nil {
		return nil, err
	}
	entity.AttachPlanItemLinks(links)
	return entity, nil
}

// List retrieves WOs with filtering and pagination.
func (r *WorkOrderRepository) List(ctx context.Context, filter workorder.ListFilter) ([]*workorder.WorkOrder, int64, error) {
	filter.Validate()

	base := `FROM work_order WHERE 1=1`
	args := []interface{}{}
	idx := 1
	if filter.Area != "" {
		base += fmt.Sprintf(` AND wo_area = $%d`, idx)
		args = append(args, filter.Area)
		idx++
	}
	if filter.Status != "" {
		base += fmt.Sprintf(` AND wo_status = $%d`, idx)
		args = append(args, filter.Status)
		idx++
	}
	if filter.MachineID != nil {
		base += fmt.Sprintf(` AND wo_machine_id = $%d`, idx)
		args = append(args, *filter.MachineID)
		idx++
	}
	if filter.PlanItemID != nil {
		base += fmt.Sprintf(` AND wo_plan_item_id = $%d`, idx)
		args = append(args, *filter.PlanItemID)
		idx++
	}
	if filter.LotNo != "" {
		base += fmt.Sprintf(` AND wo_lot_no = $%d`, idx)
		args = append(args, filter.LotNo)
		idx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND (wo_no ILIKE $%d OR wo_lot_no ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count work orders: %w", err)
	}

	sortColumnMap := map[string]string{
		"wo_no":      "wo_no",
		"lot_no":     "wo_lot_no",
		"deadline":   "wo_deadline",
		"machine_no": "wo_machine_id",
		"status":     "wo_status",
		"created_at": "wo_created_at",
	}
	orderCol := "wo_created_at"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + woColumns + ` ` + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, sortDirection(filter.SortOrder), idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list work orders: %w", err)
	}
	defer closeRows(rows)

	result, err := scanWORows(rows)
	if err != nil {
		return nil, 0, err
	}
	return result, total, nil
}

// Update persists changes to a WO header (incl. snapshots + approval audit).
func (r *WorkOrderRepository) Update(ctx context.Context, entity *workorder.WorkOrder) error {
	specJSON, err := marshalSnapshot(entity.SpecSnapshot())
	if err != nil {
		return err
	}
	packingJSON, err := marshalSnapshot(entity.PackingSnapshot())
	if err != nil {
		return err
	}
	query := `
		UPDATE work_order SET
			wo_machine_id = $2, wo_lot_no = $3, wo_qty_target = $4, wo_grade_requirement = NULLIF($5, ''),
			wo_deadline = $6, wo_prod_category = $7, wo_spec_snapshot = $8::JSONB, wo_packing_snapshot = $9::JSONB,
			wo_revision_no = $10, wo_revision_reason = NULLIF($11, ''), wo_status = $12, wo_auto_approve_disabled = $13,
			wo_pc_approved_at = $14::TIMESTAMPTZ, wo_pc_approved_by = $15::BIGINT,
			wo_pm_approved_at = $16::TIMESTAMPTZ, wo_pm_approved_by = $17::BIGINT,
			wo_plan_change_flag = $18, wo_plan_change_note = NULLIF($19, ''), wo_updated_at = $20
		WHERE wo_id = $1`
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, query,
			entity.ID(), entity.MachineID(), entity.LotNo(), entity.QtyTarget(), entity.GradeRequirement(),
			entity.Deadline(), entity.ProdCategory(), specJSON, packingJSON,
			entity.RevisionNo(), entity.RevisionReason(), entity.Status(), entity.AutoApproveDisabled(),
			timePtrArg(entity.PCApprovedAt()), int64PtrArg(entity.PCApprovedBy()),
			timePtrArg(entity.PMApprovedAt()), int64PtrArg(entity.PMApprovedBy()),
			entity.PlanChangeFlag(), entity.PlanChangeNote(), entity.UpdatedAt(),
		)
		if err != nil {
			return fmt.Errorf("failed to update work order: %w", err)
		}
		if err := checkAffected(res, workorder.ErrNotFound); err != nil {
			return err
		}
		// Links are rewritten only when the entity carries them. An entity
		// loaded by GetByID always does; a header-only reconstruction does not,
		// and must not silently unlink the WO's plan items.
		if links := entity.PlanItemLinks(); len(links) > 0 {
			return replacePlanItemLinksTx(ctx, tx, entity.ID(), links)
		}
		return nil
	})
}

// Delete removes a WO and its child rows by its ID.
func (r *WorkOrderRepository) Delete(ctx context.Context, id int64) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for _, child := range []string{
			`DELETE FROM wo_parameter WHERE wop_wo_id = $1`,
			`DELETE FROM wo_execution WHERE woe_wo_id = $1`,
			`DELETE FROM wo_rm_allocation WHERE wra_wo_id = $1`,
			`DELETE FROM wo_actual_log WHERE wal_wo_id = $1`,
			`DELETE FROM wo_plan_item_link WHERE wpl_wo_id = $1`,
			`DELETE FROM wo_production_actual WHERE wpa_wo_id = $1`,
		} {
			if _, err := tx.ExecContext(ctx, child, id); err != nil {
				return fmt.Errorf("failed to delete work order children: %w", err)
			}
		}
		res, err := tx.ExecContext(ctx, `DELETE FROM work_order WHERE wo_id = $1`, id)
		if err != nil {
			return fmt.Errorf("failed to delete work order: %w", err)
		}
		return checkAffected(res, workorder.ErrNotFound)
	})
}

// ListPendingApprovals lists submitted/PC-approved WOs still awaiting approval
// that were updated at or before cutoff.
func (r *WorkOrderRepository) ListPendingApprovals(ctx context.Context, cutoff time.Time) ([]*workorder.WorkOrder, error) {
	query := `SELECT ` + woColumns + ` FROM work_order
		WHERE wo_status IN ('SUBMITTED', 'PC_APPROVED')
		  AND wo_updated_at <= $1
		ORDER BY wo_id`
	rows, err := r.db.QueryContext(ctx, query, cutoff)
	if err != nil {
		return nil, fmt.Errorf("failed to list pending approvals: %w", err)
	}
	defer closeRows(rows)
	return scanWORows(rows)
}

// MaxRevisionNo returns the highest revision number for a WO revision chain.
func (r *WorkOrderRepository) MaxRevisionNo(ctx context.Context, refID int64) (int32, error) {
	var maxNo sql.NullInt32
	err := r.db.QueryRowContext(ctx,
		`SELECT MAX(wo_revision_no) FROM work_order WHERE wo_ref_wo_id = $1 OR wo_id = $1`, refID).Scan(&maxNo)
	if err != nil {
		return 0, fmt.Errorf("failed to get max revision no: %w", err)
	}
	return maxNo.Int32, nil
}

func scanWORows(rows *sql.Rows) ([]*workorder.WorkOrder, error) {
	var result []*workorder.WorkOrder
	for rows.Next() {
		var dto woDTO
		if err := rows.Scan(dto.dest()...); err != nil {
			return nil, fmt.Errorf("failed to scan work order row: %w", err)
		}
		entity, err := dto.toEntity()
		if err != nil {
			return nil, err
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating work order rows: %w", err)
	}
	return result, nil
}

type woDTO struct {
	ID                  int64
	WoNo                string
	LotNo               string
	Area                string
	MachineID           int64
	CrhHeadID           int64
	CrhVersion          int32
	PlanItemID          int64
	DemandID            sql.NullInt64
	RefWoID             sql.NullInt64
	RefType             sql.NullString
	QtyTarget           float64
	GradeRequirement    sql.NullString
	Deadline            time.Time
	ProdCategory        string
	SpecSnapshot        []byte
	PackingSnapshot     []byte
	RevisionNo          int32
	RevisionReason      sql.NullString
	Status              string
	AutoApproveDisabled bool
	PCApprovedAt        sql.NullTime
	PCApprovedBy        sql.NullInt64
	PMApprovedAt        sql.NullTime
	PMApprovedBy        sql.NullInt64
	PlanChangeFlag      bool
	PlanChangeNote      sql.NullString
	CreatedBy           int64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

func (d *woDTO) dest() []interface{} {
	return []interface{}{
		&d.ID, &d.WoNo, &d.LotNo, &d.Area, &d.MachineID, &d.CrhHeadID, &d.CrhVersion,
		&d.PlanItemID, &d.DemandID, &d.RefWoID, &d.RefType, &d.QtyTarget, &d.GradeRequirement,
		&d.Deadline, &d.ProdCategory, &d.SpecSnapshot, &d.PackingSnapshot, &d.RevisionNo, &d.RevisionReason,
		&d.Status, &d.AutoApproveDisabled, &d.PCApprovedAt, &d.PCApprovedBy, &d.PMApprovedAt, &d.PMApprovedBy,
		&d.PlanChangeFlag, &d.PlanChangeNote, &d.CreatedBy, &d.CreatedAt, &d.UpdatedAt,
	}
}

func (d *woDTO) toEntity() (*workorder.WorkOrder, error) {
	spec, err := unmarshalSnapshot(d.SpecSnapshot)
	if err != nil {
		return nil, err
	}
	packing, err := unmarshalSnapshot(d.PackingSnapshot)
	if err != nil {
		return nil, err
	}
	return workorder.Reconstruct(workorder.ReconstructParams{
		ID:                  d.ID,
		WoNo:                d.WoNo,
		LotNo:               d.LotNo,
		AreaCode:            d.Area,
		MachineID:           d.MachineID,
		CrhHeadID:           d.CrhHeadID,
		CrhVersion:          d.CrhVersion,
		PlanItemID:          d.PlanItemID,
		DemandID:            nullInt64Ptr(d.DemandID),
		RefWoID:             nullInt64Ptr(d.RefWoID),
		RefType:             nullString(d.RefType),
		QtyTarget:           d.QtyTarget,
		GradeRequirement:    nullString(d.GradeRequirement),
		Deadline:            d.Deadline,
		ProdCategory:        d.ProdCategory,
		SpecSnapshot:        spec,
		PackingSnapshot:     packing,
		RevisionNo:          d.RevisionNo,
		RevisionReason:      nullString(d.RevisionReason),
		Status:              d.Status,
		AutoApproveDisabled: d.AutoApproveDisabled,
		PCApprovedBy:        nullInt64Ptr(d.PCApprovedBy),
		PCApprovedAt:        nullTimePtr(d.PCApprovedAt),
		PMApprovedBy:        nullInt64Ptr(d.PMApprovedBy),
		PMApprovedAt:        nullTimePtr(d.PMApprovedAt),
		PlanChangeFlag:      d.PlanChangeFlag,
		PlanChangeNote:      nullString(d.PlanChangeNote),
		CreatedBy:           d.CreatedBy,
		CreatedAt:           d.CreatedAt,
		UpdatedAt:           d.UpdatedAt,
	}), nil
}

// marshalSnapshot encodes a snapshot map to JSON bytes (nil map → nil, stored as
// SQL NULL via the ::JSONB cast).
func marshalSnapshot(m map[string]any) ([]byte, error) {
	if m == nil {
		return nil, nil
	}
	b, err := json.Marshal(m)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal snapshot: %w", err)
	}
	return b, nil
}

// unmarshalSnapshot decodes JSONB bytes to a snapshot map (empty → nil).
func unmarshalSnapshot(b []byte) (map[string]any, error) {
	if len(b) == 0 {
		return nil, nil //nolint:nilnil // absent snapshot is a valid nil map, not an error
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("failed to unmarshal snapshot: %w", err)
	}
	return m, nil
}
