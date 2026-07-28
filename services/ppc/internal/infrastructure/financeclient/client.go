// Package financeclient wraps the gRPC client for finance.v1.CostMasterLookupService.
// PPC reads finance masters (cost product master, routes, product grades) through
// the additive read-only *ForPPC RPCs. The configured internal service token is
// sent in the x-internal-token metadata header so finance accepts the call
// without a JWT. A short-TTL in-memory cache fronts the per-product lookups used
// at write-time validation to keep hot-path validations cheap.
package financeclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/rs/zerolog/log"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

const (
	// internalTokenHeader is the metadata key finance inspects to grant a
	// synthetic internal identity to trusted service-to-service callers.
	internalTokenHeader = "x-internal-token"

	// productCacheTTL bounds how long a resolved product projection is trusted
	// before a fresh finance lookup is required.
	productCacheTTL = 5 * time.Minute
)

// Client is a thin handle around the CostMasterLookupService gRPC client plus a
// short-TTL product cache. A zero-value host disables the client (degraded
// mode): dials are skipped, read/display paths degrade to empty results, and
// write-path validation fails closed with ErrProductValidationUnavailable.
type Client struct {
	conn        *grpc.ClientConn
	cc          financev1.CostMasterLookupServiceClient
	authToken   string
	callTimeout time.Duration
	degraded    bool

	mu    sync.Mutex
	cache map[int64]cacheEntry
}

type cacheEntry struct {
	product   *financev1.CostMasterProduct
	expiresAt time.Time
}

// New dials finance's gRPC server with insecure transport (TLS terminated by the
// cluster mesh). An empty host returns a degraded client that performs no
// network calls; callers keep working with graceful degradation.
func New(host string, port int, authToken string, callTimeout time.Duration) (*Client, error) {
	if host == "" {
		log.Warn().Msg("financeclient: empty host, running in degraded mode (product validation allowed without finance)")
		return &Client{degraded: true, cache: make(map[int64]cacheEntry)}, nil
	}
	if callTimeout <= 0 {
		callTimeout = 10 * time.Second
	}
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return nil, fmt.Errorf("dial finance %s: %w", addr, err)
	}
	return &Client{
		conn:        conn,
		cc:          financev1.NewCostMasterLookupServiceClient(conn),
		authToken:   authToken,
		callTimeout: callTimeout,
		cache:       make(map[int64]cacheEntry),
	}, nil
}

// Close shuts down the underlying gRPC connection. Safe on a degraded client.
func (c *Client) Close() error {
	if c.conn == nil {
		return nil
	}
	if err := c.conn.Close(); err != nil {
		return fmt.Errorf("close finance grpc: %w", err)
	}
	return nil
}

// outgoingContext applies the call timeout, injects the internal auth token, and
// propagates the active trace context into outgoing gRPC metadata.
func (c *Client) outgoingContext(ctx context.Context) (context.Context, context.CancelFunc) {
	callCtx, cancel := context.WithTimeout(ctx, c.callTimeout)
	if c.authToken != "" {
		callCtx = metadata.AppendToOutgoingContext(callCtx, internalTokenHeader, c.authToken)
	}
	md, ok := metadata.FromOutgoingContext(callCtx)
	if !ok {
		md = metadata.MD{}
	} else {
		md = md.Copy()
	}
	otel.GetTextMapPropagator().Inject(callCtx, propagation.TextMapCarrier(metadataCarrier(md)))
	callCtx = metadata.NewOutgoingContext(callCtx, md)
	return callCtx, cancel
}

// GetProduct resolves one product projection by sys id, serving from the
// short-TTL cache when warm. Returns ErrDegraded when the client is disabled.
func (c *Client) GetProduct(ctx context.Context, sysID int64) (*financev1.CostMasterProduct, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	if p, ok := c.cacheGet(sysID); ok {
		return p, nil
	}

	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	resp, err := c.cc.GetCostProductMasterForPPC(callCtx, &financev1.GetCostProductMasterForPPCRequest{ProductSysId: sysID})
	if err != nil {
		return nil, fmt.Errorf("get cost product master %d: %w", sysID, err)
	}
	if resp.GetData() == nil {
		return nil, fmt.Errorf("%w: sys id %d", ErrProductNotFound, sysID)
	}
	c.cachePut(resp.GetData())
	return resp.GetData(), nil
}

