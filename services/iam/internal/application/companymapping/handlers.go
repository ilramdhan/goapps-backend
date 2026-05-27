// Package companymapping provides application layer handlers for Company
// Mapping operations (CRUD only — no Excel import/export).
package companymapping

import (
	"context"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/iam/internal/domain/companymapping"
	"github.com/mutugading/goapps-backend/services/iam/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/iam/pkg/safeconv"
)

// =============================================================================
// CREATE
// =============================================================================

// CreateCommand creates a new company mapping.
type CreateCommand struct {
	Code         string
	Name         string
	CompanyID    string
	DivisionID   string
	DepartmentID string
	SectionID    *string
	CreatedBy    string
}

// CreateHandler handles CreateCompanyMapping commands.
type CreateHandler struct {
	repo companymapping.Repository
}

// NewCreateHandler returns a CreateHandler.
func NewCreateHandler(repo companymapping.Repository) *CreateHandler {
	return &CreateHandler{repo: repo}
}

// Handle executes the command.
func (h *CreateHandler) Handle(ctx context.Context, cmd CreateCommand) (*companymapping.CompanyMapping, error) {
	code, err := companymapping.NewCode(cmd.Code)
	if err != nil {
		return nil, err
	}
	name, err := companymapping.NewName(cmd.Name)
	if err != nil {
		return nil, err
	}
	hierarchy, err := buildHierarchy(cmd.CompanyID, cmd.DivisionID, cmd.DepartmentID, cmd.SectionID)
	if err != nil {
		return nil, err
	}

	exists, err := h.repo.ExistsByCode(ctx, code.String())
	if err != nil {
		return nil, err
	}
	if exists {
		return nil, shared.ErrAlreadyExists
	}

	entity, err := companymapping.NewCompanyMapping(code, name, hierarchy, cmd.CreatedBy)
	if err != nil {
		return nil, err
	}
	if err := h.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	// Re-read to populate denormalized hierarchy code/name fields.
	return h.repo.GetByID(ctx, entity.ID())
}

func buildHierarchy(companyID, divisionID, departmentID string, sectionID *string) (companymapping.Hierarchy, error) {
	cID, err := uuid.Parse(companyID)
	if err != nil {
		return companymapping.Hierarchy{}, companymapping.ErrInvalidCompanyID
	}
	dID, err := uuid.Parse(divisionID)
	if err != nil {
		return companymapping.Hierarchy{}, companymapping.ErrInvalidDivisionID
	}
	dpID, err := uuid.Parse(departmentID)
	if err != nil {
		return companymapping.Hierarchy{}, companymapping.ErrInvalidDepartmentID
	}
	h := companymapping.Hierarchy{
		CompanyID:    cID,
		DivisionID:   dID,
		DepartmentID: dpID,
	}
	if sectionID != nil && *sectionID != "" {
		sID, sErr := uuid.Parse(*sectionID)
		if sErr != nil {
			return companymapping.Hierarchy{}, sErr
		}
		h.SectionID = &sID
	}
	return h, nil
}

// =============================================================================
// GET
// =============================================================================

// GetQuery fetches a mapping by ID.
type GetQuery struct {
	CompanyMappingID string
}

// GetHandler handles GetCompanyMapping queries.
type GetHandler struct {
	repo companymapping.Repository
}

// NewGetHandler returns a GetHandler.
func NewGetHandler(repo companymapping.Repository) *GetHandler {
	return &GetHandler{repo: repo}
}

// Handle executes the query.
func (h *GetHandler) Handle(ctx context.Context, q GetQuery) (*companymapping.CompanyMapping, error) {
	id, err := uuid.Parse(q.CompanyMappingID)
	if err != nil {
		return nil, shared.ErrNotFound
	}
	return h.repo.GetByID(ctx, id)
}

// =============================================================================
// UPDATE
// =============================================================================

// UpdateCommand updates a company mapping.
type UpdateCommand struct {
	CompanyMappingID string
	Name             *string
	CompanyID        *string
	DivisionID       *string
	DepartmentID     *string
	SectionID        *string
	ClearSection     bool
	IsActive         *bool
	UpdatedBy        string
}

// UpdateHandler handles UpdateCompanyMapping commands.
type UpdateHandler struct {
	repo companymapping.Repository
}

// NewUpdateHandler returns an UpdateHandler.
func NewUpdateHandler(repo companymapping.Repository) *UpdateHandler {
	return &UpdateHandler{repo: repo}
}

