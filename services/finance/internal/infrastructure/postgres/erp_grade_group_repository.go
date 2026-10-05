package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// ErpGradeGroupRepository implements erprule.GradeGroupRepository over the
// cost_erp_grade replica (migrations 000104, 000547; design Part 1 §4.1;
// plan-03 P2-T3).
//
// It reads the replica and writes ceg_grade_group ONLY. No other replica
// column (name, active flag, synced_at) is ever written here; those belong
// to the read-only master sync (P0-T15), which in turn never writes
// ceg_grade_group (AC-11).
type ErpGradeGroupRepository struct{ db *DB }

// NewErpGradeGroupRepository constructs the repository.
func NewErpGradeGroupRepository(db *DB) *ErpGradeGroupRepository {
	return &ErpGradeGroupRepository{db: db}
}

var _ erprule.GradeGroupRepository = (*ErpGradeGroupRepository)(nil)

const gradeColumns = `ceg_grade_code, COALESCE(ceg_grade_name, ''), ceg_is_active, ceg_grade_group`

// setGradeGroupSQL is the only statement in the service that writes
// ceg_grade_group from user action. Its SET list must stay this one column.
const setGradeGroupSQL = `UPDATE cost_erp_grade SET ceg_grade_group = $2 WHERE ceg_grade_code = $1`

// GetGrade returns the grade, or erprule.ErrGradeNotFound.
func (r *ErpGradeGroupRepository) GetGrade(ctx context.Context, gradeCode string) (*erprule.Grade, error) {
	q := `SELECT ` + gradeColumns + ` FROM cost_erp_grade WHERE ceg_grade_code = $1`
	g, err := scanGrade(r.db.QueryRowContext(ctx, q, gradeCode))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, erprule.ErrGradeNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("erp grade get: %w", err)
	}
	return g, nil
}

// ListGrades returns one page of grades ordered by code, and the total
// count. UnassignedOnly returns the V-08 worklist (ceg_grade_group IS NULL).
// Search is a case-insensitive substring match on code or name, with LIKE
// wildcards in the input matched literally. PageSize <= 0 returns every match.
func (r *ErpGradeGroupRepository) ListGrades(ctx context.Context, f erprule.GradeFilter) (out []*erprule.Grade, total int64, err error) {
	var (
		conds []string
		args  []any
	)
	if f.UnassignedOnly {
		conds = append(conds, "ceg_grade_group IS NULL")
	}
	if s := strings.TrimSpace(f.Search); s != "" {
		args = append(args, "%"+escapeLikePattern(s)+"%")
		p := "$" + strconv.Itoa(len(args))
		conds = append(conds, "(ceg_grade_code ILIKE "+p+` ESCAPE '\' OR ceg_grade_name ILIKE `+p+` ESCAPE '\')`)
	}
	where := ""
	if len(conds) > 0 {
		where = " WHERE " + strings.Join(conds, " AND ")
	}
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cost_erp_grade`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("erp grade count: %w", err)
	}
	q := `SELECT ` + gradeColumns + ` FROM cost_erp_grade` + where + ` ORDER BY ceg_grade_code`
	q, args = appendPage(q, args, f.Page, f.PageSize)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("erp grade list: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("erp grade list close: %w", cerr)
		}
	}()
	for rows.Next() {
		g, serr := scanGrade(rows)
		if serr != nil {
			return nil, 0, fmt.Errorf("erp grade list scan: %w", serr)
		}
		out = append(out, g)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("erp grade list rows: %w", err)
	}
	return out, total, nil
}

// SetGradeGroup writes the grade's current group (nil clears it) to
// ceg_grade_group. erprule.ErrGradeNotFound when the grade does not exist.
func (r *ErpGradeGroupRepository) SetGradeGroup(ctx context.Context, grade *erprule.Grade) error {
	if grade == nil {
		return errors.New("erp grade set group: nil grade")
	}
	var group any
	if g := grade.Group(); g != nil {
		group = g.String()
	}
	res, err := r.db.ExecContext(ctx, setGradeGroupSQL, grade.Code(), group)
	if err != nil {
		return fmt.Errorf("erp grade set group: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp grade set group rows: %w", err)
	}
	if n == 0 {
		return erprule.ErrGradeNotFound
	}
	return nil
}

func scanGrade(s rowScanner) (*erprule.Grade, error) {
	var (
		code, name string
		isActive   bool
		group      sql.NullString
	)
	if err := s.Scan(&code, &name, &isActive, &group); err != nil {
		return nil, err
	}
	var gp *erprule.GradeGroup
	if group.Valid {
		g := erprule.GradeGroup(group.String)
		gp = &g
	}
	return erprule.ReconstructGrade(code, name, isActive, gp), nil
}

// escapeLikePattern escapes the LIKE metacharacters (\ % _) so user input is
// matched literally under ESCAPE '\'.
func escapeLikePattern(s string) string {
	return strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(s)
}
