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
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
)

// uniqueComboConstraint is the name of the unique index protecting the
// (company, division, department, section) combination.
const uniqueComboConstraint = "unique_mapping_combo"

// pgFKViolationCode is PostgreSQL SQLSTATE for foreign key violation.
const pgFKViolationCode = "23503"

// CompanyMappingRepository implements companymapping.Repository using PostgreSQL.
type CompanyMappingRepository struct {
	db *DB
}

// NewCompanyMappingRepository creates a new CompanyMappingRepository.
func NewCompanyMappingRepository(db *DB) *CompanyMappingRepository {
	return &CompanyMappingRepository{db: db}
}

var _ companymapping.Repository = (*CompanyMappingRepository)(nil)

// =============================================================================
// CRUD
// =============================================================================

// Create inserts a new company mapping.
func (r *CompanyMappingRepository) Create(ctx context.Context, m *companymapping.CompanyMapping) error {
	query := `
		INSERT INTO mst_company_mapping (
			company_mapping_id, code, name,
			company_id, division_id, department_id, section_id,
			is_active, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
	`
	h := m.Hierarchy()
	_, err := r.db.ExecContext(ctx, query,
		m.ID(), m.Code().String(), m.Name().String(),
		h.CompanyID, h.DivisionID, h.DepartmentID, sectionPtr(h.SectionID),
		m.IsActive(), m.Audit().CreatedAt, m.Audit().CreatedBy,
	)
	if err != nil {
		return r.mapWriteError(err)
	}
	return nil
}

// GetByID retrieves a company mapping by ID with denormalized hierarchy.
func (r *CompanyMappingRepository) GetByID(ctx context.Context, id uuid.UUID) (*companymapping.CompanyMapping, error) {
	query := r.selectSQL() + ` WHERE m.company_mapping_id = $1 AND m.deleted_at IS NULL`
	return r.queryOne(ctx, query, id)
}

