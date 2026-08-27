// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// ShadeRepository implements shade.Repository using PostgreSQL, backed by the
// cost_erp_shade table (R8: activated from its 000105 shell by migration 000493).
type ShadeRepository struct {
	db *DB
}

// NewShadeRepository creates a new ShadeRepository.
func NewShadeRepository(db *DB) *ShadeRepository {
	return &ShadeRepository{db: db}
}

var _ shade.Repository = (*ShadeRepository)(nil)

const shadeSelectColumns = `
	ces_shade_id, ces_shade_code, ces_shade_name, ces_shade_short_name,
	ces_is_active, ces_shade_source, ces_source_created_at, ces_source_updated_at,
	ces_source_created_by, ces_source_updated_by, ces_synced_at,
	created_at, created_by, updated_at, updated_by`

// Create persists a hand-authored shade and assigns its ID.
func (r *ShadeRepository) Create(ctx context.Context, entity *shade.Shade) error {
	query := `
		INSERT INTO cost_erp_shade (
			ces_shade_code, ces_shade_name, ces_shade_short_name,
			ces_is_active, ces_shade_source, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING ces_shade_id`

	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Code(), entity.Name(), entity.ShortName(),
		entity.IsActive(), entity.Source(), entity.CreatedAt(), entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return shade.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create shade: %w", err)
	}

	entity.SetID(id)
	return nil
}

// GetByID retrieves a shade by its ID.
func (r *ShadeRepository) GetByID(ctx context.Context, id int64) (*shade.Shade, error) {
	query := `SELECT ` + shadeSelectColumns + ` FROM cost_erp_shade WHERE ces_shade_id = $1`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a shade by its code, matched case-insensitively.
func (r *ShadeRepository) GetByCode(ctx context.Context, code string) (*shade.Shade, error) {
	query := `SELECT ` + shadeSelectColumns + ` FROM cost_erp_shade WHERE LOWER(ces_shade_code) = LOWER($1)`
	return r.scanRow(r.db.QueryRowContext(ctx, query, shade.NormalizeCode(code)))
}

// shadePredicate builds the shared WHERE clause for List.
func shadePredicate(filter shade.ListFilter) (string, []interface{}) {
	base := ` FROM cost_erp_shade WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(
			` AND (ces_shade_code ILIKE $%d OR ces_shade_name ILIKE $%d OR ces_shade_short_name ILIKE $%d)`,
			argIdx, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND ces_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}
	if filter.SourceFilter != "" {
		base += fmt.Sprintf(` AND ces_shade_source = $%d`, argIdx)
		args = append(args, strings.ToUpper(filter.SourceFilter))
	}
	return base, args
}

// shadeSortColumns maps API sort keys onto real columns. A key that is not in
// the map silently falls back to the code, never into the SQL string.
var shadeSortColumns = map[string]string{
	"code":       "ces_shade_code",
	"name":       "ces_shade_name",
	"short_name": "ces_shade_short_name",
	"is_active":  "ces_is_active",
	"source":     "ces_shade_source",
	"synced_at":  "ces_synced_at",
	"created_at": "created_at",
}

// List retrieves shades with filtering and pagination.
func (r *ShadeRepository) List(ctx context.Context, filter shade.ListFilter) ([]*shade.Shade, int64, error) {
	filter.Validate()
	base, args := shadePredicate(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count shades: %w", err)
	}

	orderCol := "ces_shade_code"
	if mapped, ok := shadeSortColumns[filter.SortBy]; ok {
		orderCol = mapped
	}
	orderDir := sortASC
	if strings.EqualFold(filter.SortOrder, "desc") {
		orderDir = sortDESC
	}

	query := `SELECT ` + shadeSelectColumns + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
			orderCol, orderDir, len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, filter.Offset())

	items, err := r.query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Update persists changes to an existing shade. The code is not mutable.
func (r *ShadeRepository) Update(ctx context.Context, entity *shade.Shade) error {
	query := `
		UPDATE cost_erp_shade
		SET ces_shade_name = $2, ces_shade_short_name = $3, ces_is_active = $4,
			updated_at = $5, updated_by = $6
		WHERE ces_shade_id = $1`

	res, err := r.db.ExecContext(ctx, query,
		entity.ID(), entity.Name(), entity.ShortName(), entity.IsActive(),
		entity.UpdatedAt(), entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update shade: %w", err)
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to check update result: %w", err)
	}
	if rows == 0 {
		return shade.ErrNotFound
	}
	return nil
}

// UpsertSourced merges one Oracle-sourced shade, keyed on code. A row a finance
// user created by hand (source MANUAL) is never overwritten: the user's intent
// outranks a code collision with Orion. This is the safety net R8 requires.
func (r *ShadeRepository) UpsertSourced(ctx context.Context, src shade.Sourced) (shade.UpsertOutcome, error) {
	code := shade.NormalizeCode(src.Code)
	if code == "" {
		return shade.OutcomeSkipped, nil
	}

	var (
		existingID int64
		source     string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT ces_shade_id, ces_shade_source FROM cost_erp_shade WHERE LOWER(ces_shade_code) = LOWER($1)`, code,
	).Scan(&existingID, &source)

	switch decideUpsertAction(err, source) {
	case upsertActionInsert:
		return r.insertSourced(ctx, code, src)
	case upsertActionLookupFailed:
		return shade.OutcomeSkipped, fmt.Errorf("lookup shade %q: %w", code, err)
	case upsertActionSkipManual:
		// The finance user's manual edit outranks Orion — never overwritten by sync.
		return shade.OutcomeSkipped, nil
	default: // upsertActionUpdate
		return r.updateSourced(ctx, existingID, src)
	}
}

