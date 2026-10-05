package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// ErpVallossRuleRepository implements erprule.VallossRuleRepository over
// cst_erp_valloss_rule (migration 000547; design Part 1 §4.1; plan-03 P2-T3).
//
// Deletion is soft: a deactivated rule is persisted through Update with
// cevr_is_active = false. uk_cevr_key allows one ACTIVE rule per
// (fg_type, prod_type, grade_group), so a clash maps to
// erprule.ErrDuplicateRule. Value loss is bound and scanned as text so it
// never passes through a float.
type ErpVallossRuleRepository struct{ db *DB }

// NewErpVallossRuleRepository constructs the repository.
func NewErpVallossRuleRepository(db *DB) *ErpVallossRuleRepository {
	return &ErpVallossRuleRepository{db: db}
}

var _ erprule.VallossRuleRepository = (*ErpVallossRuleRepository)(nil)

// uk_cevr_key is the partial unique index on the active rule key.
const cevrKeyConstraint = "uk_cevr_key"

const vallossRuleColumns = `
	cevr_id, cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis,
	cevr_val_loss::text, cevr_is_active, created_at, created_by,
	updated_at, COALESCE(updated_by, '')`

const insertVallossRuleSQL = `
INSERT INTO cst_erp_valloss_rule
       (cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss,
        cevr_is_active, created_at, created_by)
VALUES ($1, $2, $3, $4, $5::numeric, $6, $7, $8)
RETURNING ` + vallossRuleColumns

const selectVallossRuleByIDSQL = `SELECT ` + vallossRuleColumns + `
  FROM cst_erp_valloss_rule WHERE cevr_id = $1`

// updateVallossRuleSQL never re-activates an inactive row: an Update of an
// active domain rule whose row was concurrently soft-deleted affects 0 rows
// (reported as ErrRuleInactive) instead of resurrecting it.
const updateVallossRuleSQL = `
UPDATE cst_erp_valloss_rule
   SET cevr_basis     = $2,
       cevr_val_loss  = $3::numeric,
       cevr_is_active = $4,
       updated_at     = $5,
       updated_by     = $6
 WHERE cevr_id = $1
   AND (cevr_is_active OR NOT $4)`

const existsVallossRuleSQL = `SELECT cevr_is_active FROM cst_erp_valloss_rule WHERE cevr_id = $1`

// Create inserts an active rule and returns it with cevr_id set.
func (r *ErpVallossRuleRepository) Create(ctx context.Context, rule *erprule.VallossRule) (*erprule.VallossRule, error) {
	if rule == nil {
		return nil, errors.New("erp valloss rule create: nil rule")
	}
	k := rule.Key()
	row := r.db.QueryRowContext(ctx, insertVallossRuleSQL,
		k.FgType.String(), k.ProdType.String(), k.GradeGroup.String(), rule.Basis().String(),
		rule.ValLoss().String(), rule.IsActive(), rule.CreatedAt(), rule.CreatedBy())
	out, err := scanVallossRule(row)
	if err != nil {
		if isVallossKeyViolation(err) {
			return nil, fmt.Errorf("%w: %s", erprule.ErrDuplicateRule, k)
		}
		return nil, fmt.Errorf("erp valloss rule create: %w", err)
	}
	return out, nil
}

