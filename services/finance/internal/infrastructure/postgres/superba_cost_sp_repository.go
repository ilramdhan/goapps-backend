package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/superbacostsp"
)

// SuperbaCostSpRepository implements superbacostsp.Repository on cost_superba_cost_sp.
type SuperbaCostSpRepository struct {
	db *DB
}

// NewSuperbaCostSpRepository creates a new SuperbaCostSpRepository.
func NewSuperbaCostSpRepository(db *DB) *SuperbaCostSpRepository {
	return &SuperbaCostSpRepository{db: db}
}

var _ superbacostsp.Repository = (*SuperbaCostSpRepository)(nil)

// superbaResolveSQL is THE shade resolution rule (design section 3): both sides
// normalized with UPPER(TRIM()), only active + non-deleted rows, duplicate shade
// -> the row with the largest legacy_sys_id.
const superbaResolveSQL = `
	SELECT DISTINCT ON (UPPER(TRIM(shade_code)))
		UPPER(TRIM(shade_code)), old_value, COALESCE(colour_name, ''), legacy_sys_id
	FROM cost_superba_cost_sp
	WHERE is_active AND deleted_at IS NULL
	  AND UPPER(TRIM(shade_code)) = ANY($1)
	ORDER BY UPPER(TRIM(shade_code)), legacy_sys_id DESC`

const superbaCols = `id::text, legacy_sys_id, shade_code, colour_name, old_value, new_value,
	source, is_active, created_at, created_by, updated_at, updated_by`