// Handle executes the command.
func (h *UpdateHandler) Handle(ctx context.Context, cmd UpdateCommand) (*companymapping.CompanyMapping, error) {
	id, err := uuid.Parse(cmd.CompanyMappingID)
	if err != nil {
		return nil, shared.ErrNotFound
	}
	entity, err := h.repo.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var namePtr *companymapping.Name
	if cmd.Name != nil {
		n, nErr := companymapping.NewName(*cmd.Name)
		if nErr != nil {
			return nil, nErr
		}
		namePtr = &n
	}
	companyID, err := parseOptionalUUIDStr(cmd.CompanyID)
	if err != nil {
		return nil, err
	}
	divisionID, err := parseOptionalUUIDStr(cmd.DivisionID)
	if err != nil {
		return nil, err
	}
	departmentID, err := parseOptionalUUIDStr(cmd.DepartmentID)
	if err != nil {
		return nil, err
	}
	sectionID, err := parseOptionalUUIDStr(cmd.SectionID)
	if err != nil {
		return nil, err
	}

	if err := entity.Update(namePtr, companyID, divisionID, departmentID, sectionID, cmd.ClearSection, cmd.IsActive, cmd.UpdatedBy); err != nil {
		return nil, err
	}
	if err := h.repo.Update(ctx, entity); err != nil {
		return nil, err
	}
	return h.repo.GetByID(ctx, entity.ID())
}

func parseOptionalUUIDStr(s *string) (*uuid.UUID, error) {
	if s == nil || *s == "" {
		return nil, nil //nolint:nilnil // intentional: nil means "not set"
	}
	id, err := uuid.Parse(*s)
	if err != nil {
		return nil, err
	}
	return &id, nil
}

// =============================================================================
// DELETE
// =============================================================================

// DeleteCommand soft-deletes a company mapping.
type DeleteCommand struct {
	CompanyMappingID string
	DeletedBy        string
}

// DeleteHandler handles DeleteCompanyMapping commands.
type DeleteHandler struct {
	repo companymapping.Repository
}

// NewDeleteHandler returns a DeleteHandler.
func NewDeleteHandler(repo companymapping.Repository) *DeleteHandler {
	return &DeleteHandler{repo: repo}
}

// Handle executes the command.
func (h *DeleteHandler) Handle(ctx context.Context, cmd DeleteCommand) error {
	id, err := uuid.Parse(cmd.CompanyMappingID)
	if err != nil {
		return shared.ErrNotFound
	}
	return h.repo.Delete(ctx, id, cmd.DeletedBy)
}

// =============================================================================
// LIST
// =============================================================================

// ListQuery is the query for listing company mappings.
type ListQuery struct {
	Page         int
	PageSize     int
	Search       string
	CompanyID    *uuid.UUID
	DivisionID   *uuid.UUID
	DepartmentID *uuid.UUID
	SectionID    *uuid.UUID
	IsActive     *bool
	SortBy       string
	SortOrder    string
}

// ListResult is the result of listing company mappings.
type ListResult struct {
	Items       []*companymapping.CompanyMapping
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListHandler handles ListCompanyMappings queries.
type ListHandler struct {
	repo companymapping.Repository
}

// NewListHandler returns a ListHandler.
func NewListHandler(repo companymapping.Repository) *ListHandler {
	return &ListHandler{repo: repo}
}

// Handle executes the query.
func (h *ListHandler) Handle(ctx context.Context, q ListQuery) (*ListResult, error) {
	page := q.Page
	if page < 1 {
		page = 1
	}
	pageSize := q.PageSize
	if pageSize < 1 {
		pageSize = 10
	}
	if pageSize > 100 {
		pageSize = 100
	}

	params := companymapping.ListParams{
		Page:         page,
		PageSize:     pageSize,
		Search:       q.Search,
		CompanyID:    q.CompanyID,
		DivisionID:   q.DivisionID,
		DepartmentID: q.DepartmentID,
		SectionID:    q.SectionID,
		IsActive:     q.IsActive,
		SortBy:       q.SortBy,
		SortOrder:    q.SortOrder,
	}

	items, total, err := h.repo.List(ctx, params)
	if err != nil {
		return nil, err
	}

	var totalPages int32
	if pageSize > 0 && total > 0 {
		computed := (total + int64(pageSize) - 1) / int64(pageSize)
		totalPages = safeconv.Int64ToInt32(computed)
	}

	return &ListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages,
		CurrentPage: safeconv.IntToInt32(page),
		PageSize:    safeconv.IntToInt32(pageSize),
	}, nil
}
