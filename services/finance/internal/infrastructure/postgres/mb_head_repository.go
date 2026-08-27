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

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// MBHeadRepository implements mbhead.Repository using PostgreSQL.
type MBHeadRepository struct {
	db              *DB
	compositionRepo *MBCompositionRepository
}

// NewMBHeadRepository creates a new MBHeadRepository instance. compositionRepo is used by
// Transition to snapshot the composition version atomically on a VALIDATE transition.
func NewMBHeadRepository(db *DB, compositionRepo *MBCompositionRepository) *MBHeadRepository {
	return &MBHeadRepository{db: db, compositionRepo: compositionRepo}
}

// Verify interface implementation at compile time.
var _ mbhead.Repository = (*MBHeadRepository)(nil)

// Create persists a new MB Head.
func (r *MBHeadRepository) Create(ctx context.Context, entity *mbhead.Entity) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO mst_mb_head (
			mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_mgt_name,
			mbh_denier, mbh_filament, mbh_dozing,
			mbh_check_status, mbh_status, mbh_ldr_prsn, mbh_final_product, mbh_code,
			mbh_is_active, created_at, created_by,
			mbh_is_boughtout, mbh_dev_code, mbh_shade_code, mbh_shade_name,
			mbh_cross_section, mbh_lusture_code, mbh_machine_id, mbh_run_ldr_pct,
			mbh_vs_number, mbh_no_of_process,
			mbh_check_status_calc
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26)
	`,
		entity.ID(),
		entity.OracleSysID(),
		entity.MBCosting(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		// §11 item 106: structurally always NULL on insert — NewParams no longer carries
		// a check-status field, so a newly created head has no Oracle import trace. The
		// column stays in the INSERT only to keep the column/placeholder lists aligned.
		entity.MBHCheckStatus(),
		entity.MBHStatus(),
		entity.MBHLdrPrsn(),
		entity.MBHFinalProduct(),
		entity.MBHCode(),
		entity.IsActive(),
		entity.CreatedAt(),
		entity.CreatedBy(),
		entity.IsBoughtout(),
		entity.DevCode(),
		entity.ShadeCode(),
		entity.ShadeName(),
		entity.CrossSection(),
		entity.LustureCode(),
		entity.MachineID(),
		entity.MBHRunLdrPct(),
		entity.VSNumber(),
		entity.NoOfProcess(),
		// Derived column (000487). mbh_check_status above stays a pure passthrough of
		// whatever the caller supplied — ⛔ this value is NEVER written there (K-1).
		entity.MBHCheckStatusCalc(),
	)
	if err != nil {
		if isMBHeadUniqueViolation(err) {
			return mbhead.ErrAlreadyExists
		}
		return fmt.Errorf("create mb head: %w", err)
	}
	return nil
}

// GetByID retrieves an MB Head by its UUID primary key.
func (r *MBHeadRepository) GetByID(ctx context.Context, id uuid.UUID) (*mbhead.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbh_id = $1 AND deleted_at IS NULL`, id))
}

// GetByMBCosting retrieves an MB Head by its unique mb_costing value.
func (r *MBHeadRepository) GetByMBCosting(ctx context.Context, mbCosting string) (*mbhead.Entity, error) {
	return r.scanOne(r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbh_mb_costing = $1 AND deleted_at IS NULL`, mbCosting))
}

// buildMBHeadListWhere renders the WHERE clause and its positional args for List, kept as
// a standalone function so a unit test can pin its shape without a live database (same
// structural-test approach used for the sweep query in mb_head_relock_internal_test.go —
// this package has no SQL mock and no test database).
//
// ⭐ DIPERBARUI 2026-08-26 (R16) — added the optional CostProductID predicate, which lets
// the product detail page look up its MB Head(s) from the product side. An unset
// CostProductID (nil) leaves the clause and arg list byte-for-byte identical to before —
// this filter must never change List's existing behavior when omitted.
func buildMBHeadListWhere(filter mbhead.ListFilter) (string, []interface{}) {
	base := whereNotDeleted
	args := make([]interface{}, 0)
	idx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(
			` AND (mbh_mb_costing ILIKE $%d OR mbh_mgt_name ILIKE $%d OR mbh_dev_code ILIKE $%d OR mbh_shade_code ILIKE $%d OR mbh_shade_name ILIKE $%d)`,
			idx, idx, idx, idx, idx,
		)
		args = append(args, "%"+filter.Search+"%")
		idx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND mbh_is_active = $%d`, idx)
		args = append(args, *filter.IsActive)
		idx++
	}
	if filter.CostProductID != nil {
		base += fmt.Sprintf(` AND mbh_cost_product_id = $%d`, idx)
		args = append(args, *filter.CostProductID)
	}

	return base, args
}

