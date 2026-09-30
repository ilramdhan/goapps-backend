// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// YarnTxWeightRepository implements yarntxweight.Repository using PostgreSQL
// (table mst_yarn_tx_weight, migration 000528).
type YarnTxWeightRepository struct {
	db *DB
}

// NewYarnTxWeightRepository creates a new YarnTxWeightRepository.
func NewYarnTxWeightRepository(db *DB) *YarnTxWeightRepository {
	return &YarnTxWeightRepository{db: db}
}

// Verify interface implementation at compile time.
var _ yarntxweight.Repository = (*YarnTxWeightRepository)(nil)

// yarnTxWeightSelect joins cost_product_type so the entity carries the type
// code/name for display. Filters are appended after the FROM/JOIN.
// Since 000536 group-owned rule rows carry a NULL ytw_product_type_id, so it
// is COALESCEd to 0 to keep the int32 scan safe for this legacy view.
const yarnTxWeightSelect = `
	SELECT w.ytw_id, COALESCE(w.ytw_product_type_id, 0), COALESCE(pt.cpt_type_code, ''), COALESCE(pt.cpt_type_name, ''),
	       w.ytw_grade, w.ytw_mode, w.ytw_value, w.ytw_description, w.ytw_oracle_sys_id,
	       w.created_at, w.created_by, w.updated_at, w.updated_by, w.deleted_at, w.deleted_by
	FROM mst_yarn_tx_weight w
	LEFT JOIN cost_product_type pt ON pt.cpt_type_id = w.ytw_product_type_id
`

// Create persists a new rule.
func (r *YarnTxWeightRepository) Create(ctx context.Context, e *yarntxweight.Entity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_yarn_tx_weight (
			ytw_id, ytw_product_type_id, ytw_grade, ytw_mode, ytw_value,
			ytw_description, ytw_oracle_sys_id, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)
	`,
		e.ID(), e.ProductTypeID(), e.Grade().String(), e.Mode().String(), e.Value(),
		nullableString(e.Description()), nullableString(e.OracleSysID()), e.CreatedAt(), e.CreatedBy(),
	)
	if err != nil {
		if isPGUniqueViolation(err) {
			return yarntxweight.ErrAlreadyExists
		}
		return fmt.Errorf("create yarn tx weight: %w", err)
	}
	return nil
}

// GetByID retrieves a live rule by id.
func (r *YarnTxWeightRepository) GetByID(ctx context.Context, id uuid.UUID) (*yarntxweight.Entity, error) {
	e, err := scanYarnTxWeight(r.db.QueryRowContext(ctx,
		yarnTxWeightSelect+` WHERE w.ytw_id = $1 AND w.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, yarntxweight.ErrNotFound
	}
	return e, err
}

// List retrieves live rules with filtering and pagination.
func (r *YarnTxWeightRepository) List(ctx context.Context, filter yarntxweight.ListFilter) ([]*yarntxweight.Entity, int64, error) {
	filter.Validate()

	where := ` WHERE w.deleted_at IS NULL`
	args := make([]any, 0, 5)
	idx := 1
	if filter.Search != "" {
		where += fmt.Sprintf(` AND (pt.cpt_type_code ILIKE $%d OR pt.cpt_type_name ILIKE $%d OR w.ytw_description ILIKE $%d)`, idx, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.ProductTypeID > 0 {
		where += fmt.Sprintf(` AND w.ytw_product_type_id = $%d`, idx)
		args = append(args, filter.ProductTypeID)
		idx++
	}
	if filter.Grade != "" {
		where += fmt.Sprintf(` AND w.ytw_grade = $%d`, idx)
		args = append(args, filter.Grade.String())
		idx++
	}

	var total int64
	countQ := `SELECT COUNT(*) FROM mst_yarn_tx_weight w LEFT JOIN cost_product_type pt ON pt.cpt_type_id = w.ytw_product_type_id` + where
	if err := r.db.QueryRowContext(ctx, countQ, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count yarn tx weight: %w", err)
	}

	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}
	q := yarnTxWeightSelect + where +
		fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, yarnTxWeightOrderBy(filter.SortBy, dir), idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list yarn tx weight: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	var items []*yarntxweight.Entity
	for rows.Next() {
		e, scanErr := scanYarnTxWeight(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate yarn tx weight: %w", err)
	}
	return items, total, nil
}

// Update persists mode/value/description changes.
func (r *YarnTxWeightRepository) Update(ctx context.Context, e *yarntxweight.Entity) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE mst_yarn_tx_weight SET
			ytw_mode        = $2,
			ytw_value       = $3,
			ytw_description = $4,
			updated_at      = $5,
			updated_by      = $6
		WHERE ytw_id = $1 AND deleted_at IS NULL
	`, e.ID(), e.Mode().String(), e.Value(), nullableString(e.Description()), e.UpdatedAt(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("update yarn tx weight: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return yarntxweight.ErrNotFound
	}
	return nil
}

// SoftDelete marks a live rule as deleted.
func (r *YarnTxWeightRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE mst_yarn_tx_weight SET deleted_at=$2, deleted_by=$3 WHERE ytw_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy)
	if err != nil {
		return fmt.Errorf("soft delete yarn tx weight: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return yarntxweight.ErrNotFound
	}
	return nil
}

// ExistsByTypeGrade reports whether a live rule exists for the pair.
func (r *YarnTxWeightRepository) ExistsByTypeGrade(ctx context.Context, productTypeID int32, grade yarntxweight.Grade) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_yarn_tx_weight WHERE ytw_product_type_id=$1 AND ytw_grade=$2 AND deleted_at IS NULL)`,
		productTypeID, grade.String()).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by type grade: %w", err)
	}
	return exists, nil
}