// GetByID returns the rule (active or not), or erprule.ErrRuleNotFound.
func (r *ErpVallossRuleRepository) GetByID(ctx context.Context, id int64) (*erprule.VallossRule, error) {
	out, err := scanVallossRule(r.db.QueryRowContext(ctx, selectVallossRuleByIDSQL, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, erprule.ErrRuleNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("erp valloss rule get: %w", err)
	}
	return out, nil
}

// List returns one page of rules ordered by (fg type, prod type, grade
// group, id) and the total count. PageSize <= 0 returns every match.
func (r *ErpVallossRuleRepository) List(ctx context.Context, f erprule.VallossRuleFilter) (out []*erprule.VallossRule, total int64, err error) {
	where, args := vallossRuleWhere(f)
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM cst_erp_valloss_rule`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("erp valloss rule count: %w", err)
	}
	q := `SELECT ` + vallossRuleColumns + ` FROM cst_erp_valloss_rule` + where +
		` ORDER BY cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_id`
	q, args = appendPage(q, args, f.Page, f.PageSize)
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("erp valloss rule list: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("erp valloss rule list close: %w", cerr)
		}
	}()
	for rows.Next() {
		rule, serr := scanVallossRule(rows)
		if serr != nil {
			return nil, 0, fmt.Errorf("erp valloss rule list scan: %w", serr)
		}
		out = append(out, rule)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("erp valloss rule list rows: %w", err)
	}
	return out, total, nil
}

// Update persists basis, value loss, is_active and updated_at/by. It returns
// ErrRuleNotFound for an unknown id and ErrRuleInactive when the stored row
// is already inactive and the rule would be re-activated.
func (r *ErpVallossRuleRepository) Update(ctx context.Context, rule *erprule.VallossRule) error {
	if rule == nil {
		return errors.New("erp valloss rule update: nil rule")
	}
	var updatedAt any
	if t := rule.UpdatedAt(); t != nil {
		updatedAt = *t
	}
	var updatedBy any
	if u := rule.UpdatedBy(); u != "" {
		updatedBy = u
	}
	res, err := r.db.ExecContext(ctx, updateVallossRuleSQL,
		rule.ID(), rule.Basis().String(), rule.ValLoss().String(), rule.IsActive(), updatedAt, updatedBy)
	if err != nil {
		if isVallossKeyViolation(err) {
			return fmt.Errorf("%w: %s", erprule.ErrDuplicateRule, rule.Key())
		}
		return fmt.Errorf("erp valloss rule update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("erp valloss rule update rows: %w", err)
	}
	if n > 0 {
		return nil
	}
	var active bool
	err = r.db.QueryRowContext(ctx, existsVallossRuleSQL, rule.ID()).Scan(&active)
	if errors.Is(err, sql.ErrNoRows) {
		return erprule.ErrRuleNotFound
	}
	if err != nil {
		return fmt.Errorf("erp valloss rule update check: %w", err)
	}
	return erprule.ErrRuleInactive
}

func vallossRuleWhere(f erprule.VallossRuleFilter) (string, []any) {
	var (
		conds []string
		args  []any
	)
	add := func(col, v string) {
		if v == "" {
			return
		}
		args = append(args, v)
		conds = append(conds, col+" = $"+strconv.Itoa(len(args)))
	}
	add("cevr_fg_type", f.FgType.String())
	add("cevr_prod_type", f.ProdType.String())
	add("cevr_grade_group", f.GradeGroup.String())
	add("cevr_basis", f.Basis.String())
	if !f.IncludeInactive {
		conds = append(conds, "cevr_is_active")
	}
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// appendPage adds LIMIT/OFFSET for a 1-based page. pageSize <= 0 means no
// limit; page < 1 is treated as 1.
func appendPage(q string, args []any, page, pageSize int) (string, []any) {
	if pageSize <= 0 {
		return q, args
	}
	if page < 1 {
		page = 1
	}
	args = append(args, pageSize, (page-1)*pageSize)
	return q + fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args
}

func scanVallossRule(s rowScanner) (*erprule.VallossRule, error) {
	var (
		id                                          int64
		fgType, prodType, group, basis, valLossText string
		isActive                                    bool
		createdAt                                   time.Time
		createdBy, updatedBy                        string
		updatedAt                                   sql.NullTime
	)
	if err := s.Scan(&id, &fgType, &prodType, &group, &basis, &valLossText, &isActive,
		&createdAt, &createdBy, &updatedAt, &updatedBy); err != nil {
		return nil, err
	}
	valLoss, err := decimal.NewFromString(valLossText)
	if err != nil {
		return nil, fmt.Errorf("parse cevr_val_loss %q: %w", valLossText, err)
	}
	key := erprule.RuleKey{
		FgType:     erprule.FgType(fgType),
		ProdType:   erprule.ProdType(prodType),
		GradeGroup: erprule.GradeGroup(group),
	}
	return erprule.ReconstructVallossRule(id, key, erprule.Basis(basis), valLoss, isActive,
		createdAt, createdBy, nullTimePtr(updatedAt), updatedBy), nil
}

// isVallossKeyViolation reports a unique violation of the active-key index.
// A unique violation without a constraint name (driver fallback path) is
// also treated as a key clash: uk_cevr_key is the table's only unique index
// besides the SERIAL primary key.
func isVallossKeyViolation(err error) bool {
	state, constraint, ok := pgErrorInfo(err)
	if !ok || state != sqlStateUniqueViolation {
		return false
	}
	return constraint == "" || constraint == cevrKeyConstraint
}
