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
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
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
			mbh_check_status_calc, mbh_orion_item_code,
			mbh_fg_product_id, mbh_fg_item_code
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29)
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
		entity.OrionItemCode(),
		entity.FgProductID(),
		entity.FgItemCode(),
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
			` AND (mbh_mb_costing ILIKE $%d OR mbh_mgt_name ILIKE $%d OR mbh_dev_code ILIKE $%d OR mbh_shade_code ILIKE $%d OR mbh_shade_name ILIKE $%d OR mbh_orion_item_code ILIKE $%d)`,
			idx, idx, idx, idx, idx, idx,
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
		idx++
	}
	if len(filter.EntryStatuses) > 0 {
		base += fmt.Sprintf(` AND mbh_entry_status = ANY($%d)`, idx)
		args = append(args, pq.Array(filter.EntryStatuses))
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
	return r.updateOn(ctx, r.db, entity)
}

// UpdateWithCascade persists the head exactly like Update and, in the SAME transaction,
// cascades the recipe onto every row the Validate auto-gen copied it into (MB Recipe
// cascade design §3.2) — once the head has a cost product (entity.CostProductID() != 0):
//
//  1. mst_mb_head itself (updateOn).
//  2. Every live MB Spin of the head — root AND children — gets the head's identity/spec
//     fields (mbCascadeSpinQuery). Shade code/name/cross-section follow the old
//     syncRootSpinShadeFromHead rule: written together, or not at all when all are empty.
//  3. The root spin's LDR via the existing guarded sync (mbs_ldr_is_actual = FALSE), then
//     the root spin's FG link (syncRootSpinFgTx, design §2.2).
//  4. The linked cost product's name and shade — only while cpm_source = MB_RECIPE.
//  5. When params is non-nil: the frozen mbh_param_* columns and the CPP freeze
//     (mbFreezeCostParams), the same writes RefreezeCostParams performs.
//
// A head WITHOUT a cost product keeps the pre-cascade behavior: only an MB Costing rename
// (oldCosting != entity.MBCosting()) reaches its spins still carrying oldCosting — legacy
// Oracle-imported spins can hang off a head that was never validated here.
//
// Returns the mbs_id of every spin the full cascade touched (nil on the legacy path) so the
// caller can queue lookup syncs after commit. Any failure rolls back the whole update.
//
// ⛔ Product param values that captured the old costing text are NOT rewritten here.
func (r *MBHeadRepository) UpdateWithCascade(ctx context.Context, entity *mbhead.Entity, oldCosting string, params *mbhead.ParamSnapshot) ([]uuid.UUID, error) {
	actor := ""
	if entity.UpdatedBy() != nil {
		actor = *entity.UpdatedBy()
	}
	var spinIDs []uuid.UUID
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.updateOn(ctx, tx, entity); err != nil {
			return err
		}
		productID := entity.CostProductID()
		if productID == 0 {
			return renameSpinCostingTx(ctx, tx, entity, oldCosting, actor)
		}
		ids, err := cascadeHeadToSpinsTx(ctx, tx, entity, actor)
		if err != nil {
			return err
		}
		if err := cascadeRootAndProductTx(ctx, tx, entity, productID, params, actor); err != nil {
			return err
		}
		spinIDs = ids
		return nil
	})
	if err != nil {
		return nil, err
	}
	return spinIDs, nil
}

// cascadeRootAndProductTx runs the root-spin / cost-product steps (3-5 of UpdateWithCascade).
func cascadeRootAndProductTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, productID int64, params *mbhead.ParamSnapshot, actor string) error {
	if err := syncRootSpinLDRFromHead(ctx, tx, entity.ID(), entity, actor); err != nil {
		return err
	}
	if err := syncRootSpinFgTx(ctx, tx, entity, actor); err != nil {
		return err
	}
	if err := cascadeHeadToCostProductTx(ctx, tx, entity, productID, actor); err != nil {
		return err
	}
	if params == nil {
		return nil
	}
	if err := updateFrozenParamsTx(ctx, tx, entity.ID(), params); err != nil {
		return err
	}
	if err := mbFreezeCostParams(ctx, tx, productID, entity, actor); err != nil {
		return fmt.Errorf("cascade: freeze CPP: %w", err)
	}
	return nil
}

