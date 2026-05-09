// Package prdrequest holds application-layer command handlers for the ProductRequest aggregate.
package prdrequest

import (
	"context"

	domainproduct "github.com/mutugading/goapps-backend/services/finance/internal/domain/product"
)

// SearchExistingCommand carries inputs to SearchExistingHandler.
type SearchExistingCommand struct {
	Query     string
	ShadeCode string
	Limit     int // 1–50; clamped by the product repository.
}

// SearchExistingHandler performs a full-text product search to help requesters
// identify whether an existing product already satisfies their requirement.
// It reads from product.Repository — this is an intentional cross-aggregate read
// permitted at the application layer per Clean Architecture.
type SearchExistingHandler struct {
	products domainproduct.Repository
}

// NewSearchExistingHandler constructs a SearchExistingHandler.
func NewSearchExistingHandler(products domainproduct.Repository) *SearchExistingHandler {
	return &SearchExistingHandler{products: products}
}

// Handle delegates to product.Repository.SearchByText and returns the matching products.
func (h *SearchExistingHandler) Handle(ctx context.Context, cmd SearchExistingCommand) ([]*domainproduct.Product, error) {
	return h.products.SearchByText(ctx, domainproduct.SearchOptions{
		Query:     cmd.Query,
		ShadeCode: cmd.ShadeCode,
		Limit:     cmd.Limit,
	})
}
