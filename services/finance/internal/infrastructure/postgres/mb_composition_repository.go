// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbcomposition"
)

// MBCompositionRepository implements mbcomposition.Repository using PostgreSQL.
type MBCompositionRepository struct {
	db *DB
}

// NewMBCompositionRepository creates a new MBCompositionRepository instance.
func NewMBCompositionRepository(db *DB) *MBCompositionRepository {
	return &MBCompositionRepository{db: db}
}

// Verify interface implementation at compile time.
var _ mbcomposition.Repository = (*MBCompositionRepository)(nil)

// mbCompositionParentLockQuery takes the row lock that makes the composition-sum
// rule hold under concurrency (G24). Package-level so the FOR UPDATE clause is
// pinned by a test — dropping it would silently reintroduce the race, and there is
// no DB constraint that would catch the resulting bad total.
const mbCompositionParentLockQuery = `SELECT 1 FROM mst_mb_head WHERE mbh_id = $1 AND deleted_at IS NULL FOR UPDATE`

// rowQuerier is the subset of *sql.DB / *sql.Tx the composition write paths need,
// so one implementation serves both the plain and the transactional call sites.
type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
}

// Create persists a new composition row.
func (r *MBCompositionRepository) Create(ctx context.Context, e *mbcomposition.Entity) error {
	return r.createOn(ctx, r.db, e)
}

// CreateWithSumGuard inserts e with the composition-sum rule checked atomically (G24).
//
// The lock is what makes the rule hold under concurrency: SELECT ... FOR UPDATE on
// the parent mst_mb_head row serializes every guarded writer for that mbh_id, so the
// sum a writer reads cannot be invalidated by another writer between the read and the
// insert. Without it, two requests each observing a total of 90 could both add 10 and
// both pass, storing 110 — and nothing in the schema would catch it, since there is
// deliberately no CHECK constraint (4 legacy recipes would fail it).
//
// A nil guard means the feature flag is off: no lock is taken and no sum is read, so
// the unenforced path costs exactly what it did before.
func (r *MBCompositionRepository) CreateWithSumGuard(ctx context.Context, e *mbcomposition.Entity, guard mbcomposition.SumGuard) error {
	if guard == nil {
		return r.Create(ctx, e)
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.guardSumTx(ctx, tx, e.MbhID(), guard); err != nil {
			return err
		}
		return r.createOn(ctx, tx, e)
	})
}

// UpdateWithSumGuard updates e with the composition-sum rule checked atomically (G24).
// See CreateWithSumGuard for why the parent-row lock is required.
func (r *MBCompositionRepository) UpdateWithSumGuard(ctx context.Context, e *mbcomposition.Entity, guard mbcomposition.SumGuard) error {
	if guard == nil {
		return r.Update(ctx, e)
	}
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := r.guardSumTx(ctx, tx, e.MbhID(), guard); err != nil {
			return err
		}
		return r.updateOn(ctx, tx, e)
	})
}

// guardSumTx locks the parent head row, reads the current non-carrier percentage sum
// under that lock, and hands it to guard. Must be called inside a transaction.
//
// ⚠ The lock is taken on mst_mb_head, ⛔ NOT on the mst_mb_composition rows: the rule
// being protected is a property of the whole composition set for one mbh_id, and a
// concurrent INSERT adds a row that no pre-existing row lock could cover. Locking the
// single parent row is both sufficient and the narrowest scope that is.
func (r *MBCompositionRepository) guardSumTx(ctx context.Context, tx *sql.Tx, mbhID string, guard mbcomposition.SumGuard) error {
	var one int
	if err := tx.QueryRowContext(ctx, mbCompositionParentLockQuery, mbhID).Scan(&one); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return mbcomposition.ErrParentHeadNotFound
		}
		return fmt.Errorf("mb_composition_repository: lock parent head: %w", err)
	}

	const sumQ = `
		SELECT COALESCE(SUM(mbcm_composition_pct), 0)::text
		FROM mst_mb_composition
		WHERE mbcm_mbh_id = $1 AND deleted_at IS NULL AND mbcm_is_carrier = FALSE`
	var total string
	if err := tx.QueryRowContext(ctx, sumQ, mbhID).Scan(&total); err != nil {
		return fmt.Errorf("mb_composition_repository: sum percentage under lock: %w", err)
	}
	return guard(total)
}

