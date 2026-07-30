// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
)

// CustomerRepository implements customer.Repository using PostgreSQL.
type CustomerRepository struct {
	db *DB
}

// NewCustomerRepository creates a new CustomerRepository.
func NewCustomerRepository(db *DB) *CustomerRepository {
	return &CustomerRepository{db: db}
}

var _ customer.Repository = (*CustomerRepository)(nil)

const customerSelectColumns = `
	customer_id, customer_code, customer_name, customer_short_name, customer_tax_no,
	customer_parent_code, customer_is_active, customer_source, source_created_at,
	source_updated_at, synced_at, created_at, created_by, updated_at, updated_by`

// Create persists a hand-authored customer and assigns its ID.
func (r *CustomerRepository) Create(ctx context.Context, entity *customer.Customer) error {
	query := `
		INSERT INTO customer (
			customer_code, customer_name, customer_short_name, customer_tax_no,
			customer_parent_code, customer_is_active, customer_source, created_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		RETURNING customer_id`

	var id int64
	err := r.db.QueryRowContext(ctx, query,
		entity.Code(), entity.Name(), entity.ShortName(), entity.TaxNo(),
		entity.ParentCode(), entity.IsActive(), entity.Source(),
		entity.CreatedAt(), entity.CreatedBy(),
	).Scan(&id)
	if err != nil {
		if isUniqueViolation(err) {
			return customer.ErrAlreadyExists
		}
		return fmt.Errorf("failed to create customer: %w", err)
	}

	entity.SetID(id)
	return nil
}

// GetByID retrieves a customer by its ID.
func (r *CustomerRepository) GetByID(ctx context.Context, id int64) (*customer.Customer, error) {
	query := `SELECT ` + customerSelectColumns + ` FROM customer WHERE customer_id = $1`
	return r.scanRow(r.db.QueryRowContext(ctx, query, id))
}

// GetByCode retrieves a customer by its code, matched case-insensitively.
func (r *CustomerRepository) GetByCode(ctx context.Context, code string) (*customer.Customer, error) {
	query := `SELECT ` + customerSelectColumns + ` FROM customer WHERE LOWER(customer_code) = LOWER($1)`
	return r.scanRow(r.db.QueryRowContext(ctx, query, customer.NormalizeCode(code)))
}

// customerPredicate builds the shared WHERE clause for List and ListAll so the
// count a planner sees and the rows they export can never diverge.
func customerPredicate(filter customer.ListFilter) (string, []interface{}) {
	base := ` FROM customer WHERE 1=1`
	args := []interface{}{}
	argIdx := 1

	if filter.Search != "" {
		base += fmt.Sprintf(
			` AND (customer_code ILIKE $%d OR customer_name ILIKE $%d OR customer_short_name ILIKE $%d)`,
			argIdx, argIdx, argIdx)
		args = append(args, "%"+filter.Search+"%")
		argIdx++
	}
	if filter.IsActive != nil {
		base += fmt.Sprintf(` AND customer_is_active = $%d`, argIdx)
		args = append(args, *filter.IsActive)
		argIdx++
	}
	if filter.Source != "" {
		base += fmt.Sprintf(` AND customer_source = $%d`, argIdx)
		args = append(args, strings.ToUpper(filter.Source))
	}
	return base, args
}

// customerSortColumns maps API sort keys onto real columns. A key that is not in
// the map silently falls back to the code, never into the SQL string.
var customerSortColumns = map[string]string{
	"code":       "customer_code",
	"name":       "customer_name",
	"short_name": "customer_short_name",
	"is_active":  "customer_is_active",
	"source":     "customer_source",
	"synced_at":  "synced_at",
	"created_at": "created_at",
}

