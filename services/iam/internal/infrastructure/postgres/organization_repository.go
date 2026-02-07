// Package postgres provides PostgreSQL repository implementations.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/organization"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

const (
	sortASC  = "ASC"
	sortDESC = "DESC"
)

// =============================================================================
// COMPANY REPOSITORY
// =============================================================================

// CompanyRepository implements organization.CompanyRepository interface.
type CompanyRepository struct {
	db *DB
}

// NewCompanyRepository creates a new CompanyRepository.
func NewCompanyRepository(db *DB) *CompanyRepository {
	return &CompanyRepository{db: db}
}

// Create inserts a new company into the database.
func (r *CompanyRepository) Create(ctx context.Context, company *organization.Company) error {
	query := `
		INSERT INTO mst_company (company_id, company_code, company_name, description, is_active, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7)`
	_, err := r.db.ExecContext(ctx, query,
		company.ID(), company.Code(), company.Name(), company.Description(),
		company.IsActive(), company.Audit().CreatedAt, company.Audit().CreatedBy)
	return err
}

// GetByID retrieves a company by its unique identifier.
func (r *CompanyRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Company, error) {
	query := `
		SELECT company_id, company_code, company_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_company WHERE company_id = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanCompany(row)
}

// GetByCode retrieves a company by its unique code.
func (r *CompanyRepository) GetByCode(ctx context.Context, code string) (*organization.Company, error) {
	query := `
		SELECT company_id, company_code, company_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_company WHERE company_code = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, code)
	return r.scanCompany(row)
}

// Update persists changes to an existing company in the database.
func (r *CompanyRepository) Update(ctx context.Context, company *organization.Company) error {
	query := `
		UPDATE mst_company SET company_name = $2, description = $3, is_active = $4, updated_at = $5, updated_by = $6
		WHERE company_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, company.ID(), company.Name(), company.Description(),
		company.IsActive(), company.Audit().UpdatedAt, company.Audit().UpdatedBy)
	return err
}

// Delete soft-deletes a company by setting its deleted_at timestamp.
func (r *CompanyRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_company SET deleted_at = $2, deleted_by = $3, is_active = false WHERE company_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// List retrieves a paginated list of companies with optional filtering and sorting.
func (r *CompanyRepository) List(ctx context.Context, params organization.ListParams) ([]*organization.Company, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, "deleted_at IS NULL")
	if params.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(company_code) LIKE $%d OR LOWER(company_name) LIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(params.Search)+"%")
		argIdx++
	}
	if params.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *params.IsActive)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_company WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "created_at DESC"
	if params.SortBy != "" {
		order := sortASC
		if strings.ToUpper(params.SortOrder) == sortDESC {
			order = sortDESC
		}
		orderBy = fmt.Sprintf("%s %s", sanitizeColumn(params.SortBy), order)
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT company_id, company_code, company_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_company WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in company list")
		}
	}()

	var companies []*organization.Company
	for rows.Next() {
		company, err := r.scanCompanyRows(rows)
		if err != nil {
			return nil, 0, err
		}
		companies = append(companies, company)
	}
	return companies, total, nil
}

// ExistsByCode checks whether a company with the given code already exists.
func (r *CompanyRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_company WHERE company_code = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, code).Scan(&exists)
	return exists, err
}

// BatchCreate inserts multiple companies in a single transaction and returns the count of successfully inserted records.
func (r *CompanyRepository) BatchCreate(ctx context.Context, companies []*organization.Company) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
	}()

	count := 0
	for _, c := range companies {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO mst_company (company_id, company_code, company_name, description, is_active, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7)`,
			c.ID(), c.Code(), c.Name(), c.Description(), c.IsActive(), c.Audit().CreatedAt, c.Audit().CreatedBy)
		if err == nil {
			count++
		}
	}
	return count, tx.Commit()
}

