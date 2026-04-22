// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// Verify full interface implementation at compile time (both head + detail methods).
var _ rmgroup.Repository = (*RMGroupRepository)(nil)

// =============================================================================
// Detail operations
// =============================================================================

// AddDetail persists a new Detail row and writes a CREATE audit row in the same tx.
func (r *RMGroupRepository) AddDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		query := `
			INSERT INTO cst_rm_group_detail (
				group_detail_id, group_head_id, item_code, item_name, item_type_code,
				grade_code, item_grade, uom_code,
				market_percentage, market_value_rp,
				sort_order, is_active, is_dummy, created_at, created_by
			) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)
		`
		_, err := tx.ExecContext(ctx, query,
			detail.ID(), detail.HeadID(), detail.ItemCode().String(),
			nullableString(detail.ItemName()), nullableString(detail.ItemTypeCode()),
			nullableString(detail.GradeCode()), nullableString(detail.ItemGrade()), nullableString(detail.UOMCode()),
			detail.MarketPercentage(), detail.MarketValueRp(),
			detail.SortOrder(), detail.IsActive(), detail.IsDummy(),
			detail.CreatedAt(), detail.CreatedBy(),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return rmgroup.ErrItemAlreadyInOtherGroup
			}
			return fmt.Errorf("add rm group detail: %w", err)
		}
		return insertDetailAudit(ctx, tx, detail, auditActionCreate, detail.CreatedBy())
	})
}

// UpdateDetail persists changes to an existing detail and writes an UPDATE audit row in the same tx.
func (r *RMGroupRepository) UpdateDetail(ctx context.Context, detail *rmgroup.Detail) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		query := `
			UPDATE cst_rm_group_detail SET
				item_name=$2, item_type_code=$3, grade_code=$4, item_grade=$5, uom_code=$6,
				market_percentage=$7, market_value_rp=$8,
				sort_order=$9, is_active=$10, is_dummy=$11,
				updated_at=$12, updated_by=$13
			WHERE group_detail_id=$1 AND deleted_at IS NULL
		`
		res, err := tx.ExecContext(ctx, query,
			detail.ID(),
			nullableString(detail.ItemName()), nullableString(detail.ItemTypeCode()),
			nullableString(detail.GradeCode()), nullableString(detail.ItemGrade()), nullableString(detail.UOMCode()),
			detail.MarketPercentage(), detail.MarketValueRp(),
			detail.SortOrder(), detail.IsActive(), detail.IsDummy(),
			detail.UpdatedAt(), detail.UpdatedBy(),
		)
		if err != nil {
			if isUniqueViolation(err) {
				return rmgroup.ErrItemAlreadyInOtherGroup
			}
			return fmt.Errorf("update rm group detail: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			return rmgroup.ErrDetailNotFound
		}
		return insertDetailAudit(ctx, tx, detail, auditActionUpdate, derefString(detail.UpdatedBy()))
	})
}

// GetDetailByID retrieves a detail by ID.
func (r *RMGroupRepository) GetDetailByID(ctx context.Context, id uuid.UUID) (*rmgroup.Detail, error) {
	return r.scanDetail(r.db.QueryRowContext(ctx, detailSelectSQL+` WHERE group_detail_id=$1 AND deleted_at IS NULL`, id))
}

// GetActiveDetailByItemCodeGrade looks up the single active detail holding the
// given (item_code, grade_code) pair. The natural key for a RM "variant"
// matches the Oracle sync feed's (item_code, grade_code) key, so variants
// with the same item_code but different grade_code are independent.
// gradeCode "" matches rows with NULL or empty grade_code (mirrors the
// migration 000018 unique index that COALESCEs NULL to ”).
func (r *RMGroupRepository) GetActiveDetailByItemCodeGrade(ctx context.Context, itemCode rmgroup.ItemCode, gradeCode string) (*rmgroup.Detail, error) {
	return r.scanDetail(r.db.QueryRowContext(ctx,
		detailSelectSQL+` WHERE item_code=$1 AND COALESCE(grade_code,'')=$2 AND is_active=true AND deleted_at IS NULL LIMIT 1`,
		itemCode.String(), gradeCode))
}

// ListDetailsByHeadID returns every non-deleted detail for the given head, ordered by sort_order.
func (r *RMGroupRepository) ListDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	return r.listDetails(ctx,
		detailSelectSQL+` WHERE group_head_id=$1 AND deleted_at IS NULL ORDER BY sort_order ASC, item_code ASC`,
		headID)
}

// ListActiveDetailsByHeadID returns only active, non-deleted details (used by the calc engine).
func (r *RMGroupRepository) ListActiveDetailsByHeadID(ctx context.Context, headID uuid.UUID) ([]*rmgroup.Detail, error) {
	return r.listDetails(ctx,
		detailSelectSQL+` WHERE group_head_id=$1 AND is_active=true AND deleted_at IS NULL ORDER BY sort_order ASC, item_code ASC`,
		headID)
}

