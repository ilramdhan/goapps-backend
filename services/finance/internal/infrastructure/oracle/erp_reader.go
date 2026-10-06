package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// Master read queries (SELECT only; they pass CheckReadOnly). Column names
// follow the MGTDAT master-table family (see OM_GRADE_CODE_2 in
// shade_repository.go); the FRZ flag column names are to be confirmed
// against the ERP data dictionary before the first prod run.
const (
	erpMasterItemQuery  = `SELECT ITEM_CODE, ITEM_NAME, ITEM_FRZ_FLAG_NUM FROM MGTDAT.OM_ITEM`
	erpMasterGradeQuery = `SELECT GRADE_CODE, GRADE_NAME, GRADE_FRZ_FLAG_NUM FROM MGTDAT.OM_GRADE_CODE_1`
)

// frozenFlag is the FRZ_FLAG_NUM value marking a frozen (inactive) row.
const frozenFlag = 1

// ErpMasterReader reads OM_ITEM / OM_GRADE_CODE_1 via the read-only querier.
type ErpMasterReader struct {
	q ReadOnlyQuerier
}

var _ erpintegration.MasterReader = (*ErpMasterReader)(nil)

// NewErpMasterReader builds the reader. q is normally Client.ReadOnly().
func NewErpMasterReader(q ReadOnlyQuerier) *ErpMasterReader {
	return &ErpMasterReader{q: q}
}

// masterRow is the common shape of both master queries.
type masterRow struct {
	code   string
	name   string
	active bool
}

// ListItems implements erpintegration.MasterReader.
func (r *ErpMasterReader) ListItems(ctx context.Context) ([]erpintegration.MasterItem, error) {
	rows, err := r.readMaster(ctx, erpMasterItemQuery)
	if err != nil {
		return nil, fmt.Errorf("read OM_ITEM: %w", err)
	}
	out := make([]erpintegration.MasterItem, 0, len(rows))
	for _, m := range rows {
		out = append(out, erpintegration.MasterItem{Code: m.code, Name: m.name, Active: m.active})
	}
	return out, nil
}

// ListGrades implements erpintegration.MasterReader.
func (r *ErpMasterReader) ListGrades(ctx context.Context) ([]erpintegration.MasterGrade, error) {
	rows, err := r.readMaster(ctx, erpMasterGradeQuery)
	if err != nil {
		return nil, fmt.Errorf("read OM_GRADE_CODE_1: %w", err)
	}
	out := make([]erpintegration.MasterGrade, 0, len(rows))
	for _, m := range rows {
		out = append(out, erpintegration.MasterGrade{Code: m.code, Name: m.name, Active: m.active})
	}
	return out, nil
}

// readMaster runs one master query and scans code/name/frz rows. Rows with a
// blank code are skipped.
func (r *ErpMasterReader) readMaster(ctx context.Context, query string) (out []masterRow, err error) {
	if r == nil || r.q == nil {
		return nil, ErrNoReadConnection
	}
	rows, err := r.q.QueryRO(ctx, query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		var code, name sql.NullString
		var frz sql.NullInt64
		if err := rows.Scan(&code, &name, &frz); err != nil {
			return nil, fmt.Errorf("scan: %w", err)
		}
		c := strings.TrimSpace(code.String)
		if c == "" {
			continue
		}
		out = append(out, masterRow{
			code:   c,
			name:   strings.TrimSpace(name.String),
			active: !frz.Valid || frz.Int64 != frozenFlag,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate: %w", err)
	}
	return out, nil
}