// List retrieves MB Heads with filtering and pagination.
func (r *MBHeadRepository) List(ctx context.Context, filter mbhead.ListFilter) ([]*mbhead.Entity, int64, error) {
	filter.Validate()

	base, args := buildMBHeadListWhere(filter)
	idx := len(args) + 1

	var total int64
	if err := r.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM mst_mb_head "+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count mb heads: %w", err)
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
		return nil, 0, fmt.Errorf("list mb heads: %w", err)
	}
	defer closeRows(rows)

	var items []*mbhead.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate mb heads: %w", err)
	}
	return items, total, nil
}

// Update persists changes to an existing MB Head.
func (r *MBHeadRepository) Update(ctx context.Context, entity *mbhead.Entity) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_mb_head SET
			mbh_mb_costing    = $2,
			mbh_mgt_name      = $3,
			mbh_denier        = $4,
			mbh_filament      = $5,
			mbh_dozing        = $6,
			mbh_check_status  = $7,
			mbh_status        = $8,
			mbh_ldr_prsn      = $9,
			mbh_final_product = $10,
			mbh_code          = $11,
			mbh_is_active     = $12,
			updated_at        = $13,
			updated_by        = $14,
			mbh_dev_code      = $15,
			mbh_shade_code    = $16,
			mbh_shade_name    = $17,
			mbh_cross_section = $18,
			mbh_lusture_code  = $19,
			mbh_machine_id    = $20,
			mbh_run_ldr_pct   = $21,
			mbh_vs_number     = $22,
			mbh_no_of_process = $23
		WHERE mbh_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.MBCosting(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		// §11 item 106: this is a WRITE-BACK OF THE UNCHANGED STORED VALUE, never a
		// client-supplied one. The entity came from GetByID and nothing in the domain
		// can mutate mbhCheckStatus any more (UpdateInput has no such field), so this
		// rewrites the Oracle trace byte-for-byte. ⛔ Do NOT drop the column from the
		// UPDATE and do NOT null it out — either would ERASE the trace. Keeping it
		// here is what keeps it INTACT.
		entity.MBHCheckStatus(),
		entity.MBHStatus(),
		entity.MBHLdrPrsn(),
		entity.MBHFinalProduct(),
		entity.MBHCode(),
		entity.IsActive(),
		entity.UpdatedAt(),
		entity.UpdatedBy(),
		entity.DevCode(),
		entity.ShadeCode(),
		entity.ShadeName(),
		entity.CrossSection(),
		entity.LustureCode(),
		entity.MachineID(),
		entity.MBHRunLdrPct(),
		entity.VSNumber(),
		entity.NoOfProcess(),
	)
	if err != nil {
		return fmt.Errorf("update mb head: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}

// SoftDelete marks an MB Head as deleted.
func (r *MBHeadRepository) SoftDelete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	result, err := r.db.ExecContext(ctx,
		`UPDATE mst_mb_head SET deleted_at=$2,deleted_by=$3,mbh_is_active=false WHERE mbh_id=$1 AND deleted_at IS NULL`,
		id, time.Now(), deletedBy,
	)
	if err != nil {
		return fmt.Errorf("soft delete mb head: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbhead.ErrNotFound
	}
	return nil
}

// ExistsByMBCosting checks if an MB Head with the given mb_costing exists.
func (r *MBHeadRepository) ExistsByMBCosting(ctx context.Context, mbCosting string) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_head WHERE mbh_mb_costing=$1 AND deleted_at IS NULL)`, mbCosting,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by mb_costing: %w", err)
	}
	return exists, nil
}

// ExistsByID checks if an MB Head with the given UUID exists.
func (r *MBHeadRepository) ExistsByID(ctx context.Context, id uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM mst_mb_head WHERE mbh_id=$1 AND deleted_at IS NULL)`, id,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("exists by id: %w", err)
	}
	return exists, nil
}

