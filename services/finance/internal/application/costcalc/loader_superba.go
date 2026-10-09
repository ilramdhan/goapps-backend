package costcalc

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/lib/pq"
)

// ScopeKeySuperbaMBCost is the reserved engine-injected key consumed by
// F_YARN_MB_COST (migration 000567) for SUPERBA products. Like IS_POY it is
// deliberately NOT an mst_parameter row and has no formula_param edge.
const ScopeKeySuperbaMBCost = "SUPERBA_MB_COST"

// SuperbaCost is the per-product Superba Cost SP resolution for a SUPERBA-class
// product. Found is false when the product's shade has no active master row
// (or the product has no shade code); ComputeProduct then blocks the product.
type SuperbaCost struct {
	// ShadeCode is the product's cpm_shade_code as stored (trimmed), for messages.
	ShadeCode string
	// Found reports whether an active cost_superba_cost_sp row matched.
	Found bool
	// OldValue is the master's old_value (the value the calc always uses).
	OldValue float64
	// ColourName is the master's Superba colour name (display only).
	ColourName string
	// LegacySysID is the chosen row's legacy_sys_id (largest on duplicate shades).
	LegacySysID int64
}

// SuperbaCostSource resolves the SUPERBA MB cost per product. The default
// implementation reads the Superba Cost SP master; a future formula-based
// source can replace it without touching the expression or ComputeProduct.
// Only SUPERBA-class products appear in the result.
type SuperbaCostSource interface {
	ResolveSuperbaCost(ctx context.Context, productSysIDs []int64) (map[int64]*SuperbaCost, error)
}

// loadSuperbaCostQuery returns one row per SUPERBA-class product (same
// cost_product_master -> cost_product_type join as loadOilContextQuery).
// Shades are matched on UPPER(TRIM()); a duplicate shade resolves to the row
// with the largest legacy_sys_id (LATERAL ... ORDER BY ... LIMIT 1), mirroring
// superbacostsp ResolveByShades. sc.id IS NULL means no active row matched.
const loadSuperbaCostQuery = `
	SELECT pm.cpm_product_sys_id,
	       COALESCE(btrim(pm.cpm_shade_code), '')  AS shade_code,
	       sc.id IS NOT NULL                       AS found,
	       COALESCE(sc.old_value, 0)               AS old_value,
	       COALESCE(sc.colour_name, '')            AS colour_name,
	       COALESCE(sc.legacy_sys_id, 0)           AS legacy_sys_id
	FROM cost_product_master pm
	JOIN cost_product_type pt
	     ON pt.cpt_type_id = pm.cpm_product_type_id
	    AND pt.cpt_oil_class = 'SUPERBA'
	LEFT JOIN LATERAL (
	     SELECT s.id, s.old_value, s.colour_name, s.legacy_sys_id
	     FROM cost_superba_cost_sp s
	     WHERE s.is_active AND s.deleted_at IS NULL
	       AND UPPER(TRIM(s.shade_code)) = UPPER(TRIM(pm.cpm_shade_code))
	     ORDER BY s.legacy_sys_id DESC
	     LIMIT 1
	) sc ON TRUE
	WHERE pm.cpm_product_sys_id = ANY($1)`

// masterSuperbaCostSource is the default SuperbaCostSource (master table).
type masterSuperbaCostSource struct {
	db *sql.DB
}

// ResolveSuperbaCost implements SuperbaCostSource with a single query per chunk.
func (m *masterSuperbaCostSource) ResolveSuperbaCost(ctx context.Context, productSysIDs []int64) (map[int64]*SuperbaCost, error) {
	defer observeLoad(loaderKindSuperbaCost, time.Now())
	out := map[int64]*SuperbaCost{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := m.db.QueryContext(ctx, loadSuperbaCostQuery, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load superba cost: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			pid int64
			c   SuperbaCost
		)
		if err := rows.Scan(&pid, &c.ShadeCode, &c.Found, &c.OldValue, &c.ColourName, &c.LegacySysID); err != nil {
			return nil, fmt.Errorf("scan superba cost row: %w", err)
		}
		out[pid] = &c
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate superba cost rows: %w", err)
	}
	return out, nil
}

// LoadSuperbaCost implements ProductLoader.LoadSuperbaCost by delegating to the
// configured SuperbaCostSource (default: the master table).
func (l *productLoader) LoadSuperbaCost(ctx context.Context, productSysIDs []int64) (map[int64]*SuperbaCost, error) {
	src := l.superba
	if src == nil {
		src = &masterSuperbaCostSource{db: l.db}
	}
	return src.ResolveSuperbaCost(ctx, productSysIDs)
}

// SuperbaColourLoader is a small optional capability (like OilGroupNameLoader) so
// existing ProductLoader fakes are unaffected: it returns the Superba colour name
// per resolved SUPERBA product, for display-only overrides of MB_SP_DYE.
type SuperbaColourLoader interface {
	LoadSuperbaColours(ctx context.Context, productSysIDs []int64) (map[int64]string, error)
}

// LoadSuperbaColours implements SuperbaColourLoader. Only SUPERBA-class products
// with a resolved master row and a non-empty colour name appear in the result.
func (l *productLoader) LoadSuperbaColours(ctx context.Context, productSysIDs []int64) (map[int64]string, error) {
	res, err := l.LoadSuperbaCost(ctx, productSysIDs)
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for pid, c := range res {
		if c != nil && c.Found && c.ColourName != "" {
			out[pid] = c.ColourName
		}
	}
	return out, nil
}

// NewSuperbaColourLoader returns the default SuperbaColourLoader over db, for
// callers outside the calc engine (e.g. the Param tab read model).
func NewSuperbaColourLoader(db *sql.DB) SuperbaColourLoader {
	return &productLoader{db: db}
}

// ParamCodeMBSpDye is the TOP 64 text param whose displayed value is replaced by
// the Superba colour name for SUPERBA products (display only, never stored).
const ParamCodeMBSpDye = "MB_SP_DYE"