// mbCostingRenameSpinQuery renames the head's spin rows still carrying the old costing.
// Orion Item Code is NOT touched here — see fillRootSpinOrionTx (root spin only, and only
// when no other live spin already carries the head code).
const mbCostingRenameSpinQuery = `
		UPDATE mst_mb_spin
		SET mbs_mb_costing = $1,
		    updated_at = NOW(), updated_by = $2
		WHERE mbs_mbh_id = $3 AND mbs_mb_costing = $4 AND deleted_at IS NULL`

// renameSpinCostingTx is the legacy (no cost product) rename: a no-op unless the costing
// really changed. Before renaming, the head's root spin — if it is one of the rows being
// renamed (still on the old costing) — gets an empty Orion Item Code filled.
func renameSpinCostingTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, oldCosting, actor string) error {
	if oldCosting == entity.MBCosting() {
		return nil
	}
	if err := fillRootSpinOrionTx(ctx, tx, entity, actor, &oldCosting); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, mbCostingRenameSpinQuery,
		entity.MBCosting(), actor, entity.ID(), oldCosting,
	); err != nil {
		return fmt.Errorf("rename mb spin costing: %w", err)
	}
	return nil
}

// mbFillRootSpinOrionQuery fills the Orion Item Code of ONE spin of a head — its oldest live
// root spin (mbs_parent_spin_id IS NULL), optionally restricted to a costing ($4, NULL = any)
// — with the head code ($1), and only when:
//   - that root's code is empty (NULL / blank), and
//   - no OTHER live spin (any head) already carries the code (TRIM compare).
//
// Children never receive the head code. This keeps MB_SP_CODE text-keyed yarn rows
// unambiguous: a second live spin with the same code would trip SkipMBSpinAmbiguous /
// ResolveUniqueByOrionItemCode and shift GetByOrionItemCode's oldest-wins winner.
const mbFillRootSpinOrionQuery = `
		UPDATE mst_mb_spin r
		SET mbs_orion_item_code = CAST($1 AS TEXT), updated_at = NOW(), updated_by = $2
		WHERE r.mbs_id = (
		        SELECT x.mbs_id FROM mst_mb_spin x
		        WHERE x.mbs_mbh_id = $3 AND x.mbs_parent_spin_id IS NULL AND x.deleted_at IS NULL
		          AND (CAST($4 AS TEXT) IS NULL OR x.mbs_mb_costing = CAST($4 AS TEXT))
		        ORDER BY x.created_at ASC, x.mbs_id ASC LIMIT 1)
		  AND NULLIF(TRIM(r.mbs_orion_item_code), '') IS NULL
		  AND NOT EXISTS (
		        SELECT 1 FROM mst_mb_spin o
		        WHERE o.deleted_at IS NULL AND o.mbs_id <> r.mbs_id
		          AND TRIM(o.mbs_orion_item_code) = CAST($1 AS TEXT))`

// fillRootSpinOrionTx runs mbFillRootSpinOrionQuery; a head without a code is a no-op.
func fillRootSpinOrionTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, actor string, costing *string) error {
	code := entity.OrionItemCode()
	if code == nil || strings.TrimSpace(*code) == "" {
		return nil
	}
	if _, err := tx.ExecContext(ctx, mbFillRootSpinOrionQuery, strings.TrimSpace(*code), actor, entity.ID(), costing); err != nil {
		return fmt.Errorf("fill root mb spin orion item code: %w", err)
	}
	return nil
}

// mbCascadeSpinQuery overwrites the head-derived columns of every live spin of a head
// (decision 1: no manual-edit detection). Nullable spec fields (final product, VS
// number, lusture) are overwritten even with NULL — user decision "overwrite all".
//
// Shade code, shade name and cross-section mirror syncRootSpinShadeFromHead exactly:
// when $14 (any of the three is non-empty on the head) they are written as a set — an
// empty one as NULL — so a code and a name from different shades can never be mixed;
// when all three are empty the spin keeps its values. mbs_cc follows the shade code (000499).
//
// mbs_orion_item_code is NOT written here: cascadeHeadToSpinsTx follows up with
// fillRootSpinOrionTx (root spin only, only when empty and no other live spin has the code).
//
// ⛔ Deliberately untouched: mbs_status, mbs_is_active, mbs_cost_rate_mkt, mbs_ldr_*,
// mbs_run_ldr_pct, mbs_ldr_prsn and the *_is_fixed flags.
const mbCascadeSpinQuery = `
		UPDATE mst_mb_spin SET
			mbs_mb_costing    = $1,
			mbs_mgt_name      = $2,
			mbs_denier        = $3,
			mbs_filament      = $4,
			mbs_dozing        = $5,
			mbs_final_product = $6,
			mbs_vs_number     = $7,
			mbs_lusture_code  = $8,
			mbs_cross_section = CASE WHEN $14 THEN $9 ELSE mbs_cross_section END,
			mbs_shade_code    = CASE WHEN $14 THEN $10 ELSE mbs_shade_code END,
			mbs_shade_name    = CASE WHEN $14 THEN $11 ELSE mbs_shade_name END,
			mbs_cc            = CASE WHEN $14 THEN $10 ELSE mbs_cc END,
			updated_at        = NOW(),
			updated_by        = $12
		WHERE mbs_mbh_id = $13 AND deleted_at IS NULL
		RETURNING mbs_id`

