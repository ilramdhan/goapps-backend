package postgres

import (
	"context"
	"fmt"
	"strings"

	"github.com/lib/pq"

	cppdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// OilGroupPolicyRepository implements costproductparameter.OilGroupPolicy by
// joining product → type (cpt_oil_class) → cost_product_type_oil_group →
// v_rm_group_oil (oil-cost-rm-group §4.5).
type OilGroupPolicyRepository struct {
	db *DB
}

// NewOilGroupPolicyRepository constructs the repo.
func NewOilGroupPolicyRepository(db *DB) *OilGroupPolicyRepository {
	return &OilGroupPolicyRepository{db: db}
}

var _ cppdomain.OilGroupPolicy = (*OilGroupPolicyRepository)(nil)

// oilGroupRulesQuery returns one row per product whose type has an oil class.
// Allowed/default codes only include groups that are currently oil groups
// (flagged, active, not deleted — v_rm_group_oil).
const oilGroupRulesQuery = `
SELECT pm.cpm_product_sys_id,
       pt.cpt_type_code,
       pt.cpt_oil_class,
       COALESCE(string_agg(v.group_code, ',' ORDER BY v.group_code)
                FILTER (WHERE og.cptog_is_default AND v.group_code IS NOT NULL), '') AS default_code,
       COALESCE(string_agg(v.group_code, ',' ORDER BY v.group_code)
                FILTER (WHERE v.group_code IS NOT NULL), '')                         AS allowed_codes
FROM cost_product_master pm
JOIN cost_product_type pt
  ON pt.cpt_type_id = pm.cpm_product_type_id
 AND pt.cpt_oil_class IS NOT NULL
LEFT JOIN cost_product_type_oil_group og ON og.cptog_type_id = pt.cpt_type_id
LEFT JOIN v_rm_group_oil v
  ON v.group_head_id = og.cptog_group_head_id
 AND v.deleted_at IS NULL
WHERE pm.cpm_product_sys_id = ANY($1)
GROUP BY pm.cpm_product_sys_id, pt.cpt_type_code, pt.cpt_oil_class`

// RuleForProduct returns the rule for one product; nil when its type has no oil class.
func (r *OilGroupPolicyRepository) RuleForProduct(ctx context.Context, productSysID int64) (*cppdomain.OilGroupRule, error) {
	rules, err := r.RulesForProducts(ctx, []int64{productSysID})
	if err != nil {
		return nil, err
	}
	return rules[productSysID], nil
}

// RulesForProducts returns rules keyed by product sys id. Products whose type
// has no oil class (or that do not exist) are absent from the map.
func (r *OilGroupPolicyRepository) RulesForProducts(ctx context.Context, productSysIDs []int64) (map[int64]*cppdomain.OilGroupRule, error) {
	out := make(map[int64]*cppdomain.OilGroupRule, len(productSysIDs))
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := r.db.QueryContext(ctx, oilGroupRulesQuery, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("query oil group rules: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			pid                            int64
			typeCode, oilClass, def, allow string
		)
		if err := rows.Scan(&pid, &typeCode, &oilClass, &def, &allow); err != nil {
			return nil, fmt.Errorf("scan oil group rule: %w", err)
		}
		out[pid] = &cppdomain.OilGroupRule{
			TypeCode: typeCode,
			OilClass: oilClass,
			Allowed:  splitOilCodes(allow),
			Default:  firstOilCode(splitOilCodes(def)),
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate oil group rules: %w", err)
	}
	return out, nil
}

func splitOilCodes(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstOilCode(s []string) string {
	if len(s) == 0 {
		return ""
	}
	return s[0]
}
