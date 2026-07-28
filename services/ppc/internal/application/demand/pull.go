package demand

import (
	"context"
	"strings"
	"time"

	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
	"github.com/mutugading/goapps-backend/services/ppc/pkg/safeconv"
)

// StagingListQuery carries inputs for listing SO-staging rows.
type StagingListQuery struct {
	Page         int
	PageSize     int
	Search       string
	CustomerCode string
	ItemCode     string
	UnpulledOnly bool
	SortBy       string
	SortOrder    string
}

// StagingListResult holds a page of SO-staging rows plus pagination.
type StagingListResult struct {
	Items       []*demanddomain.SalesOrderStaging
	TotalItems  int64
	TotalPages  int32
	CurrentPage int32
	PageSize    int32
}

// ListStaging returns a paginated page of the Orion SO staging inbox (LOV).
func (s *Service) ListStaging(ctx context.Context, q StagingListQuery) (*StagingListResult, error) {
	filter := demanddomain.StagingListFilter{
		Search:       q.Search,
		CustomerCode: q.CustomerCode,
		ItemCode:     q.ItemCode,
		UnpulledOnly: q.UnpulledOnly,
		Page:         q.Page,
		PageSize:     q.PageSize,
		SortBy:       q.SortBy,
		SortOrder:    q.SortOrder,
	}
	filter.Validate()

	items, total, err := s.repo.ListStaging(ctx, filter)
	if err != nil {
		return nil, err
	}
	// Lazily resolve products when the page still carries unresolved rows, then
	// re-read so the caller sees the resolution it just triggered. Non-fatal by
	// design: a finance hiccup must never break the staging inbox.
	if s.resolveStagingIfNeeded(ctx, items) {
		items, total, err = s.repo.ListStaging(ctx, filter)
		if err != nil {
			return nil, err
		}
	}
	return &StagingListResult{
		Items:       items,
		TotalItems:  total,
		TotalPages:  totalPages(total, filter.PageSize),
		CurrentPage: safeconv.IntToInt32(filter.Page),
		PageSize:    safeconv.IntToInt32(filter.PageSize),
	}, nil
}

// StagingIDsQuery carries the row-selection filter for a "select all matching"
// over the SO-staging inbox. It has no paging: the caller wants the whole
// matching set, bounded only by the service cap.
type StagingIDsQuery struct {
	Search       string
	CustomerCode string
	ItemCode     string
	UnpulledOnly bool
}

// StagingIDsResult holds the (possibly truncated) ids plus the honest totals a
// caller needs to tell a planner what was actually selected.
type StagingIDsResult struct {
	SosIDs []int64
	// TotalMatched is the untruncated match count. Greater than len(SosIDs)
	// means the set was capped.
	TotalMatched int64
	// Limit is the cap that was applied.
	Limit int
}

// ListStagingIDs returns the ids of the staging rows matching a filter, capped
// at MaxSelectAllStagingIDs. Deliberately does not run the lazy product
// resolution ListStaging does — selecting rows does not need their products,
// and resolving several hundred rows on a click would be a poor trade.
func (s *Service) ListStagingIDs(ctx context.Context, q StagingIDsQuery) (*StagingIDsResult, error) {
	limit := demanddomain.MaxSelectAllStagingIDs
	ids, total, err := s.repo.ListStagingIDs(ctx, demanddomain.StagingIDsFilter(q), limit)
	if err != nil {
		return nil, err
	}
	return &StagingIDsResult{SosIDs: ids, TotalMatched: total, Limit: limit}, nil
}

// SetStagingProductCommand carries a planner's manual product pick for one
// staging row.
type SetStagingProductCommand struct {
	SosID           int64
	CpmProductSysID int64
}

// SetStagingProduct persists a manual product pick on a staging row, marking it
// MANUAL so automatic resolution leaves it alone. The product is validated
// against finance first — this is a write path, so it fails closed.
func (s *Service) SetStagingProduct(ctx context.Context, cmd SetStagingProductCommand) (*demanddomain.SalesOrderStaging, error) {
	if cmd.CpmProductSysID <= 0 {
		return nil, demanddomain.ErrInvalidProduct
	}
	if s.validator != nil {
		if err := s.validator.ValidateProduct(ctx, cmd.CpmProductSysID); err != nil {
			return nil, err
		}
	}
	return s.repo.SetStagingProduct(ctx, cmd.SosID, cmd.CpmProductSysID)
}

// PullFromOrionCommand carries inputs for creating demands from SO staging.
// The planning month is derived from each staging row's deadline, so the
// caller does not supply one.
type PullFromOrionCommand struct {
	SosIDs    []int64
	SubType   string
	CreatedBy int64
}

