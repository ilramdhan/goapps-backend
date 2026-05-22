package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// CostProductMasterRepository implements costproductmaster.Repository.
type CostProductMasterRepository struct{ db *DB }

// NewCostProductMasterRepository constructs the repository.
func NewCostProductMasterRepository(db *DB) *CostProductMasterRepository {
	return &CostProductMasterRepository{db: db}
}

var _ costproductmaster.Repository = (*CostProductMasterRepository)(nil)

const cpmColumns = `
	cpm_product_sys_id,cpm_product_code,cpm_product_type_id,cpm_product_name,
	cpm_shade_code,cpm_grade_code,cpm_description,
	cpm_erp_item_code,cpm_erp_grade_code_1,cpm_erp_grade_code_2,
	cpm_erp_linked_at,cpm_erp_linked_by,
	cpm_is_active,
	cpm_created_at,cpm_created_by,cpm_updated_at,cpm_updated_by`

// Create inserts the product. product_code is generated atomically via generate_cost_product_code()
// inside the same INSERT, returning the new sys_id and code.
func (r *CostProductMasterRepository) Create(ctx context.Context, p *costproductmaster.CostProductMaster) error {
	const q = `
		INSERT INTO cost_product_master (
			cpm_product_code,cpm_product_type_id,cpm_product_name,cpm_shade_code,cpm_grade_code,cpm_description,
			cpm_is_active,cpm_created_at,cpm_created_by,cpm_updated_at,cpm_updated_by
		)
		VALUES (
			generate_cost_product_code($1, $7), $1, $2, $3, $4, $5,
			$6, $7, $8, $7, $8
		)
		RETURNING cpm_product_sys_id,cpm_product_code`
	var sysID int64
	var code string
	if err := r.db.QueryRowContext(ctx, q,
		p.ProductTypeID(), p.ProductName(), p.ShadeCode(), p.GradeCode(), p.Description(),
		p.IsActive(), p.CreatedAt(), p.CreatedBy(),
	).Scan(&sysID, &code); err != nil {
		if isProductMasterUniqueViolation(err) {
			return costproductmaster.ErrAlreadyExists
		}
		return fmt.Errorf("create cost_product_master: %w", err)
	}
	p.SetGeneratedCode(sysID, code)
	return nil
}

// GetBySysID loads by sys_id.
func (r *CostProductMasterRepository) GetBySysID(ctx context.Context, sysID int64) (*costproductmaster.CostProductMaster, error) {
	q := `SELECT ` + cpmColumns + ` FROM cost_product_master WHERE cpm_product_sys_id=$1`
	return r.scanRow(r.db.QueryRowContext(ctx, q, sysID))
}

// GetByCode loads by product_code.
func (r *CostProductMasterRepository) GetByCode(ctx context.Context, code string) (*costproductmaster.CostProductMaster, error) {
	q := `SELECT ` + cpmColumns + ` FROM cost_product_master WHERE cpm_product_code=$1`
	return r.scanRow(r.db.QueryRowContext(ctx, q, code))
}

// Update saves descriptive fields + ERP linkage + active flag.
func (r *CostProductMasterRepository) Update(ctx context.Context, p *costproductmaster.CostProductMaster) error {
	const q = `
		UPDATE cost_product_master SET
			cpm_product_name=$2,cpm_shade_code=$3,cpm_grade_code=$4,cpm_description=$5,
			cpm_erp_item_code=$6,cpm_erp_grade_code_1=$7,cpm_erp_grade_code_2=$8,
			cpm_erp_linked_at=$9,cpm_erp_linked_by=$10,
			cpm_is_active=$11,cpm_updated_at=$12,cpm_updated_by=$13
		WHERE cpm_product_sys_id=$1`
	var erpItem, erpGrade1, erpGrade2, erpBy sql.NullString
	if p.ErpItemCode() != "" {
		erpItem = sql.NullString{String: p.ErpItemCode(), Valid: true}
	}
	if p.ErpGradeCode1() != "" {
		erpGrade1 = sql.NullString{String: p.ErpGradeCode1(), Valid: true}
	}
	if p.ErpGradeCode2() != "" {
		erpGrade2 = sql.NullString{String: p.ErpGradeCode2(), Valid: true}
	}
	if p.ErpLinkedBy() != "" {
		erpBy = sql.NullString{String: p.ErpLinkedBy(), Valid: true}
	}
	var erpAt sql.NullTime
	if p.ErpLinkedAt() != nil {
		erpAt = sql.NullTime{Time: *p.ErpLinkedAt(), Valid: true}
	}
	res, err := r.db.ExecContext(ctx, q,
		p.ProductSysID(), p.ProductName(), p.ShadeCode(), p.GradeCode(), p.Description(),
		erpItem, erpGrade1, erpGrade2, erpAt, erpBy,
		p.IsActive(), p.UpdatedAt(), p.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update cost_product_master: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return costproductmaster.ErrNotFound
	}
	return nil
}

