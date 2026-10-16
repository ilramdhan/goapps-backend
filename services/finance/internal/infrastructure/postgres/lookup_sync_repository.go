package postgres

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/lookupsync"
)

// lookupSyncQuerier is the subset of *sql.DB / *sql.Tx the repository needs, so
// the same implementation serves pooled reads (preview) and the worker's
// tx-scoped re-plan.
type lookupSyncQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// LookupSyncRepository implements lookupsync.SyncReader.
type LookupSyncRepository struct {
	q lookupSyncQuerier
}

var _ lookupsync.SyncReader = (*LookupSyncRepository)(nil)

// NewLookupSyncRepository creates a pool-backed reader.
func NewLookupSyncRepository(db *DB) *LookupSyncRepository {
	return &LookupSyncRepository{q: db.DB}
}

// ForTx returns a reader bound to tx, so reads happen inside the caller's
// transaction (the worker's re-plan sees the rows it is about to write).
func (r *LookupSyncRepository) ForTx(tx *sql.Tx) *LookupSyncRepository {
	return &LookupSyncRepository{q: tx}
}

// ListTriggerRows implements lookupsync.SyncReader (keyset paging by product).
func (r *LookupSyncRepository) ListTriggerRows(ctx context.Context, scope lookupsync.Scope, afterProductSysID int64, limit int) ([]lookupsync.TriggerRow, error) {
	if limit < 1 {
		limit = 1
	}
	args := []any{pq.Array(lookupsync.SyncableMasters), afterProductSysID}
	next := func(v any) string {
		args = append(args, v)
		return fmt.Sprintf("$%d", len(args))
	}

	var where strings.Builder
	if ids := scope.ProductSysIDs; ids != nil {
		where.WriteString(" AND cpm.cpm_product_sys_id = ANY(" + next(pq.Array(ids)) + "::bigint[])")
	}
	if scope.Filter != nil {
		writeLookupSyncFilter(&where, scope.Filter, next)
	}
	if scope.MasterRef != nil {
		writeLookupSyncMasterRef(&where, scope.MasterRef, next)
	}
	limitArg := next(limit)

	q := `
WITH trig AS (
	SELECT cpm.cpm_product_sys_id AS pid, cpm.cpm_product_code AS pcode, cpm.cpm_product_name AS pname,
	       cpm.cpm_is_locked AS locked,
	       mp.id AS param_id, mp.param_code AS param_code, mp.lookup_master_code AS master_code,
	       COALESCE(cpp.cpp_value_text, '') AS stored_key, cpp.cpp_value_mb_spin_id AS spin_id
	FROM cost_product_master cpm
	JOIN cost_product_applicable_param capp ON capp.capp_product_sys_id = cpm.cpm_product_sys_id
	JOIN mst_parameter mp ON mp.id = capp.capp_param_id
	     AND mp.deleted_at IS NULL AND mp.lookup_master_code = ANY($1::text[])
	LEFT JOIN cost_product_parameter cpp ON cpp.cpp_product_sys_id = cpm.cpm_product_sys_id
	     AND cpp.cpp_param_id = mp.id
	WHERE cpm.cpm_product_sys_id > $2` + where.String() + `
)
SELECT pid, pcode, pname, locked, param_id, param_code, master_code, stored_key, spin_id
FROM trig
WHERE pid IN (SELECT DISTINCT pid FROM trig ORDER BY pid LIMIT ` + limitArg + `)
ORDER BY pid, param_code`

	rows, err := r.q.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, fmt.Errorf("list lookup sync trigger rows: %w", err)
	}
	defer closeRows(rows)

	var out []lookupsync.TriggerRow
	for rows.Next() {
		var tr lookupsync.TriggerRow
		var spin uuid.NullUUID
		if err := rows.Scan(&tr.ProductSysID, &tr.ProductCode, &tr.ProductName, &tr.IsLocked,
			&tr.TriggerParamID, &tr.TriggerParamCode, &tr.LookupMasterCode, &tr.StoredKey, &spin); err != nil {
			return nil, fmt.Errorf("scan lookup sync trigger row: %w", err)
		}
		if spin.Valid {
			id := spin.UUID
			tr.MBSpinID = &id
		}
		out = append(out, tr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup sync trigger rows: %w", err)
	}
	return out, nil
}

// LoadChildStates implements lookupsync.SyncReader (applicable cells only).
func (r *LookupSyncRepository) LoadChildStates(ctx context.Context, productSysIDs []int64, paramIDs []uuid.UUID) (map[lookupsync.ProductParamKey]lookupsync.ChildState, error) {
	out := map[lookupsync.ProductParamKey]lookupsync.ChildState{}
	if len(productSysIDs) == 0 || len(paramIDs) == 0 {
		return out, nil
	}
	pids := make([]string, len(paramIDs))
	for i, id := range paramIDs {
		pids[i] = id.String()
	}
	rows, err := r.q.QueryContext(ctx, `
SELECT capp.capp_product_sys_id, capp.capp_param_id,
       cpp.cpp_value_numeric, cpp.cpp_value_text, cpp.cpp_value_flag,
       (cpp.cpp_value_id IS NOT NULL)
FROM cost_product_applicable_param capp
LEFT JOIN cost_product_parameter cpp ON cpp.cpp_product_sys_id = capp.capp_product_sys_id
     AND cpp.cpp_param_id = capp.capp_param_id
WHERE capp.capp_product_sys_id = ANY($1::bigint[]) AND capp.capp_param_id = ANY($2::uuid[])`,
		pq.Array(productSysIDs), pq.Array(pids))
	if err != nil {
		return nil, fmt.Errorf("load lookup sync child states: %w", err)
	}
	defer closeRows(rows)

	for rows.Next() {
		var (
			pid    int64
			param  uuid.UUID
			num    decimal.NullDecimal
			text   sql.NullString
			flag   sql.NullBool
			exists bool
		)
		if err := rows.Scan(&pid, &param, &num, &text, &flag, &exists); err != nil {
			return nil, fmt.Errorf("scan lookup sync child state: %w", err)
		}
		st := lookupsync.ChildState{}
		if exists {
			sv := &lookupsync.StoredValue{}
			if num.Valid {
				d := num.Decimal
				sv.Numeric = &d
			}
			if text.Valid {
				s := text.String
				sv.Text = &s
			}
			if flag.Valid {
				b := flag.Bool
				sv.Flag = &b
			}
			st.Stored = sv
		}
		out[lookupsync.ProductParamKey{ProductSysID: pid, ParamID: param}] = st
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate lookup sync child states: %w", err)
	}
	return out, nil
}

