package postgres

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/xuri/excelize/v2"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// mbSpinTableName is the mst_lookup_master table name registered for the
// MB_SPIN lookup master. ListMasterOptions special-cases it to additionally
// surface denier/filament/LDR (planned + actual) in the combobox
// (U-mbspin-lookup-detail),
// since mst_lookup_master only stores a single code/label column pair and has
// no generic mechanism for extra display columns.
const mbSpinTableName = "mst_mb_spin"

// defaultMasterOptionsLimit caps ListMasterOptions results when the caller
// does not specify a limit (limit == 0). 200 matches the render cap the
// frontend combobox previously enforced client-side
// (master-lookup-field.tsx MAX_RENDERED_OPTIONS, now server-enforced instead)
// — comfortably larger than any lookup master except MB_SPIN (~2700 rows in
// production), while keeping a single unfiltered fetch cheap.
//
// ⭐ DIPERBARUI 2026-08-26 (perf: SP Code dropdown lag, server-side search).
// A negative limit means "no LIMIT clause at all" — only internal callers
// that need the complete, authoritative option set (e.g. costimportetl
// import validation, which must not silently treat rows past the combobox's
// default page as "invalid") should pass one.
const defaultMasterOptionsLimit = 200

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

// ListAllColumns returns every row of mst_lookup_master_column across all masters.
// The startup registry-divergence check needs the table as it actually exists in
// the running database — the UI "Add Column" action writes here directly, so the
// live contents can differ from anything the migrations declare.
func (r *LookupMasterRepository) ListAllColumns(ctx context.Context) ([]*lookupmaster.Column, error) {
	const q = `SELECT lmc_id::text, lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order
	           FROM mst_lookup_master_column
	           ORDER BY lmc_master_code, lmc_sort_order, lmc_column_name`

	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("list all lookup master columns: %w", err)
	}
	defer closeRows(rows)

	var out []*lookupmaster.Column
	for rows.Next() {
		c := &lookupmaster.Column{}
		if scanErr := rows.Scan(&c.ID, &c.MasterCode, &c.ColumnName, &c.DisplayName, &c.DataType, &c.SortOrder); scanErr != nil {
			return nil, fmt.Errorf("scan lookup master column: %w", scanErr)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all lookup master columns: %w", err)
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
func (r *LookupMasterRepository) ListMasterOptions(ctx context.Context, masterCode, search string, limit int) ([]lookupmaster.MasterOption, error) {
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
	// Filter out NULL code-field rows (e.g. mbs_orion_item_code is nullable) to
	// avoid scan errors and meaningless empty-string options in the validation set.
	//
	// ⭐ DIPERBARUI 2026-08-26 (U-mbspin-lookup-detail): for the MB_SPIN master
	// (mst_mb_spin table), also select mbs_denier/mbs_filament/mbs_ldr_prsn/
	// mbs_run_ldr_pct so the combobox can show them alongside the label to
	// disambiguate similar entries. These four column names are fixed literals
	// for this one known table, not registry input, so they don't need
	// quoteIdent.
	// ~~Previously selected mbs_dozing instead of the two LDR columns, but
	// mbs_dozing was withdrawn per D30 contamination on explicit user decision
	// 2026-08-26.~~
	// ⭐ DIPERBARUI 2026-08-26 (U-mbspin-lookup-detail, putaran ke-3): MB Spin
	// rows created through the application never get an Orion item code (see
	// mbspin.CreateCommand — it has no OrionItemCode field), so filtering on
	// "codeField IS NOT NULL" silently hid every app-created spin from this
	// dropdown. For mst_mb_spin only, the value expression falls back to
	// mbs_id (its permanent primary key) when mbs_orion_item_code is NULL,
	// and the NOT NULL filter is dropped so those rows are no longer excluded.
	// Every other lookup master keeps the original NOT NULL filter unchanged.
	isMBSpin := tableName == mbSpinTableName
	extraCols := ""
	if isMBSpin {
		extraCols = ", mbs_denier, mbs_filament, mbs_ldr_prsn, mbs_run_ldr_pct"
	}
	valueExpr := quoteIdent(codeField)
	notNullClause := fmt.Sprintf(" AND %s IS NOT NULL", quoteIdent(codeField))
	if isMBSpin {
		valueExpr = fmt.Sprintf("COALESCE(%s::text, mbs_id::text)", quoteIdent(codeField))
		notNullClause = ""
	}
	// ⭐ DIPERBARUI 2026-08-26 (perf: SP Code dropdown lag, server-side search):
	// search (bound as a query parameter — NEVER string-concatenated) matches
	// against both codeField and labelField so staff can search by either the
	// master's code or its display name. An empty/whitespace-only search adds
	// no predicate at all, so the empty-keyword result set is unchanged from
	// before this change (aside from the new default LIMIT below).
	var args []any
	searchClause := ""
	if trimmed := strings.TrimSpace(search); trimmed != "" {
		args = append(args, "%"+trimmed+"%")
		searchClause = fmt.Sprintf(" AND (%s ILIKE $%d OR %s ILIKE $%d)",
			quoteIdent(codeField), len(args), quoteIdent(labelField), len(args))
	}

	// limit == 0 (not specified by the caller) falls back to a sane default
	// rather than returning the whole table; a negative limit (only used by
	// internal callers, e.g. import validation) means "no LIMIT clause".
	effectiveLimit := limit
	if effectiveLimit == 0 {
		effectiveLimit = defaultMasterOptionsLimit
	}
	limitClause := ""
	if effectiveLimit > 0 {
		args = append(args, effectiveLimit)
		limitClause = fmt.Sprintf(" LIMIT $%d", len(args))
	}

	q := fmt.Sprintf(
		`SELECT %s::text, COALESCE(%s::text,'')%s FROM %s WHERE deleted_at IS NULL%s%s ORDER BY %s%s`,
		valueExpr, quoteIdent(labelField), extraCols,
		quoteIdent(tableName),
		notNullClause,
		searchClause,
		quoteIdent(labelField),
		limitClause,
	)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list master options for %q: %w", masterCode, err)
	}
	var out []lookupmaster.MasterOption
	for rows.Next() {
		opt, scanErr := scanMasterOptionRow(rows, isMBSpin)
		if scanErr != nil {
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

// scanMasterOptionRow scans one ListMasterOptions row. When includeMBSpinFields
// is true, it also scans the trailing mbs_denier/mbs_filament/mbs_ldr_prsn/
// mbs_run_ldr_pct columns (nullable — an absent DB value stays absent as nil,
// never a synthesized default; D13).
func scanMasterOptionRow(rows *sql.Rows, includeMBSpinFields bool) (lookupmaster.MasterOption, error) {
	var opt lookupmaster.MasterOption
	if !includeMBSpinFields {
		if err := rows.Scan(&opt.Value, &opt.Label); err != nil {
			return opt, err
		}
		return opt, nil
	}
	var denier, ldrPrsn, runLdrPct sql.NullFloat64
	var filament sql.NullInt64
	if err := rows.Scan(&opt.Value, &opt.Label, &denier, &filament, &ldrPrsn, &runLdrPct); err != nil {
		return opt, err
	}
	opt.Denier = nullableFloat64Ptr(denier)
	opt.Filament = nullableIntPtr(filament)
	opt.LdrPrsn = nullableFloat64Ptr(ldrPrsn)
	opt.RunLdrPct = nullableFloat64Ptr(runLdrPct)
	return opt, nil
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
	for i, h := range []string{"Code", "Display Name", "Table Name", "Code Field", "Label Field", "Is Active"} {
		if err := excelSetCell(f, sheet, i+1, 1, h); err != nil {
			return err
		}
	}
	for row, m := range masters {
		for col, v := range []interface{}{m.Code, m.DisplayName, m.TableName, m.CodeField, m.LabelField, m.IsActive} {
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
	defer func() {
		if cerr := f.Close(); cerr != nil {
			_ = cerr // non-critical: file already fully read
		}
	}()
	rows, err := f.GetRows("Lookup Masters")
	if err != nil {
		return 0, 0, 0, nil, fmt.Errorf("read Lookup Masters sheet: %w", err)
	}
	for i, row := range rows[1:] { // skip header
		s, sk, fa, rowErrs := r.importMasterRow(ctx, i, row)
		success += s
		skipped += sk
		failed += fa
		errs = append(errs, rowErrs...)
	}
	return success, skipped, failed, errs, nil
}

// importMasterRow processes a single import row; extracted to keep ImportMasters complexity under limit.
func (r *LookupMasterRepository) importMasterRow(ctx context.Context, i int, row []string) (success, skipped, failed int, errs []string) {
	if len(row) < 3 {
		if len(row) == 0 || row[0] == "" {
			return 0, 1, 0, nil
		}
		return 0, 0, 1, []string{fmt.Sprintf("row %d: insufficient columns", i+2)}
	}
	code, displayName, tableName, codeField, labelField := extractLookupImportRow(row)
	if code == "" {
		return 0, 1, 0, nil
	}
	if insertErr := r.CreateMaster(ctx, &lookupmaster.LookupMaster{
		Code:        code,
		DisplayName: displayName,
		TableName:   tableName,
		CodeField:   codeField,
		LabelField:  labelField,
		IsActive:    true,
	}, "import"); insertErr != nil {
		return 0, 0, 1, []string{fmt.Sprintf("row %d (%s): %v", i+2, code, insertErr)}
	}
	return 1, 0, 0, nil
}

// extractLookupImportRow reads column values from an import row, defaulting missing columns to "".
func extractLookupImportRow(row []string) (code, displayName, tableName, codeField, labelField string) {
	if len(row) > 0 {
		code = row[0]
	}
	if len(row) > 1 {
		displayName = row[1]
	}
	if len(row) > 2 {
		tableName = row[2]
	}
	if len(row) > 3 {
		codeField = row[3]
	}
	if len(row) > 4 {
		labelField = row[4]
	}
	return
}
