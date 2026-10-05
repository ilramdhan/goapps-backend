package erprule

import (
	"context"
	"strings"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// Paging defaults for the listings.
const (
	DefaultPageSize = 20
	MaxPageSize     = 500
)

// normalizePage clamps paging: page < 1 becomes 1, pageSize < 1 becomes
// DefaultPageSize and anything above MaxPageSize becomes MaxPageSize.
func normalizePage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	switch {
	case pageSize < 1:
		pageSize = DefaultPageSize
	case pageSize > MaxPageSize:
		pageSize = MaxPageSize
	}
	return page, pageSize
}

// ListVallossRulesQuery filters the rule listing. Empty strings mean "any".
type ListVallossRulesQuery struct {
	FgType          string
	ProdType        string
	GradeGroup      string
	Basis           string
	IncludeInactive bool
	Page            int
	PageSize        int
}

// ListVallossRulesResult is one page of rules.
type ListVallossRulesResult struct {
	Items    []*domain.VallossRule
	Total    int64
	Page     int
	PageSize int
}

// ListVallossRulesHandler lists rules (ListValLossRules).
type ListVallossRulesHandler struct{ repo domain.VallossRuleRepository }

// NewListVallossRulesHandler builds the handler.
func NewListVallossRulesHandler(repo domain.VallossRuleRepository) *ListVallossRulesHandler {
	return &ListVallossRulesHandler{repo: repo}
}

// Handle validates the filters and lists one page.
func (h *ListVallossRulesHandler) Handle(ctx context.Context, q ListVallossRulesQuery) (ListVallossRulesResult, error) {
	f, err := vallossFilterOf(q)
	if err != nil {
		return ListVallossRulesResult{}, err
	}
	f.Page, f.PageSize = normalizePage(q.Page, q.PageSize)
	items, total, err := h.repo.List(ctx, f)
	if err != nil {
		return ListVallossRulesResult{}, err
	}
	return ListVallossRulesResult{Items: items, Total: total, Page: f.Page, PageSize: f.PageSize}, nil
}

// vallossFilterOf parses the optional filter values (paging is left zero).
func vallossFilterOf(q ListVallossRulesQuery) (domain.VallossRuleFilter, error) {
	f := domain.VallossRuleFilter{IncludeInactive: q.IncludeInactive}
	var err error
	if strings.TrimSpace(q.FgType) != "" {
		if f.FgType, err = domain.ParseFgType(q.FgType); err != nil {
			return f, err
		}
	}
	if strings.TrimSpace(q.ProdType) != "" {
		if f.ProdType, err = domain.ParseProdType(q.ProdType); err != nil {
			return f, err
		}
	}
	if strings.TrimSpace(q.GradeGroup) != "" {
		if f.GradeGroup, err = domain.ParseRuleGradeGroup(q.GradeGroup); err != nil {
			return f, err
		}
	}
	if strings.TrimSpace(q.Basis) != "" {
		if f.Basis, err = domain.ParseBasis(q.Basis); err != nil {
			return f, err
		}
	}
	return f, nil
}

// ListSellPricesQuery filters the sell-price listing.
type ListSellPricesQuery struct {
	IncludeInactive bool
}

// ListSellPricesHandler lists sell prices (ListSellPrices). There are at
// most three rows, so the listing is not paged.
type ListSellPricesHandler struct{ repo domain.SellPriceRepository }

// NewListSellPricesHandler builds the handler.
func NewListSellPricesHandler(repo domain.SellPriceRepository) *ListSellPricesHandler {
	return &ListSellPricesHandler{repo: repo}
}

// Handle lists the prices ordered by basis.
func (h *ListSellPricesHandler) Handle(ctx context.Context, q ListSellPricesQuery) ([]*domain.SellPrice, error) {
	return h.repo.List(ctx, q.IncludeInactive)
}

// ListGradeGroupsQuery filters the grade listing. UnassignedOnly returns
// the grades with ceg_grade_group IS NULL: the V-08 worklist.
type ListGradeGroupsQuery struct {
	UnassignedOnly bool
	Search         string
	Page           int
	PageSize       int
}

// ListGradeGroupsResult is one page of grades.
type ListGradeGroupsResult struct {
	Items    []*domain.Grade
	Total    int64
	Page     int
	PageSize int
}

// ListGradeGroupsHandler lists ERP grades and their groups (ListGradeGroups).
type ListGradeGroupsHandler struct{ repo domain.GradeGroupRepository }

// NewListGradeGroupsHandler builds the handler.
func NewListGradeGroupsHandler(repo domain.GradeGroupRepository) *ListGradeGroupsHandler {
	return &ListGradeGroupsHandler{repo: repo}
}

// Handle lists one page of grades.
func (h *ListGradeGroupsHandler) Handle(ctx context.Context, q ListGradeGroupsQuery) (ListGradeGroupsResult, error) {
	page, size := normalizePage(q.Page, q.PageSize)
	items, total, err := h.repo.ListGrades(ctx, domain.GradeFilter{
		UnassignedOnly: q.UnassignedOnly,
		Search:         strings.TrimSpace(q.Search),
		Page:           page,
		PageSize:       size,
	})
	if err != nil {
		return ListGradeGroupsResult{}, err
	}
	return ListGradeGroupsResult{Items: items, Total: total, Page: page, PageSize: size}, nil
}

// ListUnassignedGradesHandler is the V-08 worklist: every grade without a
// group, one page at a time. It is ListGradeGroups with UnassignedOnly
// forced on.
type ListUnassignedGradesHandler struct{ inner *ListGradeGroupsHandler }

// NewListUnassignedGradesHandler builds the handler.
func NewListUnassignedGradesHandler(repo domain.GradeGroupRepository) *ListUnassignedGradesHandler {
	return &ListUnassignedGradesHandler{inner: NewListGradeGroupsHandler(repo)}
}

// Handle lists one page of unassigned grades.
func (h *ListUnassignedGradesHandler) Handle(ctx context.Context, search string, page, pageSize int) (ListGradeGroupsResult, error) {
	return h.inner.Handle(ctx, ListGradeGroupsQuery{
		UnassignedOnly: true, Search: search, Page: page, PageSize: pageSize,
	})
}