func (r *CompanyRepository) scanCompany(row *sql.Row) (*organization.Company, error) {
	var id uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := row.Scan(&id, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructCompany(id, code, name, nullStringValue(description), isActive, audit), nil
}

func (r *CompanyRepository) scanCompanyRows(rows *sql.Rows) (*organization.Company, error) {
	var id uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := rows.Scan(&id, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructCompany(id, code, name, nullStringValue(description), isActive, audit), nil
}

// =============================================================================
// DIVISION REPOSITORY
// =============================================================================

// DivisionRepository implements organization.DivisionRepository interface.
type DivisionRepository struct {
	db *DB
}

// NewDivisionRepository creates a new DivisionRepository.
func NewDivisionRepository(db *DB) *DivisionRepository {
	return &DivisionRepository{db: db}
}

// Create inserts a new division into the database.
func (r *DivisionRepository) Create(ctx context.Context, division *organization.Division) error {
	query := `
		INSERT INTO mst_division (division_id, company_id, division_code, division_name, description, is_active, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.ExecContext(ctx, query,
		division.ID(), division.CompanyID(), division.Code(), division.Name(), division.Description(),
		division.IsActive(), division.Audit().CreatedAt, division.Audit().CreatedBy)
	return err
}

// GetByID retrieves a division by its unique identifier.
func (r *DivisionRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Division, error) {
	query := `
		SELECT division_id, company_id, division_code, division_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_division WHERE division_id = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanDivision(row)
}

// GetByCode retrieves a division by its unique code.
func (r *DivisionRepository) GetByCode(ctx context.Context, code string) (*organization.Division, error) {
	query := `
		SELECT division_id, company_id, division_code, division_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_division WHERE division_code = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, code)
	return r.scanDivision(row)
}

// Update persists changes to an existing division in the database.
func (r *DivisionRepository) Update(ctx context.Context, division *organization.Division) error {
	query := `
		UPDATE mst_division SET division_name = $2, description = $3, is_active = $4, updated_at = $5, updated_by = $6
		WHERE division_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, division.ID(), division.Name(), division.Description(),
		division.IsActive(), division.Audit().UpdatedAt, division.Audit().UpdatedBy)
	return err
}

// Delete soft-deletes a division by setting its deleted_at timestamp.
func (r *DivisionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_division SET deleted_at = $2, deleted_by = $3, is_active = false WHERE division_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// List retrieves a paginated list of divisions with optional filtering and sorting.
func (r *DivisionRepository) List(ctx context.Context, params organization.DivisionListParams) ([]*organization.Division, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, "deleted_at IS NULL")
	if params.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(division_code) LIKE $%d OR LOWER(division_name) LIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(params.Search)+"%")
		argIdx++
	}
	if params.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("is_active = $%d", argIdx))
		args = append(args, *params.IsActive)
		argIdx++
	}
	if params.CompanyID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("company_id = $%d", argIdx))
		args = append(args, *params.CompanyID)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM mst_division WHERE %s", whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "created_at DESC"
	if params.SortBy != "" {
		order := sortASC
		if strings.ToUpper(params.SortOrder) == sortDESC {
			order = sortDESC
		}
		orderBy = fmt.Sprintf("%s %s", sanitizeColumn(params.SortBy), order)
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT division_id, company_id, division_code, division_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_division WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in division list")
		}
	}()

	var divisions []*organization.Division
	for rows.Next() {
		division, err := r.scanDivisionRows(rows)
		if err != nil {
			return nil, 0, err
		}
		divisions = append(divisions, division)
	}
	return divisions, total, nil
}

// ExistsByCode checks whether a division with the given code already exists.
func (r *DivisionRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_division WHERE division_code = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, code).Scan(&exists)
	return exists, err
}

// BatchCreate inserts multiple divisions in a single transaction and returns the count of successfully inserted records.
func (r *DivisionRepository) BatchCreate(ctx context.Context, divisions []*organization.Division) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
	}()

	count := 0
	for _, d := range divisions {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO mst_division (division_id, company_id, division_code, division_name, description, is_active, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			d.ID(), d.CompanyID(), d.Code(), d.Name(), d.Description(), d.IsActive(), d.Audit().CreatedAt, d.Audit().CreatedBy)
		if err == nil {
			count++
		}
	}
	return count, tx.Commit()
}