// MBHeadCandidate is the minimal MB Head projection needed by MB Push-to-Head preview/execute.
// Kept postgres-native (not the mbpush application package's type) so this package never imports
// internal/application/mbpush — callers in mbpush adapt this into their own port type.
type MBHeadCandidate struct {
	MBHID          string
	Code           string
	Name           string
	CostProductID  int64
	IsBoughtout    bool
	CurrentVersion int32
}

// ListValidated returns all VALIDATED MB Heads, the candidate set for a push-to-head
// preview/execute pass (PR-01/PR-02), and — via CurrentVersion — for the mbbatch DAG
// builder's per-mbh_id version resolution (Task 21b) without a separate GetByID call.
func (r *MBHeadRepository) ListValidated(ctx context.Context) ([]MBHeadCandidate, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT mbh_id, mbh_mb_costing, COALESCE(mbh_mgt_name, ''),
		       COALESCE(mbh_cost_product_id, 0), mbh_is_boughtout, mbh_current_version
		FROM mst_mb_head
		WHERE mbh_entry_status = 'VALIDATED' AND deleted_at IS NULL
	`)
	if err != nil {
		return nil, fmt.Errorf("list validated mb heads: %w", err)
	}
	defer closeRows(rows)

	var out []MBHeadCandidate
	for rows.Next() {
		var c MBHeadCandidate
		if err := rows.Scan(&c.MBHID, &c.Code, &c.Name, &c.CostProductID, &c.IsBoughtout, &c.CurrentVersion); err != nil {
			return nil, fmt.Errorf("scan validated mb head: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate validated mb heads: %w", err)
	}
	return out, nil
}

// buildMBHeadExportWhere renders the WHERE clause and its positional args for ListAll,
// kept as a standalone function so a unit test can pin its shape without a live database
// (same structural-test approach as buildMBHeadListWhere and the sweep query in
// mb_head_relock_internal_test.go — this package has no SQL mock and no test database).
//
// ⭐ DIPERBARUI (§11 item 140) — added the IncludeRejected predicate. Previously this
// clause filtered only on deleted_at and the optional mbh_is_active, so a head whose
// workflow status is StatusRejected leaked into every export: mbh_is_active is an
// INDEPENDENT flag (Reject() never turns it off), so a rejected head stayed exported.
// Default (IncludeRejected=false) now excludes StatusRejected via a parameterized value
// — never a string-interpolated literal.
func buildMBHeadExportWhere(filter mbhead.ExportFilter) (string, []interface{}) {
	base := whereNotDeleted
	args := make([]interface{}, 0)

	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND mbh_is_active = $%d`, len(args)+1)
		args = append(args, *filter.IsActive)
	}
	if !filter.IncludeRejected {
		base += fmt.Sprintf(` AND mbh_entry_status != $%d`, len(args)+1)
		args = append(args, mbhead.StatusRejected)
	}

	return base, args
}