// List returns a filtered paginated list.
func (r *CostProductMasterRepository) List(ctx context.Context, f costproductmaster.Filter) ([]*costproductmaster.CostProductMaster, int64, error) {
	where := "FROM cost_product_master WHERE 1=1"
	args := []any{}
	idx := 1
	if f.Search != "" {
		where += fmt.Sprintf(` AND (LOWER(cpm_product_code) LIKE LOWER($%d) OR LOWER(cpm_product_name) LIKE LOWER($%d) OR LOWER(COALESCE(cpm_erp_item_code,'')) LIKE LOWER($%d))`, idx, idx, idx)
		args = append(args, "%"+f.Search+"%")
		idx++
	}
	if f.ProductTypeID > 0 {
		where += fmt.Sprintf(` AND cpm_product_type_id=$%d`, idx)
		args = append(args, f.ProductTypeID)
		idx++
	}
	if f.ShadeCode != "" {
		where += fmt.Sprintf(` AND cpm_shade_code=$%d`, idx)
		args = append(args, f.ShadeCode)
		idx++
	}
	switch f.ActiveFilter {
	case "active":
		where += ` AND cpm_is_active=TRUE`
	case "inactive":
		where += ` AND cpm_is_active=FALSE`
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) "+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count cost_product_master: %w", err)
	}

	sortCol := `cpm_product_code`
	switch f.SortBy {
	case "product_name":
		sortCol = `cpm_product_name`
	case "created_at":
		sortCol = `cpm_created_at`
	case "updated_at":
		sortCol = `cpm_updated_at`
	}
	dir := "ASC"
	if strings.EqualFold(f.SortOrder, "desc") {
		dir = "DESC"
	}
	page := max(f.Page, 1)
	pageSize := f.PageSize
	if pageSize < 1 {
		pageSize = 20
	}
	pageSize = min(pageSize, 200)
	offset := (page - 1) * pageSize

	q := `SELECT ` + cpmColumns + ` ` + where + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, sortCol, dir, idx, idx+1)
	args = append(args, pageSize, offset)

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list cost_product_master: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := []*costproductmaster.CostProductMaster{}
	for rows.Next() {
		p, sErr := r.scanRows(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		items = append(items, p)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate cost_product_master: %w", err)
	}
	return items, total, nil
}

// =============================================================================
// scan helpers
// =============================================================================

type cpmRow struct {
	sysID         int64
	code          string
	typeID        int32
	name          string
	shade, grade  sql.NullString
	desc          sql.NullString
	erpItem       sql.NullString
	erpG1, erpG2  sql.NullString
	erpAt         sql.NullTime
	erpBy         sql.NullString
	active        bool
	createdAt     time.Time
	createdBy     string
	updatedAt     time.Time
	updatedBy     string
}

func (r *CostProductMasterRepository) scanRow(row *sql.Row) (*costproductmaster.CostProductMaster, error) {
	var d cpmRow
	if err := row.Scan(
		&d.sysID, &d.code, &d.typeID, &d.name,
		&d.shade, &d.grade, &d.desc,
		&d.erpItem, &d.erpG1, &d.erpG2,
		&d.erpAt, &d.erpBy,
		&d.active,
		&d.createdAt, &d.createdBy, &d.updatedAt, &d.updatedBy,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, costproductmaster.ErrNotFound
		}
		return nil, fmt.Errorf("scan cost_product_master: %w", err)
	}
	return cpmFromRow(d), nil
}

func (r *CostProductMasterRepository) scanRows(rows *sql.Rows) (*costproductmaster.CostProductMaster, error) {
	var d cpmRow
	if err := rows.Scan(
		&d.sysID, &d.code, &d.typeID, &d.name,
		&d.shade, &d.grade, &d.desc,
		&d.erpItem, &d.erpG1, &d.erpG2,
		&d.erpAt, &d.erpBy,
		&d.active,
		&d.createdAt, &d.createdBy, &d.updatedAt, &d.updatedBy,
	); err != nil {
		return nil, fmt.Errorf("scan cost_product_master row: %w", err)
	}
	return cpmFromRow(d), nil
}

func cpmFromRow(d cpmRow) *costproductmaster.CostProductMaster {
	var erpAt *time.Time
	if d.erpAt.Valid {
		t := d.erpAt.Time
		erpAt = &t
	}
	grade := d.grade.String
	if grade == "" {
		grade = "AX"
	}
	return costproductmaster.Reconstruct(
		d.sysID, d.code, d.typeID,
		d.name, d.shade.String, grade, d.desc.String,
		d.erpItem.String, d.erpG1.String, d.erpG2.String,
		erpAt, d.erpBy.String,
		d.active,
		d.createdAt, d.createdBy, d.updatedAt, d.updatedBy,
	)
}

func isProductMasterUniqueViolation(err error) bool {
	var pqErr *pq.Error
	if errors.As(err, &pqErr) {
		return pqErr.Code == "23505"
	}
	return false
}