// PullFromOrion creates a CONTRACT demand per selected staging row and marks
// each row pulled. Each new demand is source ORION_PULL.
//
// The product comes from the row's recorded resolution (see ResolveStaging).
// A resolved product is validated against finance; an unresolved row still
// pulls, with no product written — never a stand-in id.
func (s *Service) PullFromOrion(ctx context.Context, cmd PullFromOrionCommand) ([]*demanddomain.Demand, error) {
	rows, err := s.repo.GetStagingByIDs(ctx, cmd.SosIDs)
	if err != nil {
		return nil, err
	}

	// Resolve the Orion customer codes on the selected rows against the PPC
	// customer master in one lookup. Customer lives in ppc_db (unlike the finance
	// product, which needs gRPC), so resolving at pull time is a single cheap
	// query and avoids caching a second resolution column on the staging row.
	customerIDs := s.resolveStagingCustomers(ctx, rows)

	created := make([]*demanddomain.Demand, 0, len(rows))
	for _, row := range rows {
		if err := s.validateStagingProduct(ctx, row); err != nil {
			return nil, err
		}
		entity, buildErr := s.demandFromStaging(row, cmd, customerIDs)
		if buildErr != nil {
			return nil, buildErr
		}
		if err := s.repo.Create(ctx, entity); err != nil {
			return nil, err
		}
		if err := s.repo.MarkStagingPulled(ctx, row.SosID, entity.ID()); err != nil {
			return nil, err
		}
		created = append(created, entity)
	}
	return created, nil
}

// resolveStagingCustomers maps the distinct Orion customer codes on the given
// staging rows onto PPC customer-master ids. Nil-safe and non-fatal: with no
// resolver wired, or on a lookup failure, it yields an empty map and the pulled
// demands simply carry no customer.
func (s *Service) resolveStagingCustomers(
	ctx context.Context,
	rows []*demanddomain.SalesOrderStaging,
) map[string]int64 {
	if s.customers == nil || len(rows) == 0 {
		return map[string]int64{}
	}
	codes := make([]string, 0, len(rows))
	for _, row := range rows {
		if code := strings.TrimSpace(row.CustomerCode); code != "" {
			codes = append(codes, code)
		}
	}
	if len(codes) == 0 {
		return map[string]int64{}
	}
	resolved, err := s.customers.ResolveCodes(ctx, codes)
	if err != nil || resolved == nil {
		return map[string]int64{}
	}
	return resolved
}

// validateStagingProduct validates a staging row's resolved product against
// finance. A row with no resolved product is left for the deferred-link flow
// and skips validation rather than inventing an id to validate.
func (s *Service) validateStagingProduct(ctx context.Context, row *demanddomain.SalesOrderStaging) error {
	if s.validator == nil || row.CpmProductSysID == nil {
		return nil
	}
	return s.validator.ValidateProduct(ctx, *row.CpmProductSysID)
}

// stagingLinkReason maps a staging row's resolution outcome onto a demand
// product-link reason. Empty when the row already carries a product — a linked
// demand must not keep a reason (the DB CHECK pairs the two).
func stagingLinkReason(row *demanddomain.SalesOrderStaging) string {
	if row.CpmProductSysID != nil {
		return ""
	}
	if row.MatchStatus == demanddomain.MatchStatusAmbiguous {
		return demanddomain.LinkReasonAmbiguous
	}
	// NOT_FOUND, UNRESOLVED and anything else all mean automatic resolution did
	// not produce a product for this row.
	return demanddomain.LinkReasonAutoMatchFailed
}

// demandFromStaging maps one SO-staging row into a new demand for the target
// month. Contract-linked pulls default to the NEW_EXPORT sub-type.
func (s *Service) demandFromStaging(
	row *demanddomain.SalesOrderStaging,
	cmd PullFromOrionCommand,
	customerIDs map[string]int64,
) (*demanddomain.Demand, error) {
	subType := cmd.SubType
	if subType == "" {
		subType = demanddomain.SubTypeNewExport
	}
	deadline := time.Now()
	if row.Deadline != nil {
		deadline = *row.Deadline
	}
	sosRef := row.SosID
	qty := row.QtyRemaining
	if qty <= 0 {
		qty = row.QtyOrdered
	}
	// An unresolved row pulls with no product. Writing any other id here (the
	// contract sys id was the old fallback) silently corrupts the product link.
	// The demand lands PENDING_PRODUCT_LINK and records why, so a planner can
	// tell a failed match from an ambiguous one.
	var productSysID int64
	if row.CpmProductSysID != nil {
		productSysID = *row.CpmProductSysID
	}
	// An unmatched customer code simply leaves the demand's customer unset —
	// never a stand-in id.
	var customerID *int64
	if id, ok := customerIDs[strings.ToUpper(strings.TrimSpace(row.CustomerCode))]; ok {
		customerID = &id
	}
	return demanddomain.New(demanddomain.NewParams{
		CustomerID:        customerID,
		ProductLinkReason: stagingLinkReason(row),
		Type:              demanddomain.TypeContract,
		SubType:           subType,
		Source:            demanddomain.SourceOrionPull,
		CpmProductSysID:   productSysID,
		QtyOriginal:       qty,
		Deadline:          deadline,
		GradeReq:          demanddomain.GradeReqNone,
		ShadeCode:         row.ShadeCode,
		ShadeName:         row.ShadeName,
		SosRef:            &sosRef,
		ContractNo:        row.ContractNo,
		ContractDate:      row.ContractDate,
		CreatedBy:         cmd.CreatedBy,
	})
}