// ListAll retrieves all non-deleted MB Heads matching filter, unpaginated (for export).
func (r *MBHeadRepository) ListAll(ctx context.Context, filter mbhead.ExportFilter) ([]*mbhead.Entity, error) {
	base, args := buildMBHeadExportWhere(filter)
	query := r.selectCols() + base + ` ORDER BY mbh_mb_costing ASC`

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all mb heads: %w", err)
	}
	defer closeRows(rows)

	var items []*mbhead.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all mb heads: %w", err)
	}
	return items, nil
}

// =============================================================================
// Helpers
// =============================================================================

func (r *MBHeadRepository) selectCols() string {
	return `
		SELECT mbh_id, mbh_oracle_sys_id, mbh_mb_costing, mbh_mgt_name,
		       mbh_denier, mbh_filament, mbh_dozing,
		       mbh_check_status, mbh_status, mbh_ldr_prsn, mbh_run_ldr_pct, mbh_final_product, mbh_code,
		       mbh_is_active,
		       created_at, created_by, updated_at, updated_by, deleted_at, deleted_by,
		       mbh_entry_status, mbh_is_boughtout, mbh_current_version, mbh_machine_fixed_total,
		       mbh_state_reason, mbh_dev_code, mbh_shade_code, mbh_shade_name, mbh_cross_section,
		       mbh_lusture_code, mbh_cost_product_id, mbh_cost_generated_at, mbh_cost_generated_by,
		       mbh_param_waste, mbh_param_quality_loss, mbh_param_efficiency, mbh_param_dev_expense,
		       mbh_param_packing, mbh_param_mb_prod_per_day, mbh_param_throughput_per_hour,
		       mbh_param_no_of_process, mbh_machine_id,
		       mbh_vs_number, mbh_no_of_process,
		       COALESCE(mbh_is_locked, FALSE), mbh_unlock_requested_at, mbh_unlock_requested_by,
		       mbh_locked_at, mbh_locked_by, mbh_unlock_reason,
		       -- P10: which locked state an UNLOCK_REQUESTED head was parked FROM.
		       -- There is deliberately ⛔ NO COLUMN for this: the workflow log already
		       -- holds the answer, and a second copy would drift from it. Only the
		       -- MOST RECENT park is relevant, hence ORDER BY ... DESC LIMIT 1.
		       -- NULL for every head that is not parked — which is nearly all of them.
		       (SELECT w.mbwl_from_state
		          FROM mst_mb_workflow_log w
		         WHERE w.mbwl_mbh_id = mst_mb_head.mbh_id
		           AND w.mbwl_to_state = 'UNLOCK_REQUESTED'
		         ORDER BY w.mbwl_actor_at DESC
		         LIMIT 1),
		       mbh_check_status_calc
		FROM mst_mb_head
	`
}

func (r *MBHeadRepository) resolveSort(sortBy string) string {
	m := map[string]string{
		"mbh_mb_costing": "mbh_mb_costing", "mbh_mgt_name": "mbh_mgt_name",
		"mbh_denier": "mbh_denier", sortKeyCreatedAt: sortKeyCreatedAt,
	}
	if col, ok := m[sortBy]; ok {
		return col
	}
	return "mbh_mb_costing"
}

