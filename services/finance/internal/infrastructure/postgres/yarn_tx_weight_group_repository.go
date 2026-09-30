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
	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweightgroup"
)

// YarnTxWeightGroupRepository implements yarntxweightgroup.Repository using
// PostgreSQL (tables mst_yarn_tx_weight_group, mst_yarn_tx_weight_group_type
// and the ytw_group_id rule rows of mst_yarn_tx_weight — migration 000536).
type YarnTxWeightGroupRepository struct {
	db *DB
}

// NewYarnTxWeightGroupRepository creates a new YarnTxWeightGroupRepository.
func NewYarnTxWeightGroupRepository(db *DB) *YarnTxWeightGroupRepository {
	return &YarnTxWeightGroupRepository{db: db}
}

// Verify interface implementation at compile time.
var _ yarntxweightgroup.Repository = (*YarnTxWeightGroupRepository)(nil)

// ytwgConstraintProductType is the junction UNIQUE(product_type_id) constraint.
const ytwgConstraintProductType = "uq_mst_yarn_tx_weight_group_type_product_type"

// ytwgConstraintCode is the partial unique index on live group codes.
const ytwgConstraintCode = "uix_mst_yarn_tx_weight_group_code"

const yarnTxWeightGroupSelect = `
	SELECT g.ytwg_id, g.ytwg_code, g.ytwg_name, g.ytwg_description,
	       g.created_at, g.created_by, g.updated_at, g.updated_by, g.deleted_at, g.deleted_by
	FROM mst_yarn_tx_weight_group g
`