// SoftDeleteDetail marks a single detail row as deleted and writes a DELETE audit row in the same tx.
func (r *RMGroupRepository) SoftDeleteDetail(ctx context.Context, id uuid.UUID, deletedBy string) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		detail, err := r.scanDetail(tx.QueryRowContext(ctx,
			detailSelectSQL+` WHERE group_detail_id=$1 AND deleted_at IS NULL`, id))
		if err != nil {
			return err
		}
		res, err := tx.ExecContext(ctx,
			`UPDATE cst_rm_group_detail SET deleted_at=$2, deleted_by=$3, is_active=false
			 WHERE group_detail_id=$1 AND deleted_at IS NULL`,
			id, time.Now(), deletedBy)
		if err != nil {
			return fmt.Errorf("soft-delete detail: %w", err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		if n == 0 {
			return rmgroup.ErrDetailNotFound
		}
		return insertDetailAuditDelete(ctx, tx, detail, deletedBy)
	})
}

// =============================================================================
// Detail scanning helpers
// =============================================================================

const detailSelectSQL = `
	SELECT group_detail_id, group_head_id, item_code, item_name, item_type_code,
	       grade_code, item_grade, uom_code,
	       market_percentage, market_value_rp,
	       sort_order, is_active, is_dummy, created_at, created_by,
	       updated_at, updated_by, deleted_at, deleted_by
	FROM cst_rm_group_detail`

type detailDTO struct {
	ID               uuid.UUID
	HeadID           uuid.UUID
	ItemCode         string
	ItemName         sql.NullString
	ItemTypeCode     sql.NullString
	GradeCode        sql.NullString
	ItemGrade        sql.NullString
	UOMCode          sql.NullString
	MarketPercentage sql.NullFloat64
	MarketValueRp    sql.NullFloat64
	SortOrder        int32
	IsActive         bool
	IsDummy          bool
	CreatedAt        time.Time
	CreatedBy        string
	UpdatedAt        sql.NullTime
	UpdatedBy        sql.NullString
	DeletedAt        sql.NullTime
	DeletedBy        sql.NullString
}

func (d *detailDTO) toEntity() (*rmgroup.Detail, error) {
	itemCode, err := rmgroup.NewItemCode(d.ItemCode)
	if err != nil {
		return nil, fmt.Errorf("invalid item_code from db: %w", err)
	}
	return rmgroup.ReconstructDetail(
		d.ID, d.HeadID, itemCode,
		nullStringVal(d.ItemName), nullStringVal(d.ItemTypeCode),
		nullStringVal(d.GradeCode), nullStringVal(d.ItemGrade), nullStringVal(d.UOMCode),
		nullFloatPtr(d.MarketPercentage), nullFloatPtr(d.MarketValueRp),
		d.SortOrder, d.IsActive, d.IsDummy,
		d.CreatedAt, d.CreatedBy,
		nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
		nullTimePtr(d.DeletedAt), nullStringPtr(d.DeletedBy),
	), nil
}

func (r *RMGroupRepository) scanDetail(row *sql.Row) (*rmgroup.Detail, error) {
	var d detailDTO
	err := row.Scan(
		&d.ID, &d.HeadID, &d.ItemCode, &d.ItemName, &d.ItemTypeCode,
		&d.GradeCode, &d.ItemGrade, &d.UOMCode,
		&d.MarketPercentage, &d.MarketValueRp,
		&d.SortOrder, &d.IsActive, &d.IsDummy, &d.CreatedAt, &d.CreatedBy,
		&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rmgroup.ErrDetailNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan rm group detail: %w", err)
	}
	return d.toEntity()
}

func (r *RMGroupRepository) listDetails(ctx context.Context, query string, args ...any) ([]*rmgroup.Detail, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list rm group details: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*rmgroup.Detail
	for rows.Next() {
		var d detailDTO
		if err := rows.Scan(
			&d.ID, &d.HeadID, &d.ItemCode, &d.ItemName, &d.ItemTypeCode,
			&d.GradeCode, &d.ItemGrade, &d.UOMCode,
			&d.MarketPercentage, &d.MarketValueRp,
			&d.SortOrder, &d.IsActive, &d.IsDummy, &d.CreatedAt, &d.CreatedBy,
			&d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		); err != nil {
			return nil, fmt.Errorf("scan detail row: %w", err)
		}
		entity, err := d.toEntity()
		if err != nil {
			return nil, err
		}
		out = append(out, entity)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate details: %w", err)
	}
	return out, nil
}

// nullTimePtr converts a sql.NullTime to *time.Time. Defined here to avoid
// clashing with the same-named helper in job_repository.go by reusing that one.
// (No redeclaration — this comment documents intent; actual helper lives in job_repository.go.)
