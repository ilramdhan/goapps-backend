package workorder

import "context"

// Parameter-resolution source labels (mirror proto ParamResolutionSource).
const (
	SourceWORef          = "WO_REF"
	SourceProductMachine = "PRODUCT_MACHINE"
	SourceProduct        = "PRODUCT"
	SourceDefault        = "DEFAULT"
	SourceNone           = ""
)

// DualParamCodes are the 8 machine parameters that carry independent PPC and PC
// values (from the MLR `_PC` columns). All other params mirror PPC into PC.
var DualParamCodes = []string{
	"SPEED", "DISC", "NOZZLE1", "NOZZLE2", "AIR_BAR", "AIR_M3", "OIL", "OPU", "TAPER_ANGLE",
}

// IsDualCode reports whether a parameter code carries a dual PPC/PC value.
func IsDualCode(code string) bool {
	for _, c := range DualParamCodes {
		if c == code {
			return true
		}
	}
	return false
}

// ParamDef is a definition of a parameter (projected from mst_parameter).
type ParamDef struct {
	ParamID      string
	ParamCode    string
	ParamName    string
	DataType     string // NUMBER / TEXT / BOOLEAN
	DisplayGroup string
	DisplayOrder int32
	DefaultValue string // decimal-as-string / text / "true"|"false"
}

// TypedValue is a typed parameter value; exactly one field is expected to be set
// according to the parameter data_type.
type TypedValue struct {
	Num  *float64
	Text *string
	Flag *bool
}

// ResolvedParam is a parameter definition paired with its resolved value and the
// layer that supplied it.
type ResolvedParam struct {
	ParamDef
	IsDual bool
	TypedValue
	Source string
}

// ParamDefSource lists parameter definitions for a display group.
type ParamDefSource interface {
	ListParamDefs(ctx context.Context, displayGroup string) ([]ParamDef, error)
}

// ProductMachineValueSource returns per-(product,machine) parameter values keyed
// by param id (resolution layer 2).
type ProductMachineValueSource interface {
	ProductMachineValues(ctx context.Context, productSysID, machineID int64) (map[string]TypedValue, error)
}

// ProductValueSource returns per-product parameter values keyed by param id
// (resolution layer 3, finance cost_product_parameter).
type ProductValueSource interface {
	ProductValues(ctx context.Context, productSysID int64, paramIDs []string) (map[string]TypedValue, error)
}

// WORefValueSource returns a referenced WO's PPC parameter values keyed by param
// id (resolution layer 1).
type WORefValueSource interface {
	RefParamValues(ctx context.Context, refWoID int64) (map[string]TypedValue, error)
}

// RouteRmComponent is one RM component projected from a product's released route
// (cost_route_rm), used to auto-materialize RM allocation suggestions.
type RouteRmComponent struct {
	CrmRmID   int64
	RmType    string  // PRODUCT / ITEM / GROUP
	ShadeCode string  // from the parent route stage
	Ratio     float64 // route_rm_ratio (fraction of target qty)
}

// RouteRmSource resolves the RM components of a product's released route.
type RouteRmSource interface {
	// RouteRmComponents returns the RM components of a product's released route.
	RouteRmComponents(ctx context.Context, productSysID int64) ([]RouteRmComponent, error)
}

// Resolver resolves parameter values for a (product, machine) using the v1.2
// four-layer chain: WO-reference → product_machine_parameter → cost_product_parameter
// → mst_parameter.default_value. Any source may be nil, in which case its layer
// is skipped (degrades gracefully).
type Resolver struct {
	defs           ParamDefSource
	productMachine ProductMachineValueSource
	product        ProductValueSource
	ref            WORefValueSource
}

// NewResolver builds a parameter resolver from its (nil-safe) value sources.
func NewResolver(defs ParamDefSource, pm ProductMachineValueSource, product ProductValueSource, ref WORefValueSource) *Resolver {
	return &Resolver{defs: defs, productMachine: pm, product: product, ref: ref}
}