func (r *DivisionRepository) scanDivision(row *sql.Row) (*organization.Division, error) {
	var id, companyID uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := row.Scan(&id, &companyID, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructDivision(id, companyID, code, name, nullStringValue(description), isActive, audit), nil
}

func (r *DivisionRepository) scanDivisionRows(rows *sql.Rows) (*organization.Division, error) {
	var id, companyID uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := rows.Scan(&id, &companyID, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructDivision(id, companyID, code, name, nullStringValue(description), isActive, audit), nil
}

// =============================================================================
// DEPARTMENT REPOSITORY
// =============================================================================

// DepartmentRepository implements organization.DepartmentRepository interface.
type DepartmentRepository struct {
	db *DB
}

// NewDepartmentRepository creates a new DepartmentRepository.
func NewDepartmentRepository(db *DB) *DepartmentRepository {
	return &DepartmentRepository{db: db}
}

// Create inserts a new department into the database.
func (r *DepartmentRepository) Create(ctx context.Context, department *organization.Department) error {
	query := `
		INSERT INTO mst_department (department_id, division_id, department_code, department_name, description, is_active, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.ExecContext(ctx, query,
		department.ID(), department.DivisionID(), department.Code(), department.Name(), department.Description(),
		department.IsActive(), department.Audit().CreatedAt, department.Audit().CreatedBy)
	return err
}

// GetByID retrieves a department by its unique identifier.
func (r *DepartmentRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Department, error) {
	query := `
		SELECT department_id, division_id, department_code, department_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_department WHERE department_id = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanDepartment(row)
}

// GetByCode retrieves a department by its unique code.
func (r *DepartmentRepository) GetByCode(ctx context.Context, code string) (*organization.Department, error) {
	query := `
		SELECT department_id, division_id, department_code, department_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_department WHERE department_code = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, code)
	return r.scanDepartment(row)
}

// Update persists changes to an existing department in the database.
func (r *DepartmentRepository) Update(ctx context.Context, department *organization.Department) error {
	query := `
		UPDATE mst_department SET department_name = $2, description = $3, is_active = $4, updated_at = $5, updated_by = $6
		WHERE department_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, department.ID(), department.Name(), department.Description(),
		department.IsActive(), department.Audit().UpdatedAt, department.Audit().UpdatedBy)
	return err
}

// Delete soft-deletes a department by setting its deleted_at timestamp.
func (r *DepartmentRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_department SET deleted_at = $2, deleted_by = $3, is_active = false WHERE department_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// List retrieves a paginated list of departments with optional filtering and sorting.
func (r *DepartmentRepository) List(ctx context.Context, params organization.DepartmentListParams) ([]*organization.Department, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, "d.deleted_at IS NULL")
	if params.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(d.department_code) LIKE $%d OR LOWER(d.department_name) LIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(params.Search)+"%")
		argIdx++
	}
	if params.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("d.is_active = $%d", argIdx))
		args = append(args, *params.IsActive)
		argIdx++
	}
	if params.DivisionID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("d.division_id = $%d", argIdx))
		args = append(args, *params.DivisionID)
		argIdx++
	}
	if params.CompanyID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("div.company_id = $%d", argIdx))
		args = append(args, *params.CompanyID)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM mst_department d 
		LEFT JOIN mst_division div ON d.division_id = div.division_id
		WHERE %s`, whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "d.created_at DESC"
	if params.SortBy != "" {
		order := sortASC
		if strings.ToUpper(params.SortOrder) == sortDESC {
			order = sortDESC
		}
		orderBy = fmt.Sprintf("d.%s %s", sanitizeColumn(params.SortBy), order)
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT d.department_id, d.division_id, d.department_code, d.department_name, d.description, d.is_active, d.created_at, d.created_by, d.updated_at, d.updated_by, d.deleted_at, d.deleted_by
		FROM mst_department d
		LEFT JOIN mst_division div ON d.division_id = div.division_id
		WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in department list")
		}
	}()

	var departments []*organization.Department
	for rows.Next() {
		department, err := r.scanDepartmentRows(rows)
		if err != nil {
			return nil, 0, err
		}
		departments = append(departments, department)
	}
	return departments, total, nil
}

// ExistsByCode checks whether a department with the given code already exists.
func (r *DepartmentRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_department WHERE department_code = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, code).Scan(&exists)
	return exists, err
}

// BatchCreate inserts multiple departments in a single transaction and returns the count of successfully inserted records.
func (r *DepartmentRepository) BatchCreate(ctx context.Context, departments []*organization.Department) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
	}()

	count := 0
	for _, d := range departments {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO mst_department (department_id, division_id, department_code, department_name, description, is_active, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			d.ID(), d.DivisionID(), d.Code(), d.Name(), d.Description(), d.IsActive(), d.Audit().CreatedAt, d.Audit().CreatedBy)
		if err == nil {
			count++
		}
	}
	return count, tx.Commit()
}

func (r *DepartmentRepository) scanDepartment(row *sql.Row) (*organization.Department, error) {
	var id, divisionID uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := row.Scan(&id, &divisionID, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructDepartment(id, divisionID, code, name, nullStringValue(description), isActive, audit), nil
}

func (r *DepartmentRepository) scanDepartmentRows(rows *sql.Rows) (*organization.Department, error) {
	var id, divisionID uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := rows.Scan(&id, &divisionID, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructDepartment(id, divisionID, code, name, nullStringValue(description), isActive, audit), nil
}

// =============================================================================
// SECTION REPOSITORY
// =============================================================================

// SectionRepository implements organization.SectionRepository interface.
type SectionRepository struct {
	db *DB
}

// NewSectionRepository creates a new SectionRepository.
func NewSectionRepository(db *DB) *SectionRepository {
	return &SectionRepository{db: db}
}

// Create inserts a new section into the database.
func (r *SectionRepository) Create(ctx context.Context, section *organization.Section) error {
	query := `
		INSERT INTO mst_section (section_id, department_id, section_code, section_name, description, is_active, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`
	_, err := r.db.ExecContext(ctx, query,
		section.ID(), section.DepartmentID(), section.Code(), section.Name(), section.Description(),
		section.IsActive(), section.Audit().CreatedAt, section.Audit().CreatedBy)
	return err
}

// GetByID retrieves a section by its unique identifier.
func (r *SectionRepository) GetByID(ctx context.Context, id uuid.UUID) (*organization.Section, error) {
	query := `
		SELECT section_id, department_id, section_code, section_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_section WHERE section_id = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, id)
	return r.scanSection(row)
}

