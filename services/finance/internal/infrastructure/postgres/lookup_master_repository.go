package postgres

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

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
	q := `SELECT lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, lm_is_active, COALESCE(lm_table_name,'')
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
		if scanErr := rows.Scan(&m.Code, &m.DisplayName, &m.APIPath, &m.CodeField, &m.LabelField, &m.IsActive, &m.TableName); scanErr != nil {
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
	const q = `INSERT INTO mst_lookup_master (lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, lm_table_name, created_by)
	           VALUES ($1, $2, $3, $4, $5, $6, $7)
	           ON CONFLICT (lm_code) DO NOTHING`
	_, err := r.db.ExecContext(ctx, q, m.Code, m.DisplayName, m.APIPath, m.CodeField, m.LabelField, m.TableName, createdBy)
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

// UpdateMaster applies partial updates to an existing master.
func (r *LookupMasterRepository) UpdateMaster(ctx context.Context, code string, u lookupmaster.UpdateMaster) error {
	var sets []string
	var args []interface{}
	idx := 1
	if u.DisplayName != nil {
		sets = append(sets, fmt.Sprintf("lm_display_name = $%d", idx))
		args = append(args, *u.DisplayName)
		idx++
	}
	if u.TableName != nil {
		sets = append(sets, fmt.Sprintf("lm_table_name = $%d", idx))
		args = append(args, *u.TableName)
		idx++
	}
	if u.IsActive != nil {
		sets = append(sets, fmt.Sprintf("lm_is_active = $%d", idx))
		args = append(args, *u.IsActive)
		idx++
	}
	if len(sets) == 0 {
		return nil
	}
	args = append(args, code)
	q := fmt.Sprintf("UPDATE mst_lookup_master SET %s WHERE lm_code = $%d",
		strings.Join(sets, ", "), idx)
	_, err := r.db.ExecContext(ctx, q, args...)
	if err != nil {
		return fmt.Errorf("update lookup master %q: %w", code, err)
	}
	return nil
}

// ListTableColumns introspects information_schema.columns for a registered table.
func (r *LookupMasterRepository) ListTableColumns(ctx context.Context, tableName string) ([]*lookupmaster.TableColumn, error) {
	// Validate the table is registered to prevent dynamic-query abuse.
	var count int
	if err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mst_lookup_master WHERE lm_table_name = $1`, tableName,
	).Scan(&count); err != nil {
		return nil, fmt.Errorf("validate table name: %w", err)
	}
	if count == 0 {
		return nil, fmt.Errorf("table %q is not registered in mst_lookup_master", tableName)
	}

	const q = `
		SELECT column_name, data_type, ordinal_position
		FROM information_schema.columns
		WHERE table_name = $1 AND table_schema = 'public'
		ORDER BY ordinal_position`
	rows, err := r.db.QueryContext(ctx, q, tableName)
	if err != nil {
		return nil, fmt.Errorf("introspect table columns: %w", err)
	}
	var out []*lookupmaster.TableColumn
	for rows.Next() {
		c := &lookupmaster.TableColumn{}
		if scanErr := rows.Scan(&c.ColumnName, &c.RawType, &c.OrdinalPosition); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan table column: %w", scanErr)
		}
		c.DataType = mapPGTypeToDataType(c.RawType)
		out = append(out, c)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close introspect rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate table columns: %w", err)
	}
	return out, nil
}

// mapPGTypeToDataType maps a PostgreSQL column type to "NUMBER" or "TEXT".
func mapPGTypeToDataType(pgType string) string {
	switch pgType {
	case "numeric", "integer", "bigint", "smallint", "real",
		"double precision", "decimal", "money", "boolean":
		return "NUMBER"
	default:
		return "TEXT"
	}
}

// ListMasterOptions queries the master's registered table and returns code+label rows.
func (r *LookupMasterRepository) ListMasterOptions(ctx context.Context, masterCode string) ([]lookupmaster.MasterOption, error) {
	var tableName, codeField, labelField string
	err := r.db.QueryRowContext(ctx,
		`SELECT COALESCE(lm_table_name,''), lm_code_field, lm_label_field
		 FROM mst_lookup_master
		 WHERE lm_code = $1 AND lm_is_active = TRUE`, masterCode,
	).Scan(&tableName, &codeField, &labelField)
	if err != nil {
		return nil, fmt.Errorf("get master metadata for %q: %w", masterCode, err)
	}
	if tableName == "" {
		return nil, fmt.Errorf("master %q has no table_name configured", masterCode)
	}

	// Table and column names come from the registry (not user input).
	// quoteIdent double-quotes each identifier for safety.
	q := fmt.Sprintf(
		`SELECT %s::text, %s::text FROM %s WHERE deleted_at IS NULL ORDER BY %s LIMIT 500`,
		quoteIdent(codeField), quoteIdent(labelField),
		quoteIdent(tableName),
		quoteIdent(labelField),
	)
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list master options for %q: %w", masterCode, err)
	}
	var out []lookupmaster.MasterOption
	for rows.Next() {
		var opt lookupmaster.MasterOption
		if scanErr := rows.Scan(&opt.Value, &opt.Label); scanErr != nil {
			if closeErr := rows.Close(); closeErr != nil {
				return nil, fmt.Errorf("close rows after scan error: %w", closeErr)
			}
			return nil, fmt.Errorf("scan master option: %w", scanErr)
		}
		out = append(out, opt)
	}
	if closeErr := rows.Close(); closeErr != nil {
		return nil, fmt.Errorf("close options rows: %w", closeErr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate master options: %w", err)
	}
	return out, nil
}

