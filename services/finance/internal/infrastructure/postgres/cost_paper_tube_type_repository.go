package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costpapertubetype"
)

// CostPaperTubeTypeRepository implements costpapertubetype.Repository.
type CostPaperTubeTypeRepository struct{ db *DB }

// NewCostPaperTubeTypeRepository constructs the repo.
func NewCostPaperTubeTypeRepository(db *DB) *CostPaperTubeTypeRepository {
	return &CostPaperTubeTypeRepository{db: db}
}

var _ costpapertubetype.Repository = (*CostPaperTubeTypeRepository)(nil)

const cpttCols = `cptt_paper_tube_type_id,cptt_code,cptt_display_name,cptt_is_active`

// List returns a filtered paginated list.
func (r *CostPaperTubeTypeRepository) List(ctx context.Context, f costpapertubetype.Filter) ([]*costpapertubetype.CostPaperTubeType, int64, error) {
	where := "FROM cost_paper_tube_type WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(cptt_code) LIKE LOWER($%d) OR LOWER(cptt_display_name) LIKE LOWER($%d))`, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	switch f.ActiveFilter {
	case "active":
		where += ` AND cptt_is_active=TRUE`
	case "inactive":
		where += ` AND cptt_is_active=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_paper_tube_type: %w", err)
	}

	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 50
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + cpttCols + ` ` + where + fmt.Sprintf(` ORDER BY cptt_code ASC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, pageSize, offset)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_paper_tube_type: %w", err)
	}
	defer func() { _ = rows.Close() }()
	out := []*costpapertubetype.CostPaperTubeType{}
	for rows.Next() {
		t := &costpapertubetype.CostPaperTubeType{}
		if sErr := rows.Scan(&t.PaperTubeTypeID, &t.Code, &t.DisplayName, &t.IsActive); sErr != nil {
			return nil, 0, fmt.Errorf("scan cost_paper_tube_type row: %w", sErr)
		}
		out = append(out, t)
	}
	return out, total, rows.Err()
}

// GetByID loads one.
func (r *CostPaperTubeTypeRepository) GetByID(ctx context.Context, id int32) (*costpapertubetype.CostPaperTubeType, error) {
	q := `SELECT ` + cpttCols + ` FROM cost_paper_tube_type WHERE cptt_paper_tube_type_id=$1`
	t := &costpapertubetype.CostPaperTubeType{}
	if err := r.db.QueryRowContext(ctx, q, id).Scan(&t.PaperTubeTypeID, &t.Code, &t.DisplayName, &t.IsActive); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costpapertubetype.ErrNotFound
		}
		return nil, fmt.Errorf("get cost_paper_tube_type: %w", err)
	}
	return t, nil
}
