// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/machinegroup"
)

// MachineGroupRepository implements machinegroup.Repository using PostgreSQL.
type MachineGroupRepository struct {
	db *DB
}

// NewMachineGroupRepository creates a new MachineGroupRepository.
func NewMachineGroupRepository(db *DB) *MachineGroupRepository {
	return &MachineGroupRepository{db: db}
}

var _ machinegroup.Repository = (*MachineGroupRepository)(nil)

// Create persists a new machine group and assigns its generated ID.
func (r *MachineGroupRepository) Create(ctx context.Context, entity *machinegroup.MachineGroup) error {
	query := `
		INSERT INTO machine_group (group_name, group_area, created_at, created_by)
		VALUES ($1, $2, $3, $4)
		RETURNING group_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Name(),
		entity.Area().String(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return machinegroup.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create machine group: %w", err)
	}

	*entity = *machinegroup.Reconstruct(
		id, entity.Name(), entity.Area(), entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a machine group by its ID.
func (r *MachineGroupRepository) GetByID(ctx context.Context, id int64) (*machinegroup.MachineGroup, error) {
	query := `
		SELECT group_id, group_name, group_area, created_at, created_by, updated_at, updated_by
		FROM machine_group
		WHERE group_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves machine groups with filtering and pagination.
func (r *MachineGroupRepository) List(ctx context.Context, filter machinegroup.ListFilter) ([]*machinegroup.MachineGroup, int64, error) {
	filter.Validate()

	base := `FROM machine_group WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND group_name ILIKE $%d`, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.Area != "" {
		base += fmt.Sprintf(` AND group_area = $%d`, argIdx)
		args = append(args, filter.Area)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count machine groups: %w", err)
	}

	sortColumnMap := map[string]string{
		"name":       "group_name",
		"area":       "group_area",
		"created_at": "created_at",
	}
	orderCol := "group_name"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT group_id, group_name, group_area, created_at, created_by, updated_at, updated_by ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list machine groups: %w", err)
	}
	defer closeRows(rows)

	var result []*machinegroup.MachineGroup
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating machine group rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing machine group.
func (r *MachineGroupRepository) Update(ctx context.Context, entity *machinegroup.MachineGroup) error {
	query := `
		UPDATE machine_group
		SET group_name = $2, group_area = $3, updated_at = $4, updated_by = $5
		WHERE group_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(), entity.Name(), entity.Area().String(), entity.UpdatedAt(), entity.UpdatedBy(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return machinegroup.ErrAlreadyExists
		}
		return fmt.Errorf("failed to update machine group: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return machinegroup.ErrNotFound
	}
	return nil
}

// Delete removes a machine group by its ID.
func (r *MachineGroupRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM machine_group WHERE group_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete machine group: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return machinegroup.ErrNotFound
	}
	return nil
}

func (r *MachineGroupRepository) scanRow(row *sql.Row) (*machinegroup.MachineGroup, error) {
	var dto machineGroupDTO
	err := row.Scan(&dto.ID, &dto.Name, &dto.Area, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, machinegroup.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan machine group: %w", err)
	}
	return dto.toEntity()
}

func (r *MachineGroupRepository) scanRows(rows *sql.Rows) (*machinegroup.MachineGroup, error) {
	var dto machineGroupDTO
	if err := rows.Scan(&dto.ID, &dto.Name, &dto.Area, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy); err != nil {
		return nil, fmt.Errorf("failed to scan machine group row: %w", err)
	}
	return dto.toEntity()
}

type machineGroupDTO struct {
	ID        int64
	Name      string
	Area      string
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt sql.NullTime
	UpdatedBy sql.NullString
}

func (d *machineGroupDTO) toEntity() (*machinegroup.MachineGroup, error) {
	a, err := area.New(d.Area)
	if err != nil {
		return nil, fmt.Errorf("invalid area from db: %w", err)
	}
	return machinegroup.Reconstruct(
		d.ID, d.Name, a, d.CreatedAt, d.CreatedBy, nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	), nil
}