type mbHeadDTO struct {
	ID              uuid.UUID
	OracleSysID     sql.NullString
	MBCosting       string
	MgtName         sql.NullString
	Denier          sql.NullFloat64
	Filament        sql.NullInt64
	Dozing          sql.NullFloat64
	MBHCheckStatus  sql.NullString
	MBHStatus       sql.NullString
	MBHLdrPrsn      sql.NullFloat64
	MBHRunLdrPct    sql.NullFloat64
	MBHFinalProduct sql.NullString
	MBHCode         sql.NullString
	IsActive        bool
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString
	DeletedAt       sql.NullTime
	DeletedBy       sql.NullString

	EntryStatus            string
	IsBoughtout            bool
	CurrentVersion         int32
	MachineFixedTotal      sql.NullString
	StateReason            sql.NullString
	DevCode                sql.NullString
	ShadeCode              sql.NullString
	ShadeName              sql.NullString
	CrossSection           sql.NullString
	LustureCode            sql.NullString
	CostProductID          sql.NullInt64
	CostGeneratedAt        sql.NullTime
	CostGeneratedBy        sql.NullString
	ParamWaste             sql.NullString
	ParamQualityLoss       sql.NullString
	ParamEfficiency        sql.NullString
	ParamDevExpense        sql.NullString
	ParamPacking           sql.NullString
	ParamMBProdPerDay      sql.NullString
	ParamThroughputPerHour sql.NullString
	ParamNoOfProcess       sql.NullString
	MachineID              sql.NullString

	// P5 recipe columns. ⛔ NoOfProcess (mbh_no_of_process, the live user choice) is a
	// DIFFERENT column from ParamNoOfProcess (mbh_param_no_of_process, the frozen
	// VALIDATE snapshot). Merging them would rewrite historical cost snapshots.
	VSNumber    sql.NullString
	NoOfProcess sql.NullString

	// Lock columns (000485), read-only here — lock BEHAVIOR belongs to P10. IsLocked
	// arrives through COALESCE(mbh_is_locked, FALSE) because the column is NULLable
	// without DEFAULT: every legacy row holds NULL, which means "not locked".
	IsLocked          bool
	UnlockRequestedAt sql.NullTime
	UnlockRequestedBy sql.NullString
	LockedAt          sql.NullTime
	LockedBy          sql.NullString
	UnlockReason      sql.NullString
	// PreUnlockStatus comes from the mst_mb_workflow_log subquery in selectCols, ⛔ not
	// from a column on mst_mb_head. NULL whenever the head has never been parked.
	PreUnlockStatus sql.NullString
	// MBHCheckStatusCalc is the DERIVED column (000487). NULL = never calculated.
	MBHCheckStatusCalc sql.NullString
}

func nullTimeToStringPtr(n sql.NullTime) *string {
	if !n.Valid {
		return nil
	}
	v := n.Time.Format(time.RFC3339)
	return &v
}

func (d *mbHeadDTO) toEntity() *mbhead.Entity {
	e := mbhead.Reconstruct(
		d.ID,
		nullableStringPtr(d.OracleSysID),
		d.MBCosting,
		nullableStringPtr(d.MgtName),
		nullableFloat64Ptr(d.Denier),
		nullableIntPtr(d.Filament),
		nullableFloat64Ptr(d.Dozing),
		nullableStringPtr(d.MBHCheckStatus),
		nullableStringPtr(d.MBHStatus),
		nullableFloat64Ptr(d.MBHLdrPrsn),
		nullableFloat64Ptr(d.MBHRunLdrPct),
		nullableStringPtr(d.MBHFinalProduct),
		nullableStringPtr(d.MBHCode),
		d.IsActive,
		d.CreatedAt, d.CreatedBy,
		nullableTimePtr(d.UpdatedAt), nullableStringPtr(d.UpdatedBy),
		nullableTimePtr(d.DeletedAt), nullableStringPtr(d.DeletedBy),
		d.EntryStatus, d.IsBoughtout, d.CurrentVersion, nullableStringPtr(d.MachineFixedTotal),
		d.StateReason.String, d.DevCode.String, d.ShadeCode.String, d.ShadeName.String,
		d.CrossSection.String, d.LustureCode.String,
		d.CostProductID.Int64, nullTimeToStringPtr(d.CostGeneratedAt), d.CostGeneratedBy.String,
		nullableStringPtr(d.ParamWaste), nullableStringPtr(d.ParamQualityLoss),
		nullableStringPtr(d.ParamEfficiency), nullableStringPtr(d.ParamDevExpense),
		nullableStringPtr(d.ParamPacking), nullableStringPtr(d.ParamMBProdPerDay),
		d.ParamThroughputPerHour.String, d.ParamNoOfProcess.String,
		nullableUUIDPtr(d.MachineID),
	)
	e.HydrateExtras(mbhead.PersistedExtras{
		MBHCheckStatusCalc: nullableStringPtr(d.MBHCheckStatusCalc),
		VSNumber:           nullableStringPtr(d.VSNumber),
		NoOfProcess:        nullableStringPtr(d.NoOfProcess),
		IsLocked:           d.IsLocked,
		UnlockRequestedAt:  nullableTimePtr(d.UnlockRequestedAt),
		UnlockRequestedBy:  nullableStringPtr(d.UnlockRequestedBy),
		LockedAt:           nullableTimePtr(d.LockedAt),
		LockedBy:           nullableStringPtr(d.LockedBy),
		UnlockReason:       nullableStringPtr(d.UnlockReason),
		PreUnlockStatus:    d.PreUnlockStatus.String,
	})
	return e
}

