package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/datasource"
)

// BiDataSourceRepository implements datasource.Repository.
type BiDataSourceRepository struct {
	db *DB
}

// NewBiDataSourceRepository constructs a BiDataSourceRepository.
func NewBiDataSourceRepository(db *DB) *BiDataSourceRepository {
	return &BiDataSourceRepository{db: db}
}

var _ datasource.Repository = (*BiDataSourceRepository)(nil)

const selectDataSourceBase = `
SELECT source_id, source_code, source_name, source_type, description, is_active, created_at, updated_at
FROM bi_data_source`

// List returns data sources (optionally including inactive) ordered by source_code.
func (r *BiDataSourceRepository) List(ctx context.Context, includeInactive bool) ([]*datasource.DataSource, error) {
	q := selectDataSourceBase
	if !includeInactive {
		q += " WHERE is_active = TRUE"
	}
	q += " ORDER BY source_code"

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("query data sources: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			_ = err
		}
	}()

	var out []*datasource.DataSource
	for rows.Next() {
		ds, err := r.scanDataSource(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ds)
	}
	return out, rows.Err()
}

// GetByCode looks up a data source by its business code.
func (r *BiDataSourceRepository) GetByCode(ctx context.Context, code string) (*datasource.DataSource, error) {
	row := r.db.QueryRowContext(ctx, selectDataSourceBase+" WHERE source_code = $1", code)
	return r.scanDataSource(row.Scan)
}

// GetByID looks up by primary key.
func (r *BiDataSourceRepository) GetByID(ctx context.Context, id uuid.UUID) (*datasource.DataSource, error) {
	row := r.db.QueryRowContext(ctx, selectDataSourceBase+" WHERE source_id = $1", id)
	return r.scanDataSource(row.Scan)
}

// scanDataSource reads one row into a DataSource.
func (r *BiDataSourceRepository) scanDataSource(scan scanFunc) (*datasource.DataSource, error) {
	var (
		id          uuid.UUID
		code        string
		name        string
		typ         string
		description sql.NullString
		isActive    bool
		createdAt   sql.NullTime
		updatedAt   sql.NullTime
	)
	err := scan(&id, &code, &name, &typ, &description, &isActive, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, datasource.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan data source: %w", err)
	}
	return &datasource.DataSource{
		ID:          id,
		Code:        code,
		Name:        name,
		Type:        typ,
		Description: nullToString(description),
		IsActive:    isActive,
		CreatedAt:   nullTimeOrZero(createdAt),
		UpdatedAt:   nullTimeOrZero(updatedAt),
	}, nil
}
