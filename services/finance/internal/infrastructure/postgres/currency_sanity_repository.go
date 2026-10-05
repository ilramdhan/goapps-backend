package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	erpapp "github.com/mutugading/goapps-backend/services/finance/internal/application/erpintegration"
)

// CurrencySanityRepository serves the T-CUR report (design Part 2 §7.3,
// P0-T10a). Every statement is a SELECT; it never writes. NUMERIC values are
// scanned as text into shopspring decimals (no float round trip).
type CurrencySanityRepository struct {
	db *DB
}

// NewCurrencySanityRepository constructs the repository.
func NewCurrencySanityRepository(db *DB) *CurrencySanityRepository {
	return &CurrencySanityRepository{db: db}
}

var _ erpapp.CurrencySanityReader = (*CurrencySanityRepository)(nil)

// currencySanityBase selects active ACTUAL rows. $1 = period (” = all),
// $2 = linked_only. Callers append further predicates.
const currencySanityBase = `
		FROM cst_product_cost cpc
		JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpc.cpc_product_sys_id
		WHERE cpc.cpc_calculation_type = 'ACTUAL'
		  AND cpc.cpc_status <> 'SUPERSEDED'
		  AND ($1 = '' OR cpc.cpc_period = $1)
		  AND (NOT $2::boolean OR cpm.cpm_erp_item_code IS NOT NULL)`

// currencyOutlierPredicate is the §7.3 outlier rule in SQL; it must stay equal
// to erpintegration.IsCurrencyOutlier. $3 = guarded prefixes, $4 = prefix
// threshold, $5 = overall threshold.
const currencyOutlierPredicate = `
		  AND (cpc.cpc_cost_per_unit > $5::numeric
		       OR (upper(left(COALESCE(cpm.cpm_erp_item_code, ''), 3)) = ANY($3::text[])
		           AND cpc.cpc_cost_per_unit > $4::numeric))`

const currencyLabelDistributionQuery = `
		SELECT cpc.cpc_period, cpc.cpc_currency_code, count(*)` + currencySanityBase + `
		GROUP BY 1, 2
		ORDER BY 1, 2`

const currencyPercentilesQuery = `
		SELECT count(*),
		       min(cpc.cpc_cost_per_unit)::text,
		       (percentile_cont(0.50) WITHIN GROUP (ORDER BY cpc.cpc_cost_per_unit))::numeric(20,6)::text,
		       (percentile_cont(0.90) WITHIN GROUP (ORDER BY cpc.cpc_cost_per_unit))::numeric(20,6)::text,
		       (percentile_cont(0.99) WITHIN GROUP (ORDER BY cpc.cpc_cost_per_unit))::numeric(20,6)::text,
		       max(cpc.cpc_cost_per_unit)::text` + currencySanityBase

const currencyOutlierCountQuery = `
		SELECT count(*)` + currencySanityBase + currencyOutlierPredicate

const currencyOutlierListQuery = `
		SELECT cpc.cpc_cost_id, cpc.cpc_product_sys_id, cpm.cpm_product_code,
		       COALESCE(cpm.cpm_erp_item_code, ''), cpc.cpc_period, cpc.cpc_currency_code,
		       cpc.cpc_status, cpc.cpc_cost_per_unit::text` + currencySanityBase + currencyOutlierPredicate + `
		ORDER BY cpc.cpc_period, cpc.cpc_cost_per_unit DESC, cpc.cpc_cost_id
		LIMIT $6`

// currencyPeriodSummaryQuery is the per-period input for the 000557 guard. It
// deliberately ignores LinkedOnly: the relabel guard counts every active
// ACTUAL row of the period. $1 = period (” = all), $2..$4 as above.
const currencyPeriodSummaryQuery = `
		SELECT cpc.cpc_period,
		       count(*),
		       count(*) FILTER (WHERE cpc.cpc_status = 'APPROVED'),
		       count(*) FILTER (WHERE cpc.cpc_currency_code = 'IDR'),
		       count(*) FILTER (WHERE cpc.cpc_currency_code = 'USD'),
		       max(cpc.cpc_cost_per_unit)::text,
		       count(*) FILTER (WHERE cpc.cpc_cost_per_unit > $4::numeric),
		       count(*) FILTER (WHERE cpc.cpc_cost_per_unit > $4::numeric
		                         OR (upper(left(COALESCE(cpm.cpm_erp_item_code, ''), 3)) = ANY($2::text[])
		                             AND cpc.cpc_cost_per_unit > $3::numeric))
		FROM cst_product_cost cpc
		LEFT JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpc.cpc_product_sys_id
		WHERE cpc.cpc_calculation_type = 'ACTUAL'
		  AND cpc.cpc_status <> 'SUPERSEDED'
		  AND ($1 = '' OR cpc.cpc_period = $1)
		GROUP BY cpc.cpc_period
		ORDER BY cpc.cpc_period`

