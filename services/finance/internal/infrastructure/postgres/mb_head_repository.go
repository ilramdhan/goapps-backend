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
			mbh_cross_section, mbh_lusture_code, mbh_machine_id, mbh_run_ldr_pct
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23)
	`,
		entity.ID(),
		entity.OracleSysID(),
		entity.MBCosting(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
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

// List retrieves MB Heads with filtering and pagination.
func (r *MBHeadRepository) List(ctx context.Context, filter mbhead.ListFilter) ([]*mbhead.Entity, int64, error) {
	filter.Validate()

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
			mbh_run_ldr_pct   = $21
		WHERE mbh_id = $1 AND deleted_at IS NULL
	`,
		entity.ID(),
		entity.MBCosting(),
		entity.MgtName(),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
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

// ListAll retrieves all non-deleted MB Heads matching filter, unpaginated (for export).
func (r *MBHeadRepository) ListAll(ctx context.Context, filter mbhead.ExportFilter) ([]*mbhead.Entity, error) {
	query := r.selectCols() + whereNotDeleted
	args := make([]interface{}, 0)
	if filter.IsActive != nil {
		query += fmt.Sprintf(` AND mbh_is_active = $%d`, len(args)+1)
		args = append(args, *filter.IsActive)
	}
	query += ` ORDER BY mbh_mb_costing ASC`

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

// UpdateEntryStatus persists a state-machine transition (entry_status + optional
// current_version bump + optional state_reason), used by Submit/Approve/Validate/
// UnApprove/Revoke application handlers after the domain entity mutates in memory.
func (r *MBHeadRepository) UpdateEntryStatus(ctx context.Context, id uuid.UUID, entryStatus string, currentVersion int32, stateReason string) error {
	result, err := r.db.ExecContext(ctx, `
		UPDATE mst_mb_head
		SET mbh_entry_status = $2, mbh_current_version = $3, mbh_state_reason = $4, updated_at = NOW()
		WHERE mbh_id = $1 AND deleted_at IS NULL
	`, id, entryStatus, currentVersion, stateReason)
	if err != nil {
		return fmt.Errorf("update mb head entry status: %w", err)
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
		       mbh_param_no_of_process, mbh_machine_id
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
}

func nullTimeToStringPtr(n sql.NullTime) *string {
	if !n.Valid {
		return nil
	}
	v := n.Time.Format(time.RFC3339)
	return &v
}

func (d *mbHeadDTO) toEntity() *mbhead.Entity {
	return mbhead.Reconstruct(
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
