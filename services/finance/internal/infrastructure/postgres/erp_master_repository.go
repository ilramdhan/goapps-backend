package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erpMasterActor is written to *_created_by / *_updated_by by the sync.
const erpMasterActor = "erp_master_sync"

// erpMasterChunk bounds the size of one array-bound upsert statement.
const erpMasterChunk = 1000

// Upsert statements. The SET lists hold replica columns only; they never
// name ceg_grade_group (GoApps-owned, AC-11) nor cei_item_type (derived once
// on insert). The WHERE clause keeps a re-run with unchanged data a no-op.
const (
	upsertErpItemsSQL = `
INSERT INTO cost_erp_item (cei_item_code, cei_item_name, cei_item_type, cei_is_active,
                           cei_synced_at, cei_created_by, cei_updated_by)
SELECT s.code, s.name, LEFT(s.code, 3), s.active, NOW(), $4, $4
  FROM unnest($1::text[], $2::text[], $3::bool[]) AS s(code, name, active)
ON CONFLICT (cei_item_code) DO UPDATE
   SET cei_item_name  = EXCLUDED.cei_item_name,
       cei_is_active  = EXCLUDED.cei_is_active,
       cei_synced_at  = NOW(),
       cei_updated_at = NOW(),
       cei_updated_by = EXCLUDED.cei_updated_by
 WHERE cost_erp_item.cei_item_name IS DISTINCT FROM EXCLUDED.cei_item_name
    OR cost_erp_item.cei_is_active IS DISTINCT FROM EXCLUDED.cei_is_active
RETURNING (xmax = 0)`

	upsertErpGradesSQL = `
INSERT INTO cost_erp_grade (ceg_grade_code, ceg_grade_name, ceg_is_active, ceg_synced_at)
SELECT s.code, s.name, s.active, NOW()
  FROM unnest($1::text[], $2::text[], $3::bool[]) AS s(code, name, active)
ON CONFLICT (ceg_grade_code) DO UPDATE
   SET ceg_grade_name = EXCLUDED.ceg_grade_name,
       ceg_is_active  = EXCLUDED.ceg_is_active,
       ceg_synced_at  = NOW()
 WHERE cost_erp_grade.ceg_grade_name IS DISTINCT FROM EXCLUDED.ceg_grade_name
    OR cost_erp_grade.ceg_is_active IS DISTINCT FROM EXCLUDED.ceg_is_active
RETURNING (xmax = 0)`

	countGradeGroupSeedSQL = `SELECT COUNT(*) FROM cst_erp_grade_group_seed`

	countAlreadyGroupedSQL = `
SELECT COUNT(*)
  FROM cost_erp_grade g
  JOIN cst_erp_grade_group_seed s ON s.cggs_grade_code = g.ceg_grade_code
 WHERE g.ceg_grade_group IS NOT NULL`

	applyGradeGroupSeedSQL = `
UPDATE cost_erp_grade g
   SET ceg_grade_group = s.cggs_grade_group
  FROM cst_erp_grade_group_seed s
 WHERE g.ceg_grade_code = s.cggs_grade_code
   AND g.ceg_grade_group IS NULL`

	missingSeedCodesSQL = `
SELECT s.cggs_grade_code
  FROM cst_erp_grade_group_seed s
  LEFT JOIN cost_erp_grade g ON g.ceg_grade_code = s.cggs_grade_code
 WHERE g.ceg_grade_code IS NULL
 ORDER BY s.cggs_grade_code`
)

// ErpMasterRepository implements erpintegration.MasterRepository.
type ErpMasterRepository struct{ db *DB }

var _ erpintegration.MasterRepository = (*ErpMasterRepository)(nil)

// NewErpMasterRepository builds the repository.
func NewErpMasterRepository(db *DB) *ErpMasterRepository {
	return &ErpMasterRepository{db: db}
}

// masterBatch is the array-bound form of a replica upsert.
type masterBatch struct {
	codes, names []string
	active       []bool
}

// UpsertItems upserts OM_ITEM rows into cost_erp_item in one transaction.
func (r *ErpMasterRepository) UpsertItems(ctx context.Context, items []erpintegration.MasterItem) (erpintegration.UpsertCounts, error) {
	b := masterBatch{}
	seen := make(map[string]struct{}, len(items))
	for _, it := range items {
		if _, dup := seen[it.Code]; dup || it.Code == "" {
			continue
		}
		seen[it.Code] = struct{}{}
		b.codes = append(b.codes, it.Code)
		b.names = append(b.names, it.Name)
		b.active = append(b.active, it.Active)
	}
	counts, err := r.upsert(ctx, upsertErpItemsSQL, b, true)
	if err != nil {
		return counts, fmt.Errorf("upsert cost_erp_item: %w", err)
	}
	counts.Read = int64(len(items))
	counts.Unchanged = counts.Read - counts.Inserted - counts.Updated
	return counts, nil
}

