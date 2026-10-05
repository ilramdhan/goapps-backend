package postgres

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// erp_batch_validate_store.go adds the erpintegration.ValidateStore methods
// to the transaction-scoped erpBatchStore (plan-06 P5-T2). Every statement
// runs inside the G11-locked step transaction; all are PG-only.
//
// cesc_validation holds Derive's issues followed by the validate-step
// findings; the latter carry "src":"validate" so a re-validation reads only
// Derive's issues back (ListStdRowsForValidation) and recomputes the rest.
// The marker is an extra JSON key: scanStdRow ignores it, so the other
// readers see both kinds as ordinary issues.

var _ erpintegration.ValidateStore = (*erpBatchStore)(nil)

// stdIssueSrcValidate marks a validate-step issue in cesc_validation.
const stdIssueSrcValidate = "validate"

// erpStdValidationIssuesExpr keeps only the issues without the validate
// marker (Derive's).
const erpStdValidationIssuesExpr = `COALESCE((
	         SELECT jsonb_agg(e ORDER BY o)
	           FROM jsonb_array_elements(cesc_validation) WITH ORDINALITY AS a(e, o)
	          WHERE COALESCE(e->>'src', '') <> '` + stdIssueSrcValidate + `'), '[]'::jsonb)::text`

// erpStdListForValidationSQL is erpStdListSQL with Derive's issues only.
var erpStdListForValidationSQL = strings.Replace(erpStdListSQL,
	"cesc_status, cesc_validation::text", "cesc_status, "+erpStdValidationIssuesExpr, 1)