// upsertAction is the decision UpsertSourced makes after looking up the
// existing row's provenance. It is a pure function of (lookup error, source)
// so the MANUAL-guard logic can be unit tested without a database.
type upsertAction int

const (
	upsertActionInsert upsertAction = iota
	upsertActionUpdate
	upsertActionSkipManual
	upsertActionLookupFailed
)

// decideUpsertAction is the pure decision core of UpsertSourced: a row with no
// existing match is inserted, a lookup error is surfaced, a MANUAL-provenance
// row is left untouched (never overwritten by sync), and anything else
// (ORACLE-provenance row) is refreshed.
func decideUpsertAction(lookupErr error, existingSource string) upsertAction {
	switch {
	case errors.Is(lookupErr, sql.ErrNoRows):
		return upsertActionInsert
	case lookupErr != nil:
		return upsertActionLookupFailed
	case existingSource == shade.SourceManual:
		return upsertActionSkipManual
	default:
		return upsertActionUpdate
	}
}

func (r *ShadeRepository) insertSourced(ctx context.Context, code string, src shade.Sourced) (shade.UpsertOutcome, error) {
	name := strings.TrimSpace(src.Name)
	if name == "" {
		name = code // ces_shade_name has no NOT NULL constraint, but an empty label helps nobody.
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO cost_erp_shade (
			ces_shade_code, ces_shade_name, ces_shade_short_name, ces_is_active, ces_shade_source,
			ces_source_created_at, ces_source_updated_at, ces_source_created_by, ces_source_updated_by,
			ces_synced_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), $10)`,
		code, name, src.ShortName, src.IsActive, shade.SourceOracle,
		src.SourceCreatedAt, src.SourceUpdatedAt, src.SourceCreatedBy, src.SourceUpdatedBy,
		"system",
	)
	if err != nil {
		return shade.OutcomeSkipped, fmt.Errorf("insert shade %q: %w", code, err)
	}
	return shade.OutcomeInserted, nil
}

func (r *ShadeRepository) updateSourced(ctx context.Context, id int64, src shade.Sourced) (shade.UpsertOutcome, error) {
	name := strings.TrimSpace(src.Name)
	if name == "" {
		name = src.Code
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE cost_erp_shade SET
			ces_shade_name = $2, ces_shade_short_name = $3, ces_is_active = $4,
			ces_source_created_at = $5, ces_source_updated_at = $6,
			ces_source_created_by = $7, ces_source_updated_by = $8,
			ces_synced_at = NOW(), updated_at = NOW(), updated_by = $9
		WHERE ces_shade_id = $1`,
		id, name, src.ShortName, src.IsActive,
		src.SourceCreatedAt, src.SourceUpdatedAt, src.SourceCreatedBy, src.SourceUpdatedBy,
		"system",
	)
	if err != nil {
		return shade.OutcomeSkipped, fmt.Errorf("update shade id %d: %w", id, err)
	}
	return shade.OutcomeUpdated, nil
}

func (r *ShadeRepository) scanRow(row *sql.Row) (*shade.Shade, error) {
	p, err := scanShadeRow(row)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, shade.ErrNotFound
		}
		return nil, fmt.Errorf("failed to scan shade: %w", err)
	}
	return shade.Reconstruct(*p), nil
}

func (r *ShadeRepository) query(ctx context.Context, query string, args ...interface{}) ([]*shade.Shade, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to query shades: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			_ = closeErr
		}
	}()

	items := make([]*shade.Shade, 0)
	for rows.Next() {
		p, scanErr := scanShadeRow(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("failed to scan shade row: %w", scanErr)
		}
		items = append(items, shade.Reconstruct(*p))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate shade rows: %w", err)
	}
	return items, nil
}

// scanShadeRow reads one cost_erp_shade row. rowScanner (satisfied by both
// *sql.Row and *sql.Rows) is defined once in cost_calc_job_repository.go.
func scanShadeRow(row rowScanner) (*shade.ReconstructParams, error) {
	var (
		id                                                     int64
		code, name                                             string
		shortName, sourceCreatedBy, sourceUpdatedBy, updatedBy sql.NullString
		isActive                                               bool
		source                                                 string
		sourceCreatedAt, sourceUpdatedAt, syncedAt, updatedAt  sql.NullTime
		createdAt                                              time.Time
		createdBy                                              string
	)

	err := row.Scan(
		&id, &code, &name, &shortName,
		&isActive, &source, &sourceCreatedAt, &sourceUpdatedAt,
		&sourceCreatedBy, &sourceUpdatedBy, &syncedAt,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	)
	if err != nil {
		return nil, err
	}

	return &shade.ReconstructParams{
		ID: id, Code: code, Name: name,
		ShortName:       nullStringPtr(shortName),
		IsActive:        isActive,
		Source:          source,
		SourceCreatedAt: nullTimePtr(sourceCreatedAt),
		SourceUpdatedAt: nullTimePtr(sourceUpdatedAt),
		SourceCreatedBy: nullStringPtr(sourceCreatedBy),
		SourceUpdatedBy: nullStringPtr(sourceUpdatedBy),
		SyncedAt:        nullTimePtr(syncedAt),
		CreatedAt:       createdAt,
		CreatedBy:       createdBy,
		UpdatedAt:       nullTimePtr(updatedAt),
		UpdatedBy:       nullStringPtr(updatedBy),
	}, nil
}