// Create persists the group, its type mappings and its rules in one transaction.
func (r *YarnTxWeightGroupRepository) Create(ctx context.Context, e *yarntxweightgroup.Entity) error {
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO mst_yarn_tx_weight_group (
				ytwg_id, ytwg_code, ytwg_name, ytwg_description, created_at, created_by
			) VALUES ($1,$2,$3,$4,$5,$6)
		`, e.ID(), e.Code(), e.Name(), nullableString(e.Description()), e.CreatedAt(), e.CreatedBy()); err != nil {
			return err
		}
		if err := insertGroupTypes(ctx, tx, e.ID(), e.ProductTypeIDs(), e.CreatedBy(), e.CreatedAt()); err != nil {
			return err
		}
		return insertGroupRules(ctx, tx, e.ID(), e.Rules(), e.CreatedBy(), e.CreatedAt())
	})
	if err != nil {
		return mapYarnTxWeightGroupWriteErr("create yarn tx weight group", err)
	}
	return nil
}

// GetByID retrieves a live group with its product types and rules.
func (r *YarnTxWeightGroupRepository) GetByID(ctx context.Context, id uuid.UUID) (*yarntxweightgroup.Entity, error) {
	p, err := scanYarnTxWeightGroup(r.db.QueryRowContext(ctx,
		yarnTxWeightGroupSelect+` WHERE g.ytwg_id = $1 AND g.deleted_at IS NULL`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, yarntxweightgroup.ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	params := []*yarntxweightgroup.ReconstructParams{p}
	if err := r.attachChildren(ctx, params); err != nil {
		return nil, err
	}
	return yarntxweightgroup.Reconstruct(*p), nil
}

// List retrieves live groups with filtering and pagination.
func (r *YarnTxWeightGroupRepository) List(ctx context.Context, filter yarntxweightgroup.ListFilter) ([]*yarntxweightgroup.Entity, int64, error) {
	filter.Validate()

	where := ` WHERE g.deleted_at IS NULL`
	args := make([]any, 0, 4)
	idx := 1
	if filter.Search != "" {
		where += fmt.Sprintf(` AND (g.ytwg_code ILIKE $%d OR g.ytwg_name ILIKE $%d OR g.ytwg_description ILIKE $%d
			OR EXISTS (SELECT 1 FROM mst_yarn_tx_weight_group_type st
			           JOIN cost_product_type spt ON spt.cpt_type_id = st.product_type_id
			           WHERE st.ytwg_id = g.ytwg_id
			             AND (spt.cpt_type_code ILIKE $%d OR spt.cpt_type_name ILIKE $%d)))`,
			idx, idx, idx, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.ProductTypeID > 0 {
		where += fmt.Sprintf(` AND EXISTS (SELECT 1 FROM mst_yarn_tx_weight_group_type ft
			WHERE ft.ytwg_id = g.ytwg_id AND ft.product_type_id = $%d)`, idx)
		args = append(args, filter.ProductTypeID)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM mst_yarn_tx_weight_group g`+where, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count yarn tx weight groups: %w", err)
	}

	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}
	q := yarnTxWeightGroupSelect + where +
		fmt.Sprintf(` ORDER BY %s LIMIT $%d OFFSET $%d`, yarnTxWeightGroupOrderBy(filter.SortBy, dir), idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	params, err := r.queryGroups(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	if err := r.attachChildren(ctx, params); err != nil {
		return nil, 0, err
	}
	items := make([]*yarntxweightgroup.Entity, len(params))
	for i, p := range params {
		items[i] = yarntxweightgroup.Reconstruct(*p)
	}
	return items, total, nil
}

// Update replaces the header, the type set and the rules in one transaction.
// Old rules are soft-deleted (audit trail kept) before the new set is inserted.
func (r *YarnTxWeightGroupRepository) Update(ctx context.Context, e *yarntxweightgroup.Entity) error {
	updatedBy := ""
	if e.UpdatedBy() != nil {
		updatedBy = *e.UpdatedBy()
	}
	now := time.Now()
	if e.UpdatedAt() != nil {
		now = *e.UpdatedAt()
	}
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `
			UPDATE mst_yarn_tx_weight_group SET
				ytwg_code = $2, ytwg_name = $3, ytwg_description = $4, updated_at = $5, updated_by = $6
			WHERE ytwg_id = $1 AND deleted_at IS NULL
		`, e.ID(), e.Code(), e.Name(), nullableString(e.Description()), now, updatedBy)
		if err != nil {
			return err
		}
		if err := requireOneRow(res); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM mst_yarn_tx_weight_group_type WHERE ytwg_id = $1`, e.ID()); err != nil {
			return err
		}
		if err := insertGroupTypes(ctx, tx, e.ID(), e.ProductTypeIDs(), updatedBy, now); err != nil {
			return err
		}
		if err := softDeleteGroupRules(ctx, tx, e.ID(), updatedBy, now); err != nil {
			return err
		}
		return insertGroupRules(ctx, tx, e.ID(), e.Rules(), updatedBy, now)
	})
	if err != nil {
		return mapYarnTxWeightGroupWriteErr("update yarn tx weight group", err)
	}
	return nil
}

// SoftDelete marks the group and its rules deleted and removes its type
// mappings so the types become free again, in one transaction.
func (r *YarnTxWeightGroupRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	now := time.Now()
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx,
			`UPDATE mst_yarn_tx_weight_group SET deleted_at = $2, deleted_by = $3 WHERE ytwg_id = $1 AND deleted_at IS NULL`,
			id, now, deletedBy)
		if err != nil {
			return err
		}
		if err := requireOneRow(res); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`DELETE FROM mst_yarn_tx_weight_group_type WHERE ytwg_id = $1`, id); err != nil {
			return err
		}
		return softDeleteGroupRules(ctx, tx, id, deletedBy, now)
	})
	if err != nil {
		return mapYarnTxWeightGroupWriteErr("soft delete yarn tx weight group", err)
	}
	return nil
}

// ExistsByCode reports whether another live group (not excludeID) uses code.
func (r *YarnTxWeightGroupRepository) ExistsByCode(ctx context.Context, code string, excludeID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx, `
		SELECT EXISTS(SELECT 1 FROM mst_yarn_tx_weight_group
		              WHERE ytwg_code = $1 AND ytwg_id <> $2 AND deleted_at IS NULL)`,
		code, excludeID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("yarn tx weight group exists by code: %w", err)
	}
	return exists, nil
}

// MissingProductTypes returns the ids absent from cost_product_type.
func (r *YarnTxWeightGroupRepository) MissingProductTypes(ctx context.Context, ids []int32) ([]int32, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT id FROM unnest($1::int[]) AS id
		WHERE NOT EXISTS (SELECT 1 FROM cost_product_type WHERE cpt_type_id = id)
		ORDER BY id`, pq.Array(ids))
	if err != nil {
		return nil, fmt.Errorf("missing product types: %w", err)
	}
	defer closeRows(rows)
	var missing []int32
	for rows.Next() {
		var id int32
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan missing product type: %w", err)
		}
		missing = append(missing, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate missing product types: %w", err)
	}
	return missing, nil
}

