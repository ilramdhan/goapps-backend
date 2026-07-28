// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/machine"
)

// MachineRepository implements machine.Repository using PostgreSQL.
type MachineRepository struct {
	db *DB
}

// NewMachineRepository creates a new MachineRepository.
func NewMachineRepository(db *DB) *MachineRepository {
	return &MachineRepository{db: db}
}

var _ machine.Repository = (*MachineRepository)(nil)

const machineSelectColumns = `
	m.machine_id, m.machine_no, m.machine_area, m.machine_line, m.machine_group_id,
	g.group_name, m.machine_doff_weight_kg, m.machine_is_active, m.machine_orion_code,
	m.source_mc_id, m.synced_at, m.created_at, m.created_by, m.updated_at, m.updated_by`

// GetByID retrieves a machine by its ID.
func (r *MachineRepository) GetByID(ctx context.Context, id int64) (*machine.Machine, error) {
	query := `SELECT ` + machineSelectColumns + `
		FROM machine m
		LEFT JOIN machine_group g ON m.machine_group_id = g.group_id
		WHERE m.machine_id = $1`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves machines with filtering and pagination.
func (r *MachineRepository) List(ctx context.Context, filter machine.ListFilter) ([]*machine.Machine, int64, error) {
	filter.Validate()

	base := `FROM machine m LEFT JOIN machine_group g ON m.machine_group_id = g.group_id WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (m.machine_no ILIKE $%d OR m.machine_line ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.Area != "" {
		base += fmt.Sprintf(` AND m.machine_area = $%d`, argIdx)
		args = append(args, filter.Area)
		argIdx++
	}
	if filter.MachineGroupID != nil {
		base += fmt.Sprintf(` AND m.machine_group_id = $%d`, argIdx)
		args = append(args, *filter.MachineGroupID)
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND m.machine_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count machines: %w", err)
	}

	sortColumnMap := map[string]string{
		"machine_no": "m.machine_no",
		"area":       "m.machine_area",
		"line":       "m.machine_line",
		"group":      "g.group_name",
		"created_at": "m.created_at",
	}
	orderCol := "m.machine_no"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + machineSelectColumns + ` ` + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
			orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list machines: %w", err)
	}
	defer closeRows(rows)

	var result []*machine.Machine
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating machine rows: %w", err)
	}
	return result, total, nil
}

// Update persists PPC-local changes to an existing machine.
func (r *MachineRepository) Update(ctx context.Context, entity *machine.Machine) error {
	query := `
		UPDATE machine
		SET machine_area = $2, machine_line = $3, machine_group_id = $4,
			machine_doff_weight_kg = $5, machine_is_active = $6, machine_orion_code = $7,
			updated_at = $8, updated_by = $9
		WHERE machine_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Area().String(),
		entity.Line(),
		entity.GroupID(),
		entity.DoffWeightKg(),
		entity.IsActive(),
		entity.OrionCode(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		if isForeignKeyViolation(err) {
			return fmt.Errorf("invalid machine_group_id: %w", err)
		}
		return fmt.Errorf("failed to update machine: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return machine.ErrNotFound
	}
	return nil
}

// syncActor is the created_by/updated_by value stamped on sync-sourced rows.
const syncActor = "sync"

// UpsertSourced merges a sync-sourced machine, preserving PPC-local fields and
// never overwriting existing values with NULL. New machines with no resolvable
// area are skipped (the area column is NOT NULL and area is otherwise PPC-local).
func (r *MachineRepository) UpsertSourced(ctx context.Context, src machine.SourcedMachine) (machine.UpsertOutcome, error) {
	var existingID int64
	err := r.db.QueryRowContext(ctx,
		`SELECT machine_id FROM machine WHERE machine_no = $1`, src.MachineNo,
	).Scan(&existingID)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return r.insertSourced(ctx, src)
	case err != nil:
		return machine.OutcomeSkipped, fmt.Errorf("lookup machine %q: %w", src.MachineNo, err)
	default:
		return r.updateSourced(ctx, existingID, src)
	}
}

// insertSourced creates a new sync-sourced machine. Requires a resolvable area
// (from Oracle MACH_DEPT); rows without one are skipped rather than failing.
func (r *MachineRepository) insertSourced(ctx context.Context, src machine.SourcedMachine) (machine.UpsertOutcome, error) {
	if src.Area == "" {
		return machine.OutcomeSkipped, nil
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO machine (machine_no, machine_area, machine_line, machine_group_id,
		                      machine_orion_code, machine_is_active, source_mc_id, synced_at, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
		src.MachineNo, src.Area, nullIfEmpty(src.Line), src.GroupID,
		nullIfEmpty(src.OrionCode), src.IsActive, src.SourceMcID, src.SyncedAt, syncActor,
	)
	if err != nil {
		return machine.OutcomeSkipped, fmt.Errorf("insert sourced machine %q: %w", src.MachineNo, err)
	}
	return machine.OutcomeInserted, nil
}

// updateSourced merges source-owned fields into an existing machine. Every
// source-derived column is COALESCE-guarded: a field the source could not
// resolve leaves the stored value untouched, so a degraded run (or a TXTMACH row
// with a NULL line/group) never blanks a value a planner set by hand.
func (r *MachineRepository) updateSourced(ctx context.Context, id int64, src machine.SourcedMachine) (machine.UpsertOutcome, error) {
	_, err := r.db.ExecContext(ctx,
		`UPDATE machine
		 SET machine_area = COALESCE($2, machine_area),
		     machine_line = COALESCE($3, machine_line),
		     machine_group_id = COALESCE($4, machine_group_id),
		     machine_orion_code = COALESCE($5, machine_orion_code),
		     machine_is_active = $6,
		     source_mc_id = COALESCE($7, source_mc_id),
		     synced_at = $8,
		     updated_at = $8,
		     updated_by = $9
		 WHERE machine_id = $1`,
		id, nullIfEmpty(src.Area), nullIfEmpty(src.Line), src.GroupID,
		nullIfEmpty(src.OrionCode), src.IsActive, src.SourceMcID, src.SyncedAt, syncActor,
	)
	if err != nil {
		return machine.OutcomeSkipped, fmt.Errorf("update sourced machine %d: %w", id, err)
	}
	return machine.OutcomeUpdated, nil
}

// nullIfEmpty maps an unresolved (empty) source string to a SQL NULL so the
// surrounding COALESCE preserves the stored value instead of blanking it.
func nullIfEmpty(s string) interface{} {
	if s == "" {
		return nil
	}
	return s
}

// EnsureGroup returns the id of the machine group with the given name and area,
// creating it when absent. Idempotent under the (group_name, group_area) unique
// constraint: a concurrent creator wins and the existing id is returned.
func (r *MachineRepository) EnsureGroup(ctx context.Context, name, groupArea string) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO machine_group (group_name, group_area, created_by)
		 VALUES ($1, $2, $3)
		 ON CONFLICT (group_name, group_area) DO UPDATE SET group_name = EXCLUDED.group_name
		 RETURNING group_id`,
		name, groupArea, syncActor,
	).Scan(&id)
	if err != nil {
		return 0, fmt.Errorf("ensure machine group %q/%q: %w", name, groupArea, err)
	}
	return id, nil
}

func (r *MachineRepository) scanRow(row *sql.Row) (*machine.Machine, error) {
	var dto machineDTO
	err := dto.scan(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, machine.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan machine: %w", err)
	}
	return dto.toEntity()
}

func (r *MachineRepository) scanRows(rows *sql.Rows) (*machine.Machine, error) {
	var dto machineDTO
	if err := dto.scan(rows.Scan); err != nil {
		return nil, fmt.Errorf("failed to scan machine row: %w", err)
	}
	return dto.toEntity()
}

type machineDTO struct {
	ID           int64
	MachineNo    string
	Area         string
	Line         sql.NullString
	GroupID      sql.NullInt64
	GroupName    sql.NullString
	DoffWeightKg sql.NullFloat64
	IsActive     bool
	OrionCode    sql.NullString
	SourceMcID   sql.NullString
	SyncedAt     sql.NullTime
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    sql.NullTime
	UpdatedBy    sql.NullString
}

func (d *machineDTO) scan(scan func(dest ...interface{}) error) error {
	return scan(
		&d.ID, &d.MachineNo, &d.Area, &d.Line, &d.GroupID,
		&d.GroupName, &d.DoffWeightKg, &d.IsActive, &d.OrionCode,
		&d.SourceMcID, &d.SyncedAt, &d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
}

func (d *machineDTO) toEntity() (*machine.Machine, error) {
	a, err := area.New(d.Area)
	if err != nil {
		return nil, fmt.Errorf("invalid area from db: %w", err)
	}
	var groupName string
	if d.GroupName.Valid {
		groupName = d.GroupName.String
	}
	return machine.Reconstruct(
		d.ID,
		d.MachineNo,
		a,
		nullStringPtr(d.Line),
		nullInt64Ptr(d.GroupID),
		groupName,
		nullFloatPtr(d.DoffWeightKg),
		d.IsActive,
		nullString(d.OrionCode),
		nullStringPtr(d.SourceMcID),
		nullTimePtr(d.SyncedAt),
		d.CreatedAt,
		d.CreatedBy,
		nullTimePtr(d.UpdatedAt),
		nullStringPtr(d.UpdatedBy),
	), nil
}
