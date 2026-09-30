package costcalc

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

// loaderKindTxWeight labels the TX Weight bulk loader stage in metrics.
const loaderKindTxWeight = "tx_weight"

// txWeightFuncName is the expr-lang built-in the grade-weight formulas call
// since migration 000530: tx_weight('AE', AX_WT, AX_WT * AE_PERC / AX_PERC).
const txWeightFuncName = "tx_weight"

// TxWeightRule is one live mst_yarn_tx_weight row (mode + value) for a grade.
type TxWeightRule struct {
	Mode  yarntxweight.Mode
	Value float64
}

// TxWeightLoader loads the TX Weight rules of each product's type. It is an
// optional capability (same shape as OilGroupNameLoader) rather than a
// ProductLoader method, so existing ProductLoader fakes are unaffected: a
// loader without it yields no rules, and tx_weight() then returns its
// fallback argument — i.e. the pre-TX-Weight ratio formula.
type TxWeightLoader interface {
	// LoadTxWeightRules returns, per product sys id, the live rules keyed by
	// grade code (AE/A9/A/B/C). Products whose type has no rule are absent.
	LoadTxWeightRules(ctx context.Context, productSysIDs []int64) (map[int64]map[string]TxWeightRule, error)
}

const loadTxWeightRulesQuery = `
	SELECT pm.cpm_product_sys_id, tw.ytw_grade, tw.ytw_mode, tw.ytw_value
	FROM cost_product_master pm
	JOIN mst_yarn_tx_weight tw
	     ON tw.ytw_product_type_id = pm.cpm_product_type_id
	    AND tw.deleted_at IS NULL
	WHERE pm.cpm_product_sys_id = ANY($1)`

// LoadTxWeightRules implements TxWeightLoader with one query per chunk.
func (l *productLoader) LoadTxWeightRules(ctx context.Context, productSysIDs []int64) (map[int64]map[string]TxWeightRule, error) {
	defer observeLoad(loaderKindTxWeight, time.Now())
	out := map[int64]map[string]TxWeightRule{}
	if len(productSysIDs) == 0 {
		return out, nil
	}
	rows, err := l.db.QueryContext(ctx, loadTxWeightRulesQuery, pq.Array(productSysIDs))
	if err != nil {
		return nil, fmt.Errorf("load tx weight rules: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()
	for rows.Next() {
		var (
			productSysID int64
			grade, mode  string
			value        float64
		)
		if err := rows.Scan(&productSysID, &grade, &mode, &value); err != nil {
			return nil, fmt.Errorf("scan tx weight rule row: %w", err)
		}
		m, err := yarntxweight.ParseMode(mode)
		if err != nil {
			continue // CHECK constraint makes this unreachable; skip rather than fail the chunk.
		}
		if out[productSysID] == nil {
			out[productSysID] = map[string]TxWeightRule{}
		}
		out[productSysID][strings.ToUpper(strings.TrimSpace(grade))] = TxWeightRule{Mode: m, Value: value}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate tx weight rule rows: %w", err)
	}
	return out, nil
}

// loadTxWeightRules resolves TX Weight rules once per chunk when the loader
// supports it. A loader without the capability yields nil (fallback path).
func (s *Service) loadTxWeightRules(ctx context.Context, products []int64) (map[int64]map[string]TxWeightRule, error) {
	twl, ok := s.loader.(TxWeightLoader)
	if !ok {
		return nil, nil //nolint:nilnil // nil rules is the documented "no capability → fallback" signal, not an error
	}
	rules, err := twl.LoadTxWeightRules(ctx, products)
	if err != nil {
		return nil, fmt.Errorf("load tx weight rules: %w", err)
	}
	return rules, nil
}

// injectTxWeight adds the tx_weight(grade, axWt, fallback) built-in to scope.
// It is ALWAYS injected (rules may be nil) so a rewired formula never calls an
// undefined function: with no rule for the grade it returns fallback, which is
// the original ratio expression — identical to pre-000530 behavior.
//
// With a rule: LESS_BY → axWt − value, MULTIPLY → axWt × value, FIXED → value.
func injectTxWeight(scope map[string]any, rules map[string]TxWeightRule) {
	scope[txWeightFuncName] = func(args ...any) (any, error) {
		return txWeight(rules, args...), nil
	}
}

// txWeight is the pure implementation behind the tx_weight() built-in.
func txWeight(rules map[string]TxWeightRule, args ...any) float64 {
	var fallback float64
	if len(args) >= 3 {
		fallback, _ = toFloat(args[2])
	}
	if len(args) < 2 {
		return fallback
	}
	grade, ok := args[0].(string)
	if !ok {
		return fallback
	}
	rule, found := rules[strings.ToUpper(strings.TrimSpace(grade))]
	if !found {
		return fallback
	}
	axWt, ok := toFloat(args[1])
	if !ok {
		return fallback
	}
	return rule.Mode.Apply(axWt, rule.Value)
}
