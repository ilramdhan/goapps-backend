package costcalc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"
)

// loadOilContextQuery reads the oil context of every requested product whose
// type carries an oil class (migration 000521). One row per product:
//   - oil_name:      stored OIL_NAME text (trimmed, "" when absent);
//   - default_code:  the type's default oil RM group (at most one, enforced
//     by uk_cptog_type_default);
//   - allowed_codes: comma-joined allowed oil RM groups of the type.
//
// Only groups still flagged is_oil_group and not soft-deleted count, so an
// un-flagged or deleted group drops out of the allowed set and a product still
// pointing at it is blocked by resolveOilRate rather than silently costed.
const loadOilContextQuery = `
	SELECT pm.cpm_product_sys_id,
	       pt.cpt_type_code,
	       pt.cpt_oil_class,
	       COALESCE(btrim(cpp.cpp_value_text), '')                                      AS oil_name,
	       COALESCE(string_agg(gh.group_code, ',') FILTER (WHERE og.cptog_is_default), '') AS default_code,
	       COALESCE(string_agg(gh.group_code, ','), '')                                 AS allowed_codes
	FROM cost_product_master pm
	JOIN cost_product_type pt
	     ON pt.cpt_type_id = pm.cpm_product_type_id
	    AND pt.cpt_oil_class IS NOT NULL
	LEFT JOIN mst_parameter mp
	     ON mp.param_code = 'OIL_NAME'
	    AND mp.deleted_at IS NULL
	LEFT JOIN cost_product_parameter cpp
	     ON cpp.cpp_product_sys_id = pm.cpm_product_sys_id
	    AND cpp.cpp_param_id = mp.id
	LEFT JOIN cost_product_type_oil_group og
	     ON og.cptog_type_id = pt.cpt_type_id
	LEFT JOIN cst_rm_group_head gh
	     ON gh.group_head_id = og.cptog_group_head_id
	    AND gh.deleted_at IS NULL
	    AND gh.is_oil_group
	WHERE pm.cpm_product_sys_id = ANY($1)
	GROUP BY 1, 2, 3, 4`

// LoadOilContext implements ProductLoader.LoadOilContext with a single query
// per chunk. See loadOilContextQuery for the row shape.
func (l *productLoader) LoadOilContext(ctx context.Context, productSysIDs []int64) (map[int64]*OilInput, error) {
	defer observeLoad(loaderKindOilContext, time.Now())
	out := map[int64]*OilInput{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := l.db.QueryContext(ctx, loadOilContextQuery, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load oil context: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			typeCode     string
			oilClass     string
			oilName      string
			defaultCode  string
			allowedCodes string
		)
		if err := rows.Scan(&productSysID, &typeCode, &oilClass, &oilName, &defaultCode, &allowedCodes); err != nil {
			return nil, fmt.Errorf("scan oil context row: %w", err)
		}
		out[productSysID] = newOilInput(typeCode, oilClass, oilName, defaultCode, allowedCodes)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate oil context rows: %w", err)
	}
	return out, nil
}

// newOilInput assembles an OilInput from one loadOilContextQuery row. The
// default code takes the first entry of the aggregate (the partial unique
// index allows at most one default per type); the allowed codes become a set.
func newOilInput(typeCode, oilClass, oilName, defaultCode, allowedCodes string) *OilInput {
	allowed := map[string]bool{}
	for _, c := range strings.Split(allowedCodes, ",") {
		if c = strings.TrimSpace(c); c != "" {
			allowed[c] = true
		}
	}
	def := ""
	if parts := strings.Split(defaultCode, ","); len(parts) > 0 {
		def = strings.TrimSpace(parts[0])
	}
	return &OilInput{
		Class:        strings.TrimSpace(oilClass),
		TypeCode:     strings.TrimSpace(typeCode),
		GroupCode:    strings.TrimSpace(oilName),
		DefaultGroup: def,
		Allowed:      allowed,
	}
}

// OilGroupNameLoader resolves RM group codes to their display names. It is a
// small optional capability rather than a ProductLoader method, so existing
// ProductLoader fakes are unaffected; GetRouteCostSheetHandler type-checks the
// loader for it and, when absent, exports the raw OIL_NAME code (D18 fallback).
type OilGroupNameLoader interface {
	// LoadRMGroupNames returns group_name keyed by group_code for the given
	// codes. Codes with no active (non-deleted) group are simply absent.
	LoadRMGroupNames(ctx context.Context, codes []string) (map[string]string, error)
}

// LoadRMGroupNames implements OilGroupNameLoader.
func (l *productLoader) LoadRMGroupNames(ctx context.Context, codes []string) (map[string]string, error) {
	out := map[string]string{}
	if len(codes) == 0 {
		return out, nil
	}
	const q = `
		SELECT group_code, group_name
		FROM cst_rm_group_head
		WHERE group_code = ANY($1)
		  AND deleted_at IS NULL`
	rows, err := l.db.QueryContext(ctx, q, pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("load RM group names: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var code, name string
		if err := rows.Scan(&code, &name); err != nil {
			return nil, fmt.Errorf("scan RM group name row: %w", err)
		}
		out[code] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate RM group name rows: %w", err)
	}
	return out, nil
}