// CountMBSpinsByOrionCode implements lookupsync.SyncReader.
func (r *LookupSyncRepository) CountMBSpinsByOrionCode(ctx context.Context, code string) (int, error) {
	var n int
	if err := r.q.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM mst_mb_spin WHERE mbs_orion_item_code = $1 AND deleted_at IS NULL`, code).Scan(&n); err != nil {
		return 0, fmt.Errorf("count mb spins by orion code: %w", err)
	}
	return n, nil
}

// ProductGradeCodeByName implements lookupsync.SyncReader (ok=false on 0 or >1).
func (r *LookupSyncRepository) ProductGradeCodeByName(ctx context.Context, name string) (string, bool, error) {
	rows, err := r.q.QueryContext(ctx,
		`SELECT pg_code FROM mst_product_grade WHERE pg_name = $1 AND deleted_at IS NULL LIMIT 2`, name)
	if err != nil {
		return "", false, fmt.Errorf("product grade code by name: %w", err)
	}
	defer closeRows(rows)
	var codes []string
	for rows.Next() {
		var c string
		if err := rows.Scan(&c); err != nil {
			return "", false, fmt.Errorf("scan product grade code: %w", err)
		}
		codes = append(codes, c)
	}
	if err := rows.Err(); err != nil {
		return "", false, fmt.Errorf("iterate product grade codes: %w", err)
	}
	if len(codes) != 1 {
		return "", false, nil
	}
	return codes[0], true, nil
}

// writeLookupSyncFilter replicates CostProductMasterRepository.List filter SQL
// (cost_product_master_repository.go): search / type ids / shade / active.
func writeLookupSyncFilter(where *strings.Builder, f *lookupsync.ProductFilter, next func(any) string) {
	// Replicates CostProductMasterRepository.List filter SQL
	// (cost_product_master_repository.go): search / type ids / shade / active.
	if f.Search != "" {
		p := next("%" + f.Search + "%")
		where.WriteString(` AND (LOWER(cpm.cpm_product_code) LIKE LOWER(` + p + `) OR LOWER(cpm.cpm_product_name) LIKE LOWER(` + p +
			`) OR LOWER(COALESCE(cpm.cpm_erp_item_code,'')) LIKE LOWER(` + p + `) OR LOWER(COALESCE(cpm.cpm_flex_02,'')) LIKE LOWER(` + p + `))`)
	}
	if len(f.ProductTypeIDs) > 0 {
		where.WriteString(" AND cpm.cpm_product_type_id = ANY(" + next(pq.Array(f.ProductTypeIDs)) + "::int[])")
	}
	if f.ShadeCode != "" {
		where.WriteString(" AND cpm.cpm_shade_code = " + next(f.ShadeCode))
	}
	switch f.ActiveFilter {
	case filterActive:
		where.WriteString(" AND cpm.cpm_is_active = TRUE")
	case filterInactive:
		where.WriteString(" AND cpm.cpm_is_active = FALSE")
	}
}

// writeLookupSyncMasterRef narrows trigger cells to those pointing at one master record.
func writeLookupSyncMasterRef(where *strings.Builder, m *lookupsync.MasterRef, next func(any) string) {
	code := next(m.LookupMasterCode)
	key := next(m.MasterKey)
	where.WriteString(" AND mp.lookup_master_code = " + code)
	switch m.LookupMasterCode {
	case lookupsync.MasterProductGrade:
		// Stored value is the pg_code, or (legacy) the grade name when that
		// name identifies exactly this grade.
		where.WriteString(` AND (cpp.cpp_value_text = ` + key + ` OR cpp.cpp_value_text IN (
			SELECT g.pg_name FROM mst_product_grade g
			WHERE g.pg_code = ` + key + ` AND g.deleted_at IS NULL
			  AND (SELECT COUNT(*) FROM mst_product_grade g2 WHERE g2.pg_name = g.pg_name AND g2.deleted_at IS NULL) = 1))`)
	case lookupsync.MasterMBSpin:
		// Products store COALESCE(orion code, mbs_id) in text; the permanent
		// pointer is cpp_value_mb_spin_id. A non-UUID key can only match text.
		if id, err := uuid.Parse(m.MasterKey); err == nil {
			idArg := next(id)
			where.WriteString(` AND (cpp.cpp_value_mb_spin_id = ` + idArg + `::uuid OR cpp.cpp_value_text = ` + key + ` OR cpp.cpp_value_text IN (
				SELECT s.mbs_orion_item_code FROM mst_mb_spin s
				WHERE s.mbs_id = ` + idArg + `::uuid AND s.deleted_at IS NULL AND s.mbs_orion_item_code IS NOT NULL))`)
		} else {
			where.WriteString(" AND cpp.cpp_value_text = " + key)
		}
	default:
		where.WriteString(" AND cpp.cpp_value_text = " + key)
	}
}