// List retrieves customers with filtering and pagination.
func (r *CustomerRepository) List(ctx context.Context, filter customer.ListFilter) ([]*customer.Customer, int64, error) {
	filter.Validate()
	base, args := customerPredicate(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*)`+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count customers: %w", err)
	}

	orderCol := "customer_code"
	if mapped, ok := customerSortColumns[filter.SortBy]; ok {
		orderCol = mapped
	}

	query := `SELECT ` + customerSelectColumns + base +
		fmt.Sprintf(` ORDER BY %s %s LIMIT $%d OFFSET $%d`,
			orderCol, sortDirection(filter.SortOrder), len(args)+1, len(args)+2)
	args = append(args, filter.PageSize, filter.Offset())

	items, err := r.query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// ListAll retrieves every customer matching a filter, unpaginated, for export.
func (r *CustomerRepository) ListAll(ctx context.Context, filter customer.ListFilter) ([]*customer.Customer, error) {
	base, args := customerPredicate(filter)
	query := `SELECT ` + customerSelectColumns + base + ` ORDER BY customer_code ASC`
	return r.query(ctx, query, args...)
}

// Update persists changes to an existing customer. The code is not mutable.
func (r *CustomerRepository) Update(ctx context.Context, entity *customer.Customer) error {
	query := `
		UPDATE customer
		SET customer_name = $2, customer_short_name = $3, customer_tax_no = $4,
			customer_parent_code = $5, customer_is_active = $6,
			updated_at = $7, updated_by = $8
		WHERE customer_id = $1`

	res, err := r.db.ExecContext(ctx, query,
		entity.ID(), entity.Name(), entity.ShortName(), entity.TaxNo(),
		entity.ParentCode(), entity.IsActive(), entity.UpdatedAt(), entity.UpdatedBy(),
	)
	if err != nil {
		return fmt.Errorf("failed to update customer: %w", err)
	}
	return checkAffected(res, customer.ErrNotFound)
}

// UpsertSourced merges one Oracle-sourced customer, keyed on code. A row a planner
// created by hand (source MANUAL) is never overwritten: the planner's intent
// outranks a code collision with Orion.
func (r *CustomerRepository) UpsertSourced(ctx context.Context, src customer.Sourced) (customer.UpsertOutcome, error) {
	code := customer.NormalizeCode(src.Code)
	if code == "" {
		return customer.OutcomeSkipped, nil
	}

	var (
		existingID int64
		source     string
	)
	err := r.db.QueryRowContext(ctx,
		`SELECT customer_id, customer_source FROM customer WHERE LOWER(customer_code) = LOWER($1)`, code,
	).Scan(&existingID, &source)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		return r.insertSourced(ctx, code, src)
	case err != nil:
		return customer.OutcomeSkipped, fmt.Errorf("lookup customer %q: %w", code, err)
	case source == customer.SourceManual:
		return customer.OutcomeSkipped, nil
	default:
		return r.updateSourced(ctx, existingID, src)
	}
}

func (r *CustomerRepository) insertSourced(ctx context.Context, code string, src customer.Sourced) (customer.UpsertOutcome, error) {
	name := strings.TrimSpace(src.Name)
	if name == "" {
		name = code // customer_name is NOT NULL; a nameless Orion row still needs a label.
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO customer (
			customer_code, customer_name, customer_short_name, customer_tax_no,
			customer_parent_code, customer_is_active, customer_source,
			source_created_at, source_updated_at, synced_at, created_by
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, NOW(), $10)`,
		code, name, src.ShortName, src.TaxNo, src.ParentCode, src.IsActive,
		customer.SourceOracle, src.SourceCreatedAt, src.SourceUpdatedAt, syncActor,
	)
	if err != nil {
		return customer.OutcomeSkipped, fmt.Errorf("insert sourced customer %q: %w", code, err)
	}
	return customer.OutcomeInserted, nil
}

// updateSourced refreshes an Oracle-owned row. It reports OutcomeSkipped when
// nothing actually changed, so a re-run of an unchanged sync reads as "unchanged"
// rather than inflating the updated count. synced_at is stamped either way.
func (r *CustomerRepository) updateSourced(ctx context.Context, id int64, src customer.Sourced) (customer.UpsertOutcome, error) {
	name := strings.TrimSpace(src.Name)
	var changed bool
	err := r.db.QueryRowContext(ctx,
		`UPDATE customer
		 SET customer_name = COALESCE(NULLIF($2, ''), customer_name),
		     customer_short_name = $3,
		     customer_tax_no = $4,
		     customer_parent_code = $5,
		     customer_is_active = $6,
		     source_created_at = COALESCE($7, source_created_at),
		     source_updated_at = COALESCE($8, source_updated_at),
		     synced_at = NOW(),
		     updated_at = NOW(),
		     updated_by = $9
		 WHERE customer_id = $1
		 RETURNING (
		     customer_name IS DISTINCT FROM $2
		     OR customer_short_name IS DISTINCT FROM $3
		     OR customer_tax_no IS DISTINCT FROM $4
		     OR customer_parent_code IS DISTINCT FROM $5
		     OR customer_is_active IS DISTINCT FROM $6
		 )`,
		id, name, src.ShortName, src.TaxNo, src.ParentCode, src.IsActive,
		src.SourceCreatedAt, src.SourceUpdatedAt, syncActor,
	).Scan(&changed)
	if err != nil {
		return customer.OutcomeSkipped, fmt.Errorf("update sourced customer %d: %w", id, err)
	}
	if !changed {
		return customer.OutcomeSkipped, nil
	}
	return customer.OutcomeUpdated, nil
}

