package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

// ErpRuleSetLoader implements erprule.RuleSetLoader and erprule.RuleHashUsage
// (design Part 1 §5.2, Part 2 §6.2, §6.6; plan-03 P2-T3, P2-T4).
//
// LoadRuleSet reads every active valloss rule, every active sell price and
// every grade with a non-NULL ceg_grade_group inside ONE read-only
// REPEATABLE READ transaction, so the three reads see the same snapshot even
// while Finance edits rules concurrently.
//
// Grade groups come from cost_erp_grade.ceg_grade_group only (design §6.2:
// "cost_erp_grade.ceg_grade_group, else NO_GRADE_GROUP"). The
// cst_erp_grade_group_seed mapping reaches that column through 000548 and
// the P0-T15b re-apply after a master sync; it is not read here.
type ErpRuleSetLoader struct{ db *DB }

// NewErpRuleSetLoader constructs the loader.
func NewErpRuleSetLoader(db *DB) *ErpRuleSetLoader {
	return &ErpRuleSetLoader{db: db}
}

var (
	_ erprule.RuleSetLoader = (*ErpRuleSetLoader)(nil)
	_ erprule.RuleHashUsage = (*ErpRuleSetLoader)(nil)
)

const (
	loadActiveRulesSQL = `
SELECT cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_basis, cevr_val_loss::text
  FROM cst_erp_valloss_rule
 WHERE cevr_is_active
 ORDER BY cevr_fg_type, cevr_prod_type, cevr_grade_group, cevr_id`

	loadActivePricesSQL = `
SELECT cesp_basis, cesp_price::text
  FROM cst_erp_sell_price
 WHERE cesp_is_active
 ORDER BY cesp_basis`

	loadGradeAssignmentsSQL = `
SELECT ceg_grade_code, ceg_grade_group
  FROM cost_erp_grade
 WHERE ceg_grade_group IS NOT NULL
 ORDER BY ceg_grade_code`

	// ruleHashInUseSQL: a non-terminal batch between DERIVED and VALUATED
	// (chk_ceib_status order: DERIVED, VALIDATED, PUSHED, VALUATED) that was
	// derived with the hash. SHADOW batches count too: deleting a rule under
	// a running backtest would make its result unreproducible.
	ruleHashInUseSQL = `
SELECT EXISTS (
	SELECT 1 FROM cst_erp_int_batch
	 WHERE ceib_rule_hash = $1
	   AND ceib_status IN ('DERIVED', 'VALIDATED', 'PUSHED', 'VALUATED'))`
)

// LoadRuleSet builds the immutable RuleSet from one REPEATABLE READ snapshot.
// Stored data the domain rejects (for example an unknown group written to
// ceg_grade_group outside GoApps) fails the load rather than being skipped.
func (l *ErpRuleSetLoader) LoadRuleSet(ctx context.Context) (rs *erprule.RuleSet, err error) {
	tx, err := l.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return nil, fmt.Errorf("load rule set: begin: %w", err)
	}
	defer func() {
		// Read-only: rollback just ends the snapshot.
		if rbErr := tx.Rollback(); rbErr != nil && !errors.Is(rbErr, sql.ErrTxDone) && err == nil {
			err = fmt.Errorf("load rule set: end snapshot: %w", rbErr)
		}
	}()

	rules, err := loadRules(ctx, tx)
	if err != nil {
		return nil, err
	}
	prices, err := loadPrices(ctx, tx)
	if err != nil {
		return nil, err
	}
	grades, err := loadGradeAssignments(ctx, tx)
	if err != nil {
		return nil, err
	}
	out, err := erprule.NewRuleSet(rules, prices, grades)
	if err != nil {
		return nil, fmt.Errorf("load rule set: %w", err)
	}
	return out, nil
}

// IsRuleHashInUse reports whether a DERIVED..VALUATED batch was derived with
// ruleHash. An empty hash is never in use.
func (l *ErpRuleSetLoader) IsRuleHashInUse(ctx context.Context, ruleHash string) (bool, error) {
	if ruleHash == "" {
		return false, nil
	}
	var used bool
	if err := l.db.QueryRowContext(ctx, ruleHashInUseSQL, ruleHash).Scan(&used); err != nil {
		return false, fmt.Errorf("rule hash in use: %w", err)
	}
	return used, nil
}

func loadRules(ctx context.Context, tx *sql.Tx) (out []erprule.Rule, err error) {
	rows, err := tx.QueryContext(ctx, loadActiveRulesSQL)
	if err != nil {
		return nil, fmt.Errorf("load rules: %w", err)
	}
	defer closeRowsInto(rows, &err, "load rules")
	for rows.Next() {
		var fgType, prodType, group, basis, valLossText string
		if err := rows.Scan(&fgType, &prodType, &group, &basis, &valLossText); err != nil {
			return nil, fmt.Errorf("load rules scan: %w", err)
		}
		v, err := decimal.NewFromString(valLossText)
		if err != nil {
			return nil, fmt.Errorf("load rules: parse cevr_val_loss %q: %w", valLossText, err)
		}
		out = append(out, erprule.Rule{
			Key: erprule.RuleKey{
				FgType:     erprule.FgType(fgType),
				ProdType:   erprule.ProdType(prodType),
				GradeGroup: erprule.GradeGroup(group),
			},
			Basis:   erprule.Basis(basis),
			ValLoss: v,
		})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load rules rows: %w", err)
	}
	return out, nil
}

func loadPrices(ctx context.Context, tx *sql.Tx) (out []erprule.Price, err error) {
	rows, err := tx.QueryContext(ctx, loadActivePricesSQL)
	if err != nil {
		return nil, fmt.Errorf("load prices: %w", err)
	}
	defer closeRowsInto(rows, &err, "load prices")
	for rows.Next() {
		var basis, priceText string
		if err := rows.Scan(&basis, &priceText); err != nil {
			return nil, fmt.Errorf("load prices scan: %w", err)
		}
		p, err := decimal.NewFromString(priceText)
		if err != nil {
			return nil, fmt.Errorf("load prices: parse cesp_price %q: %w", priceText, err)
		}
		out = append(out, erprule.Price{Basis: erprule.Basis(basis), Price: p})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load prices rows: %w", err)
	}
	return out, nil
}

func loadGradeAssignments(ctx context.Context, tx *sql.Tx) (out []erprule.GradeAssignment, err error) {
	rows, err := tx.QueryContext(ctx, loadGradeAssignmentsSQL)
	if err != nil {
		return nil, fmt.Errorf("load grade groups: %w", err)
	}
	defer closeRowsInto(rows, &err, "load grade groups")
	for rows.Next() {
		var code, group string
		if err := rows.Scan(&code, &group); err != nil {
			return nil, fmt.Errorf("load grade groups scan: %w", err)
		}
		out = append(out, erprule.GradeAssignment{GradeCode: code, Group: erprule.GradeGroup(group)})
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("load grade groups rows: %w", err)
	}
	return out, nil
}

// closeRowsInto closes rows and, when no earlier error is set, records a
// close error into *err.
func closeRowsInto(rows *sql.Rows, err *error, what string) {
	if cerr := rows.Close(); cerr != nil && *err == nil {
		*err = fmt.Errorf("%s close: %w", what, cerr)
	}
}
