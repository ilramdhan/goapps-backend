package financeclient

import (
	"context"
	"errors"
	"strconv"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// ParamDefSource adapts the finance client to the WO resolver's parameter
// definition source (mst_parameter). Degraded finance yields an empty list.
type ParamDefSource struct {
	client *Client
}

// NewParamDefSource builds a parameter-definition source over the finance client.
func NewParamDefSource(client *Client) *ParamDefSource {
	return &ParamDefSource{client: client}
}

var _ workorder.ParamDefSource = (*ParamDefSource)(nil)

// ListParamDefs lists parameter definitions for a display group.
func (s *ParamDefSource) ListParamDefs(ctx context.Context, displayGroup string) ([]workorder.ParamDef, error) {
	defs, err := s.client.ListProductParameters(ctx, displayGroup)
	if err != nil {
		if errors.Is(err, ErrDegraded) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]workorder.ParamDef, 0, len(defs))
	for _, d := range defs {
		out = append(out, workorder.ParamDef{
			ParamID:      d.ParamID,
			ParamCode:    d.ParamCode,
			ParamName:    d.ParamName,
			DataType:     d.DataType,
			DisplayGroup: d.DisplayGroup,
			DisplayOrder: d.DisplayOrder,
			DefaultValue: d.DefaultValue,
		})
	}
	return out, nil
}

// ProductValueSource adapts the finance client to the WO resolver's per-product
// value source (cost_product_parameter, resolution layer 3). Degraded finance
// yields no values so resolution falls through to defaults.
type ProductValueSource struct {
	client *Client
}

// NewProductValueSource builds a per-product parameter value source.
func NewProductValueSource(client *Client) *ProductValueSource {
	return &ProductValueSource{client: client}
}

var _ workorder.ProductValueSource = (*ProductValueSource)(nil)

// ProductValues returns typed per-product values keyed by param id.
func (s *ProductValueSource) ProductValues(ctx context.Context, productSysID int64, paramIDs []string) (map[string]workorder.TypedValue, error) {
	vals, err := s.client.BatchGetProductParameterValues(ctx, []int64{productSysID}, paramIDs)
	if err != nil {
		if errors.Is(err, ErrDegraded) {
			return nil, nil //nolint:nilnil // degraded finance yields no values, not an error
		}
		return nil, err
	}
	out := make(map[string]workorder.TypedValue, len(vals))
	for _, v := range vals {
		out[v.ParamID] = typedValueFromFinance(v)
	}
	return out, nil
}

// RouteRmSource adapts the finance client to the WO service's RM-BOM route
// source (cost_route_rm). Degraded finance yields no components so RM-BOM
// population degrades to a no-op rather than an error.
type RouteRmSource struct {
	client *Client
}

// NewRouteRmSource builds an RM-BOM route source over the finance client.
func NewRouteRmSource(client *Client) *RouteRmSource {
	return &RouteRmSource{client: client}
}

var _ workorder.RouteRmSource = (*RouteRmSource)(nil)

// RouteRmComponents flattens the released route's stages into RM components,
// carrying each stage's shade onto its RM rows and parsing the ratio.
func (s *RouteRmSource) RouteRmComponents(ctx context.Context, productSysID int64) ([]workorder.RouteRmComponent, error) {
	resp, err := s.client.GetProductRoute(ctx, productSysID)
	if err != nil {
		if errors.Is(err, ErrDegraded) {
			return nil, nil
		}
		return nil, err
	}
	route := resp.GetData()
	if route == nil {
		return nil, nil
	}
	var out []workorder.RouteRmComponent
	for _, stage := range route.GetStages() {
		shade := stage.GetRouteShadeCode()
		for _, rm := range stage.GetRms() {
			out = append(out, workorder.RouteRmComponent{
				CrmRmID:        rm.GetRmId(),
				RmType:         rm.GetRmType(),
				ShadeCode:      shade,
				Ratio:          parseRatio(rm.GetRouteRmRatio()),
				RmCode:         rm.GetRmCode(),
				RmName:         rm.GetRmName(),
				RouteStageName: stage.GetRouteName(),
				RouteLevel:     stage.GetRouteLevel(),
			})
		}
	}
	return out, nil
}

// LotSpecSource adapts the finance client to the WO service's lot-spec source.
// A generated lot is keyed by item_code + shade_code (PRD §9), both of which
// live on the finance cost product master — ppc_db and finance_db are separate
// databases, so gRPC is the only route to them.
type LotSpecSource struct {
	client *Client
}

// NewLotSpecSource builds a lot-spec source over the finance client.
func NewLotSpecSource(client *Client) *LotSpecSource {
	return &LotSpecSource{client: client}
}

var _ workorderapp.LotSpecSource = (*LotSpecSource)(nil)

// LotSpec returns the ERP item code and shade code of a finance product.
// Degraded finance yields empty codes, which the caller treats as "cannot
// generate a lot" rather than as permission to invent one.
func (s *LotSpecSource) LotSpec(ctx context.Context, productSysID int64) (itemCode, shadeCode string, err error) {
	product, err := s.client.GetProduct(ctx, productSysID)
	if err != nil {
		if errors.Is(err, ErrDegraded) || errors.Is(err, ErrProductNotFound) {
			return "", "", nil
		}
		return "", "", err
	}
	return product.GetErpItemCode(), product.GetShadeCode(), nil
}

// parseRatio parses a decimal-as-string route ratio, treating blank/invalid as 0.
func parseRatio(s string) float64 {
	if s == "" {
		return 0
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0
	}
	return v
}

func typedValueFromFinance(v ParameterValue) workorder.TypedValue {
	switch v.DataType {
	case "BOOLEAN":
		flag := v.ValueFlag
		return workorder.TypedValue{Flag: &flag}
	case "TEXT":
		text := v.ValueText
		return workorder.TypedValue{Text: &text}
	default: // NUMBER
		if v.ValueNumeric == "" {
			return workorder.TypedValue{}
		}
		n, err := strconv.ParseFloat(v.ValueNumeric, 64)
		if err != nil {
			text := v.ValueNumeric
			return workorder.TypedValue{Text: &text}
		}
		return workorder.TypedValue{Num: &n}
	}
}
