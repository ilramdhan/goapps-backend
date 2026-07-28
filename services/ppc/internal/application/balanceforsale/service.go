// Package balanceforsale provides the application usecase for the AX
// balance-for-sale dashboard view (PRD page 10).
package balanceforsale

import (
	"context"
	"fmt"
	"sort"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	"github.com/mutugading/goapps-backend/services/ppc/internal/application/notification"
	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/balanceforsale"
)

// ComponentSource gathers the PPC-sourced balance components per product and the
// commodity-watch product set. Implemented by postgres.BalanceForSaleRepository.
type ComponentSource interface {
	ListCommodityProducts(ctx context.Context) ([]int64, error)
	GatherComponents(ctx context.Context, productIDs []int64) (map[int64]*domain.Components, error)
}

// ProductLookup resolves product code/name for display. Implemented by the
// finance gRPC client; may be nil (names degrade to empty).
type ProductLookup interface {
	BatchGetProducts(ctx context.Context, sysIDs []int64) ([]*financev1.CostMasterProduct, error)
}

// Service computes the balance-for-sale rows.
type Service struct {
	source   ComponentSource
	products ProductLookup
	notifier notification.Notifier
}

// NewService builds the balance-for-sale service. products and notifier may be
// nil (the notifier degrades to a no-op via notification.Notify).
func NewService(source ComponentSource, products ProductLookup, notifier notification.Notifier) *Service {
	return &Service{source: source, products: products, notifier: notifier}
}

// NotifyCommodityWatch scans the commodity-watch products and emits a
// best-effort notification for each product whose balance-for-sale is at or
// below zero (oversold / at risk). The nil-safe notifier makes this a no-op when
// no notifier is wired. It never fails the caller — returned errors are the read
// errors only, so it is safe to call from a scheduled watcher.
func (s *Service) NotifyCommodityWatch(ctx context.Context) error {
	rows, err := s.GetBalanceForSale(ctx, Query{CommodityWatchOnly: true})
	if err != nil {
		return err
	}
	for i := range rows {
		if rows[i].BalanceForSale > 0 {
			continue
		}
		notification.Notify(ctx, s.notifier, notification.Message{
			Event:      notification.EventBFSCommodityWatch,
			Subject:    "Commodity watch: balance-for-sale at risk",
			Body:       fmt.Sprintf("%s (%s) balance-for-sale is %.2f", rows[i].ProductName, rows[i].ProductCode, rows[i].BalanceForSale),
			Recipients: []string{"MARKETING", "PPC"},
			EntityID:   rows[i].CpmProductSysID,
		})
	}
	return nil
}

// Query selects which products to include in the balance view.
type Query struct {
	CpmProductSysID    *int64 // single product, when set
	CommodityWatchOnly bool   // restrict to the commodity watchlist
}

// GetBalanceForSale returns the per-product balance breakdown. current_stock_AX
// is stubbed to 0 (no Orion inventory ETL in scope) — the rest is computed from
// running WO output, confirmed MTS plan, and committed contract demand.
func (s *Service) GetBalanceForSale(ctx context.Context, q Query) ([]domain.Row, error) {
	productIDs, err := s.scopeProductIDs(ctx, q)
	if err != nil {
		return nil, err
	}

	components, err := s.source.GatherComponents(ctx, productIDs)
	if err != nil {
		return nil, err
	}

	rows := s.buildRows(components)
	s.decorateNames(ctx, rows)
	sort.Slice(rows, func(i, j int) bool { return rows[i].CpmProductSysID < rows[j].CpmProductSysID })
	return rows, nil
}

// scopeProductIDs resolves the product-id filter from the query: a single id, or
// the commodity watchlist, or none (all products in planning data).
func (s *Service) scopeProductIDs(ctx context.Context, q Query) ([]int64, error) {
	if q.CpmProductSysID != nil {
		return []int64{*q.CpmProductSysID}, nil
	}
	if q.CommodityWatchOnly {
		return s.source.ListCommodityProducts(ctx)
	}
	return nil, nil
}

// buildRows derives balance rows from gathered components.
func (s *Service) buildRows(components map[int64]*domain.Components) []domain.Row {
	rows := make([]domain.Row, 0, len(components))
	for _, c := range components {
		rows = append(rows, domain.BuildRow(*c))
	}
	return rows
}

// decorateNames fills product code/name from the finance lookup. A nil or
// degraded lookup leaves the identity fields empty (non-fatal).
func (s *Service) decorateNames(ctx context.Context, rows []domain.Row) {
	if s.products == nil || len(rows) == 0 {
		return
	}
	ids := make([]int64, len(rows))
	for i := range rows {
		ids[i] = rows[i].CpmProductSysID
	}
	products, err := s.products.BatchGetProducts(ctx, ids)
	if err != nil {
		return
	}
	byID := make(map[int64]*financev1.CostMasterProduct, len(products))
	for _, p := range products {
		byID[p.GetProductSysId()] = p
	}
	for i := range rows {
		if p, ok := byID[rows[i].CpmProductSysID]; ok {
			rows[i].ProductCode = p.GetProductCode()
			rows[i].ProductName = p.GetProductName()
		}
	}
}