// GetByCode retrieves a section by its unique code.
func (r *SectionRepository) GetByCode(ctx context.Context, code string) (*organization.Section, error) {
	query := `
		SELECT section_id, department_id, section_code, section_name, description, is_active, created_at, created_by, updated_at, updated_by, deleted_at, deleted_by
		FROM mst_section WHERE section_code = $1 AND deleted_at IS NULL`
	row := r.db.QueryRowContext(ctx, query, code)
	return r.scanSection(row)
}

// Update persists changes to an existing section in the database.
func (r *SectionRepository) Update(ctx context.Context, section *organization.Section) error {
	query := `
		UPDATE mst_section SET section_name = $2, description = $3, is_active = $4, updated_at = $5, updated_by = $6
		WHERE section_id = $1 AND deleted_at IS NULL`
	_, err := r.db.ExecContext(ctx, query, section.ID(), section.Name(), section.Description(),
		section.IsActive(), section.Audit().UpdatedAt, section.Audit().UpdatedBy)
	return err
}

// Delete soft-deletes a section by setting its deleted_at timestamp.
func (r *SectionRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `UPDATE mst_section SET deleted_at = $2, deleted_by = $3, is_active = false WHERE section_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return err
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// List retrieves a paginated list of sections with optional filtering and sorting.
func (r *SectionRepository) List(ctx context.Context, params organization.SectionListParams) ([]*organization.Section, int64, error) {
	var whereClauses []string
	var args []interface{}
	argIdx := 1

	whereClauses = append(whereClauses, "s.deleted_at IS NULL")
	if params.Search != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("(LOWER(s.section_code) LIKE $%d OR LOWER(s.section_name) LIKE $%d)", argIdx, argIdx))
		args = append(args, "%"+strings.ToLower(params.Search)+"%")
		argIdx++
	}
	if params.IsActive != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.is_active = $%d", argIdx))
		args = append(args, *params.IsActive)
		argIdx++
	}
	if params.DepartmentID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("s.department_id = $%d", argIdx))
		args = append(args, *params.DepartmentID)
		argIdx++
	}
	if params.DivisionID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("dept.division_id = $%d", argIdx))
		args = append(args, *params.DivisionID)
		argIdx++
	}
	if params.CompanyID != nil {
		whereClauses = append(whereClauses, fmt.Sprintf("div.company_id = $%d", argIdx))
		args = append(args, *params.CompanyID)
		argIdx++
	}

	whereClause := strings.Join(whereClauses, " AND ")
	countQuery := fmt.Sprintf(`
		SELECT COUNT(*) FROM mst_section s 
		LEFT JOIN mst_department dept ON s.department_id = dept.department_id
		LEFT JOIN mst_division div ON dept.division_id = div.division_id
		WHERE %s`, whereClause)
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderBy := "s.created_at DESC"
	if params.SortBy != "" {
		order := sortASC
		if strings.ToUpper(params.SortOrder) == sortDESC {
			order = sortDESC
		}
		orderBy = fmt.Sprintf("s.%s %s", sanitizeColumn(params.SortBy), order)
	}

	offset := (params.Page - 1) * params.PageSize
	query := fmt.Sprintf(`
		SELECT s.section_id, s.department_id, s.section_code, s.section_name, s.description, s.is_active, s.created_at, s.created_by, s.updated_at, s.updated_by, s.deleted_at, s.deleted_by
		FROM mst_section s
		LEFT JOIN mst_department dept ON s.department_id = dept.department_id
		LEFT JOIN mst_division div ON dept.division_id = div.division_id
		WHERE %s ORDER BY %s LIMIT $%d OFFSET $%d`, whereClause, orderBy, argIdx, argIdx+1)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Warn().Err(err).Msg("failed to close rows in section list")
		}
	}()

	var sections []*organization.Section
	for rows.Next() {
		section, err := r.scanSectionRows(rows)
		if err != nil {
			return nil, 0, err
		}
		sections = append(sections, section)
	}
	return sections, total, nil
}

// ExistsByCode checks whether a section with the given code already exists.
func (r *SectionRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	query := `SELECT EXISTS(SELECT 1 FROM mst_section WHERE section_code = $1 AND deleted_at IS NULL)`
	var exists bool
	err := r.db.QueryRowContext(ctx, query, code).Scan(&exists)
	return exists, err
}

// BatchCreate inserts multiple sections in a single transaction and returns the count of successfully inserted records.
func (r *SectionRepository) BatchCreate(ctx context.Context, sections []*organization.Section) (int, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() {
		if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
			log.Warn().Err(err).Msg("failed to rollback transaction")
		}
	}()

	count := 0
	for _, s := range sections {
		_, err := tx.ExecContext(ctx, `
			INSERT INTO mst_section (section_id, department_id, section_code, section_name, description, is_active, created_at, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
			s.ID(), s.DepartmentID(), s.Code(), s.Name(), s.Description(), s.IsActive(), s.Audit().CreatedAt, s.Audit().CreatedBy)
		if err == nil {
			count++
		}
	}
	return count, tx.Commit()
}

