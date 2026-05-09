// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/prdrequest"
)

// validPeriod matches the YYYYMM pattern required for ticket number allocation.
var validPeriod = regexp.MustCompile(`^\d{6}$`)

// prdRequestSortColumns whitelists frontend field names → DB column names.
var prdRequestSortColumns = map[string]string{
	"ticketNo":  "ticket_no",
	"title":     "title",
	"status":    "status",
	"dueDate":   "due_date",
	"createdAt": "created_at",
	"updatedAt": "updated_at",
}

// prdRequestRepository implements prdrequest.Repository using PostgreSQL.
type prdRequestRepository struct {
	db *DB
}

// Verify interface implementations at compile time.
var _ prdrequest.Repository = (*prdRequestRepository)(nil)

// NewPrdRequestRepository constructs a prdRequestRepository.
func NewPrdRequestRepository(db *DB) prdrequest.Repository {
	return &prdRequestRepository{db: db}
}

// prdRequestTicketNoGenerator implements prdrequest.TicketNoGenerator backed by prd_request_sequence.
type prdRequestTicketNoGenerator struct {
	db *DB
}

// Verify interface implementation at compile time.
var _ prdrequest.TicketNoGenerator = (*prdRequestTicketNoGenerator)(nil)

// NewPrdRequestTicketNoGenerator constructs a generator backed by prd_request_sequence.
func NewPrdRequestTicketNoGenerator(db *DB) prdrequest.TicketNoGenerator {
	return &prdRequestTicketNoGenerator{db: db}
}

// Next allocates the next ticket number for the given period atomically.
// Uses INSERT ... ON CONFLICT DO UPDATE to increment the per-period counter
// in a single round-trip; safe under concurrent callers.
func (g *prdRequestTicketNoGenerator) Next(ctx context.Context, period string) (prdrequest.TicketNo, error) {
	if !validPeriod.MatchString(period) {
		return prdrequest.TicketNo{}, prdrequest.ErrInvalidPeriod
	}

	const q = `
		INSERT INTO prd_request_sequence (period, last_seq, updated_at)
		VALUES ($1, 1, NOW())
		ON CONFLICT (period)
		DO UPDATE SET last_seq = prd_request_sequence.last_seq + 1, updated_at = NOW()
		RETURNING last_seq
	`

	var seq int
	if err := g.db.QueryRowContext(ctx, q, period).Scan(&seq); err != nil {
		return prdrequest.TicketNo{}, fmt.Errorf("allocate ticket sequence: %w", err)
	}

	return prdrequest.FormatTicketNo(period, seq)
}

// Create persists a new Request to the database.
func (r *prdRequestRepository) Create(ctx context.Context, req *prdrequest.Request) error {
	var targetSpecsArg sql.NullString
	if !req.TargetSpecs().IsEmpty() {
		targetSpecsArg = sql.NullString{String: req.TargetSpecs().String(), Valid: true}
	}

	query := `
		INSERT INTO prd_request (
			request_id, ticket_no,
			requester_id, requester_username, requester_dept_id, requester_dept_code,
			title, description, target_specs,
			status,
			resolved_product_id, resolution_note, reject_reason,
			assigned_to, due_date,
			created_at, created_by,
			updated_at, updated_by,
			deleted_at, deleted_by
		) VALUES (
			$1, $2,
			$3, $4, $5, $6,
			$7, $8, $9,
			$10,
			$11, $12, $13,
			$14, $15,
			$16, $17,
			$18, $19,
			$20, $21
		)
	`

	_, err := r.db.ExecContext(ctx, query,
		req.ID(),
		req.TicketNo().String(),
		req.RequesterID(),
		req.RequesterUsername(),
		req.RequesterDeptID(),
		nullableStringVal(req.RequesterDeptCode()),
		req.Title().String(),
		nullableStringVal(req.Description().String()),
		targetSpecsArg,
		req.Status().String(),
		nullableUUIDVal(req.ResolvedProductID()),
		nullableStringVal(req.ResolutionNote().String()),
		nullableStringVal(req.RejectReason().String()),
		nullableUUIDVal(req.AssignedTo()),
		req.DueDate(),
		req.CreatedAt(),
		req.CreatedBy(),
		req.UpdatedAt(),
		nullableStringVal(req.UpdatedBy()),
		req.DeletedAt(),
		nullableStringVal(req.DeletedBy()),
	)
	if err != nil {
		if isUniqueViolation(err) {
			return fmt.Errorf("ticket_no already exists: %w", prdrequest.ErrAlreadyResolved)
		}
		return fmt.Errorf("failed to create product request: %w", err)
	}

	return nil
}