func (r *MBCompositionRepository) createOn(ctx context.Context, q rowQuerier, e *mbcomposition.Entity) error {
	const stmt = `
		INSERT INTO mst_mb_composition
			(mbcm_mbh_id, mbcm_seq_no, mbcm_group_head_id, mbcm_composition_pct,
			 mbcm_source_type, mbcm_mb_ref_mbh_id, mbcm_is_carrier, mbcm_created_by)
		VALUES ($1, $2, NULLIF($3, '')::uuid, $4, $5, NULLIF($6, '')::uuid, $7, $8)
		RETURNING mbcm_id`
	var id string
	err := q.QueryRowContext(ctx, stmt,
		e.MbhID(), e.SeqNo(), e.GroupHeadID(), e.CompositionPct(),
		e.SourceType(), e.MbRefMbhID(), e.IsCarrier(), e.CreatedBy(),
	).Scan(&id)
	if err != nil {
		return fmt.Errorf("mb_composition_repository: create: %w", err)
	}
	return nil
}

// Update persists changes to an existing composition row.
func (r *MBCompositionRepository) Update(ctx context.Context, e *mbcomposition.Entity) error {
	return r.updateOn(ctx, r.db, e)
}

func (r *MBCompositionRepository) updateOn(ctx context.Context, q rowQuerier, e *mbcomposition.Entity) error {
	const stmt = `
		UPDATE mst_mb_composition
		SET mbcm_group_head_id = NULLIF($2, '')::uuid, mbcm_composition_pct = $3, mbcm_source_type = $4,
		    mbcm_mb_ref_mbh_id = NULLIF($5, '')::uuid, mbcm_is_carrier = $6,
		    mbcm_updated_at = NOW(), mbcm_updated_by = $7
		WHERE mbcm_id = $1 AND deleted_at IS NULL`
	result, err := q.ExecContext(ctx, stmt, e.ID(), e.GroupHeadID(), e.CompositionPct(), e.SourceType(), e.MbRefMbhID(), e.IsCarrier(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("mb_composition_repository: update: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_composition_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbcomposition.ErrNotFound
	}
	return nil
}

// Delete soft-deletes a composition row by ID.
func (r *MBCompositionRepository) Delete(ctx context.Context, id string) error {
	const q = `UPDATE mst_mb_composition SET deleted_at = NOW() WHERE mbcm_id = $1 AND deleted_at IS NULL`
	result, err := r.db.ExecContext(ctx, q, id)
	if err != nil {
		return fmt.Errorf("mb_composition_repository: delete: %w", err)
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("mb_composition_repository: rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return mbcomposition.ErrNotFound
	}
	return nil
}

// ParentEntryStatus returns the parent head's mbh_entry_status for the DRAFT gate.
//
// It reads mst_mb_head, not mst_mb_composition: the gate's question is about the
// PARENT's workflow state. Soft-deleted heads are excluded, so a deleted head is
// reported as ErrParentHeadNotFound rather than yielding its last status and
// letting a write through against a head that no longer exists.
func (r *MBCompositionRepository) ParentEntryStatus(ctx context.Context, mbhID string) (string, error) {
	const q = `SELECT mbh_entry_status FROM mst_mb_head WHERE mbh_id = $1 AND deleted_at IS NULL`
	var status string
	if err := r.db.QueryRowContext(ctx, q, mbhID).Scan(&status); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", mbcomposition.ErrParentHeadNotFound
		}
		return "", fmt.Errorf("mb_composition_repository: parent entry status: %w", err)
	}
	return status, nil
}

// ListMBRefEdgesForBatch returns the within-batch MB-to-MB composition
// references among mbhIDs, reading the LIVE mst_mb_composition table (not the
// VALIDATED mst_mb_composition_version snapshot — at ordering time the heads
// in this batch have not necessarily been validated yet, so their current
// working recipe is what matters). Both the row's own mbh_id and its
// mbcm_mb_ref_mbh_id must be members of mbhIDs, or the row is excluded: a
// reference to a head outside the batch is not this method's concern (see
// mbcomposition.Repository.ListMBRefEdgesForBatch doc comment).
func (r *MBCompositionRepository) ListMBRefEdgesForBatch(ctx context.Context, mbhIDs []string) ([]mbcomposition.BatchRefEdge, error) {
	if len(mbhIDs) == 0 {
		return nil, nil
	}
	const q = `
		SELECT mbcm_mbh_id, mbcm_mb_ref_mbh_id
		FROM mst_mb_composition
		WHERE mbcm_mbh_id = ANY($1::uuid[])
		  AND mbcm_source_type = 'MB'
		  AND mbcm_mb_ref_mbh_id = ANY($1::uuid[])
		  AND deleted_at IS NULL`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(mbhIDs))
	if err != nil {
		return nil, fmt.Errorf("mb_composition_repository: list mb ref edges for batch: %w", err)
	}
	defer closeRows(rows)

	var out []mbcomposition.BatchRefEdge
	for rows.Next() {
		var e mbcomposition.BatchRefEdge
		if scanErr := rows.Scan(&e.MbhID, &e.RefMbhID); scanErr != nil {
			return nil, fmt.Errorf("mb_composition_repository: scan mb ref edge: %w", scanErr)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_composition_repository: iterate mb ref edges: %w", err)
	}
	return out, nil
}

// GetByID returns a single active composition row by ID.
func (r *MBCompositionRepository) GetByID(ctx context.Context, id string) (*mbcomposition.Entity, error) {
	row := r.db.QueryRowContext(ctx, r.selectCols()+` WHERE mbcm_id = $1 AND deleted_at IS NULL`, id)
	e, err := r.scanOne(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, mbcomposition.ErrNotFound
		}
		return nil, fmt.Errorf("mb_composition_repository: get by id: %w", err)
	}
	return e, nil
}