func (r *SectionRepository) scanSection(row *sql.Row) (*organization.Section, error) {
	var id, departmentID uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := row.Scan(&id, &departmentID, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if err == sql.ErrNoRows {
			return nil, shared.ErrNotFound
		}
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructSection(id, departmentID, code, name, nullStringValue(description), isActive, audit), nil
}

func (r *SectionRepository) scanSectionRows(rows *sql.Rows) (*organization.Section, error) {
	var id, departmentID uuid.UUID
	var code, name, createdBy string
	var description sql.NullString
	var isActive bool
	var createdAt time.Time
	var updatedAt, deletedAt sql.NullTime
	var updatedBy, deletedBy sql.NullString

	if err := rows.Scan(&id, &departmentID, &code, &name, &description, &isActive, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		return nil, err
	}

	audit := shared.AuditInfo{
		CreatedAt: createdAt,
		CreatedBy: createdBy,
		UpdatedAt: nullTimeToPtr(updatedAt),
		UpdatedBy: nullStringToPtr(updatedBy),
		DeletedAt: nullTimeToPtr(deletedAt),
		DeletedBy: nullStringToPtr(deletedBy),
	}
	return organization.ReconstructSection(id, departmentID, code, name, nullStringValue(description), isActive, audit), nil
}

// =============================================================================
// HELPER FUNCTIONS
// =============================================================================

// sanitizeColumn sanitizes column name to prevent SQL injection
func sanitizeColumn(column string) string {
	allowed := map[string]bool{
		"company_code":    true,
		"company_name":    true,
		"division_code":   true,
		"division_name":   true,
		"department_code": true,
		"department_name": true,
		"section_code":    true,
		"section_name":    true,
		"is_active":       true,
		"created_at":      true,
		"updated_at":      true,
	}
	if allowed[column] {
		return column
	}
	return "created_at"
}

// nullStringValue safely gets string value from sql.NullString
func nullStringValue(ns sql.NullString) string {
	if ns.Valid {
		return ns.String
	}
	return ""
}