// GetByID retrieves a non-deleted Request by its UUID.
func (r *prdRequestRepository) GetByID(ctx context.Context, id uuid.UUID) (*prdrequest.Request, error) {
	query := prdRequestSelectCols() + `
		FROM prd_request
		WHERE request_id = $1 AND deleted_at IS NULL
	`
	return scanPrdRequest(r.db.QueryRowContext(ctx, query, id))
}

// GetByTicketNo retrieves a non-deleted Request by its ticket number string.
func (r *prdRequestRepository) GetByTicketNo(ctx context.Context, ticketNo string) (*prdrequest.Request, error) {
	query := prdRequestSelectCols() + `
		FROM prd_request
		WHERE ticket_no = $1 AND deleted_at IS NULL
	`
	return scanPrdRequest(r.db.QueryRowContext(ctx, query, ticketNo))
}

// List retrieves Requests matching the filter with pagination.
// Returns the matching items, the total count across all pages, and any error.
func (r *prdRequestRepository) List(ctx context.Context, f prdrequest.ListFilter) ([]*prdrequest.Request, int, error) {
	f = normalizePrdRequestFilter(f)

	orderCol, err := resolvePrdRequestSortColumn(f.SortField)
	if err != nil {
		return nil, 0, err
	}

	orderDir := sortASC
	if f.SortDesc {
		orderDir = sortDESC
	}

	baseQuery, args, argIndex := buildPrdRequestWhereClause(f)

	var total int
	countQuery := "SELECT COUNT(*) FROM prd_request " + baseQuery
	if err := r.db.QueryRowContext(ctx, countQuery, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("failed to count product requests: %w", err)
	}

	selectQuery := prdRequestSelectCols() + " FROM prd_request " + baseQuery +
		fmt.Sprintf(" ORDER BY %s %s LIMIT $%d OFFSET $%d",
			orderCol, orderDir, argIndex, argIndex+1)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := r.db.QueryContext(ctx, selectQuery, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("failed to list product requests: %w", err)
	}
	defer closeRows(rows)

	var items []*prdrequest.Request
	for rows.Next() {
		req, scanErr := scanPrdRequestFromRows(rows)
		if scanErr != nil {
			return nil, 0, scanErr
		}
		items = append(items, req)
	}

	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("error iterating product request rows: %w", err)
	}

	return items, total, nil
}

