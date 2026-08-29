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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// MBSpinRepository implements mbspin.Repository using PostgreSQL.
type MBSpinRepository struct {
	db *DB
}

// NewMBSpinRepository creates a new MBSpinRepository instance.
func NewMBSpinRepository(db *DB) *MBSpinRepository {
	return &MBSpinRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbspin.Repository = (*MBSpinRepository)(nil)

// Create persists a new MB Spin.
func (r *MBSpinRepository) Create(ctx context.Context, entity *mbspin.Entity) error {
	return createSpinOn(ctx, r.db, entity)
}

// createSpinOn inserts an MB Spin through anything satisfying rowQuerier (already defined in
// mb_composition_repository.go), i.e. either *DB standalone or a *sql.Tx. This lets
// mb_autogen_repository.go's mbAutoGenSpin insert the auto-generated MB Spin row as part of the
// same Lock/Validate transaction, instead of Create opening its own implicit commit boundary.
func createSpinOn(ctx context.Context, q rowQuerier, entity *mbspin.Entity) error {
	_, err := q.ExecContext(ctx, `
		INSERT INTO mst_mb_spin (
			mbs_id, mbs_oracle_sys_id, mbs_orion_item_code, mbs_mbh_id, mbs_mgt_name,
			mbs_denier, mbs_filament, mbs_dozing, mbs_mb_costing,
			mbs_cc, mbs_cost_rate_mkt,
			mbs_status, mbs_ldr_prsn, mbs_run_ldr_pct, mbs_final_product,
			mbs_ldr_is_fixed, mbs_dozing_is_fixed,
			mbs_shade_code, mbs_shade_name, mbs_cross_section,
			mbs_ldr_type, mbs_ldr_calculated_pct, mbs_ldr_adjustment_pct, mbs_ldr_is_actual,
			mbs_vs_number,
			mbs_is_active, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28)
	`,
		entity.ID(),
		entity.OracleSysID(),
		entity.OrionItemCode(),
		entity.HeadID(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.MBCosting(),
		entity.CC(),
		entity.CostRateMkt(),
		entity.MBSStatus(),
		entity.MBSLdrPrsn(),
		entity.MBSRunLdrPct(),
		entity.MBSFinalProduct(),
		entity.LDRIsFixed(),
		entity.DozingIsFixed(),
		entity.ShadeCode(),
		entity.ShadeName(),
		entity.CrossSection(),
		entity.LDRType(),
		entity.LDRCalculatedPct(),
		entity.LDRAdjustmentPct(),
		entity.LDRIsActual(),
		entity.VSNumber(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
	)
	if err != nil {
		if isMBSpinUniqueViolation(err) {
			return mbspin.ErrAlreadyExists
		}
		return fmt.Errorf("create mb spin: %w", err)
	}
	return nil
}

// GetByID retrieves an MB Spin by its UUID primary key.
func (r *MBSpinRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbspin.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbs_id = $1 AND deleted_at IS NULL`, id))
}

// List retrieves MB Spins with filtering and pagination.
func (r *MBSpinRepository) List(ctx context.Context, filter mbspin.ListFilter) ([]*mbspin.Entity, int64, error) {
	filter.Validate()

	base := whereNotDeleted
	args := make([]interface{}, 0)
	idx := 1

	if filter.HeadID != uuid.Nil {
		base += fmt.Sprintf(` AND mbs_mbh_id = $%d`, idx)
		args = append(args, filter.HeadID)
		idx++
	}
	if filter.Search != "" {
		base += fmt.Sprintf(` AND (mbs_mgt_name ILIKE $%d OR mbs_mb_costing ILIKE $%d)`, idx, idx)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND mbs_is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mst_mb_spin "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mb spins: %w", err)
	}

	orderCol := r.resolveSort(filter.SortBy)
	dir := sortASC
	if strings.ToUpper(filter.SortOrder) == sortDESC {
		dir = sortDESC
	}

	q := r.selectCols() + base + fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`, orderCol, dir, idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list mb spins: %w", err)
	}
	defer closeRows(rows)

	var items []*mbspin.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate mb spins: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing MB Spin.
func (r *MBSpinRepository) Update(ctx context.Context, entity *mbspin.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_mb_spin SET
			mbs_mgt_name      = $2,
			mbs_denier        = $3,
			mbs_filament      = $4,
			mbs_dozing        = $5,
			mbs_mb_costing    = $6,
			mbs_cc            = $7,
			mbs_cost_rate_mkt = $8,
			mbs_status        = $9,
			mbs_ldr_prsn      = $10,
			mbs_run_ldr_pct   = $11,
			mbs_final_product = $12,
			mbs_ldr_is_fixed    = $13,
			mbs_dozing_is_fixed = $14,
			mbs_shade_code           = $15,
			mbs_shade_name           = $16,
			mbs_cross_section        = $17,
			mbs_ldr_type             = $18,
			mbs_ldr_calculated_pct   = $19,
			mbs_ldr_adjustment_pct   = $20,
			mbs_ldr_is_actual        = $21,
			mbs_vs_number     = $22,
			mbs_is_active     = $23,
			updated_at        = $24,
			updated_by        = $25
		WHERE mbs_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.MBCosting(),
		entity.CC(),
		entity.CostRateMkt(),
		entity.MBSStatus(),
		entity.MBSLdrPrsn(),
		entity.MBSRunLdrPct(),
		entity.MBSFinalProduct(),
		entity.LDRIsFixed(),
		entity.DozingIsFixed(),
		entity.ShadeCode(),
		entity.ShadeName(),
		entity.CrossSection(),
		entity.LDRType(),
		entity.LDRCalculatedPct(),
		entity.LDRAdjustmentPct(),
		entity.LDRIsActual(),
		entity.VSNumber(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("update mb spin: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbspin.ErrNotFound
	}
	return nil
}

// SoftDelete marks an MB Spin as deleted.
func (r *MBSpinRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_mb_spin SET deleted_at=$2,deleted_by=$3,mbs_is_active=false WHERE mbs_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete mb spin: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbspin.ErrNotFound
	}
	return nil
}

// ExistsByID checks if an MB Spin with the given UUID exists.
func (r *MBSpinRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_spin WHERE mbs_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// GetByMBCosting retrieves an MB Spin by its MB costing code.
func (r *MBSpinRepository) GetByMBCosting(ctx context.Context, code string) (*mbspin.Entity, error) {
	// D20/M2a: deterministic tie-breaker on duplicate keys; no mbs_is_active filter on purpose
	// (filtering would shift the winner and empty out keys that have zero active spins).
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbs_mb_costing = $1 AND deleted_at IS NULL ORDER BY created_at ASC, mbs_id ASC LIMIT 1`, code))
}

// GetByOrionItemCode retrieves the first active MB Spin with the given ORION item code.
func (r *MBSpinRepository) GetByOrionItemCode(ctx context.Context, code string) (*mbspin.Entity, error) {
	// D20/M2a: deterministic tie-breaker on duplicate keys; no mbs_is_active filter on purpose
	// (filtering would shift the winner and empty out keys that have zero active spins).
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbs_orion_item_code = $1 AND deleted_at IS NULL ORDER BY created_at ASC, mbs_id ASC LIMIT 1`, code))
}

// ResolveUniqueByOrionItemCode reports whether exactly one non-deleted spin
// carries the given ORION item code, WITHOUT picking a winner on ambiguity.
//
// ⚠ Deliberately no LIMIT/ORDER BY tie-breaker here (unlike GetByOrionItemCode's
// D20/M2a pattern): this is the save-time resolver for cpp_value_mb_spin_id, and
// picking an arbitrary row among the 177 duplicate ORION codes (up to 16 rows
// each, 466 rows total) would silently mis-resolve. Selecting up to 2 rows is
// enough to distinguish "exactly one" from "more than one" without scanning the
// whole duplicate set.
func (r *MBSpinRepository) ResolveUniqueByOrionItemCode(ctx context.Context, code string) (uuid.UUID, bool, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT mbs_id FROM mst_mb_spin WHERE mbs_orion_item_code = $1 AND deleted_at IS NULL LIMIT 2`,
		code,
	)
	if err != nil {
		return uuid.UUID{}, false, fmt.Errorf("resolve unique by orion item code: %w", err)
	}
	defer closeRows(rows)

	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if scanErr := rows.Scan(&id); scanErr != nil {
			return uuid.UUID{}, false, fmt.Errorf("scan orion item code match: %w", scanErr)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return uuid.UUID{}, false, fmt.Errorf("iterate orion item code matches: %w", err)
	}
	if len(ids) != 1 {
		return uuid.UUID{}, false, nil
	}
	return ids[0], true, nil
}

// ListByOrionItemCode returns every non-deleted spin sharing the given ORION
// item code, ordered deterministically. See mbspin.Repository.ListByOrionItemCode
// for the matching-rule consistency requirement this must honor.
func (r *MBSpinRepository) ListByOrionItemCode(ctx context.Context, code string) ([]*mbspin.Entity, error) {
	rows, err := r.db.QueryContext(ctx,
		r.selectCols()+` WHERE mbs_orion_item_code = $1 AND deleted_at IS NULL ORDER BY created_at ASC, mbs_id ASC`,
		code,
	)
	if err != nil {
		return nil, fmt.Errorf("list by orion item code: %w", err)
	}
	defer closeRows(rows)

	var items []*mbspin.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate orion item code matches: %w", err)
	}
	return items, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *MBSpinRepository) selectCols() string {
	return `
		SELECT mbs_id, mbs_oracle_sys_id, mbs_orion_item_code, mbs_mbh_id, mbs_mgt_name,
		       mbs_denier, mbs_filament, mbs_dozing, mbs_mb_costing,
		       mbs_cc, mbs_cost_rate_mkt,
		       mbs_status, mbs_ldr_prsn, mbs_run_ldr_pct, mbs_final_product,
		       mbs_ldr_is_fixed, mbs_dozing_is_fixed,
		       mbs_is_active,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by,
		       mbs_parent_spin_id, mbs_duplicated_at, mbs_duplicated_by,
		       mbs_last_recalc_at, mbs_last_recalc_by, mbs_cost_product_id,
		       mbs_shade_code, mbs_shade_name, mbs_cross_section,
		       mbs_ldr_type, mbs_ldr_calculated_pct, mbs_ldr_adjustment_pct, mbs_ldr_is_actual,
		       mbs_vs_number
		FROM mst_mb_spin
	`
}

func (r *MBSpinRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"mbs_mgt_name": "mbs_mgt_name", "mbs_denier": "mbs_denier",
		sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbs_mgt_name"
}

type mbSpinDTO struct {
	ID              uuid.UUID
	OracleSysID     sql.NullString
	OrionItemCode   sql.NullString
	HeadID          uuid.UUID
	MgtName         string
	Denier          sql.NullFloat64
	Filament        sql.NullInt64
	Dozing          sql.NullFloat64
	MBCosting       sql.NullString
	CC              sql.NullString
	CostRateMkt     sql.NullFloat64
	MBSStatus       sql.NullString
	MBSLdrPrsn      sql.NullFloat64
	MBSRunLdrPct    sql.NullFloat64
	MBSFinalProduct sql.NullString
	LDRIsFixed      sql.NullBool
	DozingIsFixed   sql.NullBool
	IsActive        bool
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString
	DeletedAt       sql.NullTime
	DeletedBy       sql.NullString
	// Lineage / recalc trail (migration 000484) + ownership (000490). All six are
	// NULL on every legacy Oracle row and stay that way — there is no backfill for
	// the 000484 five.
	ParentSpinID  sql.NullString
	DuplicatedAt  sql.NullTime
	DuplicatedBy  sql.NullString
	LastRecalcAt  sql.NullTime
	LastRecalcBy  sql.NullString
	CostProductID sql.NullInt64
	// Shade/cross-section copy-down + LDR provenance tracking (migration 000496).
	// LDRType and LDRIsActual are NOT NULL in storage, so they scan directly
	// without a sql.Null wrapper.
	ShadeCode        sql.NullString
	ShadeName        sql.NullString
	CrossSection     sql.NullString
	LDRType          string
	LDRCalculatedPct sql.NullFloat64
	LDRAdjustmentPct sql.NullFloat64
	LDRIsActual      bool
	// VSNumber is the VS reference number copied down from the parent MB Head
	// at MB Spin auto-generation time (migration 000414).
	VSNumber sql.NullString
}

func (d *mbSpinDTO) toEntity() *mbspin.Entity {
	e := d.reconstruct()
	e.HydrateLineage(mbspin.Lineage{
		ParentSpinID:  nullableUUIDPtr(d.ParentSpinID),
		DuplicatedAt:  nullableTimePtr(d.DuplicatedAt),
		DuplicatedBy:  nullableStringPtr(d.DuplicatedBy),
		LastRecalcAt:  nullableTimePtr(d.LastRecalcAt),
		LastRecalcBy:  nullableStringPtr(d.LastRecalcBy),
		CostProductID: nullableInt64Ptr(d.CostProductID),
	})
	e.HydrateShadeAndLDR(mbspin.ShadeAndLDR{
		ShadeCode:        nullableStringPtr(d.ShadeCode),
		ShadeName:        nullableStringPtr(d.ShadeName),
		CrossSection:     nullableStringPtr(d.CrossSection),
		LDRType:          d.LDRType,
		LDRCalculatedPct: nullableFloat64Ptr(d.LDRCalculatedPct),
		LDRAdjustmentPct: nullableFloat64Ptr(d.LDRAdjustmentPct),
		LDRIsActual:      d.LDRIsActual,
	})
	e.HydrateVSNumber(nullableStringPtr(d.VSNumber))
	return e
}

// reconstruct maps only the columns Reconstruct takes positionally; the 000484 /
// 000490 columns are applied separately by toEntity via HydrateLineage.
func (d *mbSpinDTO) reconstruct() *mbspin.Entity {
	return mbspin.Reconstruct(
		d.ID,
		nullableStringPtr(d.OracleSysID),
		nullableStringPtr(d.OrionItemCode),
		d.HeadID,
		d.MgtName,
		nullableFloat64Ptr(d.Denier),
		nullableIntPtr(d.Filament),
		nullableFloat64Ptr(d.Dozing),
		nullableStringPtr(d.MBCosting),
		nullableStringPtr(d.CC),
		nullableFloat64Ptr(d.CostRateMkt),
		nullableStringPtr(d.MBSStatus),
		nullableFloat64Ptr(d.MBSLdrPrsn),
		nullableFloat64Ptr(d.MBSRunLdrPct),
		nullableStringPtr(d.MBSFinalProduct),
		nullableBoolPtr(d.LDRIsFixed),
		nullableBoolPtr(d.DozingIsFixed),
		d.IsActive,
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
	)
}

func (r *MBSpinRepository) scanOne(row *sql.Row) (*mbspin.Entity, error) {
	var d mbSpinDTO
	err := row.Scan(
		&d.ID, &d.OracleSysID, &d.OrionItemCode, &d.HeadID, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing, &d.MBCosting,
		&d.CC, &d.CostRateMkt,
		&d.MBSStatus, &d.MBSLdrPrsn, &d.MBSRunLdrPct, &d.MBSFinalProduct,
		&d.LDRIsFixed, &d.DozingIsFixed,
		&d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		&d.ParentSpinID, &d.DuplicatedAt, &d.DuplicatedBy,
		&d.LastRecalcAt, &d.LastRecalcBy, &d.CostProductID,
		&d.ShadeCode, &d.ShadeName, &d.CrossSection,
		&d.LDRType, &d.LDRCalculatedPct, &d.LDRAdjustmentPct, &d.LDRIsActual,
		&d.VSNumber,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbspin.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan mb spin: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MBSpinRepository) scanRow(rows *sql.Rows) (*mbspin.Entity, error) {
	var d mbSpinDTO
	err := rows.Scan(
		&d.ID, &d.OracleSysID, &d.OrionItemCode, &d.HeadID, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing, &d.MBCosting,
		&d.CC, &d.CostRateMkt,
		&d.MBSStatus, &d.MBSLdrPrsn, &d.MBSRunLdrPct, &d.MBSFinalProduct,
		&d.LDRIsFixed, &d.DozingIsFixed,
		&d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		&d.ParentSpinID, &d.DuplicatedAt, &d.DuplicatedBy,
		&d.LastRecalcAt, &d.LastRecalcBy, &d.CostProductID,
		&d.ShadeCode, &d.ShadeName, &d.CrossSection,
		&d.LDRType, &d.LDRCalculatedPct, &d.LDRAdjustmentPct, &d.LDRIsActual,
		&d.VSNumber,
	)
	if err != nil {
		return nil, fmt.Errorf("scan mb spin row: %w", err)
	}
	return d.toEntity(), nil
}

func isMBSpinUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