// LabelDistribution returns row counts per (period, label).
func (r *CurrencySanityRepository) LabelDistribution(ctx context.Context, f erpapp.CurrencySanityFilter) ([]erpapp.CurrencyLabelCount, error) {
	rows, err := r.db.QueryContext(ctx, currencyLabelDistributionQuery, f.Period, f.LinkedOnly)
	if err != nil {
		return nil, fmt.Errorf("currency label distribution: %w", err)
	}
	defer closeRows(rows)
	var out []erpapp.CurrencyLabelCount
	for rows.Next() {
		var c erpapp.CurrencyLabelCount
		if err := rows.Scan(&c.Period, &c.Currency, &c.Rows); err != nil {
			return nil, fmt.Errorf("scan currency label: %w", err)
		}
		out = append(out, c)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate currency labels: %w", err)
	}
	return out, nil
}

// Percentiles returns min/p50/p90/p99/max of cpc_cost_per_unit.
func (r *CurrencySanityRepository) Percentiles(ctx context.Context, f erpapp.CurrencySanityFilter) (erpapp.CurrencyPercentiles, error) {
	var (
		p                         erpapp.CurrencyPercentiles
		minV, p50, p90, p99, maxV sql.NullString
	)
	if err := r.db.QueryRowContext(ctx, currencyPercentilesQuery, f.Period, f.LinkedOnly).
		Scan(&p.Rows, &minV, &p50, &p90, &p99, &maxV); err != nil {
		return p, fmt.Errorf("currency percentiles: %w", err)
	}
	var err error
	for _, pair := range []struct {
		dst *decimal.NullDecimal
		src sql.NullString
	}{{&p.Min, minV}, {&p.P50, p50}, {&p.P90, p90}, {&p.P99, p99}, {&p.Max, maxV}} {
		if *pair.dst, err = nullStringDecimal(pair.src); err != nil {
			return p, fmt.Errorf("currency percentiles: %w", err)
		}
	}
	return p, nil
}

// ListOutliers returns up to f.OutlierLimit outlier rows plus the exact total.
func (r *CurrencySanityRepository) ListOutliers(ctx context.Context, f erpapp.CurrencySanityFilter) ([]erpapp.CurrencyOutlier, int64, error) {
	prefixes := pq.Array(erpapp.GuardedItemPrefixes)
	prefixT := erpapp.GuardedPrefixThreshold.String()
	overallT := erpapp.OverallThreshold.String()

	var total int64
	if err := r.db.QueryRowContext(ctx, currencyOutlierCountQuery,
		f.Period, f.LinkedOnly, prefixes, prefixT, overallT).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count currency outliers: %w", err)
	}
	limit := f.OutlierLimit
	if limit <= 0 {
		limit = erpapp.DefaultOutlierLimit
	}
	rows, err := r.db.QueryContext(ctx, currencyOutlierListQuery,
		f.Period, f.LinkedOnly, prefixes, prefixT, overallT, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("list currency outliers: %w", err)
	}
	defer closeRows(rows)
	var out []erpapp.CurrencyOutlier
	for rows.Next() {
		var (
			o    erpapp.CurrencyOutlier
			cost string
		)
		if err := rows.Scan(&o.CostID, &o.ProductSysID, &o.ProductCode, &o.ErpItemCode,
			&o.Period, &o.Currency, &o.Status, &cost); err != nil {
			return nil, 0, fmt.Errorf("scan currency outlier: %w", err)
		}
		if o.CostPerUnit, err = decimal.NewFromString(cost); err != nil {
			return nil, 0, fmt.Errorf("parse currency outlier cost %q: %w", cost, err)
		}
		o.Threshold = erpapp.OverallThreshold
		if erpapp.IsGuardedItem(o.ErpItemCode) && !o.CostPerUnit.GreaterThan(erpapp.OverallThreshold) {
			o.Threshold = erpapp.GuardedPrefixThreshold
		}
		out = append(out, o)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate currency outliers: %w", err)
	}
	return out, total, nil
}

// PeriodSummaries returns the per-period 000557 guard inputs.
func (r *CurrencySanityRepository) PeriodSummaries(ctx context.Context, period string) ([]erpapp.CurrencyPeriodSummary, error) {
	rows, err := r.db.QueryContext(ctx, currencyPeriodSummaryQuery, period,
		pq.Array(erpapp.GuardedItemPrefixes),
		erpapp.GuardedPrefixThreshold.String(), erpapp.OverallThreshold.String())
	if err != nil {
		return nil, fmt.Errorf("currency period summaries: %w", err)
	}
	defer closeRows(rows)
	var out []erpapp.CurrencyPeriodSummary
	for rows.Next() {
		var (
			s    erpapp.CurrencyPeriodSummary
			maxV sql.NullString
		)
		if err := rows.Scan(&s.Period, &s.Rows, &s.ApprovedRows, &s.IDRRows, &s.USDRows,
			&maxV, &s.OverOverall, &s.OutlierRows); err != nil {
			return nil, fmt.Errorf("scan currency period summary: %w", err)
		}
		nd, err := nullStringDecimal(maxV)
		if err != nil {
			return nil, fmt.Errorf("currency period summary max: %w", err)
		}
		s.Max = nd.Decimal
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate currency period summaries: %w", err)
	}
	return out, nil
}

func nullStringDecimal(s sql.NullString) (decimal.NullDecimal, error) {
	if !s.Valid {
		return decimal.NullDecimal{}, nil
	}
	d, err := decimal.NewFromString(s.String)
	if err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("parse decimal %q: %w", s.String, err)
	}
	return decimal.NullDecimal{Decimal: d, Valid: true}, nil
}
