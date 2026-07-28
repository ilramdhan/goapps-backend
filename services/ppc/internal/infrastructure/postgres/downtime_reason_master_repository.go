// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/area"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/downtimereason"
)

// DowntimeReasonRepository implements downtimereason.Repository using PostgreSQL.
type DowntimeReasonRepository struct {
	db *DB
}

// NewDowntimeReasonRepository creates a new DowntimeReasonRepository.
func NewDowntimeReasonRepository(db *DB) *DowntimeReasonRepository {
	return &DowntimeReasonRepository{db: db}
}

var _ downtimereason.Repository = (*DowntimeReasonRepository)(nil)

// Create persists a new downtime reason and assigns its generated ID.
func (r *DowntimeReasonRepository) Create(ctx context.Context, entity *downtimereason.Reason) error {
	query := `
		INSERT INTO downtime_reason_master (
			drm_area, drm_code, drm_name, drm_category,
			drm_is_exclude_from_eff, drm_is_active, drm_sort_order,
			drm_created_at, drm_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING drm_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Area().String(),
		entity.Code(),
		entity.Name(),
		entity.Category(),
		entity.IsExcludeFromEff(),
		entity.IsActive(),
		entity.SortOrder(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return downtimereason.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create downtime reason: %w", err)
	}

	*entity = *downtimereason.Reconstruct(
		id, entity.Area(), entity.Code(), entity.Name(), entity.Category(),
		entity.IsExcludeFromEff(), entity.IsActive(), entity.SortOrder(),
		entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a downtime reason by its ID.
func (r *DowntimeReasonRepository) GetByID(ctx context.Context, id int64) (*downtimereason.Reason, error) {
	query := `
		SELECT drm_id, drm_area, drm_code, drm_name, drm_category,
			drm_is_exclude_from_eff, drm_is_active, drm_sort_order,
			drm_created_at, drm_created_by, drm_updated_at, drm_updated_by
		FROM downtime_reason_master
		WHERE drm_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves downtime reasons with filtering and pagination.
func (r *DowntimeReasonRepository) List(ctx context.Context, filter downtimereason.ListFilter) ([]*downtimereason.Reason, int64, error) {
	filter.Validate()

	base := `FROM downtime_reason_master WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(` AND (drm_code ILIKE $%d OR drm_name ILIKE $%d)`, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.Area != "" {
		base += fmt.Sprintf(` AND drm_area = $%d`, argIdx)
		args = append(args, filter.Area)
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND drm_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count downtime reasons: %w", err)
	}

	sortColumnMap := map[string]string{
		"sort_order": "drm_sort_order",
		"code":       "drm_code",
		"name":       "drm_name",
		"area":       "drm_area",
		"created_at": "drm_created_at",
	}
	orderCol := "drm_sort_order"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT drm_id, drm_area, drm_code, drm_name, drm_category, ` +
		`drm_is_exclude_from_eff, drm_is_active, drm_sort_order, ` +
		`drm_created_at, drm_created_by, drm_updated_at, drm_updated_by ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list downtime reasons: %w", err)
	}
	defer closeRows(rows)

	var result []*downtimereason.Reason
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating downtime reason rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing downtime reason.
func (r *DowntimeReasonRepository) Update(ctx context.Context, entity *downtimereason.Reason) error {
	query := `
		UPDATE downtime_reason_master
		SET drm_name = $2, drm_category = $3, drm_is_exclude_from_eff = $4,
			drm_is_active = $5, drm_sort_order = $6, drm_updated_at = $7, drm_updated_by = $8
		WHERE drm_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Name(),
		entity.Category(),
		entity.IsExcludeFromEff(),
		entity.IsActive(),
		entity.SortOrder(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return downtimereason.ErrAlreadyExists
		}
		return fmt.Errorf("failed to update downtime reason: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return downtimereason.ErrNotFound
	}
	return nil
}

// Delete removes a downtime reason by its ID.
func (r *DowntimeReasonRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM downtime_reason_master WHERE drm_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete downtime reason: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return downtimereason.ErrNotFound
	}
	return nil
}

func (r *DowntimeReasonRepository) scanRow(row *sql.Row) (*downtimereason.Reason, error) {
	var dto downtimeReasonDTO
	err := row.Scan(
		&dto.ID, &dto.Area, &dto.Code, &dto.Name, &dto.Category,
		&dto.IsExcludeFromEff, &dto.IsActive, &dto.SortOrder,
		&dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, downtimereason.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan downtime reason: %w", err)
	}
	return dto.toEntity()
}

func (r *DowntimeReasonRepository) scanRows(rows *sql.Rows) (*downtimereason.Reason, error) {
	var dto downtimeReasonDTO
	if err := rows.Scan(
		&dto.ID, &dto.Area, &dto.Code, &dto.Name, &dto.Category,
		&dto.IsExcludeFromEff, &dto.IsActive, &dto.SortOrder,
		&dto.CreatedAt, &dto.CreatedBy, &dto.UpdatedAt, &dto.UpdatedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan downtime reason row: %w", err)
	}
	return dto.toEntity()
}

type downtimeReasonDTO struct {
	ID               int64
	Area             string
	Code             string
	Name             string
	Category         string
	IsExcludeFromEff bool
	IsActive         bool
	SortOrder        int32
	CreatedAt        time.Time
	CreatedBy        string
	UpdatedAt        sql.NullTime
	UpdatedBy        sql.NullString
}

func (d *downtimeReasonDTO) toEntity() (*downtimereason.Reason, error) {
	a, err := area.New(d.Area)
	if err != nil {
		return nil, fmt.Errorf("invalid area from db: %w", err)
	}
	return downtimereason.Reconstruct(
		d.ID, a, d.Code, d.Name, d.Category,
		d.IsExcludeFromEff, d.IsActive, d.SortOrder,
		d.CreatedAt, d.CreatedBy, nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	), nil
}