// Create persists a hand-authored row and assigns its ID.
func (r *SuperbaCostSpRepository) Create(ctx context.Context, e *superbacostsp.Entry) error {
	var id string
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO cost_superba_cost_sp (legacy_sys_id, shade_code, colour_name, old_value, new_value,
			source, is_active, created_at, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING id::text`,
		e.LegacySysID(), e.ShadeCode(), e.ColourName(), e.OldValue(), e.NewValue(),
		e.Source(), e.IsActive(), e.CreatedAt(), e.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return superbacostsp.ErrDuplicateLegacySysID
		}
		return fmt.Errorf("failed to create superba cost sp: %w", err)
	}
	e.SetID(id)
	return nil
}

// GetByID retrieves a non-deleted row by id.
func (r *SuperbaCostSpRepository) GetByID(ctx context.Context, id string) (*superbacostsp.Entry, error) {
	return r.getOne(ctx, `id = $1::uuid`, id)
}

// GetByLegacySysID retrieves a non-deleted row by legacy sys id.
func (r *SuperbaCostSpRepository) GetByLegacySysID(ctx context.Context, legacySysID int64) (*superbacostsp.Entry, error) {
	return r.getOne(ctx, `legacy_sys_id = $1`, legacySysID)
}

func (r *SuperbaCostSpRepository) getOne(ctx context.Context, cond string, arg interface{}) (*superbacostsp.Entry, error) {
	q := `SELECT ` + superbaCols + `, ` + superbaEffectiveExpr + ` FROM cost_superba_cost_sp t
		WHERE ` + cond + ` AND deleted_at IS NULL`
	e, err := scanSuperba(r.db.QueryRowContext(ctx, q, arg))
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, superbacostsp.ErrNotFound
		}
		// An invalid uuid text is "not found" from the caller's point of view.
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "22P02" {
			return nil, superbacostsp.ErrNotFound
		}
		return nil, fmt.Errorf("failed to get superba cost sp: %w", err)
	}
	return e, nil
}

// superbaEffectiveExpr flags the row shade resolution picks for t's shade.
const superbaEffectiveExpr = `(t.is_active AND t.deleted_at IS NULL AND t.legacy_sys_id = (
		SELECT MAX(x.legacy_sys_id) FROM cost_superba_cost_sp x
		WHERE x.is_active AND x.deleted_at IS NULL
		  AND UPPER(TRIM(x.shade_code)) = UPPER(TRIM(t.shade_code)))) AS effective`

var superbaSortColumns = map[string]string{
	"shade_code":    "shade_code",
	"colour_name":   "colour_name",
	"old_value":     "old_value",
	"new_value":     "new_value",
	"legacy_sys_id": "legacy_sys_id",
	"source":        "source",
	"is_active":     "is_active",
	"created_at":    "created_at",
}

func superbaPredicate(f superbacostsp.ListFilter) (string, []interface{}) {
	base := ` FROM cost_superba_cost_sp t WHERE deleted_at IS NULL`
	var args []interface{}
	if f.Search != "" {
		args = append(args, "%"+f.Search+"%")
		base += fmt.Sprintf(` AND (shade_code ILIKE $%d OR colour_name ILIKE $%d)`, len(args), len(args))
	}
	if f.IsActive != nil {
		args = append(args, *f.IsActive)
		base += fmt.Sprintf(` AND is_active = $%d`, len(args))
	}
	if f.SourceFilter != "" {
		args = append(args, strings.ToUpper(f.SourceFilter))
		base += fmt.Sprintf(` AND source = $%d`, len(args))
	}
	return base, args
}

// List retrieves rows with filtering and pagination.
func (r *SuperbaCostSpRepository) List(ctx context.Context, filter superbacostsp.ListFilter) ([]*superbacostsp.Entry, int64, *time.Time, error) {
	filter.Validate()
	base, args := superbaPredicate(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, nil, fmt.Errorf("failed to count superba cost sps: %w", err)
	}

	orderCol := "shade_code"
	if c, ok := superbaSortColumns[filter.SortBy]; ok {
		orderCol = c
	}
	dir := sortASC
	if strings.EqualFold(filter.SortOrder, "desc") {
		dir = sortDESC
	}
	q := `SELECT ` + superbaCols + `, ` + superbaEffectiveExpr + base +
		fmt.Sprintf(` ORDER BY %s %s, legacy_sys_id DESC LIMIT $%d OFFSET $%d`, orderCol, dir, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, 0, nil, fmt.Errorf("failed to query superba cost sps: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	items := make([]*superbacostsp.Entry, 0)
	for rows.Next() {
		e, scanErr := scanSuperba(rows)
		if scanErr != nil {
			return nil, 0, nil, fmt.Errorf("failed to scan superba cost sp: %w", scanErr)
		}
		items = append(items, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, nil, fmt.Errorf("failed to iterate superba cost sps: %w", err)
	}

	var last sql.NullTime
	if err := r.db.QueryRowContext(ctx,
		`SELECT MAX(COALESCE(updated_at, created_at)) FROM cost_superba_cost_sp WHERE source = 'ORACLE'`,
	).Scan(&last); err != nil {
		return nil, 0, nil, fmt.Errorf("failed to read last sync time: %w", err)
	}
	return items, total, nullTimePtr(last), nil
}

// Update persists changes to an existing row (legacy_sys_id is immutable).
func (r *SuperbaCostSpRepository) Update(ctx context.Context, e *superbacostsp.Entry) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_superba_cost_sp
		SET shade_code = $2, colour_name = $3, old_value = $4, new_value = $5,
			source = $6, is_active = $7, updated_at = $8, updated_by = $9
		WHERE id = $1::uuid AND deleted_at IS NULL`,
		e.ID(), e.ShadeCode(), e.ColourName(), e.OldValue(), e.NewValue(),
		e.Source(), e.IsActive(), e.UpdatedAt(), e.UpdatedBy())
	if err != nil {
		return fmt.Errorf("failed to update superba cost sp: %w", err)
	}
	return requireAffected(res, superbacostsp.ErrNotFound)
}

// SoftDelete marks a row deleted.
func (r *SuperbaCostSpRepository) SoftDelete(ctx context.Context, id, deletedBy string) error {
	res, err := r.db.ExecContext(ctx, `
		UPDATE cost_superba_cost_sp SET deleted_at = NOW(), deleted_by = $2
		WHERE id = $1::uuid AND deleted_at IS NULL`, id, deletedBy)
	if err != nil {
		var pqErr *pq.Error
		if errors.As(err, &pqErr) && pqErr.Code == "22P02" {
			return superbacostsp.ErrNotFound
		}
		return fmt.Errorf("failed to delete superba cost sp: %w", err)
	}
	return requireAffected(res, superbacostsp.ErrNotFound)
}

func requireAffected(res sql.Result, notFound error) error {
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check affected rows: %w", err)
	}
	if n == 0 {
		return notFound
	}
	return nil
}