// FindProductTypeConflicts returns the ids already mapped to a live group
// other than excludeID, with type and owning group codes.
func (r *YarnTxWeightGroupRepository) FindProductTypeConflicts(ctx context.Context, ids []int32, excludeID uuid.UUID) ([]yarntxweightgroup.ProductTypeConflict, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	rows, err := r.db.QueryContext(ctx, `
		SELECT gt.product_type_id, COALESCE(pt.cpt_type_code, ''), g.ytwg_code
		FROM mst_yarn_tx_weight_group_type gt
		JOIN mst_yarn_tx_weight_group g ON g.ytwg_id = gt.ytwg_id AND g.deleted_at IS NULL
		LEFT JOIN cost_product_type pt ON pt.cpt_type_id = gt.product_type_id
		WHERE gt.product_type_id = ANY($1) AND gt.ytwg_id <> $2
		ORDER BY pt.cpt_type_code`, pq.Array(ids), excludeID)
	if err != nil {
		return nil, fmt.Errorf("find product type conflicts: %w", err)
	}
	defer closeRows(rows)
	var out []yarntxweightgroup.ProductTypeConflict
	for rows.Next() {
		var c yarntxweightgroup.ProductTypeConflict
		if err := rows.Scan(&c.ProductTypeID, &c.ProductTypeCode, &c.GroupCode); err != nil {
			return nil, fmt.Errorf("scan product type conflict: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate product type conflicts: %w", err)
	}
	return out, nil
}

// yarnTxWeightGroupOrderBy maps an API sort key to a whitelisted ORDER BY clause.
func yarnTxWeightGroupOrderBy(sortBy, dir string) string {
	switch sortBy {
	case yarntxweightgroup.SortByName:
		return "g.ytwg_name " + dir + ", g.ytwg_code ASC"
	case yarntxweightgroup.SortByCreatedAt:
		return "g.created_at " + dir
	case yarntxweightgroup.SortByUpdatedAt:
		return "g.updated_at " + dir + " NULLS LAST"
	default:
		return "g.ytwg_code " + dir
	}
}

func (r *YarnTxWeightGroupRepository) queryGroups(ctx context.Context, q string, args ...any) ([]*yarntxweightgroup.ReconstructParams, error) {
	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list yarn tx weight groups: %w", err)
	}
	defer closeRows(rows)
	var out []*yarntxweightgroup.ReconstructParams
	for rows.Next() {
		p, err := scanYarnTxWeightGroup(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate yarn tx weight groups: %w", err)
	}
	return out, nil
}

func insertGroupTypes(ctx context.Context, tx *sql.Tx, groupID uuid.UUID, ids []int32, by string, at time.Time) error {
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO mst_yarn_tx_weight_group_type (ytwg_id, product_type_id, created_at, created_by)
			VALUES ($1,$2,$3,$4)`, groupID, id, at, by); err != nil {
			return err
		}
	}
	return nil
}

// insertGroupRules writes the rule rows with ytw_product_type_id NULL: the
// group, not the type, owns them since 000536.
func insertGroupRules(ctx context.Context, tx *sql.Tx, groupID uuid.UUID, rules []yarntxweightgroup.Rule, by string, at time.Time) error {
	for _, rule := range rules {
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO mst_yarn_tx_weight (
				ytw_id, ytw_group_id, ytw_grade, ytw_mode, ytw_value, ytw_description, created_at, created_by
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			uuid.New(), groupID, rule.Grade.String(), rule.Mode.String(), rule.Value,
			nullableString(rule.Description), at, by); err != nil {
			return err
		}
	}
	return nil
}

func softDeleteGroupRules(ctx context.Context, tx *sql.Tx, groupID uuid.UUID, by string, at time.Time) error {
	_, err := tx.ExecContext(ctx,
		`UPDATE mst_yarn_tx_weight SET deleted_at = $2, deleted_by = $3 WHERE ytw_group_id = $1 AND deleted_at IS NULL`,
		groupID, at, by)
	return err
}

func requireOneRow(res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return yarntxweightgroup.ErrNotFound
	}
	return nil
}

// mapYarnTxWeightGroupWriteErr turns the unique indexes that back the
// application checks (lost races) into domain errors.
func mapYarnTxWeightGroupWriteErr(op string, err error) error {
	if errors.Is(err, yarntxweightgroup.ErrNotFound) {
		return yarntxweightgroup.ErrNotFound
	}
	if sqlState, constraint, ok := pgErrorInfo(err); ok && sqlState == sqlStateUniqueViolation {
		switch constraint {
		case ytwgConstraintProductType:
			return yarntxweightgroup.ErrProductTypeAlreadyMapped
		case ytwgConstraintCode:
			return yarntxweightgroup.ErrCodeAlreadyExists
		}
	}
	return fmt.Errorf("%s: %w", op, err)
}

func scanYarnTxWeightGroup(s yarnTxWeightScanner) (*yarntxweightgroup.ReconstructParams, error) {
	var (
		id          uuid.UUID
		code, name  string
		description sql.NullString
		createdAt   time.Time
		createdBy   string
		updatedAt   sql.NullTime
		updatedBy   sql.NullString
		deletedAt   sql.NullTime
		deletedBy   sql.NullString
	)
	if err := s.Scan(&id, &code, &name, &description, &createdAt, &createdBy,
		&updatedAt, &updatedBy, &deletedAt, &deletedBy); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		return nil, fmt.Errorf("scan yarn tx weight group: %w", err)
	}
	return &yarntxweightgroup.ReconstructParams{
		ID: id, Code: code, Name: name, Description: description.String,
		CreatedAt: createdAt, CreatedBy: createdBy,
		UpdatedAt: nullableTimePtr(updatedAt), UpdatedBy: nullableStringPtr(updatedBy),
		DeletedAt: nullableTimePtr(deletedAt), DeletedBy: nullableStringPtr(deletedBy),
	}, nil
}