// ListStdRowsForValidation returns the batch's std rows with Derive's issues.
func (s *erpBatchStore) ListStdRowsForValidation(ctx context.Context) (out []erpintegration.StdRow, err error) {
	rows, err := s.tx.QueryContext(ctx, erpStdListForValidationSQL, s.batchID)
	if err != nil {
		return nil, fmt.Errorf("erp std list for validation: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp std list for validation")
	for rows.Next() {
		r, err := scanStdRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp std list for validation rows: %w", err)
	}
	return out, nil
}

// stdValidationIssueJSON is stdIssueJSON plus the source marker.
type stdValidationIssueJSON struct {
	Code     string `json:"code"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
	Src      string `json:"src,omitempty"`
}

func validationJSON(derived, validation []erpintegration.Issue) (string, error) {
	out := make([]stdValidationIssueJSON, 0, len(derived)+len(validation))
	for _, is := range derived {
		out = append(out, stdValidationIssueJSON{Code: string(is.Code), Severity: string(is.Severity), Message: is.Message})
	}
	for _, is := range validation {
		out = append(out, stdValidationIssueJSON{
			Code: string(is.Code), Severity: string(is.Severity), Message: is.Message, Src: stdIssueSrcValidate,
		})
	}
	b, err := json.Marshal(out)
	if err != nil {
		return "", fmt.Errorf("erp std validation json: %w", err)
	}
	return string(b), nil
}

const erpStdUpdateValidationSQL = `
	UPDATE cst_erp_std_cost c
	   SET cesc_validation = u.validation, cesc_prev_std_cost = u.prev
	  FROM unnest($2::text[], $3::text[], $4::text[], $5::jsonb[], $6::numeric[])
	       AS u(item, grade, shade, validation, prev)
	 WHERE c.cesc_batch_id = $1
	   AND c.cesc_item_code = u.item AND c.cesc_grade_code = u.grade AND c.cesc_shade_code = u.shade`

// UpdateStdValidation writes cesc_validation and cesc_prev_std_cost.
func (s *erpBatchStore) UpdateStdValidation(ctx context.Context, updates []erpintegration.StdValidationUpdate) (int64, error) {
	var n int64
	for start := 0; start < len(updates); start += erpStdInsertChunk {
		end := min(start+erpStdInsertChunk, len(updates))
		c, err := s.updateStdValidationChunk(ctx, updates[start:end])
		if err != nil {
			return 0, fmt.Errorf("erp std validation rows %d..%d: %w", start, end-1, err)
		}
		n += c
	}
	return n, nil
}

func (s *erpBatchStore) updateStdValidationChunk(ctx context.Context, us []erpintegration.StdValidationUpdate) (int64, error) {
	item, grade, shade, val := make([]string, len(us)), make([]string, len(us)), make([]string, len(us)), make([]string, len(us))
	prev := make([]sql.NullString, len(us))
	for i, u := range us {
		item[i], grade[i], shade[i] = u.Key.ItemCode, u.Key.GradeCode, u.Key.ShadeCode
		v, err := validationJSON(u.Derived, u.Validation)
		if err != nil {
			return 0, err
		}
		val[i] = v
		prev[i] = nullDecimalText(u.PrevStd)
	}
	res, err := s.tx.ExecContext(ctx, erpStdUpdateValidationSQL, s.batchID,
		pq.Array(item), pq.Array(grade), pq.Array(shade), pq.Array(val), pq.Array(prev))
	if err != nil {
		return 0, err
	}
	c, err := res.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("rows affected: %w", err)
	}
	return c, nil
}

const (
	erpReplicaItemsSQL  = `SELECT cei_item_code FROM cost_erp_item WHERE cei_item_code = ANY($1::text[])`
	erpReplicaShadesSQL = `SELECT ces_shade_code FROM cost_erp_shade WHERE ces_shade_code = ANY($1::text[])`
	erpReplicaGradesSQL = `SELECT ceg_grade_code FROM cost_erp_grade WHERE ceg_grade_code = ANY($1::text[])`
)

// ReplicaCodes returns which codes exist in the PG ERP master replicas
// (exact match, active or not: V-06 checks existence).
func (s *erpBatchStore) ReplicaCodes(ctx context.Context, items, shades, grades []string) (erpintegration.ReplicaCodes, error) {
	var rc erpintegration.ReplicaCodes
	var err error
	if rc.Items, err = codeSet(ctx, s.tx, erpReplicaItemsSQL, items, "cost_erp_item"); err != nil {
		return erpintegration.ReplicaCodes{}, err
	}
	if rc.Shades, err = codeSet(ctx, s.tx, erpReplicaShadesSQL, shades, "cost_erp_shade"); err != nil {
		return erpintegration.ReplicaCodes{}, err
	}
	if rc.Grades, err = codeSet(ctx, s.tx, erpReplicaGradesSQL, grades, "cost_erp_grade"); err != nil {
		return erpintegration.ReplicaCodes{}, err
	}
	return rc, nil
}

func codeSet(ctx context.Context, q erpQuerier, query string, codes []string, label string) (out map[string]struct{}, err error) {
	out = map[string]struct{}{}
	if len(codes) == 0 {
		return out, nil
	}
	rows, err := q.QueryContext(ctx, query, pq.Array(codes))
	if err != nil {
		return nil, fmt.Errorf("erp replica %s: %w", label, err)
	}
	defer closeRowsInto(rows, &err, "erp replica "+label)
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return nil, fmt.Errorf("erp replica %s scan: %w", label, err)
		}
		out[c] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp replica %s rows: %w", label, err)
	}
	return out, nil
}

const erpSourceCostLabelsSQL = `
	SELECT cpc_cost_id, cpc_currency_code, cpc_cost_per_unit::text
	  FROM cst_product_cost
	 WHERE cpc_cost_id = ANY($1::bigint[])`

// SourceCostLabels returns the currency label and cost per unit per cost id.
func (s *erpBatchStore) SourceCostLabels(ctx context.Context, costIDs []int64) (out map[int64]erpintegration.CostLabel, err error) {
	out = make(map[int64]erpintegration.CostLabel, len(costIDs))
	if len(costIDs) == 0 {
		return out, nil
	}
	rows, err := s.tx.QueryContext(ctx, erpSourceCostLabelsSQL, pq.Array(costIDs))
	if err != nil {
		return nil, fmt.Errorf("erp source cost labels: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp source cost labels")
	for rows.Next() {
		var (
			id       int64
			cur, cpu string
		)
		if err := rows.Scan(&id, &cur, &cpu); err != nil {
			return nil, fmt.Errorf("erp source cost labels scan: %w", err)
		}
		v, err := decimal.NewFromString(cpu)
		if err != nil {
			return nil, fmt.Errorf("erp source cost %d value: %w", id, err)
		}
		out[id] = erpintegration.CostLabel{Currency: cur, CostPerUnit: v}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp source cost labels rows: %w", err)
	}
	return out, nil
}

// erpRelabelMarker is the ccrl_migration value written by migration 000557.
const erpRelabelMarker = "migration:000557"

// erpRelabelAppliedSQL is false when the log table does not exist (000557
// not applied): to_regclass avoids an error that would abort the tx.
const erpRelabelAppliedSQL = `SELECT to_regclass('cst_currency_relabel_log') IS NOT NULL`

const erpRelabelRowsSQL = `
	SELECT EXISTS (SELECT 1 FROM cst_currency_relabel_log
	                WHERE ccrl_period = $1 AND ccrl_migration = $2)`

// CurrencyRelabelApplied reports whether 000557 relabelled the period.
func (s *erpBatchStore) CurrencyRelabelApplied(ctx context.Context, period string) (bool, error) {
	var exists bool
	if err := s.tx.QueryRowContext(ctx, erpRelabelAppliedSQL).Scan(&exists); err != nil {
		return false, fmt.Errorf("erp relabel log probe: %w", err)
	}
	if !exists {
		return false, nil
	}
	var applied bool
	if err := s.tx.QueryRowContext(ctx, erpRelabelRowsSQL, period, erpRelabelMarker).Scan(&applied); err != nil {
		return false, fmt.Errorf("erp relabel log: %w", err)
	}
	return applied, nil
}

// erpValidationBaselineSQL is the previous active batch: the latest LIVE
// VALUATED / RECONCILED / LOCKED batch of the same or an earlier period,
// never the batch itself.
const erpValidationBaselineSQL = `
	SELECT ceib_batch_id, ceib_period, COALESCE(ceib_rule_snapshot::text, '')
	  FROM cst_erp_int_batch
	 WHERE ceib_mode = 'LIVE'
	   AND ceib_status IN ('VALUATED', 'RECONCILED', 'LOCKED')
	   AND ceib_period <= $1
	   AND ceib_batch_id <> $2
	 ORDER BY ceib_period DESC, ceib_seq DESC
	 LIMIT 1`

const erpBaselineStdSQL = `
	SELECT cesc_item_code, cesc_grade_code, cesc_shade_code, cesc_std_cost::text
	  FROM cst_erp_std_cost
	 WHERE cesc_batch_id = $1 AND cesc_status = 'OK' AND cesc_std_cost IS NOT NULL`

// FindValidationBaseline returns the V-05 baseline, or nil when none.
func (s *erpBatchStore) FindValidationBaseline(ctx context.Context, period string) (*erpintegration.ValidationBaseline, error) {
	var (
		bl   erpintegration.ValidationBaseline
		snap string
	)
	err := s.tx.QueryRowContext(ctx, erpValidationBaselineSQL, period, s.batchID).Scan(&bl.BatchID, &bl.Period, &snap)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil //nolint:nilnil // no baseline is a valid outcome (first run)
	}
	if err != nil {
		return nil, fmt.Errorf("erp validation baseline: %w", err)
	}
	if snap != "" {
		bl.RuleSnapshot = []byte(snap)
	}
	if bl.Std, err = baselineStd(ctx, s.tx, bl.BatchID); err != nil {
		return nil, err
	}
	return &bl, nil
}

func baselineStd(ctx context.Context, q erpQuerier, batchID int64) (out map[erpintegration.ErpKey]decimal.Decimal, err error) {
	rows, err := q.QueryContext(ctx, erpBaselineStdSQL, batchID)
	if err != nil {
		return nil, fmt.Errorf("erp baseline std: %w", err)
	}
	defer closeRowsInto(rows, &err, "erp baseline std")
	out = map[erpintegration.ErpKey]decimal.Decimal{}
	for rows.Next() {
		var (
			k   erpintegration.ErpKey
			std string
		)
		if err := rows.Scan(&k.ItemCode, &k.GradeCode, &k.ShadeCode, &std); err != nil {
			return nil, fmt.Errorf("erp baseline std scan: %w", err)
		}
		v, err := decimal.NewFromString(std)
		if err != nil {
			return nil, fmt.Errorf("erp baseline std %s: %w", k.String(), err)
		}
		out[k] = v
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("erp baseline std rows: %w", err)
	}
	return out, nil
}