// UpsertGrades upserts OM_GRADE_CODE_1 rows into cost_erp_grade in one
// transaction. ceg_grade_group is never written here.
func (r *ErpMasterRepository) UpsertGrades(ctx context.Context, grades []erpintegration.MasterGrade) (erpintegration.UpsertCounts, error) {
	b := masterBatch{}
	seen := make(map[string]struct{}, len(grades))
	for _, g := range grades {
		if _, dup := seen[g.Code]; dup || g.Code == "" {
			continue
		}
		seen[g.Code] = struct{}{}
		b.codes = append(b.codes, g.Code)
		b.names = append(b.names, g.Name)
		b.active = append(b.active, g.Active)
	}
	counts, err := r.upsert(ctx, upsertErpGradesSQL, b, false)
	if err != nil {
		return counts, fmt.Errorf("upsert cost_erp_grade: %w", err)
	}
	counts.Read = int64(len(grades))
	counts.Unchanged = counts.Read - counts.Inserted - counts.Updated
	return counts, nil
}

// upsert runs query per chunk inside one transaction and counts the
// inserted (xmax = 0) and updated rows it returns.
func (r *ErpMasterRepository) upsert(ctx context.Context, query string, b masterBatch, withActor bool) (erpintegration.UpsertCounts, error) {
	var counts erpintegration.UpsertCounts
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		for start := 0; start < len(b.codes); start += erpMasterChunk {
			end := min(start+erpMasterChunk, len(b.codes))
			args := []any{pq.Array(b.codes[start:end]), pq.Array(b.names[start:end]), pq.Array(b.active[start:end])}
			if withActor {
				args = append(args, erpMasterActor)
			}
			ins, upd, err := runUpsertChunk(ctx, tx, query, args)
			if err != nil {
				return err
			}
			counts.Inserted += ins
			counts.Updated += upd
		}
		return nil
	})
	return counts, err
}

// runUpsertChunk executes one upsert and tallies its RETURNING rows.
func runUpsertChunk(ctx context.Context, tx *sql.Tx, query string, args []any) (inserted, updated int64, err error) {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return 0, 0, err
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	for rows.Next() {
		var isInsert bool
		if err := rows.Scan(&isInsert); err != nil {
			return 0, 0, err
		}
		if isInsert {
			inserted++
		} else {
			updated++
		}
	}
	return inserted, updated, rows.Err()
}

// ApplyGradeGroupSeed fills NULL ceg_grade_group values from
// cst_erp_grade_group_seed (P0-T15b). It never overwrites a group already set
// and is idempotent: a re-run applies 0 rows.
func (r *ErpMasterRepository) ApplyGradeGroupSeed(ctx context.Context) (erpintegration.GradeGroupApplyReport, error) {
	var rep erpintegration.GradeGroupApplyReport
	err := r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := tx.QueryRowContext(ctx, countGradeGroupSeedSQL).Scan(&rep.SeedRows); err != nil {
			return fmt.Errorf("count seed: %w", err)
		}
		if err := tx.QueryRowContext(ctx, countAlreadyGroupedSQL).Scan(&rep.AlreadyGrouped); err != nil {
			return fmt.Errorf("count already grouped: %w", err)
		}
		res, err := tx.ExecContext(ctx, applyGradeGroupSeedSQL)
		if err != nil {
			return fmt.Errorf("apply seed: %w", err)
		}
		if rep.AppliedNow, err = res.RowsAffected(); err != nil {
			return fmt.Errorf("rows affected: %w", err)
		}
		missing, err := queryMissingSeedCodes(ctx, tx)
		if err != nil {
			return err
		}
		rep.MissingCodes = missing
		return nil
	})
	if err != nil {
		return erpintegration.GradeGroupApplyReport{}, fmt.Errorf("apply grade-group seed: %w", err)
	}
	return rep, nil
}

// queryMissingSeedCodes lists seed codes that have no replica row yet.
func queryMissingSeedCodes(ctx context.Context, tx *sql.Tx) (out []string, err error) {
	rows, err := tx.QueryContext(ctx, missingSeedCodesSQL)
	if err != nil {
		return nil, fmt.Errorf("missing seed codes: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = cerr
		}
	}()
	for rows.Next() {
		var code string
		if err := rows.Scan(&code); err != nil {
			return nil, fmt.Errorf("scan missing code: %w", err)
		}
		out = append(out, code)
	}
	return out, rows.Err()
}