// ProductTypeExists reports whether cost_product_type has the id.
func (r *YarnTxWeightRepository) ProductTypeExists(ctx context.Context, productTypeID int32) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM cost_product_type WHERE cpt_type_id=$1)`, productTypeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("product type exists: %w", err)
	}
	return exists, nil
}

// yarnTxWeightSortUpdatedAt is the API sort key for last-modified ordering.
const yarnTxWeightSortUpdatedAt = "updated_at"

// yarnTxWeightOrderBy maps an API sort key to a whitelisted ORDER BY clause.
// product_type sorts by type code and then by grade in display order.
func yarnTxWeightOrderBy(sortBy, dir string) string {
	gradeOrder := `array_position(ARRAY['AE','A9','A','B','C']::varchar[], w.ytw_grade)`
	switch sortBy {
	case "grade":
		return fmt.Sprintf("%s %s, pt.cpt_type_code ASC", gradeOrder, dir)
	case sortKeyCreatedAt:
		return "w.created_at " + dir
	case yarnTxWeightSortUpdatedAt:
		return "w.updated_at " + dir + " NULLS LAST"
	default:
		return fmt.Sprintf("pt.cpt_type_code %s, %s ASC", dir, gradeOrder)
	}
}

type yarnTxWeightScanner interface {
	Scan(dest ...any) error
}

func scanYarnTxWeight(s yarnTxWeightScanner) (*yarntxweight.Entity, error) {
	var (
		id            uuid.UUID
		productTypeID int32
		typeCode      string
		typeName      string
		grade         string
		mode          string
		value         float64
		description   sql.NullString
		oracleSysID   sql.NullString
		createdAt     time.Time
		createdBy     string
		updatedAt     sql.NullTime
		updatedBy     sql.NullString
		deletedAt     sql.NullTime
		deletedBy     sql.NullString
	)
	if err := s.Scan(&id, &productTypeID, &typeCode, &typeName, &grade, &mode, &value,
		&description, &oracleSysID, &createdAt, &createdBy, &updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("scan yarn tx weight: %w", err)
	}
	return yarntxweight.Reconstruct(yarntxweight.ReconstructParams{
		ID: id, ProductTypeID: productTypeID, ProductTypeCode: typeCode, ProductTypeName: typeName,
		Grade: yarntxweight.Grade(grade), Mode: yarntxweight.Mode(mode), Value: value,
		Description: description.String, OracleSysID: oracleSysID.String,
		CreatedAt: createdAt, CreatedBy: createdBy,
		UpdatedAt: nullableTimePtr(updatedAt), UpdatedBy: nullableStringPtr(updatedBy),
		DeletedAt: nullableTimePtr(deletedAt), DeletedBy: nullableStringPtr(deletedBy),
	}), nil
}