// ResolveCodes maps normalized customer codes to ids in one round trip. Unmatched
// codes are absent from the map rather than reported as an error.
func (r *CustomerRepository) ResolveCodes(ctx context.Context, codes []string) (map[string]int64, error) {
	result := make(map[string]int64, len(codes))
	if len(codes) == 0 {
		return result, nil
	}

	normalized := make([]string, 0, len(codes))
	seen := make(map[string]struct{}, len(codes))
	for _, c := range codes {
		n := customer.NormalizeCode(c)
		if n == "" {
			continue
		}
		if _, dup := seen[n]; dup {
			continue
		}
		seen[n] = struct{}{}
		normalized = append(normalized, n)
	}
	if len(normalized) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(normalized))
	args := make([]interface{}, len(normalized))
	for i, n := range normalized {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = n
	}

	query := `SELECT UPPER(customer_code), customer_id FROM customer
		WHERE UPPER(customer_code) IN (` + strings.Join(placeholders, ", ") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve customer codes: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			code string
			id   int64
		)
		if err := rows.Scan(&code, &id); err != nil {
			return nil, fmt.Errorf("failed to scan customer code: %w", err)
		}
		result[code] = id
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating customer code rows: %w", err)
	}
	return result, nil
}

// ResolveIDs maps customer ids to their code and name in one round trip, for
// display decoration. Unmatched ids are absent from the map rather than
// reported as an error — a demand may reference a customer that has since gone.
func (r *CustomerRepository) ResolveIDs(ctx context.Context, ids []int64) (map[int64]customer.Label, error) {
	result := make(map[int64]customer.Label, len(ids))
	distinct := make([]int64, 0, len(ids))
	seen := make(map[int64]struct{}, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		distinct = append(distinct, id)
	}
	if len(distinct) == 0 {
		return result, nil
	}

	placeholders := make([]string, len(distinct))
	args := make([]interface{}, len(distinct))
	for i, id := range distinct {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args[i] = id
	}

	query := `SELECT customer_id, customer_code, customer_name FROM customer
		WHERE customer_id IN (` + strings.Join(placeholders, ", ") + `)`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve customer ids: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			id    int64
			label customer.Label
		)
		if err := rows.Scan(&id, &label.Code, &label.Name); err != nil {
			return nil, fmt.Errorf("failed to scan customer label: %w", err)
		}
		result[id] = label
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating customer label rows: %w", err)
	}
	return result, nil
}

func (r *CustomerRepository) query(ctx context.Context, query string, args ...interface{}) ([]*customer.Customer, error) {
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list customers: %w", err)
	}
	defer closeRows(rows)

	var result []*customer.Customer
	for rows.Next() {
		var dto customerDTO
		if err := dto.scan(rows.Scan); err != nil {
			return nil, fmt.Errorf("failed to scan customer row: %w", err)
		}
		result = append(result, dto.toEntity())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating customer rows: %w", err)
	}
	return result, nil
}

func (r *CustomerRepository) scanRow(row *sql.Row) (*customer.Customer, error) {
	var dto customerDTO
	err := dto.scan(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, customer.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan customer: %w", err)
	}
	return dto.toEntity(), nil
}

type customerDTO struct {
	ID              int64
	Code            string
	Name            string
	ShortName       sql.NullString
	TaxNo           sql.NullString
	ParentCode      sql.NullString
	IsActive        bool
	Source          string
	SourceCreatedAt sql.NullTime
	SourceUpdatedAt sql.NullTime
	SyncedAt        sql.NullTime
	CreatedAt       time.Time
	CreatedBy       string
	UpdatedAt       sql.NullTime
	UpdatedBy       sql.NullString
}

func (d *customerDTO) scan(scan func(dest ...interface{}) error) error {
	return scan(
		&d.ID, &d.Code, &d.Name, &d.ShortName, &d.TaxNo,
		&d.ParentCode, &d.IsActive, &d.Source, &d.SourceCreatedAt,
		&d.SourceUpdatedAt, &d.SyncedAt, &d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
}

func (d *customerDTO) toEntity() *customer.Customer {
	return customer.Reconstruct(customer.ReconstructParams{
		ID:              d.ID,
		Code:            d.Code,
		Name:            d.Name,
		ShortName:       nullStringPtr(d.ShortName),
		TaxNo:           nullStringPtr(d.TaxNo),
		ParentCode:      nullStringPtr(d.ParentCode),
		IsActive:        d.IsActive,
		Source:          d.Source,
		SourceCreatedAt: nullTimePtr(d.SourceCreatedAt),
		SourceUpdatedAt: nullTimePtr(d.SourceUpdatedAt),
		SyncedAt:        nullTimePtr(d.SyncedAt),
		CreatedAt:       d.CreatedAt,
		CreatedBy:       d.CreatedBy,
		UpdatedAt:       nullTimePtr(d.UpdatedAt),
		UpdatedBy:       nullStringPtr(d.UpdatedBy),
	})
}