// checkCascadeFgShadeTx rejects a shade change that would leave a live spin of the head with
// a non-RECIPE FG (DUPLICATE / MANUAL / BACKFILL) whose POY shade no longer equals the new
// spin shade (P8-R backend BE-4). RECIPE-linked FGs are validated against the head itself.
func checkCascadeFgShadeTx(ctx context.Context, tx *sql.Tx, headID uuid.UUID, newShade *string) error {
	want := ""
	if newShade != nil {
		want = strings.ToUpper(strings.TrimSpace(*newShade))
	}
	var bad bool
	if err := tx.QueryRowContext(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM mst_mb_spin s
			JOIN cost_product_master p ON p.cpm_product_sys_id = s.mbs_fg_product_id
			WHERE s.mbs_mbh_id = $1 AND s.deleted_at IS NULL AND s.mbs_fg_product_id IS NOT NULL
			  AND s.mbs_fg_link_source IS DISTINCT FROM 'RECIPE'
			  AND COALESCE(UPPER(TRIM(s.mbs_shade_code)), '') <> $2
			  AND COALESCE(UPPER(TRIM(p.cpm_shade_code)), '') <> $2)`, headID, want).Scan(&bad); err != nil {
		return fmt.Errorf("cascade: check spin fg shade: %w", err)
	}
	if bad {
		return mbspin.ErrFgShadeMismatch
	}
	return nil
}

func cascadeHeadToSpinsTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, actor string) ([]uuid.UUID, error) {
	shadeCode := mbEmptyStringToPtr(entity.ShadeCode())
	shadeName := mbEmptyStringToPtr(entity.ShadeName())
	crossSection := mbEmptyStringToPtr(entity.CrossSection())
	writeShade := shadeCode != nil || shadeName != nil || crossSection != nil
	if writeShade {
		if err := checkCascadeFgShadeTx(ctx, tx, entity.ID(), shadeCode); err != nil {
			return nil, err
		}
	}
	rows, err := tx.QueryContext(ctx, mbCascadeSpinQuery,
		entity.MBCosting(),
		mbSpinAutoGenMgtName(entity),
		entity.Denier(),
		entity.Filament(),
		entity.Dozing(),
		entity.MBHFinalProduct(),
		entity.VSNumber(),
		mbEmptyStringToPtr(entity.LustureCode()),
		crossSection,
		shadeCode,
		shadeName,
		actor,
		entity.ID(),
		writeShade,
	)
	if err != nil {
		if isSpinShadeFgViolation(err) {
			return nil, mbspin.ErrSpinShadeFgExists
		}
		return nil, fmt.Errorf("cascade mb head to spins: %w", err)
	}
	defer closeRows(rows)
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan cascaded spin id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cascaded spin ids: %w", err)
	}
	if err := fillRootSpinOrionTx(ctx, tx, entity, actor, nil); err != nil {
		return nil, err
	}
	return ids, nil
}

// mbCascadeProductQuery keeps the auto-generated MB-recipe cost product's name and shade
// in step with the head. The cpm_source guard keeps a hand-made product that happens to
// be linked untouched. Shade code + name mirror syncCostProductMasterShadeFromHead: when
// $6 (either is non-empty on the head) both are written as a pair — an empty one as NULL —
// otherwise the product keeps its shade.
const mbCascadeProductQuery = `
		UPDATE cost_product_master SET
			cpm_product_name = $1,
			cpm_shade_code   = CASE WHEN $6 THEN $2 ELSE cpm_shade_code END,
			cpm_shade_name   = CASE WHEN $6 THEN $3 ELSE cpm_shade_name END,
			cpm_updated_at   = NOW(),
			cpm_updated_by   = $4
		WHERE cpm_product_sys_id = $5 AND cpm_source = $7`

func cascadeHeadToCostProductTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, productID int64, actor string) error {
	shadeCode := mbEmptyStringToPtr(entity.ShadeCode())
	shadeName := mbEmptyStringToPtr(entity.ShadeName())
	writeShade := shadeCode != nil || shadeName != nil
	if _, err := tx.ExecContext(ctx, mbCascadeProductQuery,
		entity.MBCosting(), shadeCode, shadeName,
		actor, productID, writeShade, mbCostProductSource,
	); err != nil {
		return fmt.Errorf("cascade mb head to cost product: %w", err)
	}
	return nil
}

func (r *MBHeadRepository) updateOn(ctx context.Context, q rowQuerier, entity *mbhead.Entity) error {
	result, err := q.ExecContext(ctx, `
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
			mbh_no_of_process = $23,
			mbh_orion_item_code = $24,
			mbh_fg_product_id   = $25,
			mbh_fg_item_code    = $26
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
		entity.OrionItemCode(),
		entity.FgProductID(),
		entity.FgItemCode(),
	)
	if err != nil {
		if isMBHeadUniqueViolation(err) {
			return mbhead.ErrAlreadyExists
		}
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

// ListValidated returns the VALIDATED, ACTIVE, non-deleted MB Heads: the push-to-head
// candidate set (rule 2 of mb-active-flag-rules; inactive heads are never pushed).
// The MB batch uses ListBatchCandidates instead.
func (r *MBHeadRepository) ListValidated(ctx context.Context) ([]MBHeadCandidate, error) {
	return r.listCandidates(ctx, listValidatedActiveSQL, "list validated mb heads")
}

const listValidatedActiveSQL = `
		SELECT mbh_id, mbh_mb_costing, COALESCE(mbh_mgt_name, ''),
		       COALESCE(mbh_cost_product_id, 0), mbh_is_boughtout, mbh_current_version
		FROM mst_mb_head
		WHERE mbh_entry_status = 'VALIDATED' AND mbh_is_active = TRUE AND deleted_at IS NULL
	`

// listBatchCandidatesSQL: VALIDATED+active heads, plus VALIDATED inactive heads reachable
// transitively from an active head's CURRENT composition version through source_type='MB'
// refs (rule 1). UNION (not UNION ALL) in the recursive CTE makes it cycle-safe.
const listBatchCandidatesSQL = `
		WITH RECURSIVE live AS (
		    SELECT mbh_id FROM mst_mb_head
		    WHERE mbh_entry_status = 'VALIDATED' AND mbh_is_active = TRUE AND deleted_at IS NULL
		  UNION
		    SELECT ref.mbh_id
		    FROM live l
		    JOIN mst_mb_head p ON p.mbh_id = l.mbh_id
		    JOIN mst_mb_composition_version v
		      ON v.mbcv_mbh_id = p.mbh_id AND v.mbcv_version = p.mbh_current_version
		     AND v.mbcv_source_type = 'MB'
		    JOIN mst_mb_head ref ON ref.mbh_id = v.mbcv_mb_ref_mbh_id
		     AND ref.mbh_entry_status = 'VALIDATED' AND ref.deleted_at IS NULL
		)
		SELECT h.mbh_id, h.mbh_mb_costing, COALESCE(h.mbh_mgt_name, ''),
		       COALESCE(h.mbh_cost_product_id, 0), h.mbh_is_boughtout, h.mbh_current_version
		FROM mst_mb_head h
		WHERE h.mbh_id IN (SELECT mbh_id FROM live)
	`

// ListBatchCandidates returns the MB batch candidate set (rules 1): see listBatchCandidatesSQL.
// Also drives RecomputeRevalidated's DAG.
func (r *MBHeadRepository) ListBatchCandidates(ctx context.Context) ([]MBHeadCandidate, error) {
	return r.listCandidates(ctx, listBatchCandidatesSQL, "list mb batch candidates")
}

func (r *MBHeadRepository) listCandidates(ctx context.Context, q, what string) ([]MBHeadCandidate, error) {
	rows, err := r.db.QueryContext(ctx, q)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", what, err)
	}
	defer closeRows(rows)

	var out []MBHeadCandidate
	for rows.Next() {
		var c MBHeadCandidate
		if err := rows.Scan(&c.MBHID, &c.Code, &c.Name, &c.CostProductID, &c.IsBoughtout, &c.CurrentVersion); err != nil {
			return nil, fmt.Errorf("scan %s: %w", what, err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate %s: %w", what, err)
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
		       mbh_check_status_calc, mbh_orion_item_code,
		       mbh_fg_product_id, mbh_fg_item_code,
		       mbh_superseded_by_head_id, mbh_superseded_at,
		       (SELECT p.cpm_product_code FROM cost_product_master p WHERE p.cpm_product_sys_id = mst_mb_head.mbh_fg_product_id),
		       (SELECT p.cpm_product_name FROM cost_product_master p WHERE p.cpm_product_sys_id = mst_mb_head.mbh_fg_product_id)
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
	// OrionItemCode is mbh_orion_item_code (000416). NULL = not set.
	OrionItemCode sql.NullString
	// FG link columns (000607) + read-model subqueries (POY code/name).
	FgProductID        sql.NullInt64
	FgItemCode         sql.NullString
	SupersededByHeadID sql.NullString
	SupersededAt       sql.NullTime
	FgProductCode      sql.NullString
	FgProductName      sql.NullString
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
		OrionItemCode:      nullableStringPtr(d.OrionItemCode),
	})
	e.HydrateFg(nullableInt64Ptr(d.FgProductID), nullableStringPtr(d.FgItemCode),
		nullableUUIDPtr(d.SupersededByHeadID), nullableTimePtr(d.SupersededAt))
	e.HydrateFgReadModel(mbhead.FgReadModel{
		FgProductCode: nullableStringPtr(d.FgProductCode),
		FgProductName: nullableStringPtr(d.FgProductName),
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
		&d.MBHCheckStatusCalc, &d.OrionItemCode,
		&d.FgProductID, &d.FgItemCode, &d.SupersededByHeadID, &d.SupersededAt,
		&d.FgProductCode, &d.FgProductName,
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
		&d.MBHCheckStatusCalc, &d.OrionItemCode,
		&d.FgProductID, &d.FgItemCode, &d.SupersededByHeadID, &d.SupersededAt,
		&d.FgProductCode, &d.FgProductName,
	)
	if err != nil {
		return nil, fmt.Errorf("scan mb head row: %w", err)
	}
	return d.toEntity(), nil
}

func isMBHeadUniqueViolation(err error) bool {
	return isPGUniqueViolation(err)
}

// mbSyncRootSpinFgQuery points the head's root spin(s) at the head's FG (design §2.2).
// Only rows whose FG really differs are written. When the head has NO FG ($1 NULL) only a
// link this flow created (link_source RECIPE) is cleared; a MANUAL/DUPLICATE/BACKFILL link
// is never wiped by a recipe edit.
const mbSyncRootSpinFgQuery = `
		UPDATE mst_mb_spin SET
			mbs_fg_product_id = CAST($1 AS BIGINT),
			mbs_fg_item_code  = CAST($2 AS VARCHAR),
			mbs_fg_link_source = CASE WHEN CAST($1 AS BIGINT) IS NULL THEN NULL ELSE 'RECIPE' END,
			updated_at = NOW(), updated_by = $4
		WHERE mbs_mbh_id = $3 AND mbs_parent_spin_id IS NULL AND deleted_at IS NULL
		  AND (mbs_fg_product_id IS DISTINCT FROM CAST($1 AS BIGINT)
		       OR mbs_fg_item_code IS DISTINCT FROM CAST($2 AS VARCHAR))
		  AND (CAST($1 AS BIGINT) IS NOT NULL OR mbs_fg_link_source = 'RECIPE')`

// syncRootSpinFgTx runs mbSyncRootSpinFgQuery inside the cascade transaction. A violation
// of uix_mbs_shade_fg_live (same shade + FG already live on another spin) maps to
// mbspin.ErrSpinShadeFgExists and rolls the whole update back.
func syncRootSpinFgTx(ctx context.Context, tx *sql.Tx, entity *mbhead.Entity, actor string) error {
	if _, err := tx.ExecContext(ctx, mbSyncRootSpinFgQuery,
		entity.FgProductID(), entity.FgItemCode(), entity.ID(), actor,
	); err != nil {
		if isSpinShadeFgViolation(err) {
			return mbspin.ErrSpinShadeFgExists
		}
		return fmt.Errorf("cascade mb head fg to root spin: %w", err)
	}
	return nil
}
