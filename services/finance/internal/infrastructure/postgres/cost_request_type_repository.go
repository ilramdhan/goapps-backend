package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costrequesttype"
)

// CostRequestTypeRepository implements costrequesttype.Repository.
type CostRequestTypeRepository struct{ db *DB }

// NewCostRequestTypeRepository constructs the repo.
func NewCostRequestTypeRepository(db *DB) *CostRequestTypeRepository {
	return &CostRequestTypeRepository{db: db}
}

var _ costrequesttype.Repository = (*CostRequestTypeRepository)(nil)

const crtCols = `crt_type_id,crt_code,crt_display_name,crt_state_machine_variant,crt_default_urgency,crt_is_active`

// List returns a filtered paginated list.
func (r *CostRequestTypeRepository) List(ctx context.Context, f costrequesttype.Filter) ([]*costrequesttype.CostRequestType, int64, error) {
	where := "FROM cost_request_type WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(crt_code) LIKE LOWER($%d) OR LOWER(crt_display_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	switch f.ActiveFilter {
	case filterActive:
		where += ` AND crt_is_active=TRUE`
	case filterInactive:
		where += ` AND crt_is_active=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_request_type: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + crtCols + ` ` + where + fmt.Sprintf(` ORDER BY crt_code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_request_type: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	out := []*costrequesttype.CostRequestType{}
	for rows.Next() {
		t, sErr := scanCrtRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// GetByID loads one.
func (r *CostRequestTypeRepository) GetByID(ctx context.Context, id int32) (*costrequesttype.CostRequestType, error) {
	q := `SELECT ` + crtCols + ` FROM cost_request_type WHERE crt_type_id=$1`
	t, err := scanCrtRow(r.db.QueryRowContext(ctx, q, id))
	if err != nil {
		return nil, err
	}
	return t, nil
}

func scanCrtRow(row *sql.Row) (*costrequesttype.CostRequestType, error) {
	t := &costrequesttype.CostRequestType{}
	if err := row.Scan(&t.TypeID, &t.Code, &t.DisplayName, &t.StateMachineVariant, &t.DefaultUrgency, &t.IsActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costrequesttype.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_request_type: %w", err)
	}
	return t, nil
}

func scanCrtRows(rows *sql.Rows) (*costrequesttype.CostRequestType, error) {
	t := &costrequesttype.CostRequestType{}
	if err := rows.Scan(&t.TypeID, &t.Code, &t.DisplayName, &t.StateMachineVariant, &t.DefaultUrgency, &t.IsActive); err != nil {
		return nil, fmt.Errorf("scan cost_request_type row: %w", err)
	}
	return t, nil
}
