package financeclient

import (
	"context"
	"errors"

	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// StagingResolver adapts the finance client to the demand domain's
// ProductResolver: it turns Orion staging (item, shade) keys into finance
// cost-product-master sys ids over gRPC, because ppc_db and finance_db are
// separate databases and cannot be joined.
type StagingResolver struct {
	client *Client
}

// NewStagingResolver builds a staging product resolver over the finance client.
func NewStagingResolver(client *Client) *StagingResolver {
	return &StagingResolver{client: client}
}

var _ demanddomain.ProductResolver = (*StagingResolver)(nil)

// ResolveByErpCode resolves each pair to at most one product. A degraded
// finance connection surfaces as demanddomain.ErrResolverDegraded so the caller
// can skip the pass instead of failing the request.
func (r *StagingResolver) ResolveByErpCode(ctx context.Context, pairs []demanddomain.StagingPair) ([]demanddomain.ProductResolution, error) {
	if len(pairs) == 0 {
		return nil, nil
	}
	req := make([]ErpCodePair, 0, len(pairs))
	for _, p := range pairs {
		req = append(req, ErpCodePair{ErpItemCode: p.ItemCode, ShadeCode: p.ShadeCode})
	}

	resolutions, err := r.client.ResolveProductsByErpCode(ctx, req)
	if err != nil {
		if errors.Is(err, ErrDegraded) {
			return nil, demanddomain.ErrResolverDegraded
		}
		return nil, err
	}

	out := make([]demanddomain.ProductResolution, 0, len(resolutions))
	for _, res := range resolutions {
		item := demanddomain.ProductResolution{
			Pair:       demanddomain.StagingPair{ItemCode: res.Pair.ErpItemCode, ShadeCode: res.Pair.ShadeCode},
			MatchCount: res.MatchCount,
		}
		if res.Product != nil {
			sysID := res.Product.GetProductSysId()
			item.CpmProductSysID = &sysID
		}
		out = append(out, item)
	}
	return out, nil
}