// BatchGetProducts resolves many product sys ids at once. Warm cache hits are
// served locally; only the misses hit finance in a single batch call.
func (c *Client) BatchGetProducts(ctx context.Context, sysIDs []int64) ([]*financev1.CostMasterProduct, error) {
	if c.degraded {
		return nil, ErrDegraded
	}

	result := make([]*financev1.CostMasterProduct, 0, len(sysIDs))
	missing := make([]int64, 0, len(sysIDs))
	for _, id := range sysIDs {
		if p, ok := c.cacheGet(id); ok {
			result = append(result, p)
			continue
		}
		missing = append(missing, id)
	}
	if len(missing) == 0 {
		return result, nil
	}

	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	resp, err := c.cc.BatchGetCostProductMaster(callCtx, &financev1.BatchGetCostProductMasterRequest{ProductSysIds: missing})
	if err != nil {
		return nil, fmt.Errorf("batch get cost product master: %w", err)
	}
	for _, p := range resp.GetData() {
		c.cachePut(p)
		result = append(result, p)
	}
	return result, nil
}

// ListProducts returns a paginated product-picker projection. Not cached.
func (c *Client) ListProducts(ctx context.Context, req *financev1.ListCostProductMasterForPPCRequest) (*financev1.ListCostProductMasterForPPCResponse, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	resp, err := c.cc.ListCostProductMasterForPPC(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("list cost product master: %w", err)
	}
	return resp, nil
}

// GetProductRoute returns the released route projection for a product. Not cached.
func (c *Client) GetProductRoute(ctx context.Context, sysID int64) (*financev1.GetProductRouteForPPCResponse, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	resp, err := c.cc.GetProductRouteForPPC(callCtx, &financev1.GetProductRouteForPPCRequest{ProductSysId: sysID})
	if err != nil {
		return nil, fmt.Errorf("get product route %d: %w", sysID, err)
	}
	return resp, nil
}

// ListGrades returns a paginated product-grade picker projection. Not cached.
func (c *Client) ListGrades(ctx context.Context, req *financev1.ListProductGradesForPPCRequest) (*financev1.ListProductGradesForPPCResponse, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	resp, err := c.cc.ListProductGradesForPPC(callCtx, req)
	if err != nil {
		return nil, fmt.Errorf("list product grades: %w", err)
	}
	return resp, nil
}

// ParameterDef is a typed projection of an mst_parameter definition served by
// finance's product-parameter master. PPC consumes these to render the WO
// parameter selector (filtered by display_group) and pin well-known param codes.
type ParameterDef struct {
	ParamID              string
	ParamCode            string
	ParamName            string
	ParamShortName       string
	DataType             string // NUMBER / TEXT / BOOLEAN
	ParamCategory        string // INPUT / RATE / CALCULATED / MASTER_LOOKUP
	DisplayGroup         string // Spec / Machine / Grade / Packing / Cost / ...
	LookupMasterCode     string
	UomID                string
	UomCode              string
	DefaultValue         string // decimal-as-string (resolution fallback)
	MinValue             string
	MaxValue             string
	DisplayOrder         int32
	IsRequiredForCosting bool
	IsActive             bool
}

// ParameterValue is a typed per-product parameter value (cost_product_parameter),
// resolved against a param definition by data type.
type ParameterValue struct {
	ProductSysID int64
	ParamID      string
	ParamCode    string
	DataType     string // NUMBER / TEXT / BOOLEAN
	ValueNumeric string // decimal-as-string
	ValueText    string
	ValueFlag    bool
}