// quoteIdent double-quotes an SQL identifier for safe use in dynamic queries.
// Only intended for values sourced from the mst_lookup_master registry.
func quoteIdent(s string) string {
	return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
}

// ExportMasters exports all masters and columns to an Excel workbook.
func (r *LookupMasterRepository) ExportMasters(ctx context.Context) ([]byte, string, error) {
	masters, err := r.ListMasters(ctx, false)
	if err != nil {
		return nil, "", fmt.Errorf("list masters for export: %w", err)
	}
	f := excelize.NewFile()
	if err := r.writeMastersSheet(f, masters); err != nil {
		return nil, "", err
	}
	if err := r.writeColumnsSheet(ctx, f, masters); err != nil {
		return nil, "", err
	}
	buf, writeErr := f.WriteToBuffer()
	if writeErr != nil {
		return nil, "", fmt.Errorf("write excel: %w", writeErr)
	}
	return buf.Bytes(), "lookup_masters.xlsx", nil
}

// writeMastersSheet populates the "Lookup Masters" sheet of the workbook.
func (r *LookupMasterRepository) writeMastersSheet(f *excelize.File, masters []*lookupmaster.LookupMaster) error {
	const sheet = "Lookup Masters"
	if err := f.SetSheetName("Sheet1", sheet); err != nil {
		return fmt.Errorf("rename sheet: %w", err)
	}
	for i, h := range []string{"Code", "Display Name", "Table Name", "Is Active"} {
		if err := excelSetCell(f, sheet, i+1, 1, h); err != nil {
			return err
		}
	}
	for row, m := range masters {
		for col, v := range []interface{}{m.Code, m.DisplayName, m.TableName, m.IsActive} {
			if err := excelSetCell(f, sheet, col+1, row+2, v); err != nil {
				return err
			}
		}
	}
	return nil
}

// writeColumnsSheet populates the "Columns" sheet of the workbook.
func (r *LookupMasterRepository) writeColumnsSheet(ctx context.Context, f *excelize.File, masters []*lookupmaster.LookupMaster) error {
	const sheet = "Columns"
	if _, err := f.NewSheet(sheet); err != nil {
		return fmt.Errorf("create columns sheet: %w", err)
	}
	for i, h := range []string{"Master Code", "Column Name", "Display Name", "Data Type", "Sort Order"} {
		if err := excelSetCell(f, sheet, i+1, 1, h); err != nil {
			return err
		}
	}
	rowIdx := 2
	for _, m := range masters {
		cols, err := r.ListColumns(ctx, m.Code)
		if err != nil {
			return fmt.Errorf("list columns for master %q: %w", m.Code, err)
		}
		for _, c := range cols {
			for col, v := range []interface{}{c.MasterCode, c.ColumnName, c.DisplayName, c.DataType, c.SortOrder} {
				if setErr := excelSetCell(f, sheet, col+1, rowIdx, v); setErr != nil {
					return setErr
				}
			}
			rowIdx++
		}
	}
	return nil
}

// excelSetCell writes a value to a cell identified by (col, row) coordinates.
func excelSetCell(f *excelize.File, sheet string, col, row int, value interface{}) error {
	cell, err := excelize.CoordinatesToCellName(col, row)
	if err != nil {
		return fmt.Errorf("coordinates (%d,%d): %w", col, row, err)
	}
	if setErr := f.SetCellValue(sheet, cell, value); setErr != nil {
		return fmt.Errorf("set cell %s: %w", cell, setErr)
	}
	return nil
}

// ImportMasters imports masters from an Excel workbook (Lookup Masters sheet).
func (r *LookupMasterRepository) ImportMasters(ctx context.Context, content []byte) (success, skipped, failed int, errs []string, retErr error) {
	f, err := excelize.OpenReader(bytes.NewReader(content))
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("open excel: %w", err)
	}
	rows, err := f.GetRows("Lookup Masters")
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("read Lookup Masters sheet: %w", err)
	}
	for i, row := range rows[1:] { // skip header
		if len(row) < 3 {
			errs = append(errs, fmt.Sprintf("row %d: insufficient columns", i+2))
			failed++
			continue
		}
		code, displayName, tableName := row[0], row[1], row[2]
		if code == "" {
			skipped++
			continue
		}
		if insertErr := r.CreateMaster(ctx, &lookupmaster.LookupMaster{
			Code:        code,
			DisplayName: displayName,
			TableName:   tableName,
			IsActive:    true,
		}, "import"); insertErr != nil {
			errs = append(errs, fmt.Sprintf("row %d (%s): %v", i+2, code, insertErr))
			failed++
			continue
		}
		success++
	}
	return success, skipped, failed, errs, nil
}
