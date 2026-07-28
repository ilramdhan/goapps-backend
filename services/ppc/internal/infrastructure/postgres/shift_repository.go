// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/shift"
)

// ShiftRepository implements shift.Repository using PostgreSQL. Start/end TIME
// columns are read via to_char(..,'HH24:MI') and written as HH:MM strings that
// PostgreSQL casts to TIME, keeping the driver time-type handling unambiguous.
type ShiftRepository struct {
	db *DB
}

// NewShiftRepository creates a new ShiftRepository.
func NewShiftRepository(db *DB) *ShiftRepository {
	return &ShiftRepository{db: db}
}

var _ shift.Repository = (*ShiftRepository)(nil)

const shiftSelectCols = `ps_id, ps_code, ps_name, ` +
	`to_char(ps_start_time, 'HH24:MI'), to_char(ps_end_time, 'HH24:MI'), ` +
	`ps_is_active, ps_created_at, ps_created_by, ps_updated_at, ps_updated_by`

// Create persists a new shift and assigns its generated ID.
func (r *ShiftRepository) Create(ctx context.Context, entity *shift.Shift) error {
	query := `
		INSERT INTO ppc_shift (
			ps_code, ps_name, ps_start_time, ps_end_time,
			ps_is_active, ps_created_at, ps_created_by
		)
		VALUES ($1, $2, $3::time, $4::time, $5, $6, $7)
		RETURNING ps_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Code(),
		nullableText(entity.Name()),
		entity.StartTime(),
		entity.EndTime(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return shift.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create shift: %w", err)
	}

	*entity = *shift.Reconstruct(
		id, entity.Code(), entity.Name(), entity.StartTime(), entity.EndTime(),
		entity.IsActive(), entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a shift by its ID.
func (r *ShiftRepository) GetByID(ctx context.Context, id int64) (*shift.Shift, error) {
	query := `SELECT ` + shiftSelectCols + ` FROM ppc_shift WHERE ps_id = $1`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves shifts with filtering and pagination.
func (r *ShiftRepository) List(ctx context.Context, filter shift.ListFilter) ([]*shift.Shift, int64, error) {
	filter.Validate()

	base := `FROM ppc_shift WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND ps_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count shifts: %w", err)
	}

	query := `SELECT ` + shiftSelectCols + ` ` + base +
		fmt.Sprintf(` ORDER BY ps_code ASC LIMIT $%d OFFSET $%d`, argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list shifts: %w", err)
	}
	defer closeRows(rows)

	var result []*shift.Shift
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating shift rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing shift.
func (r *ShiftRepository) Update(ctx context.Context, entity *shift.Shift) error {
	query := `
		UPDATE ppc_shift
		SET ps_name = $2, ps_start_time = $3::time, ps_end_time = $4::time,
			ps_is_active = $5, ps_updated_at = $6, ps_updated_by = $7
		WHERE ps_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		nullableText(entity.Name()),
		entity.StartTime(),
		entity.EndTime(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update shift: %w", err)
	}
	return checkAffected(res, shift.ErrNotFound)
}

// Delete removes a shift by its ID.
func (r *ShiftRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM ppc_shift WHERE ps_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete shift: %w", err)
	}
	return checkAffected(res, shift.ErrNotFound)
}

func (r *ShiftRepository) scanRow(row *sql.Row) (*shift.Shift, error) {
	var dto shiftDTO
	err := row.Scan(
		&dto.ID, &dto.Code, &dto.Name, &dto.StartTime, &dto.EndTime,
		&dto.IsActive, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shift.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan shift: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *ShiftRepository) scanRows(rows *sql.Rows) (*shift.Shift, error) {
	var dto shiftDTO
	if err := rows.Scan(
		&dto.ID, &dto.Code, &dto.Name, &dto.StartTime, &dto.EndTime,
		&dto.IsActive, &dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan shift row: %w", err)
	}
	return dto.toEntity(), nil
}

type shiftDTO struct {
	ID        int64
	Code      string
	Name      sql.NullString
	StartTime string
	EndTime   string
	IsActive  bool
	CreatedAt time.Time
	CreatedBy string
	UpdatedAt sql.NullTime
	UpdatedBy sql.NullString
}

func (d *shiftDTO) toEntity() *shift.Shift {
	return shift.Reconstruct(
		d.ID, d.Code, nullString(d.Name), d.StartTime, d.EndTime,
		d.IsActive, d.CreatedAt, d.CreatedBy, nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	)
}
