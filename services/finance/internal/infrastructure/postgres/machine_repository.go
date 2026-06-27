// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/machine"
)

// MachineRepository implements machine.Repository using PostgreSQL.
type MachineRepository struct {
	db *DB
}

// NewMachineRepository creates a new MachineRepository.
func NewMachineRepository(db *DB) *MachineRepository {
	return &MachineRepository{db: db}
}

// Verify interface implementation at compile time.
var _ machine.Repository = (*MachineRepository)(nil)

// Create persists a new machine.
func (r *MachineRepository) Create(ctx context.Context, entity *machine.Entity) error {
	query := `
		INSERT INTO mst_machine (
			mc_id, mc_code, mc_name, mc_type, mc_location,
			no_of_position, no_of_end, mc_speed, machine_rpm,
			mc_efficiency, power_per_day,
			mp_per_day, ohs_per_day, spares_per_day, kgs_lost_change,
			vb1_qty, vb2_qty, vb3_qty, vb4_qty, vb5_qty,
			mc_poy_bobbin_weight, mc_tot_fxd_cst, mc_bobbin_per_trolly,
			mc_box_cost, mc_captive_per_bobbin, mc_weightage,
			is_active, notes,
			created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30)
	`
	_, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Code(),
		entity.Name(),
		nullableString(entity.MCType()),
		nullableString(entity.Location()),
		entity.NoOfPosition(),
		entity.NoOfEnd(),
		entity.MCSpeed(),
		entity.MachineRPM(),
		entity.MCEfficiency(),
		entity.PowerPerDay(),
		entity.MpPerDay(),
		entity.OhsPerDay(),
		entity.SparesPerDay(),
		entity.KgsLostChange(),
		entity.Vb1Qty(),
		entity.Vb2Qty(),
		entity.Vb3Qty(),
		entity.Vb4Qty(),
		entity.Vb5Qty(),
		entity.McPoyBobbinWeight(),
		entity.McTotFxdCst(),
		entity.McBobbinPerTrolly(),
		entity.McBoxCost(),
		entity.McCaptivePerBobbin(),
		entity.McWeightage(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isMachineUniqueViolation(err) {
			return machine.ErrAlreadyExists
		}
		return fmt.Errorf("create machine: %w", err)
	}
	return nil
}

// GetByID retrieves a machine by its UUID primary key.
func (r *MachineRepository) GetByID(ctx context.Context, id uuid.UUID) (*machine.Entity, error) {
	query := r.selectCols() + ` WHERE mc_id = $1 AND deleted_at IS NULL`
	return r.scanOne(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a machine by its unique code.
func (r *MachineRepository) GetByCode(ctx context.Context, code string) (*machine.Entity, error) {
	query := r.selectCols() + ` WHERE mc_code = $1 AND deleted_at IS NULL`
	return r.scanOne(r.db.QueryRowContext(ctx, query, code))
}

// List retrieves machines with filtering, searching, and pagination.
func (r *MachineRepository) List(ctx context.Context, filter machine.ListFilter) ([]*machine.Entity, int64, error) {
	filter.Validate()

	base := whereNotDeleted
	args := make([]any, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (mc_code ILIKE $%d OR mc_name ILIKE $%d OR mc_type ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.MCType != "" {
		base += fmt.Sprintf(` AND mc_type = $%d`, idx)
		args = append(args, filter.MCType)
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mst_machine "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count machines: %w", err)
	}

	orderCol := r.resolveSort(filter.SortBy)
	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}

	q := r.selectCols() + base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, dir, idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list machines: %w", err)
	}
	var entities []*machine.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, 0, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, 0, scanErr
		}
		entities = append(entities, e)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, 0, fmt.Errorf("close machine rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate machines: %w", err)
	}
	return entities, total, nil
}