// Update persists changes to an existing company mapping.
func (r *CompanyMappingRepository) Update(ctx context.Context, m *companymapping.CompanyMapping) error {
	query := `
		UPDATE mst_company_mapping SET
			name = $2,
			company_id = $3, division_id = $4, department_id = $5, section_id = $6,
			is_active = $7, updated_at = $8, updated_by = $9
		WHERE company_mapping_id = $1 AND deleted_at IS NULL
	`
	h := m.Hierarchy()
	result, err := r.db.ExecContext(ctx, query,
		m.ID(), m.Name().String(),
		h.CompanyID, h.DivisionID, h.DepartmentID, sectionPtr(h.SectionID),
		m.IsActive(), m.Audit().UpdatedAt, m.Audit().UpdatedBy,
	)
	if err != nil {
		return r.mapWriteError(err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// Delete soft-deletes a company mapping.
func (r *CompanyMappingRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	// Reject deletes when any active (non-deleted) user still references this mapping.
	var inUse bool
	if err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(
			SELECT 1 FROM user_company_mappings ucm
			INNER JOIN mst_user u ON u.user_id = ucm.user_id AND u.deleted_at IS NULL
			WHERE ucm.company_mapping_id = $1
		)`, id,
	).Scan(&inUse); err != nil {
		return fmt.Errorf("failed to check mapping references: %w", err)
	}
	if inUse {
		return companymapping.ErrAssignedToUser
	}

	query := `
		UPDATE mst_company_mapping SET
			is_active = false, deleted_at = $2, deleted_by = $3
		WHERE company_mapping_id = $1 AND deleted_at IS NULL
	`
	result, err := r.db.ExecContext(ctx, query, id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete company mapping: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// List lists company mappings with pagination, search, and filters.
func (r *CompanyMappingRepository) List(ctx context.Context, params companymapping.ListParams) ([]*companymapping.CompanyMapping, int64, error) {
	conditions, args := r.buildListWhere(params)
	whereClause := strings.Join(conditions, " AND ")

	countQuery := `
		SELECT COUNT(*)
		FROM mst_company_mapping m
		LEFT JOIN mst_company    co ON co.company_id    = m.company_id
		LEFT JOIN mst_division   dv ON dv.division_id   = m.division_id
		LEFT JOIN mst_department dp ON dp.department_id = m.department_id
		LEFT JOIN mst_section    se ON se.section_id    = m.section_id
		WHERE ` + whereClause
	var total int64
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count company mappings: %w", err)
	}

	sortBy, sortOrder := r.resolveSort(params.SortBy, params.SortOrder)

	offset := (params.Page - 1) * params.PageSize
	argPos := len(args) + 1
	query := fmt.Sprintf(
		"%s WHERE %s ORDER BY %s %s, m.code ASC LIMIT $%d OFFSET $%d",
		r.selectSQL(), whereClause, sortBy, sortOrder, argPos, argPos+1,
	)
	args = append(args, params.PageSize, offset)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list company mappings: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("failed to close company_mapping list rows")
		}
	}()

	var results []*companymapping.CompanyMapping
	for rows.Next() {
		entity, sErr := r.scanRow(rows)
		if sErr != nil {
			return nil, 0, sErr
		}
		results = append(results, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating company mapping rows: %w", err)
	}
	return results, total, nil
}

// ExistsByCode returns whether a record with the given code exists.
func (r *CompanyMappingRepository) ExistsByCode(ctx context.Context, code string) (bool, error) {
	var exists bool
	query := "SELECT EXISTS(SELECT 1 FROM mst_company_mapping WHERE code = $1 AND deleted_at IS NULL)"
	if err := r.db.QueryRowContext(ctx, query, code).Scan(&exists); err != nil {
		return false, fmt.Errorf("failed to check company mapping existence: %w", err)
	}
	return exists, nil
}

// =============================================================================
// User ↔ mapping junction
// =============================================================================

// AssignToUser inserts (or updates) the user-mapping junction. When isPrimary
// is true the existing primary (if any) is unset transactionally.
func (r *CompanyMappingRepository) AssignToUser(ctx context.Context, userID, mappingID uuid.UUID, isPrimary bool, assignedBy string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if isPrimary {
			if _, err := tx.ExecContext(ctx,
				"UPDATE user_company_mappings SET is_primary = false WHERE user_id = $1 AND is_primary = true",
				userID,
			); err != nil {
				return fmt.Errorf("failed to clear existing primary mapping: %w", err)
			}
		}
		_, err := tx.ExecContext(ctx, `
			INSERT INTO user_company_mappings (user_id, company_mapping_id, is_primary, assigned_at, assigned_by)
			VALUES ($1, $2, $3, NOW(), $4)
			ON CONFLICT (user_id, company_mapping_id)
			DO UPDATE SET is_primary = EXCLUDED.is_primary, assigned_at = EXCLUDED.assigned_at, assigned_by = EXCLUDED.assigned_by
		`, userID, mappingID, isPrimary, assignedBy)
		if err != nil {
			var pgErr *pgconn.PgError
			if errors.As(err, &pgErr) && pgErr.Code == pgFKViolationCode {
				return shared.ErrNotFound
			}
			return fmt.Errorf("failed to assign company mapping to user: %w", err)
		}
		return nil
	})
}

// RemoveFromUser removes a user-mapping link.
func (r *CompanyMappingRepository) RemoveFromUser(ctx context.Context, userID, mappingID uuid.UUID) error {
	result, err := r.db.ExecContext(ctx,
		"DELETE FROM user_company_mappings WHERE user_id = $1 AND company_mapping_id = $2",
		userID, mappingID,
	)
	if err != nil {
		return fmt.Errorf("failed to remove user company mapping: %w", err)
	}
	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to read rows affected: %w", err)
	}
	if rows == 0 {
		return shared.ErrNotFound
	}
	return nil
}

// ListByUser fetches all mappings assigned to a user along with the primary id.
func (r *CompanyMappingRepository) ListByUser(ctx context.Context, userID uuid.UUID) ([]companymapping.UserAssignment, *uuid.UUID, error) {
	query := r.selectSQL() + `
		INNER JOIN user_company_mappings ucm ON ucm.company_mapping_id = m.company_mapping_id
		WHERE ucm.user_id = $1 AND m.deleted_at IS NULL
		ORDER BY ucm.is_primary DESC, m.code ASC
	`
	rows, err := r.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to list user company mappings: %w", err)
	}
	defer func() {
		if cErr := rows.Close(); cErr != nil {
			log.Warn().Err(cErr).Msg("failed to close user_company_mappings rows")
		}
	}()

	primaryQuery := `SELECT company_mapping_id FROM user_company_mappings WHERE user_id = $1 AND is_primary = true`
	var primaryID uuid.UUID
	var primaryPtr *uuid.UUID
	if err := r.db.QueryRowContext(ctx, primaryQuery, userID).Scan(&primaryID); err == nil {
		id := primaryID
		primaryPtr = &id
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, nil, fmt.Errorf("failed to fetch primary mapping: %w", err)
	}

	var results []companymapping.UserAssignment
	for rows.Next() {
		entity, sErr := r.scanRow(rows)
		if sErr != nil {
			return nil, nil, sErr
		}
		isPrimary := primaryPtr != nil && entity.ID() == *primaryPtr
		results = append(results, companymapping.UserAssignment{
			Mapping:   entity,
			IsPrimary: isPrimary,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, nil, fmt.Errorf("error iterating user mapping rows: %w", err)
	}
	return results, primaryPtr, nil
}

// =============================================================================
// Helpers
// =============================================================================

func sectionPtr(id *uuid.UUID) interface{} {
	if id == nil {
		return nil
	}
	return *id
}

func (r *CompanyMappingRepository) selectSQL() string {
	return `
		SELECT
			m.company_mapping_id, m.code, m.name,
			m.company_id, COALESCE(co.company_code,''), COALESCE(co.company_name,''),
			m.division_id, COALESCE(dv.division_code,''), COALESCE(dv.division_name,''),
			m.department_id, COALESCE(dp.department_code,''), COALESCE(dp.department_name,''),
			m.section_id, COALESCE(se.section_code,''), COALESCE(se.section_name,''),
			m.is_active,
			m.created_at, m.created_by, m.updated_at, m.updated_by, m.deleted_at, m.deleted_by
		FROM mst_company_mapping m
		LEFT JOIN mst_company    co ON co.company_id    = m.company_id
		LEFT JOIN mst_division   dv ON dv.division_id   = m.division_id
		LEFT JOIN mst_department dp ON dp.department_id = m.department_id
		LEFT JOIN mst_section    se ON se.section_id    = m.section_id
	`
}

func (r *CompanyMappingRepository) buildListWhere(params companymapping.ListParams) ([]string, []interface{}) {
	conditions := []string{"m.deleted_at IS NULL"}
	var args []interface{}
	argPos := 1

	if params.Search != "" {
		conditions = append(conditions, fmt.Sprintf(
			"(m.code ILIKE $%d OR m.name ILIKE $%d OR co.company_name ILIKE $%d OR dv.division_name ILIKE $%d OR dp.department_name ILIKE $%d OR COALESCE(se.section_name,'') ILIKE $%d)",
			argPos, argPos, argPos, argPos, argPos, argPos))
		args = append(args, "%"+params.Search+"%")
		argPos++
	}
	if params.CompanyID != nil {
		conditions = append(conditions, fmt.Sprintf("m.company_id = $%d", argPos))
		args = append(args, *params.CompanyID)
		argPos++
	}
	if params.DivisionID != nil {
		conditions = append(conditions, fmt.Sprintf("m.division_id = $%d", argPos))
		args = append(args, *params.DivisionID)
		argPos++
	}
	if params.DepartmentID != nil {
		conditions = append(conditions, fmt.Sprintf("m.department_id = $%d", argPos))
		args = append(args, *params.DepartmentID)
		argPos++
	}
	if params.SectionID != nil {
		conditions = append(conditions, fmt.Sprintf("m.section_id = $%d", argPos))
		args = append(args, *params.SectionID)
		argPos++
	}
	if params.IsActive != nil {
		conditions = append(conditions, fmt.Sprintf("m.is_active = $%d", argPos))
		args = append(args, *params.IsActive)
		argPos++ //nolint:ineffassign,wastedassign // keep counter consistent for future additions
		_ = argPos
	}
	return conditions, args
}

func (r *CompanyMappingRepository) resolveSort(sortBy, sortOrder string) (string, string) {
	sortColumnMap := map[string]string{
		"code":       "m.code",
		"name":       "m.name",
		"created_at": "m.created_at",
	}
	column := "m.code"
	if mapped, ok := sortColumnMap[sortBy]; ok {
		column = mapped
	}
	dir := sortASC
	if strings.EqualFold(sortOrder, sortDESC) {
		dir = sortDESC
	}
	return column, dir
}

func (r *CompanyMappingRepository) queryOne(ctx context.Context, query string, args ...interface{}) (*companymapping.CompanyMapping, error) {
	var row companyMappingRow
	err := r.db.QueryRowContext(ctx, query, args...).Scan(row.scanTargets()...)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, shared.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to fetch company mapping: %w", err)
	}
	return row.toDomain()
}

func (r *CompanyMappingRepository) scanRow(rows *sql.Rows) (*companymapping.CompanyMapping, error) {
	var row companyMappingRow
	if err := rows.Scan(row.scanTargets()...); err != nil {
		return nil, fmt.Errorf("failed to scan company mapping: %w", err)
	}
	return row.toDomain()
}

func (r *CompanyMappingRepository) mapWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) { //nolint:nestif // cohesive branch, extraction would scatter tightly-coupled logic
		if pgErr.Code == pgUniqueViolationCode {
			if pgErr.ConstraintName == uniqueComboConstraint {
				return companymapping.ErrComboTaken
			}
			return shared.ErrAlreadyExists
		}
		if pgErr.Code == pgFKViolationCode {
			return shared.ErrNotFound
		}
	}
	return fmt.Errorf("failed to write company mapping: %w", err)
}

// companyMappingRow is the scan target.
type companyMappingRow struct {
	ID             uuid.UUID
	Code           string
	Name           string
	CompanyID      uuid.UUID
	CompanyCode    string
	CompanyName    string
	DivisionID     uuid.UUID
	DivisionCode   string
	DivisionName   string
	DepartmentID   uuid.UUID
	DepartmentCode string
	DepartmentName string
	SectionID      *uuid.UUID
	SectionCode    string
	SectionName    string
	IsActive       bool
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      *time.Time
	UpdatedBy      *string
	DeletedAt      *time.Time
	DeletedBy      *string
}

func (r *companyMappingRow) scanTargets() []interface{} {
	return []interface{}{
		&r.ID, &r.Code, &r.Name,
		&r.CompanyID, &r.CompanyCode, &r.CompanyName,
		&r.DivisionID, &r.DivisionCode, &r.DivisionName,
		&r.DepartmentID, &r.DepartmentCode, &r.DepartmentName,
		&r.SectionID, &r.SectionCode, &r.SectionName,
		&r.IsActive,
		&r.CreatedAt, &r.CreatedBy, &r.UpdatedAt, &r.UpdatedBy, &r.DeletedAt, &r.DeletedBy,
	}
}

func (r *companyMappingRow) toDomain() (*companymapping.CompanyMapping, error) {
	code, err := companymapping.NewCode(r.Code)
	if err != nil {
		return nil, fmt.Errorf("invalid code from db: %w", err)
	}
	name, err := companymapping.NewName(r.Name)
	if err != nil {
		return nil, fmt.Errorf("invalid name from db: %w", err)
	}
	hierarchy := companymapping.Hierarchy{
		CompanyID:      r.CompanyID,
		CompanyCode:    r.CompanyCode,
		CompanyName:    r.CompanyName,
		DivisionID:     r.DivisionID,
		DivisionCode:   r.DivisionCode,
		DivisionName:   r.DivisionName,
		DepartmentID:   r.DepartmentID,
		DepartmentCode: r.DepartmentCode,
		DepartmentName: r.DepartmentName,
		SectionID:      r.SectionID,
		SectionCode:    r.SectionCode,
		SectionName:    r.SectionName,
	}
	audit := shared.AuditInfo{
		CreatedAt: r.CreatedAt,
		CreatedBy: r.CreatedBy,
		UpdatedAt: r.UpdatedAt,
		UpdatedBy: r.UpdatedBy,
		DeletedAt: r.DeletedAt,
		DeletedBy: r.DeletedBy,
	}
	return companymapping.Reconstruct(r.ID, code, name, hierarchy, r.IsActive, audit), nil
}