// UpsertByLegacySysID inserts or overwrites one sourced row (plain UNIQUE
// legacy_sys_id is the ON CONFLICT arbiter). Overwrites MANUAL and soft-deleted
// rows too (approved decision): source -> ORACLE, deleted_at -> NULL. A row that
// already matches the source exactly is left untouched (OutcomeUnchanged).
func (r *SuperbaCostSpRepository) UpsertByLegacySysID(ctx context.Context, src superbacostsp.Sourced) (superbacostsp.UpsertOutcome, error) {
	shade := superbacostsp.NormalizeShade(src.ShadeCode)
	if shade == "" || src.LegacySysID <= 0 {
		return superbacostsp.OutcomeSkipped, nil
	}
	var inserted bool
	err := r.db.QueryRowContext(ctx, `
		INSERT INTO cost_superba_cost_sp AS t (legacy_sys_id, shade_code, colour_name, old_value, new_value, source, created_by)
		VALUES ($1, $2, $3, $4, $5, 'ORACLE', 'system')
		ON CONFLICT (legacy_sys_id) DO UPDATE SET
			shade_code = EXCLUDED.shade_code, colour_name = EXCLUDED.colour_name,
			old_value = EXCLUDED.old_value, new_value = EXCLUDED.new_value,
			source = 'ORACLE', deleted_at = NULL, deleted_by = NULL,
			updated_at = NOW(), updated_by = 'system'
		WHERE (t.shade_code, t.colour_name, t.old_value, t.new_value, t.source, t.deleted_at)
			IS DISTINCT FROM (EXCLUDED.shade_code, EXCLUDED.colour_name, EXCLUDED.old_value, EXCLUDED.new_value, 'ORACLE', NULL::timestamptz)
		RETURNING (xmax = 0)`,
		src.LegacySysID, shade, src.ColourName, src.OldValue, src.NewValue,
	).Scan(&inserted)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return superbacostsp.OutcomeUnchanged, nil
	case err != nil:
		return superbacostsp.OutcomeSkipped, fmt.Errorf("upsert superba cost sp %d: %w", src.LegacySysID, err)
	case inserted:
		return superbacostsp.OutcomeInserted, nil
	default:
		return superbacostsp.OutcomeUpdated, nil
	}
}

// ResolveByShades returns the effective row per normalized shade.
func (r *SuperbaCostSpRepository) ResolveByShades(ctx context.Context, shades []string) (map[string]superbacostsp.Resolved, error) {
	out := make(map[string]superbacostsp.Resolved, len(shades))
	if len(shades) == 0 {
		return out, nil
	}
	norm := make([]string, 0, len(shades))
	for _, s := range shades {
		if n := superbacostsp.NormalizeShade(s); n != "" {
			norm = append(norm, n)
		}
	}
	rows, err := r.db.QueryContext(ctx, superbaResolveSQL, pq.Array(norm))
	if err != nil {
		return nil, fmt.Errorf("failed to resolve superba cost sp: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()
	for rows.Next() {
		var shade string
		var res superbacostsp.Resolved
		if err := rows.Scan(&shade, &res.OldValue, &res.ColourName, &res.LegacySysID); err != nil {
			return nil, fmt.Errorf("failed to scan superba resolution: %w", err)
		}
		out[shade] = res
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate superba resolution: %w", err)
	}
	return out, nil
}

func scanSuperba(row rowScanner) (*superbacostsp.Entry, error) {
	var (
		p          superbacostsp.ReconstructParams
		colour     sql.NullString
		newV       sql.NullFloat64
		updatedAt  sql.NullTime
		updatedBy  sql.NullString
		isEffected bool
	)
	if err := row.Scan(&p.ID, &p.LegacySysID, &p.ShadeCode, &colour, &p.OldValue, &newV,
		&p.Source, &p.IsActive, &p.CreatedAt, &p.CreatedBy, &updatedAt, &updatedBy, &isEffected); err != nil {
		return nil, err
	}
	p.ColourName = nullStringPtr(colour)
	if newV.Valid {
		v := newV.Float64
		p.NewValue = &v
	}
	p.UpdatedAt = nullTimePtr(updatedAt)
	p.UpdatedBy = nullStringPtr(updatedBy)
	p.Effective = isEffected
	return superbacostsp.Reconstruct(p), nil
}