// Update persists changes to an existing machine.
func (r *MachineRepository) Update(ctx context.Context, entity *machine.Entity) error {
	query := `
		UPDATE mst_machine SET
			mc_name              = $2,
			mc_type              = $3,
			mc_location          = $4,
			no_of_position       = $5,
			no_of_end            = $6,
			mc_speed             = $7,
			machine_rpm          = $8,
			mc_efficiency        = $9,
			power_per_day        = $10,
			mp_per_day           = $11,
			ohs_per_day          = $12,
			spares_per_day       = $13,
			kgs_lost_change      = $14,
			vb1_qty              = $15,
			vb2_qty              = $16,
			vb3_qty              = $17,
			vb4_qty              = $18,
			vb5_qty              = $19,
			mc_poy_bobbin_weight = $20,
			mc_tot_fxd_cst       = $21,
			mc_bobbin_per_trolly = $22,
			mc_box_cost          = $23,
			mc_captive_per_bobbin = $24,
			mc_weightage         = $25,
			is_active            = $26,
			notes                = $27,
			updated_at           = $28,
			updated_by           = $29
		WHERE mc_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		nullableString(entity.MCType()),
		nullableString(entity.Location()),
		entity.NoOfPosition(),
		entity.NoOfEnd(),
		entity.MCSpeed(),
		entity.MachineRPM(),
		entity.MCEfficiency(),
		entity.PowerPerDay(),
		entity.MpPerDay(),
		entity.OhsPerDay(),
		entity.SparesPerDay(),
		entity.KgsLostChange(),
		entity.Vb1Qty(),
		entity.Vb2Qty(),
		entity.Vb3Qty(),
		entity.Vb4Qty(),
		entity.Vb5Qty(),
		entity.McPoyBobbinWeight(),
		entity.McTotFxdCst(),
		entity.McBobbinPerTrolly(),
		entity.McBoxCost(),
		entity.McCaptivePerBobbin(),
		entity.McWeightage(),
		entity.IsActive(),
		nullableString(entity.Notes()),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update machine: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return machine.ErrNotFound
	}
	return nil
}

// SoftDelete marks a machine as deleted.
func (r *MachineRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_machine SET deleted_at=$2,deleted_by=$3,is_active=false WHERE mc_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete machine: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return machine.ErrNotFound
	}
	return nil
}

// ExistsByCode reports whether a non-deleted machine with the given code exists.
func (r *MachineRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_machine WHERE mc_code=$1 AND deleted_at IS NULL)`, code,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by code: %w", err)
	}
	return exists, nil
}

// ExistsByID reports whether a non-deleted machine with the given UUID exists.
func (r *MachineRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_machine WHERE mc_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *MachineRepository) selectCols() string {
	return `
		SELECT mc_id, mc_code, mc_name, mc_type, mc_location,
		       no_of_position, no_of_end, mc_speed, machine_rpm,
		       mc_efficiency, power_per_day,
		       mp_per_day, ohs_per_day, spares_per_day, kgs_lost_change,
		       vb1_qty, vb2_qty, vb3_qty, vb4_qty, vb5_qty,
		       mc_poy_bobbin_weight, mc_tot_fxd_cst, mc_bobbin_per_trolly,
		       mc_box_cost, mc_captive_per_bobbin, mc_weightage,
		       is_active, notes,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_machine
	`
}

func (r *MachineRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"mc_code": "mc_code", "mc_name": "mc_name", "mc_type": "mc_type",
		"code": "mc_code", "name": "mc_name",
		sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mc_code"
}

type machineDTO struct {
	ID                 uuid.UUID
	Code               string
	Name               string
	MCType             sql.NullString
	Location           sql.NullString
	NoOfPosition       int
	NoOfEnd            int
	MCSpeed            float64
	MachineRPM         sql.NullFloat64
	MCEfficiency       float64
	PowerPerDay        sql.NullFloat64
	MpPerDay           sql.NullFloat64
	OhsPerDay          sql.NullFloat64
	SparesPerDay       sql.NullFloat64
	KgsLostChange      sql.NullFloat64
	Vb1Qty             sql.NullFloat64
	Vb2Qty             sql.NullFloat64
	Vb3Qty             sql.NullFloat64
	Vb4Qty             sql.NullFloat64
	Vb5Qty             sql.NullFloat64
	McPoyBobbinWeight  sql.NullFloat64
	McTotFxdCst        sql.NullFloat64
	McBobbinPerTrolly  sql.NullFloat64
	McBoxCost          sql.NullFloat64
	McCaptivePerBobbin sql.NullFloat64
	McWeightage        sql.NullFloat64
	IsActive           bool
	Notes              sql.NullString
	CreatedAt          time.Time
	CreatedBy          string
	UpdatedAt          sql.NullTime
	UpdatedBy          sql.NullString
	DeletedAt          sql.NullTime
	DeletedBy          sql.NullString
}