func (r *MBHeadRepository) scanOne(row *sql.Row) (*mbhead.Entity, error) {
	var d mbHeadDTO
	err := row.Scan(
		&d.ID, &d.OracleSysID, &d.MBCosting, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing,
		&d.MBHCheckStatus, &d.MBHStatus, &d.MBHLdrPrsn, &d.MBHRunLdrPct, &d.MBHFinalProduct, &d.MBHCode,
		&d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		&d.EntryStatus, &d.IsBoughtout, &d.CurrentVersion, &d.MachineFixedTotal,
		&d.StateReason, &d.DevCode, &d.ShadeCode, &d.ShadeName, &d.CrossSection,
		&d.LustureCode, &d.CostProductID, &d.CostGeneratedAt, &d.CostGeneratedBy,
		&d.ParamWaste, &d.ParamQualityLoss, &d.ParamEfficiency, &d.ParamDevExpense,
		&d.ParamPacking, &d.ParamMBProdPerDay, &d.ParamThroughputPerHour, &d.ParamNoOfProcess,
		&d.MachineID,
		&d.VSNumber, &d.NoOfProcess,
		&d.IsLocked, &d.UnlockRequestedAt, &d.UnlockRequestedBy,
		&d.LockedAt, &d.LockedBy, &d.UnlockReason, &d.PreUnlockStatus,
		&d.MBHCheckStatusCalc,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, mbhead.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan mb head: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MBHeadRepository) scanRow(rows *sql.Rows) (*mbhead.Entity, error) {
	var d mbHeadDTO
	err := rows.Scan(
		&d.ID, &d.OracleSysID, &d.MBCosting, &d.MgtName,
		&d.Denier, &d.Filament, &d.Dozing,
		&d.MBHCheckStatus, &d.MBHStatus, &d.MBHLdrPrsn, &d.MBHRunLdrPct, &d.MBHFinalProduct, &d.MBHCode,
		&d.IsActive,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
		&d.EntryStatus, &d.IsBoughtout, &d.CurrentVersion, &d.MachineFixedTotal,
		&d.StateReason, &d.DevCode, &d.ShadeCode, &d.ShadeName, &d.CrossSection,
		&d.LustureCode, &d.CostProductID, &d.CostGeneratedAt, &d.CostGeneratedBy,
		&d.ParamWaste, &d.ParamQualityLoss, &d.ParamEfficiency, &d.ParamDevExpense,
		&d.ParamPacking, &d.ParamMBProdPerDay, &d.ParamThroughputPerHour, &d.ParamNoOfProcess,
		&d.MachineID,
		&d.VSNumber, &d.NoOfProcess,
		&d.IsLocked, &d.UnlockRequestedAt, &d.UnlockRequestedBy,
		&d.LockedAt, &d.LockedBy, &d.UnlockReason, &d.PreUnlockStatus,
		&d.MBHCheckStatusCalc,
	)
	if err != nil {
		return nil, fmt.Errorf("scan mb head row: %w", err)
	}
	return d.toEntity(), nil
}

func isMBHeadUniqueViolation(err error) bool {
	var pqErr *pq.Error
	return errors.As(err, &pqErr) && pqErr.Code == "23505"
}
