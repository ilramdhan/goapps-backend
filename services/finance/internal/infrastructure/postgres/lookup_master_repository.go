package postgres

import (
	"context"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// LookupMasterRepository implements lookupmaster.Repository against PostgreSQL.
type LookupMasterRepository struct {
	db *DB
}

// NewLookupMasterRepository creates a new LookupMasterRepository.
func NewLookupMasterRepository(db *DB) *LookupMasterRepository {
	return &LookupMasterRepository{db: db}
}

// Verify interface implementation at compile time.
var _ lookupmaster.Repository = (*LookupMasterRepository)(nil)

// ListMasters returns lookup master records, optionally filtered to active only.
func (r *LookupMasterRepository) ListMasters(ctx context.Context, activeOnly bool) ([]*lookupmaster.LookupMaster, error) {
	q := `SELECT lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, lm_is_active
	      FROM mst_lookup_master`
	if activeOnly {
		q += ` WHERE lm_is_active = TRUE`
	}
	q += ` ORDER BY lm_code`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list lookup masters: %w", err)
	}

	var out []*lookupmaster.LookupMaster
	for rows.Next() {
		m := &lookupmaster.LookupMaster{}
		if scanErr := rows.Scan(&m.Code, &m.DisplayName, &m.APIPath, &m.CodeField, &m.LabelField, &m.IsActive); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan lookup master: %w", scanErr)
		}
		out = append(out, m)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close lookup master rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup masters: %w", err)
	}
	return out, nil
}

// ListColumns returns fillable columns for the given master code, sorted by sort_order.
func (r *LookupMasterRepository) ListColumns(ctx context.Context, masterCode string) ([]*lookupmaster.Column, error) {
	const q = `SELECT lmc_id::text, lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order
	           FROM mst_lookup_master_column
	           WHERE lmc_master_code = $1
	           ORDER BY lmc_sort_order, lmc_column_name`

	rows, err := r.db.QueryContext(ctx, q, masterCode)
	if err != nil {
		return nil, fmt.Errorf("list lookup master columns: %w", err)
	}

	var out []*lookupmaster.Column
	for rows.Next() {
		c := &lookupmaster.Column{}
		if scanErr := rows.Scan(&c.ID, &c.MasterCode, &c.ColumnName, &c.DisplayName, &c.DataType, &c.SortOrder); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan lookup master column: %w", scanErr)
		}
		out = append(out, c)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close lookup master column rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup master columns: %w", err)
	}
	return out, nil
}

// CreateMaster inserts a new lookup master into the registry.
func (r *LookupMasterRepository) CreateMaster(ctx context.Context, m *lookupmaster.LookupMaster, createdBy string) error {
	const q = `INSERT INTO mst_lookup_master (lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, created_by)
	           VALUES ($1, $2, $3, $4, $5, $6)
	           ON CONFLICT (lm_code) DO NOTHING`
	_, err := r.db.ExecContext(ctx, q, m.Code, m.DisplayName, m.APIPath, m.CodeField, m.LabelField, createdBy)
	if err != nil {
		return fmt.Errorf("create lookup master: %w", err)
	}
	return nil
}

// DeleteMaster removes a lookup master from the registry by code.
func (r *LookupMasterRepository) DeleteMaster(ctx context.Context, code string) error {
	const q = `DELETE FROM mst_lookup_master WHERE lm_code = $1`
	_, err := r.db.ExecContext(ctx, q, code)
	if err != nil {
		return fmt.Errorf("delete lookup master: %w", err)
	}
	return nil
}

// CreateColumn adds a fillable column to a master and returns the new UUID.
func (r *LookupMasterRepository) CreateColumn(ctx context.Context, c *lookupmaster.Column, _ string) (string, error) {
	const q = `INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
	           VALUES ($1, $2, $3, $4, $5)
	           ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING
	           RETURNING lmc_id::text`
	var id string
	err := r.db.QueryRowContext(ctx, q, c.MasterCode, c.ColumnName, c.DisplayName, c.DataType, c.SortOrder).Scan(&id)
	if err != nil {
		return "", fmt.Errorf("create lookup master column: %w", err)
	}
	return id, nil
}

// DeleteColumn removes a lookup master column by its UUID.
func (r *LookupMasterRepository) DeleteColumn(ctx context.Context, id string) error {
	const q = `DELETE FROM mst_lookup_master_column WHERE lmc_id = $1`
	_, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("delete lookup master column: %w", err)
	}
	return nil
}
