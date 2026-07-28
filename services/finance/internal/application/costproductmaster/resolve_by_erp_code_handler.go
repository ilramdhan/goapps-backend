package costproductmaster

import (
	"context"
	"fmt"
	"strings"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres"
)

// MaxErpCodePairs is the server-side cap on one resolve request. It mirrors the
// proto max_items rule so a caller that bypasses validation is still bounded.
const MaxErpCodePairs = 500

// ErpCodePair is one (erp_item_code, shade_code) lookup key. Both components
// are matched trimmed and case-insensitively; ShadeCode may be empty.
type ErpCodePair struct {
	ErpItemCode string
	ShadeCode   string
}

// ErpCodeResolution is the outcome for one requested pair. MatchCount is 0 for
// no match, 1 for a unique match, and >=2 when the pair is ambiguous. Product
// is set only when MatchCount == 1.
type ErpCodeResolution struct {
	Pair       ErpCodePair
	MatchCount int32
	Product    *postgres.LookupProduct
}

// ErpCodeResolverRepository is the narrow read port used by this handler.
type ErpCodeResolverRepository interface {
	ResolveByErpCodes(ctx context.Context, itemCodes, shadeCodes []string) ([]postgres.LookupProduct, error)
}

// ResolveByErpCodeHandler resolves ERP item/shade code pairs to CPM products.
// The database returns every candidate row; this handler groups them per pair
// so ambiguity is reported explicitly instead of silently picking a winner.
type ResolveByErpCodeHandler struct {
	repo ErpCodeResolverRepository
}

// NewResolveByErpCodeHandler creates a new ResolveByErpCodeHandler.
func NewResolveByErpCodeHandler(repo ErpCodeResolverRepository) *ResolveByErpCodeHandler {
	return &ResolveByErpCodeHandler{repo: repo}
}

// erpCodeKey normalizes a pair into its match key: trimmed + uppercased on both
// components, joined by a separator that cannot occur in either code.
func erpCodeKey(itemCode, shadeCode string) string {
	return strings.ToUpper(strings.TrimSpace(itemCode)) + "\x00" + strings.ToUpper(strings.TrimSpace(shadeCode))
}

// Handle resolves the requested pairs. The result has exactly one entry per
// requested pair, in request order — duplicates in the input produce duplicate
// (identical) resolutions rather than being collapsed, so the caller can index
// by position.
func (h *ResolveByErpCodeHandler) Handle(ctx context.Context, pairs []ErpCodePair) ([]ErpCodeResolution, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	if len(pairs) > MaxErpCodePairs {
		return nil, fmt.Errorf("too many pairs: %d (max %d)", len(pairs), MaxErpCodePairs)
	}

	itemCodes := make([]string, 0, len(pairs))
	shadeCodes := make([]string, 0, len(pairs))
	for _, p := range pairs {
		itemCodes = append(itemCodes, p.ErpItemCode)
		shadeCodes = append(shadeCodes, p.ShadeCode)
	}

	products, err := h.repo.ResolveByErpCodes(ctx, itemCodes, shadeCodes)
	if err != nil {
		return nil, fmt.Errorf("resolve products by erp code: %w", err)
	}

	byKey := make(map[string][]postgres.LookupProduct, len(pairs))
	for i := range products {
		p := products[i]
		k := erpCodeKey(p.ErpItemCode, p.ShadeCode)
		byKey[k] = append(byKey[k], p)
	}

	out := make([]ErpCodeResolution, 0, len(pairs))
	for _, p := range pairs {
		matches := byKey[erpCodeKey(p.ErpItemCode, p.ShadeCode)]
		res := ErpCodeResolution{Pair: p, MatchCount: safeLenToInt32(len(matches))}
		if len(matches) == 1 {
			m := matches[0]
			res.Product = &m
		}
		out = append(out, res)
	}
	return out, nil
}

// safeLenToInt32 converts a slice length to int32. Lengths here are bounded by
// the CPM row count for at most MaxErpCodePairs keys, but the clamp keeps the
// conversion provably safe.
func safeLenToInt32(n int) int32 {
	const maxInt32 = 1<<31 - 1
	if n > maxInt32 {
		return maxInt32
	}
	return int32(n) //nolint:gosec // bounds checked above
}
