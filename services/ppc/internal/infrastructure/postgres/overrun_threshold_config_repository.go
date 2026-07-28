// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/overrun"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/threshold"
)

// ThresholdRepository implements threshold.Repository using PostgreSQL.
type ThresholdRepository struct {
	db *DB
}

// NewThresholdRepository creates a new ThresholdRepository.
func NewThresholdRepository(db *DB) *ThresholdRepository {
	return &ThresholdRepository{db: db}
}

var (
	_ threshold.Repository = (*ThresholdRepository)(nil)
	_ overrun.ConfigLookup = (*ThresholdRepository)(nil)
)

// FindThreshold resolves the active threshold config for a level+ref, or nil
// when none is configured. Implements overrun.ConfigLookup for the resolver.
func (r *ThresholdRepository) FindThreshold(ctx context.Context, level string, refID *int64) (*overrun.Threshold, error) {
	query := `SELECT otc_threshold_unit, otc_warning_value, otc_block_value
		FROM overrun_threshold_config
		WHERE otc_is_active = TRUE AND otc_level = $1
		  AND (($2::BIGINT IS NULL AND otc_ref_id IS NULL) OR otc_ref_id = $2::BIGINT)
		ORDER BY otc_id DESC LIMIT 1`
	var unit string
	var warning, block float64
	err := r.db.QueryRowContext(ctx, query, level, int64PtrArg(refID)).Scan(&unit, &warning, &block)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // absence of a config is a valid "no fence" result
	}
	if err != nil {
		return nil, fmt.Errorf("failed to find threshold: %w", err)
	}
	return &overrun.Threshold{
		Level:        level,
		Unit:         strings.TrimSpace(unit),
		WarningValue: warning,
		BlockValue:   block,
	}, nil
}

// Create persists a new config and assigns its generated ID.
func (r *ThresholdRepository) Create(ctx context.Context, entity *threshold.Config) error {
	query := `
		INSERT INTO overrun_threshold_config (
			otc_level, otc_ref_id, otc_threshold_unit, otc_warning_value,
			otc_block_value, otc_notes, otc_is_active, otc_created_at, otc_created_by
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING otc_id
	`
	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Level(),
		entity.RefID(),
		entity.Unit(),
		entity.WarningValue(),
		entity.BlockValue(),
		entity.Notes(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("failed to create overrun threshold config: %w", err)
	}

	*entity = *threshold.Reconstruct(
		id, entity.Level(), entity.RefID(), entity.Unit(),
		entity.WarningValue(), entity.BlockValue(), entity.Notes(), entity.IsActive(),
		entity.CreatedAt(), entity.CreatedBy(), nil, nil,
	)
	return nil
}

// GetByID retrieves a config by its ID.
func (r *ThresholdRepository) GetByID(ctx context.Context, id int64) (*threshold.Config, error) {
	query := `
		SELECT otc_id, otc_level, otc_ref_id, otc_threshold_unit, otc_warning_value,
			otc_block_value, otc_notes, otc_is_active, otc_created_at, otc_created_by,
			otc_updated_at, otc_updated_by
		FROM overrun_threshold_config
		WHERE otc_id = $1
	`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// List retrieves configs with filtering and pagination.
func (r *ThresholdRepository) List(ctx context.Context, filter threshold.ListFilter) ([]*threshold.Config, int64, error) {
	filter.Validate()

	base := `FROM overrun_threshold_config WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Level != "" {
		base += fmt.Sprintf(` AND otc_level = $%d`, argIdx)
		args = append(args, filter.Level)
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND otc_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count overrun threshold configs: %w", err)
	}

	sortColumnMap := map[string]string{
		"level":         "otc_level",
		"unit":          "otc_threshold_unit",
		"warning_value": "otc_warning_value",
		"block_value":   "otc_block_value",
		"created_at":    "otc_created_at",
	}
	orderCol := "otc_level"
	if mapped, ok := sortColumnMap[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT otc_id, otc_level, otc_ref_id, otc_threshold_unit, otc_warning_value,
		otc_block_value, otc_notes, otc_is_active, otc_created_at, otc_created_by,
		otc_updated_at, otc_updated_by ` +
		base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
		orderCol, sortDirection(filter.SortOrder), argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list overrun threshold configs: %w", err)
	}
	defer closeRows(rows)

	var result []*threshold.Config
	for rows.Next() {
		entity, scanErr := r.scanRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		result = append(result, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating overrun threshold config rows: %w", err)
	}
	return result, total, nil
}

// Update persists changes to an existing config.
func (r *ThresholdRepository) Update(ctx context.Context, entity *threshold.Config) error {
	query := `
		UPDATE overrun_threshold_config
		SET otc_threshold_unit = $2, otc_warning_value = $3, otc_block_value = $4,
			otc_notes = $5, otc_is_active = $6, otc_updated_at = $7, otc_updated_by = $8
		WHERE otc_id = $1
	`
	res, err := r.db.ExecContext(ctx, query,
		entity.ID(),
		entity.Unit(),
		entity.WarningValue(),
		entity.BlockValue(),
		entity.Notes(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update overrun threshold config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return threshold.ErrNotFound
	}
	return nil
}

// Delete removes a config by its ID.
func (r *ThresholdRepository) Delete(ctx context.Context, id int64) error {
	res, err := r.db.ExecContext(ctx, `DELETE FROM overrun_threshold_config WHERE otc_id = $1`, id)
	if err != nil {
		return fmt.Errorf("failed to delete overrun threshold config: %w", err)
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if affected == 0 {
		return threshold.ErrNotFound
	}
	return nil
}

func (r *ThresholdRepository) scanRow(row *sql.Row) (*threshold.Config, error) {
	var dto thresholdDTO
	err := row.Scan(
		&dto.ID, &dto.Level, &dto.RefID, &dto.Unit, &dto.WarningValue,
		&dto.BlockValue, &dto.Notes, &dto.IsActive, &dto.CreatedAt, &dto.CreatedBy,
		&dto.UpdatedAt, &dto.UpdatedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, threshold.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan overrun threshold config: %w", err)
	}
	return dto.toEntity(), nil
}

func (r *ThresholdRepository) scanRows(rows *sql.Rows) (*threshold.Config, error) {
	var dto thresholdDTO
	if err := rows.Scan(
		&dto.ID, &dto.Level, &dto.RefID, &dto.Unit, &dto.WarningValue,
		&dto.BlockValue, &dto.Notes, &dto.IsActive, &dto.CreatedAt, &dto.CreatedBy,
		&dto.UpdatedAt, &dto.UpdatedBy,
	); err != nil {
		return nil, fmt.Errorf("failed to scan overrun threshold config row: %w", err)
	}
	return dto.toEntity(), nil
}

type thresholdDTO struct {
	ID           int64
	Level        string
	RefID        sql.NullInt64
	Unit         string
	WarningValue float64
	BlockValue   float64
	Notes        sql.NullString
	IsActive     bool
	CreatedAt    time.Time
	CreatedBy    string
	UpdatedAt    sql.NullTime
	UpdatedBy    sql.NullString
}

func (d *thresholdDTO) toEntity() *threshold.Config {
	notes := ""
	if d.Notes.Valid {
		notes = d.Notes.String
	}
	return threshold.Reconstruct(
		d.ID, d.Level, nullInt64Ptr(d.RefID), strings.TrimSpace(d.Unit), d.WarningValue, d.BlockValue,
		notes, d.IsActive, d.CreatedAt, d.CreatedBy,
		nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	)
}