// ListByMbhID returns all active composition rows for a parent MB head, ordered by sequence.
func (r *MBCompositionRepository) ListByMbhID(ctx context.Context, mbhID string) ([]*mbcomposition.Entity, error) {
	rows, err := r.db.QueryContext(ctx, r.selectCols()+` WHERE mbcm_mbh_id = $1 AND deleted_at IS NULL ORDER BY mbcm_seq_no ASC`, mbhID)
	if err != nil {
		return nil, fmt.Errorf("mb_composition_repository: list: %w", err)
	}
	defer closeRows(rows)

	var out []*mbcomposition.Entity
	for rows.Next() {
		e, scanErr := r.scanRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_composition_repository: iterate: %w", err)
	}
	return out, nil
}

// ListVersionsByMbhID returns the frozen composition snapshot rows for mbhID at the given
// version. version == 0 resolves to the latest version available for mbhID.
func (r *MBCompositionRepository) ListVersionsByMbhID(ctx context.Context, mbhID string, version int32) ([]mbcomposition.VersionRow, error) {
	const q = `
		SELECT mbcv_id, mbcv_mbh_id, mbcv_version, mbcv_validated_at::text, mbcv_validated_by,
		       mbcv_seq_no, mbcv_group_head_id, mbcv_composition_pct, mbcv_source_type,
		       COALESCE(mbcv_mb_ref_mbh_id::text, ''), mbcv_is_carrier
		FROM mst_mb_composition_version
		WHERE mbcv_mbh_id = $1
		  AND mbcv_version = CASE WHEN $2::int = 0 THEN (
		        SELECT MAX(mbcv_version) FROM mst_mb_composition_version WHERE mbcv_mbh_id = $1
		      ) ELSE $2::int END
		ORDER BY mbcv_seq_no ASC`
	rows, err := r.db.QueryContext(ctx, q, mbhID, version)
	if err != nil {
		return nil, fmt.Errorf("mb_composition_repository: list versions: %w", err)
	}
	defer closeRows(rows)

	var out []mbcomposition.VersionRow
	for rows.Next() {
		var v mbcomposition.VersionRow
		if scanErr := rows.Scan(&v.ID, &v.MbhID, &v.Version, &v.ValidatedAt, &v.ValidatedBy,
			&v.SeqNo, &v.GroupHeadID, &v.CompositionPct, &v.SourceType, &v.MbRefMbhID, &v.IsCarrier); scanErr != nil {
			return nil, fmt.Errorf("mb_composition_repository: scan version row: %w", scanErr)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_composition_repository: iterate versions: %w", err)
	}
	return out, nil
}

// SumPercentageByMbhID returns the sum of non-carrier composition percentages for a parent MB head.
func (r *MBCompositionRepository) SumPercentageByMbhID(ctx context.Context, mbhID string) (string, error) {
	const q = `SELECT COALESCE(SUM(mbcm_composition_pct), 0) FROM mst_mb_composition WHERE mbcm_mbh_id = $1 AND deleted_at IS NULL AND mbcm_is_carrier = FALSE`
	var total string
	err := r.db.QueryRowContext(ctx, q, mbhID).Scan(&total)
	if err != nil {
		return "", fmt.Errorf("mb_composition_repository: sum percentage: %w", err)
	}
	return total, nil
}

// SnapshotVersion copies all current mst_mb_composition rows for mbhID into
// mst_mb_composition_version at the given version number, called once per VALIDATED
// transition. Must run inside the same transaction as the mst_mb_head status update.
func (r *MBCompositionRepository) SnapshotVersion(ctx context.Context, tx *sql.Tx, mbhID string, version int32, validatedBy string) error {
	const q = `
		INSERT INTO mst_mb_composition_version
			(mbcv_mbh_id, mbcv_version, mbcv_validated_at, mbcv_validated_by,
			 mbcv_seq_no, mbcv_group_head_id, mbcv_composition_pct, mbcv_source_type,
			 mbcv_mb_ref_mbh_id, mbcv_is_carrier)
		SELECT mbcm_mbh_id, $2, NOW(), $3,
		       mbcm_seq_no, mbcm_group_head_id, mbcm_composition_pct, mbcm_source_type,
		       mbcm_mb_ref_mbh_id, mbcm_is_carrier
		FROM mst_mb_composition
		WHERE mbcm_mbh_id = $1 AND deleted_at IS NULL`
	_, err := tx.ExecContext(ctx, q, mbhID, version, validatedBy)
	if err != nil {
		return fmt.Errorf("mb_composition_repository: snapshot version: %w", err)
	}
	return nil
}

// VersionRow is one frozen composition line read back from mst_mb_composition_version,
// joined to cst_rm_group_head for the human-readable group code needed by cost_route_rm.
type VersionRow struct {
	SeqNo          int32
	GroupHeadID    string
	GroupCode      string
	CompositionPct string
	SourceType     string
	MbRefMbhID     string
	IsCarrier      bool
}

// ListVersionByMbhIDAndVersion reads back the frozen composition snapshot for mbhID at
// version, joined to cst_rm_group_head for group_code. Must run inside the same
// transaction as the snapshot write (called immediately after SnapshotVersion within
// Task 20b's auto-gen sequence).
func (r *MBCompositionRepository) ListVersionByMbhIDAndVersion(ctx context.Context, tx *sql.Tx, mbhID string, version int32) ([]VersionRow, error) {
	// LEFT JOIN so MB/CARRIER snapshot rows (NULL group_head_id) are NOT dropped — the
	// auto-gen route builder (mbInsertRouteRMs) needs every row, resolving MB rows by
	// mb_ref instead of group_code. group_head_id/group_code are COALESCE'd to '' for the
	// plain-string VersionRow scan targets.
	const q = `
		SELECT v.mbcv_seq_no, COALESCE(v.mbcv_group_head_id::text, ''), COALESCE(g.group_code, ''),
		       v.mbcv_composition_pct,
		       v.mbcv_source_type, COALESCE(v.mbcv_mb_ref_mbh_id::text, ''), v.mbcv_is_carrier
		FROM mst_mb_composition_version v
		LEFT JOIN cst_rm_group_head g ON g.group_head_id = v.mbcv_group_head_id
		WHERE v.mbcv_mbh_id = $1 AND v.mbcv_version = $2
		ORDER BY v.mbcv_seq_no ASC`
	rows, err := tx.QueryContext(ctx, q, mbhID, version)
	if err != nil {
		return nil, fmt.Errorf("mb_composition_repository: list version: %w", err)
	}
	defer closeRows(rows)

	var out []VersionRow
	for rows.Next() {
		var v VersionRow
		if scanErr := rows.Scan(&v.SeqNo, &v.GroupHeadID, &v.GroupCode, &v.CompositionPct,
			&v.SourceType, &v.MbRefMbhID, &v.IsCarrier); scanErr != nil {
			return nil, fmt.Errorf("mb_composition_repository: scan version row: %w", scanErr)
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_composition_repository: iterate version: %w", err)
	}
	return out, nil
}

// MBEdgeRow is one MB-to-MB composition edge read back from mst_mb_composition_version,
// for the mbbatch DAG builder (Task 21b). Unlike VersionRow it carries no RM-group data —
// only the fields needed to build a dependency graph across the whole VALIDATED MB set.
type MBEdgeRow struct {
	MbhID          string
	Version        int32
	RefMbhID       string
	CompositionPct string
}

// ListMBEdgesBulk reads mst_mb_composition_version rows with source_type = 'MB' across all
// (mbhID, version) pairs supplied, for the mbbatch DAG builder (Task 21b). Unlike
// ListVersionByMbhIDAndVersion this runs standalone (no *sql.Tx) because the DAG build reads
// across the whole VALIDATED set of MB heads, not a single MB's own transaction. mbhIDs and
// versions must be parallel slices (same index refers to the same MB head).
func (r *MBCompositionRepository) ListMBEdgesBulk(ctx context.Context, mbhIDs []string, versions []int32) ([]MBEdgeRow, error) {
	if len(mbhIDs) == 0 {
		return nil, nil
	}
	const q = `
		SELECT v.mbcv_mbh_id, v.mbcv_version, v.mbcv_mb_ref_mbh_id::text, v.mbcv_composition_pct
		FROM mst_mb_composition_version v
		JOIN UNNEST($1::uuid[], $2::int[]) AS want(mbh_id, version)
		     ON want.mbh_id = v.mbcv_mbh_id AND want.version = v.mbcv_version
		WHERE v.mbcv_source_type = 'MB'`
	rows, err := r.db.QueryContext(ctx, q, pq.Array(mbhIDs), pq.Array(versions))
	if err != nil {
		return nil, fmt.Errorf("mb_composition_repository: list mb edges bulk: %w", err)
	}
	defer closeRows(rows)

	var out []MBEdgeRow
	for rows.Next() {
		var e MBEdgeRow
		if scanErr := rows.Scan(&e.MbhID, &e.Version, &e.RefMbhID, &e.CompositionPct); scanErr != nil {
			return nil, fmt.Errorf("mb_composition_repository: scan mb edge row: %w", scanErr)
		}
		out = append(out, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mb_composition_repository: iterate mb edges: %w", err)
	}
	return out, nil
}

func (r *MBCompositionRepository) selectCols() string {
	return `
		SELECT mbcm_id, mbcm_mbh_id, mbcm_seq_no, mbcm_group_head_id, mbcm_composition_pct,
		       mbcm_source_type, COALESCE(mbcm_mb_ref_mbh_id::text, ''), mbcm_is_carrier,
		       COALESCE(mbcm_legacy_sys_id, ''),
		       mbcm_created_at, mbcm_created_by,
		       COALESCE(mbcm_updated_at::text, ''), COALESCE(mbcm_updated_by, ''),
		       COALESCE(deleted_at::text, ''), COALESCE(deleted_by, '')
		FROM mst_mb_composition
	`
}

type mbCompositionDTO struct {
	ID             string
	MbhID          string
	SeqNo          int32
	GroupHeadID    string
	CompositionPct string
	SourceType     string
	MbRefMbhID     string
	IsCarrier      bool
	LegacySysID    string
	CreatedAt      string
	CreatedBy      string
	UpdatedAt      string
	UpdatedBy      string
	DeletedAt      string
	DeletedBy      string
}

func (d *mbCompositionDTO) toEntity() *mbcomposition.Entity {
	return mbcomposition.Reconstruct(
		d.ID, d.MbhID, d.SeqNo, d.GroupHeadID, d.CompositionPct, d.SourceType,
		d.MbRefMbhID, d.IsCarrier, d.LegacySysID,
		d.CreatedAt, d.CreatedBy, d.UpdatedAt, d.UpdatedBy, d.DeletedAt, d.DeletedBy,
	)
}

func (r *MBCompositionRepository) scanRow(rows *sql.Rows) (*mbcomposition.Entity, error) {
	var d mbCompositionDTO
	err := rows.Scan(
		&d.ID, &d.MbhID, &d.SeqNo, &d.GroupHeadID, &d.CompositionPct, &d.SourceType,
		&d.MbRefMbhID, &d.IsCarrier, &d.LegacySysID,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, fmt.Errorf("mb_composition_repository: scan row: %w", err)
	}
	return d.toEntity(), nil
}

func (r *MBCompositionRepository) scanOne(row *sql.Row) (*mbcomposition.Entity, error) {
	var d mbCompositionDTO
	err := row.Scan(
		&d.ID, &d.MbhID, &d.SeqNo, &d.GroupHeadID, &d.CompositionPct, &d.SourceType,
		&d.MbRefMbhID, &d.IsCarrier, &d.LegacySysID,
		&d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy, &d.DeletedAt, &d.DeletedBy,
	)
	if err != nil {
		return nil, err
	}
	return d.toEntity(), nil
}