// Update persists mutations to an existing Request.
func (r *prdRequestRepository) Update(ctx context.Context, req *prdrequest.Request) error {
	var targetSpecsArg sql.NullString
	if !req.TargetSpecs().IsEmpty() {
		targetSpecsArg = sql.NullString{String: req.TargetSpecs().String(), Valid: true}
	}

	query := `
		UPDATE prd_request SET
			title               = $2,
			description         = $3,
			target_specs        = $4,
			status              = $5,
			resolved_product_id = $6,
			resolution_note     = $7,
			reject_reason       = $8,
			assigned_to         = $9,
			due_date            = $10,
			updated_at          = $11,
			updated_by          = $12,
			deleted_at          = $13,
			deleted_by          = $14
		WHERE request_id = $1
	`

	result, err := r.db.ExecContext(ctx, query,
		req.ID(),
		req.Title().String(),
		nullableStringVal(req.Description().String()),
		targetSpecsArg,
		req.Status().String(),
		nullableUUIDVal(req.ResolvedProductID()),
		nullableStringVal(req.ResolutionNote().String()),
		nullableStringVal(req.RejectReason().String()),
		nullableUUIDVal(req.AssignedTo()),
		req.DueDate(),
		req.UpdatedAt(),
		nullableStringVal(req.UpdatedBy()),
		req.DeletedAt(),
		nullableStringVal(req.DeletedBy()),
	)
	if err != nil {
		return fmt.Errorf("failed to update product request: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return prdrequest.ErrNotFound
	}

	return nil
}

// Delete soft-deletes a Request by its UUID.
func (r *prdRequestRepository) Delete(ctx context.Context, id uuid.UUID, deletedBy string) error {
	query := `
		UPDATE prd_request SET
			deleted_at = $2,
			deleted_by = $3
		WHERE request_id = $1 AND deleted_at IS NULL
	`

	result, err := r.db.ExecContext(ctx, query, id, time.Now().UTC(), deletedBy)
	if err != nil {
		return fmt.Errorf("failed to delete product request: %w", err)
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rowsAffected == 0 {
		return prdrequest.ErrNotFound
	}

	return nil
}

// =============================================================================
// SQL construction helpers
// =============================================================================

// prdRequestSelectCols returns the SELECT column list for prd_request.
func prdRequestSelectCols() string {
	return `
		SELECT
			request_id, ticket_no,
			requester_id, requester_username, requester_dept_id, requester_dept_code,
			title, description, target_specs,
			status,
			resolved_product_id, resolution_note, reject_reason,
			assigned_to, due_date,
			created_at, created_by,
			updated_at, updated_by,
			deleted_at, deleted_by
	`
}

// buildPrdRequestWhereClause builds the WHERE clause and args for List queries.
// Returns the clause string (starting with WHERE), args slice, and the next arg index.
func buildPrdRequestWhereClause(f prdrequest.ListFilter) (string, []interface{}, int) {
	clause := "WHERE deleted_at IS NULL"
	args := []interface{}{}
	idx := 1

	if f.Search != "" {
		clause += fmt.Sprintf(` AND to_tsvector('simple',
			coalesce(title,'') || ' ' || coalesce(description,'')
		) @@ plainto_tsquery('simple', $%d)`, idx)
		args = append(args, f.Search)
		idx++
	}

	if f.Status != "" {
		clause += fmt.Sprintf(" AND status = $%d", idx)
		args = append(args, f.Status)
		idx++
	}

	if f.RequesterID != nil {
		clause += fmt.Sprintf(" AND requester_id = $%d", idx)
		args = append(args, *f.RequesterID)
		idx++
	}

	if f.AssignedTo != nil {
		clause += fmt.Sprintf(" AND assigned_to = $%d", idx)
		args = append(args, *f.AssignedTo)
		idx++
	}

	if f.RequesterDeptID != nil {
		clause += fmt.Sprintf(" AND requester_dept_id = $%d", idx)
		args = append(args, *f.RequesterDeptID)
		idx++
	}

	return clause, args, idx
}

// resolvePrdRequestSortColumn maps the frontend sort field to a DB column.
// Returns the default "created_at" if the field is empty.
func resolvePrdRequestSortColumn(field string) (string, error) {
	if field == "" {
		return "created_at", nil
	}
	col, ok := prdRequestSortColumns[field]
	if !ok {
		return "", fmt.Errorf("invalid sort field %q", field)
	}
	return col, nil
}

// normalizePrdRequestFilter applies defaults to a ListFilter.
func normalizePrdRequestFilter(f prdrequest.ListFilter) prdrequest.ListFilter {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 20
	}
	return f
}

// =============================================================================
// Scan helpers
// =============================================================================

// prdRequestDTO is a data transfer object for prd_request rows.
type prdRequestDTO struct {
	ID                uuid.UUID
	TicketNo          string
	RequesterID       uuid.UUID
	RequesterUsername string
	RequesterDeptID   uuid.UUID
	RequesterDeptCode sql.NullString
	Title             string
	Description       sql.NullString
	TargetSpecs       sql.NullString
	Status            string
	ResolvedProductID *uuid.UUID
	ResolutionNote    sql.NullString
	RejectReason      sql.NullString
	AssignedTo        *uuid.UUID
	DueDate           sql.NullTime
	CreatedAt         time.Time
	CreatedBy         string
	UpdatedAt         sql.NullTime
	UpdatedBy         sql.NullString
	DeletedAt         sql.NullTime
	DeletedBy         sql.NullString
}

// scanPrdRequestDTO scans the standard column set of prd_request into a prdRequestDTO.
func scanPrdRequestDTO(scanner interface {
	Scan(dest ...interface{}) error
}) (*prdRequestDTO, error) {
	var dto prdRequestDTO
	err := scanner.Scan(
		&dto.ID,
		&dto.TicketNo,
		&dto.RequesterID,
		&dto.RequesterUsername,
		&dto.RequesterDeptID,
		&dto.RequesterDeptCode,
		&dto.Title,
		&dto.Description,
		&dto.TargetSpecs,
		&dto.Status,
		&dto.ResolvedProductID,
		&dto.ResolutionNote,
		&dto.RejectReason,
		&dto.AssignedTo,
		&dto.DueDate,
		&dto.CreatedAt,
		&dto.CreatedBy,
		&dto.UpdatedAt,
		&dto.UpdatedBy,
		&dto.DeletedAt,
		&dto.DeletedBy,
	)
	return &dto, err
}

// scanPrdRequest scans a single *sql.Row into a Request entity.
func scanPrdRequest(row *sql.Row) (*prdrequest.Request, error) {
	dto, err := scanPrdRequestDTO(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, prdrequest.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("failed to scan product request: %w", err)
	}
	return dtoToPrdRequest(dto)
}

// scanPrdRequestFromRows scans a *sql.Rows row into a Request entity.
func scanPrdRequestFromRows(rows *sql.Rows) (*prdrequest.Request, error) {
	dto, err := scanPrdRequestDTO(rows)
	if err != nil {
		return nil, fmt.Errorf("failed to scan product request row: %w", err)
	}
	return dtoToPrdRequest(dto)
}

// dtoToPrdRequest converts a prdRequestDTO to a domain Request.
func dtoToPrdRequest(dto *prdRequestDTO) (*prdrequest.Request, error) {
	deptCode := ""
	if dto.RequesterDeptCode.Valid {
		deptCode = dto.RequesterDeptCode.String
	}

	description := ""
	if dto.Description.Valid {
		description = dto.Description.String
	}

	targetSpecs := ""
	if dto.TargetSpecs.Valid {
		targetSpecs = dto.TargetSpecs.String
	}

	resolvedProductID := uuid.Nil
	if dto.ResolvedProductID != nil {
		resolvedProductID = *dto.ResolvedProductID
	}

	resolutionNote := ""
	if dto.ResolutionNote.Valid {
		resolutionNote = dto.ResolutionNote.String
	}

	rejectReason := ""
	if dto.RejectReason.Valid {
		rejectReason = dto.RejectReason.String
	}

	assignedTo := uuid.Nil
	if dto.AssignedTo != nil {
		assignedTo = *dto.AssignedTo
	}

	var dueDate *time.Time
	if dto.DueDate.Valid {
		dueDate = &dto.DueDate.Time
	}

	var updatedAt *time.Time
	if dto.UpdatedAt.Valid {
		updatedAt = &dto.UpdatedAt.Time
	}

	updatedBy := ""
	if dto.UpdatedBy.Valid {
		updatedBy = dto.UpdatedBy.String
	}

	var deletedAt *time.Time
	if dto.DeletedAt.Valid {
		deletedAt = &dto.DeletedAt.Time
	}

	deletedBy := ""
	if dto.DeletedBy.Valid {
		deletedBy = dto.DeletedBy.String
	}

	return prdrequest.ReconstructRequest(
		dto.ID,
		dto.TicketNo,
		dto.RequesterID,
		dto.RequesterUsername,
		dto.RequesterDeptID,
		deptCode,
		dto.Title,
		description,
		targetSpecs,
		dto.Status,
		resolvedProductID,
		resolutionNote,
		rejectReason,
		assignedTo,
		dueDate,
		dto.CreatedAt,
		dto.CreatedBy,
		updatedAt,
		updatedBy,
		deletedAt,
		deletedBy,
	), nil
}