// ListProductParameters returns the mst_parameter definitions for a display
// group (empty = all groups), used to drive the WO parameter selector. Not
// cached — the caller controls call frequency. Returns ErrDegraded when the
// client is disabled.
func (c *Client) ListProductParameters(ctx context.Context, displayGroup string) ([]ParameterDef, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	defs := make([]ParameterDef, 0)
	page := int32(1)
	const pageSize = int32(100)
	for {
		resp, err := c.cc.ListProductParametersForPPC(callCtx, &financev1.ListProductParametersForPPCRequest{
			Page:         page,
			PageSize:     pageSize,
			DisplayGroup: displayGroup,
		})
		if err != nil {
			return nil, fmt.Errorf("list product parameters (group=%q): %w", displayGroup, err)
		}
		for _, d := range resp.GetData() {
			defs = append(defs, parameterDefFromProto(d))
		}
		if len(resp.GetData()) < int(pageSize) {
			break
		}
		page++
	}
	return defs, nil
}

// BatchGetProductParameterValues resolves per-product parameter values for the
// given product sys ids, optionally filtered to specific param UUIDs (empty =
// all). Not cached. Returns ErrDegraded when the client is disabled.
func (c *Client) BatchGetProductParameterValues(ctx context.Context, productSysIDs []int64, paramIDs []string) ([]ParameterValue, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	resp, err := c.cc.BatchGetProductParameterValues(callCtx, &financev1.BatchGetProductParameterValuesRequest{
		ProductSysIds: productSysIDs,
		ParamIds:      paramIDs,
	})
	if err != nil {
		return nil, fmt.Errorf("batch get product parameter values: %w", err)
	}
	values := make([]ParameterValue, 0, len(resp.GetData()))
	for _, v := range resp.GetData() {
		values = append(values, parameterValueFromProto(v))
	}
	return values, nil
}

func parameterDefFromProto(d *financev1.CostMasterProductParameterDef) ParameterDef {
	return ParameterDef{
		ParamID:              d.GetParamId(),
		ParamCode:            d.GetParamCode(),
		ParamName:            d.GetParamName(),
		ParamShortName:       d.GetParamShortName(),
		DataType:             d.GetDataType(),
		ParamCategory:        d.GetParamCategory(),
		DisplayGroup:         d.GetDisplayGroup(),
		LookupMasterCode:     d.GetLookupMasterCode(),
		UomID:                d.GetUomId(),
		UomCode:              d.GetUomCode(),
		DefaultValue:         d.GetDefaultValue(),
		MinValue:             d.GetMinValue(),
		MaxValue:             d.GetMaxValue(),
		DisplayOrder:         d.GetDisplayOrder(),
		IsRequiredForCosting: d.GetIsRequiredForCosting(),
		IsActive:             d.GetIsActive(),
	}
}

func parameterValueFromProto(v *financev1.CostMasterProductParameterValue) ParameterValue {
	return ParameterValue{
		ProductSysID: v.GetProductSysId(),
		ParamID:      v.GetParamId(),
		ParamCode:    v.GetParamCode(),
		DataType:     v.GetDataType(),
		ValueNumeric: v.GetValueNumeric(),
		ValueText:    v.GetValueText(),
		ValueFlag:    v.GetValueFlag(),
	}
}

// ValidateProduct asserts that a cost-product-master sys id exists and is active
// in finance. This is the write-path guard: in degraded mode it returns
// ErrProductValidationUnavailable rather than letting an unvalidated id persist.
// Read and display paths should call ValidateProductForDisplay instead.
func (c *Client) ValidateProduct(ctx context.Context, sysID int64) error {
	if c.degraded {
		log.Warn().Int64("cpm_product_sys_id", sysID).Msg("financeclient degraded: refusing write-path product validation")
		return ErrProductValidationUnavailable
	}
	return c.validateProduct(ctx, sysID)
}

// ValidateProductForDisplay is the read-path variant of ValidateProduct: in
// degraded mode it logs a warning and reports success so listing and rendering
// keep working while finance is unreachable. Never use it before a write.
func (c *Client) ValidateProductForDisplay(ctx context.Context, sysID int64) error {
	if c.degraded {
		log.Warn().Int64("cpm_product_sys_id", sysID).Msg("financeclient degraded: skipping display-path product validation")
		return nil
	}
	return c.validateProduct(ctx, sysID)
}

// validateProduct is the shared non-degraded validation body.
func (c *Client) validateProduct(ctx context.Context, sysID int64) error {
	if sysID <= 0 {
		return ErrProductNotFound
	}
	product, err := c.GetProduct(ctx, sysID)
	if err != nil {
		return err
	}
	if !product.GetIsActive() {
		return fmt.Errorf("%w: sys id %d is inactive", ErrProductInactive, sysID)
	}
	return nil
}