func (d *machineDTO) toEntity() *machine.Entity {
	return machine.Reconstruct(
		d.ID,
		d.Code,
		d.Name,
		d.MCType.String,
		d.Location.String,
		d.NoOfPosition,
		d.NoOfEnd,
		d.MCSpeed,
		nullableFloat64Ptr(d.MachineRPM),
		d.MCEfficiency,
		nullableFloat64Ptr(d.PowerPerDay),
		nullableFloat64Ptr(d.MpPerDay),
		nullableFloat64Ptr(d.OhsPerDay),
		nullableFloat64Ptr(d.SparesPerDay),
		nullableFloat64Ptr(d.KgsLostChange),
		nullableFloat64Ptr(d.Vb1Qty),
		nullableFloat64Ptr(d.Vb2Qty),
		nullableFloat64Ptr(d.Vb3Qty),
		nullableFloat64Ptr(d.Vb4Qty),
		nullableFloat64Ptr(d.Vb5Qty),
		nullableFloat64Ptr(d.McPoyBobbinWeight),
		nullableFloat64Ptr(d.McTotFxdCst),
		nullableFloat64Ptr(d.McBobbinPerTrolly),
		nullableFloat64Ptr(d.McBoxCost),
		nullableFloat64Ptr(d.McCaptivePerBobbin),
		nullableFloat64Ptr(d.McWeightage),
		d.IsActive,
		d.Notes.String,
		d.CreatedAt,
		d.CreatedBy,
		nullableTimePtr(d.UpdatedAt),
		nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt),
		nullableStringPtr(d.DeletedBy),
	)
}

func (r *MachineRepository) scanOne(row *sql.Row) (*machine.Entity, error) {
	var d machineDTO
	err := row.Scan(
		&d.ID, &d.Code, &d.Name, &d.MCType, &d.Location,
		&d.NoOfPosition, &d.NoOfEnd, &d.MCSpeed, &d.MachineRPM,
		&d.MCEfficiency, &d.PowerPerDay,
		&d.MpPerDay, &d.OhsPerDay, &d.SparesPerDay, &d.KgsLostChange,
		&d.Vb1Qty, &d.Vb2Qty, &d.Vb3Qty, &d.Vb4Qty, &d.Vb5Qty,
		&d.McPoyBobbinWeight, &d.McTotFxdCst, &d.McBobbinPerTrolly,
		&d.McBoxCost, &d.McCaptivePerBobbin, &d.McWeightage,
		&d.IsActive, &d.Notes,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, machine.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan machine: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MachineRepository) scanRow(rows *sql.Rows) (*machine.Entity, error) {
	var d machineDTO
	err := rows.Scan(
		&d.ID, &d.Code, &d.Name, &d.MCType, &d.Location,
		&d.NoOfPosition, &d.NoOfEnd, &d.MCSpeed, &d.MachineRPM,
		&d.MCEfficiency, &d.PowerPerDay,
		&d.MpPerDay, &d.OhsPerDay, &d.SparesPerDay, &d.KgsLostChange,
		&d.Vb1Qty, &d.Vb2Qty, &d.Vb3Qty, &d.Vb4Qty, &d.Vb5Qty,
		&d.McPoyBobbinWeight, &d.McTotFxdCst, &d.McBobbinPerTrolly,
		&d.McBoxCost, &d.McCaptivePerBobbin, &d.McWeightage,
		&d.IsActive, &d.Notes,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("scan machine row: %w", err)
	}
	return d.toEntity(), nil
}

func isMachineUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