// ResolveRequest carries the inputs for a resolution pass.
type ResolveRequest struct {
	ProductSysID int64
	MachineID    int64
	RefWoID      *int64
	DisplayGroup string // filter (e.g. "Machine"); empty = all WO groups
}

// Resolve returns the resolved parameters for the request, one per definition in
// the requested display group, each tagged with the layer that supplied its value.
func (r *Resolver) Resolve(ctx context.Context, req ResolveRequest) ([]ResolvedParam, error) {
	if r.defs == nil {
		return nil, nil
	}
	defs, err := r.defs.ListParamDefs(ctx, req.DisplayGroup)
	if err != nil {
		return nil, err
	}

	paramIDs := make([]string, 0, len(defs))
	for _, d := range defs {
		paramIDs = append(paramIDs, d.ParamID)
	}

	refVals := r.refValues(ctx, req.RefWoID)
	pmVals := r.pmValues(ctx, req.ProductSysID, req.MachineID)
	prodVals := r.prodValues(ctx, req.ProductSysID, paramIDs)

	resolved := make([]ResolvedParam, 0, len(defs))
	for _, d := range defs {
		resolved = append(resolved, resolveOne(d, refVals, pmVals, prodVals))
	}
	return resolved, nil
}

func resolveOne(d ParamDef, refVals, pmVals, prodVals map[string]TypedValue) ResolvedParam {
	rp := ResolvedParam{ParamDef: d, IsDual: IsDualCode(d.ParamCode)}
	if v, ok := refVals[d.ParamID]; ok {
		rp.TypedValue, rp.Source = v, SourceWORef
		return rp
	}
	if v, ok := pmVals[d.ParamID]; ok {
		rp.TypedValue, rp.Source = v, SourceProductMachine
		return rp
	}
	if v, ok := prodVals[d.ParamID]; ok {
		rp.TypedValue, rp.Source = v, SourceProduct
		return rp
	}
	rp.TypedValue = defaultTypedValue(d)
	rp.Source = SourceDefault
	return rp
}

func (r *Resolver) refValues(ctx context.Context, refWoID *int64) map[string]TypedValue {
	if r.ref == nil || refWoID == nil || *refWoID <= 0 {
		return nil
	}
	v, err := r.ref.RefParamValues(ctx, *refWoID)
	if err != nil {
		return nil
	}
	return v
}

func (r *Resolver) pmValues(ctx context.Context, productSysID, machineID int64) map[string]TypedValue {
	if r.productMachine == nil {
		return nil
	}
	v, err := r.productMachine.ProductMachineValues(ctx, productSysID, machineID)
	if err != nil {
		return nil
	}
	return v
}

func (r *Resolver) prodValues(ctx context.Context, productSysID int64, paramIDs []string) map[string]TypedValue {
	if r.product == nil {
		return nil
	}
	v, err := r.product.ProductValues(ctx, productSysID, paramIDs)
	if err != nil {
		return nil
	}
	return v
}

// PinWellKnown maps each efficiency-critical parameter code to its resolved
// param id, so the efficiency engine can look values up by stable code.
func PinWellKnown(defs []ParamDef) map[string]string {
	pinned := make(map[string]string, len(WellKnownCodes))
	for _, d := range defs {
		if IsWellKnownCode(d.ParamCode) {
			pinned[d.ParamCode] = d.ParamID
		}
	}
	return pinned
}

// defaultTypedValue parses a definition's default_value into a typed value by
// data type. An empty default yields an all-nil typed value.
func defaultTypedValue(d ParamDef) TypedValue {
	if d.DefaultValue == "" {
		return TypedValue{}
	}
	switch d.DataType {
	case "BOOLEAN":
		b := d.DefaultValue == "true" || d.DefaultValue == "TRUE" || d.DefaultValue == "1"
		return TypedValue{Flag: &b}
	case "TEXT":
		t := d.DefaultValue
		return TypedValue{Text: &t}
	default: // NUMBER (and unknown types fall back to numeric parse)
		if n, ok := parseFloat(d.DefaultValue); ok {
			return TypedValue{Num: &n}
		}
		t := d.DefaultValue
		return TypedValue{Text: &t}
	}
}