// ErpCodePair is one (erp_item_code, shade_code) lookup key sent to finance.
// Matching is trimmed and case-insensitive on both components; ShadeCode may be
// empty.
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
	Product    *financev1.CostMasterProduct
}

// MaxErpCodePairs mirrors the finance-side cap on one resolve request.
const MaxErpCodePairs = 500

// ResolveProductsByErpCode resolves (erp_item_code, shade_code) pairs to cost
// product masters over gRPC. ppc_db and finance_db are separate databases, so
// this RPC is the only way to link Orion staging rows to finance products.
// The response has one resolution per requested pair, in request order.
// Requests larger than MaxErpCodePairs are chunked transparently.
func (c *Client) ResolveProductsByErpCode(ctx context.Context, pairs []ErpCodePair) ([]ErpCodeResolution, error) {
	if c.degraded {
		return nil, ErrDegraded
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	out := make([]ErpCodeResolution, 0, len(pairs))
	for start := 0; start < len(pairs); start += MaxErpCodePairs {
		end := start + MaxErpCodePairs
		if end > len(pairs) {
			end = len(pairs)
		}
		chunk, err := c.resolveErpChunk(ctx, pairs[start:end])
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
	}
	return out, nil
}

// resolveErpChunk issues one resolve RPC for at most MaxErpCodePairs pairs and
// caches every unique match it learns about.
func (c *Client) resolveErpChunk(ctx context.Context, pairs []ErpCodePair) ([]ErpCodeResolution, error) {
	callCtx, cancel := c.outgoingContext(ctx)
	defer cancel()

	reqPairs := make([]*financev1.ErpCodePair, 0, len(pairs))
	for _, p := range pairs {
		reqPairs = append(reqPairs, &financev1.ErpCodePair{
			ErpItemCode: p.ErpItemCode,
			ShadeCode:   p.ShadeCode,
		})
	}
	resp, err := c.cc.ResolveCostProductMasterByErpCode(callCtx, &financev1.ResolveCostProductMasterByErpCodeRequest{Pairs: reqPairs})
	if err != nil {
		return nil, fmt.Errorf("resolve cost product master by erp code: %w", err)
	}
	resolutions := resp.GetResolutions()
	out := make([]ErpCodeResolution, 0, len(resolutions))
	for _, r := range resolutions {
		res := ErpCodeResolution{
			Pair: ErpCodePair{
				ErpItemCode: r.GetPair().GetErpItemCode(),
				ShadeCode:   r.GetPair().GetShadeCode(),
			},
			MatchCount: r.GetMatchCount(),
			Product:    r.GetProduct(),
		}
		if res.Product != nil {
			c.cachePut(res.Product)
		}
		out = append(out, res)
	}
	return out, nil
}

func (c *Client) cacheGet(sysID int64) (*financev1.CostMasterProduct, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.cache[sysID]
	if !ok || time.Now().After(entry.expiresAt) {
		return nil, false
	}
	return entry.product, true
}

func (c *Client) cachePut(product *financev1.CostMasterProduct) {
	if product == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[product.GetProductSysId()] = cacheEntry{
		product:   product,
		expiresAt: time.Now().Add(productCacheTTL),
	}
}

// metadataCarrier adapts gRPC metadata.MD to propagation.TextMapCarrier so the
// W3C TraceContext propagator can write trace headers into outgoing metadata.
type metadataCarrier metadata.MD

// Get returns the first value for key, or "" if absent.
func (c metadataCarrier) Get(key string) string {
	vals := metadata.MD(c).Get(key)
	if len(vals) == 0 {
		return ""
	}
	return vals[0]
}

// Set overwrites key with value.
func (c metadataCarrier) Set(key, value string) {
	metadata.MD(c).Set(key, value)
}

// Keys returns all metadata keys.
func (c metadataCarrier) Keys() []string {
	keys := make([]string, 0, len(c))
	for k := range c {
		keys = append(keys, k)
	}
	return keys
}
