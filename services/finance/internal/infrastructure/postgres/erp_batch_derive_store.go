package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/lib/pq"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erp_batch_derive_store.go adds the erpintegration.DeriveStore methods to the
// transaction-scoped erpBatchStore (plan-05 P4-T4). Every read runs inside the
// G11-locked step transaction.

var _ erpintegration.DeriveStore = (*erpBatchStore)(nil)

// ListCoverage returns the batch's coverage rows ordered by (item, shade).
func (s *erpBatchStore) ListCoverage(ctx context.Context) ([]erpintegration.CoverageLine, error) {
	return listCoverage(ctx, s.tx, s.batchID)
}

// IsPeriodLocked is the G10 check inside the step transaction (FOR SHARE).
func (s *erpBatchStore) IsPeriodLocked(ctx context.Context, period, calcType string) (bool, error) {
	return IsPeriodLockedForShare(ctx, s.tx, period, calcType)
}

// LoadAxComponents returns the AX inputs of the given cost ids (see
// erpintegration.DeriveStore).
func (s *erpBatchStore) LoadAxComponents(ctx context.Context, period string, costIDs []int64) (map[int64]erpintegration.AxComponents, error) {
	return loadAxComponents(ctx, s.tx, period, costIDs)
}

// ShadeNames returns cost_erp_shade names keyed by trimmed upper code.
func (s *erpBatchStore) ShadeNames(ctx context.Context, shadeCodes []string) (map[string]string, error) {
	return shadeNames(ctx, s.tx, shadeCodes)
}

// ReplaceStdRows replaces the batch's std rows; a later Save keeps them.
func (s *erpBatchStore) ReplaceStdRows(ctx context.Context, period string, rows []erpintegration.StdRow) (int64, error) {
	n, err := replaceStdRows(ctx, s.tx, s.batchID, period, rows)
	if err != nil {
		return 0, err
	}
	s.replacedStd = true
	return n, nil
}

// listCoverage is the tx-scoped, unfiltered form of ErpCoverageRepository.List.
func listCoverage(ctx context.Context, q erpQuerier, batchID int64) (out []erpintegration.CoverageLine, err error) {
	rows, err := q.QueryContext(ctx, erpCoverageListSQL, batchID, pq.Array([]string{}))
	if err != nil {
		return nil, fmt.Errorf("erp coverage list: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp coverage list")
	for rows.Next() {
		l, err := scanCoverageLine(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp coverage list rows: %w", err)
	}
	return out, nil
}

// erpAxComponentsSQL reads the ACTUAL APPROVED cost rows of the period for
// the given ids with the product's ERP attributes (000551). Rows no longer
// APPROVED, of another period, or without cpc_total_rm_cost are absent.
const erpAxComponentsSQL = `
	SELECT c.cpc_cost_id, c.cpc_version, c.cpc_product_sys_id,
	       c.cpc_cost_per_unit::text, c.cpc_total_rm_cost::text,
	       COALESCE(m.cpm_erp_fg_type, ''), COALESCE(m.cpm_erp_chp_item_code, ''),
	       COALESCE(m.cpm_erp_ms_batch_item, ''), COALESCE(m.cpm_erp_item_type, ''),
	       m.cpm_erp_prd_per_day::text
	  FROM cst_product_cost c
	  JOIN cost_product_master m ON m.cpm_product_sys_id = c.cpc_product_sys_id
	 WHERE c.cpc_cost_id = ANY($1::bigint[])
	   AND c.cpc_period = $2
	   AND c.cpc_calculation_type = 'ACTUAL'
	   AND c.cpc_status = 'APPROVED'
	   AND c.cpc_total_rm_cost IS NOT NULL`

func loadAxComponents(ctx context.Context, q erpQuerier, period string, costIDs []int64) (out map[int64]erpintegration.AxComponents, err error) {
	out = make(map[int64]erpintegration.AxComponents, len(costIDs))
	if len(costIDs) == 0 {
		return out, nil
	}
	if err := erpintegration.ValidateBatchPeriod(period); err != nil {
		return nil, fmt.Errorf("erp ax components: %w", err)
	}
	rows, err := q.QueryContext(ctx, erpAxComponentsSQL, pq.Array(costIDs), period)
	if err != nil {
		return nil, fmt.Errorf("erp ax components: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp ax components")
	for rows.Next() {
		a, err := scanAxComponents(rows)
		if err != nil {
			return nil, err
		}
		out[a.CostID] = a
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp ax components rows: %w", err)
	}
	return out, nil
}

func scanAxComponents(s rowScanner) (erpintegration.AxComponents, error) {
	var (
		a       erpintegration.AxComponents
		cpu, rm string
		prdDay  sql.NullString
	)
	if err := s.Scan(&a.CostID, &a.Version, &a.ProductSysID, &cpu, &rm,
		&a.FgType, &a.ChpItemCode, &a.MsBatchItem, &a.ItemType, &prdDay); err != nil {
		return erpintegration.AxComponents{}, fmt.Errorf("erp ax components scan: %w", err)
	}
	var err error
	if a.CostPerUnit, err = erpintegration.ParseDecimal(cpu); err != nil {
		return erpintegration.AxComponents{}, fmt.Errorf("erp ax cost_per_unit %d: %w", a.CostID, err)
	}
	if a.TotalRMCost, err = erpintegration.ParseDecimal(rm); err != nil {
		return erpintegration.AxComponents{}, fmt.Errorf("erp ax total_rm_cost %d: %w", a.CostID, err)
	}
	nd, err := parseNullDecimalText(prdDay)
	if err != nil {
		return erpintegration.AxComponents{}, fmt.Errorf("erp ax prd_per_day %d: %w", a.CostID, err)
	}
	if nd.Valid {
		d := nd.Decimal
		a.PrdPerDay = &d
	}
	return a, nil
}

const erpShadeNamesSQL = `
	SELECT UPPER(TRIM(ces_shade_code)), COALESCE(ces_shade_name, '')
	  FROM cost_erp_shade
	 WHERE UPPER(TRIM(ces_shade_code)) = ANY($1::text[])`

func shadeNames(ctx context.Context, q erpQuerier, shadeCodes []string) (out map[string]string, err error) {
	out = make(map[string]string, len(shadeCodes))
	keys := make([]string, 0, len(shadeCodes))
	for _, c := range shadeCodes {
		if k := strings.ToUpper(strings.TrimSpace(c)); k != "" {
			keys = append(keys, k)
		}
	}
	if len(keys) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, erpShadeNamesSQL, pq.Array(keys))
	if err != nil {
		return nil, fmt.Errorf("erp shade names: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp shade names")
	for rows.Next() {
		var code, name string
		if err := rows.Scan(&code, &name); err != nil {
			return nil, fmt.Errorf("erp shade names scan: %w", err)
		}
		out[code] = name
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp shade names rows: %w", err)
	}
	return out, nil
}
