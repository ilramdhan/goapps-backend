// Package postgres provides PostgreSQL implementations for domain repositories.
package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// RMCostRepository implements rmcost.Repository using PostgreSQL.
type RMCostRepository struct {
	db *DB
}

// NewRMCostRepository creates a new RMCostRepository instance.
func NewRMCostRepository(db *DB) *RMCostRepository {
	return &RMCostRepository{db: db}
}

// Verify interface implementation at compile time.
var _ rmcost.Repository = (*RMCostRepository)(nil)

// Upsert writes the Cost row keyed on (period, rm_code) and appends one History
// row within the same transaction. On INSERT the row is created with its
// calculated_* and created_* fields; on UPDATE the mutable snapshot columns are
// overwritten and updated_* is set.
func (r *RMCostRepository) Upsert(ctx context.Context, cost *rmcost.Cost, hist rmcost.History) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if err := upsertCost(ctx, tx, cost); err != nil {
			return err
		}
		return insertHistory(ctx, tx, hist)
	})
}

func upsertCost(ctx context.Context, tx *sql.Tx, c *rmcost.Cost) error {
	rates := c.Rates()
	query := `
		INSERT INTO cst_rm_cost (
			rm_cost_id, period, rm_code, rm_type, group_head_id, item_code, rm_name, uom_code,
			cons_rate, stores_rate, dept_rate, po_rate_1, po_rate_2, po_rate_3,
			cost_val, cost_mark, cost_sim,
			flag_valuation, flag_marketing, flag_simulation,
			flag_valuation_used, flag_marketing_used, flag_simulation_used,
			calculated_at, calculated_by, created_at, created_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27)
		ON CONFLICT (period, rm_code) DO UPDATE SET
			group_head_id = EXCLUDED.group_head_id,
			item_code = EXCLUDED.item_code,
			rm_name = EXCLUDED.rm_name,
			uom_code = EXCLUDED.uom_code,
			cons_rate = EXCLUDED.cons_rate,
			stores_rate = EXCLUDED.stores_rate,
			dept_rate = EXCLUDED.dept_rate,
			po_rate_1 = EXCLUDED.po_rate_1,
			po_rate_2 = EXCLUDED.po_rate_2,
			po_rate_3 = EXCLUDED.po_rate_3,
			cost_val = EXCLUDED.cost_val,
			cost_mark = EXCLUDED.cost_mark,
			cost_sim = EXCLUDED.cost_sim,
			flag_valuation = EXCLUDED.flag_valuation,
			flag_marketing = EXCLUDED.flag_marketing,
			flag_simulation = EXCLUDED.flag_simulation,
			flag_valuation_used = EXCLUDED.flag_valuation_used,
			flag_marketing_used = EXCLUDED.flag_marketing_used,
			flag_simulation_used = EXCLUDED.flag_simulation_used,
			calculated_at = EXCLUDED.calculated_at,
			calculated_by = EXCLUDED.calculated_by,
			updated_at = EXCLUDED.calculated_at,
			updated_by = EXCLUDED.calculated_by
	`
	_, err := tx.ExecContext(ctx, query,
		c.ID(), c.Period(), c.RMCode(), c.RMType().String(),
		c.GroupHeadID(), c.ItemCode(), nullableString(c.RMName()), nullableString(c.UOMCode()),
		rates.Cons, rates.Stores, rates.Dept, rates.PO1, rates.PO2, rates.PO3,
		c.CostValuation(), c.CostMarketing(), c.CostSimulation(),
		c.FlagValuation().String(), c.FlagMarketing().String(), c.FlagSimulation().String(),
		c.FlagValuationUsed().String(), c.FlagMarketingUsed().String(), c.FlagSimulationUsed().String(),
		c.CalculatedAt(), c.CalculatedBy(), c.CreatedAt(), c.CreatedBy(),
	)
	if err != nil {
		return fmt.Errorf("upsert rm_cost: %w", err)
	}
	return nil
}

func insertHistory(ctx context.Context, tx *sql.Tx, h rmcost.History) error {
	query := `
		INSERT INTO aud_rm_cost_history (
			history_id, rm_cost_id, job_id, period, rm_code, rm_type, group_head_id,
			cons_rate, stores_rate, dept_rate, po_rate_1, po_rate_2, po_rate_3,
			cost_percentage, cost_per_kg,
			flag_valuation, flag_marketing, flag_simulation,
			init_val_valuation, init_val_marketing, init_val_simulation,
			cost_val, cost_mark, cost_sim,
			flag_valuation_used, flag_marketing_used, flag_simulation_used,
			source_item_count, trigger_reason, calculated_at, calculated_by
		) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,$27,$28,$29,$30,$31)
	`
	_, err := tx.ExecContext(ctx, query,
		h.ID, h.RMCostID, h.JobID, h.Period, h.RMCode, h.RMType.String(), h.GroupHeadID,
		h.Rates.Cons, h.Rates.Stores, h.Rates.Dept, h.Rates.PO1, h.Rates.PO2, h.Rates.PO3,
		h.CostPercentage, h.CostPerKg,
		h.FlagValuation.String(), h.FlagMarketing.String(), h.FlagSimulation.String(),
		h.InitValValuation, h.InitValMarketing, h.InitValSimulation,
		h.CostValuation, h.CostMarketing, h.CostSimulation,
		h.FlagValuationUsed.String(), h.FlagMarketingUsed.String(), h.FlagSimulationUsed.String(),
		h.SourceItemCount, string(h.TriggerReason), h.CalculatedAt, h.CalculatedBy,
	)
	if err != nil {
		return fmt.Errorf("insert rm_cost history: %w", err)
	}
	return nil
}

// GetByID retrieves a Cost by its primary key.
func (r *RMCostRepository) GetByID(ctx context.Context, id uuid.UUID) (*rmcost.Cost, error) {
	return r.scanCost(r.db.QueryRowContext(ctx, costSelectSQL+` WHERE rm_cost_id = $1`, id))
}

// GetByPeriodAndCode retrieves a Cost by (period, rm_code).
func (r *RMCostRepository) GetByPeriodAndCode(ctx context.Context, period, rmCode string) (*rmcost.Cost, error) {
	return r.scanCost(r.db.QueryRowContext(ctx, costSelectSQL+` WHERE period = $1 AND rm_code = $2`, period, rmCode))
}

// List returns a page of Cost rows plus total count.
func (r *RMCostRepository) List(ctx context.Context, filter rmcost.ListFilter) ([]*rmcost.Cost, int64, error) {
	filter.Validate()
	base, args := buildCostListWhere(filter)

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rm_costs: %w", err)
	}

	orderCol := map[string]string{
		"period":         "period",
		"rm_code":        "rm_code",
		"rm_name":        "rm_name",
		"calculated_at":  "calculated_at",
	}[filter.SortBy]
	if orderCol == "" {
		orderCol = "period"
	}
	orderDir := sortDESC
	if filter.SortOrder == "asc" || filter.SortOrder == "ASC" {
		orderDir = sortASC
	}

	argIdx := len(args) + 1
	query := costSelectColumnsSQL + " " + base +
		fmt.Sprintf(` ORDER BY %s %s, rm_code ASC LIMIT $%d OFFSET $%d`, orderCol, orderDir, argIdx, argIdx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list rm_costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*rmcost.Cost
	for rows.Next() {
		cost, err := r.scanCostRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, cost)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rm_costs: %w", err)
	}
	return out, total, nil
}

// ListAll returns every cost row matching the filter with no pagination,
// ordered by period DESC then rm_code ASC. Used by the Excel export path.
func (r *RMCostRepository) ListAll(ctx context.Context, filter rmcost.ExportFilter) ([]*rmcost.Cost, error) {
	base, args := buildCostExportWhere(filter)
	query := costSelectColumnsSQL + " " + base + ` ORDER BY period DESC, rm_code ASC`
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("list all rm_costs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []*rmcost.Cost
	for rows.Next() {
		cost, err := r.scanCostRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, cost)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate all rm_costs: %w", err)
	}
	return out, nil
}

func buildCostExportWhere(f rmcost.ExportFilter) (string, []any) {
	base := `FROM cst_rm_cost WHERE 1=1`
	args := []any{}
	idx := 1
	if f.Period != "" {
		base += fmt.Sprintf(` AND period = $%d`, idx)
		args = append(args, f.Period)
		idx++
	}
	if f.RMType != "" {
		base += fmt.Sprintf(` AND rm_type = $%d`, idx)
		args = append(args, f.RMType.String())
		idx++
	}
	if f.GroupHeadID != nil {
		base += fmt.Sprintf(` AND group_head_id = $%d`, idx)
		args = append(args, *f.GroupHeadID)
		idx++
	}
	if f.Search != "" {
		base += fmt.Sprintf(` AND (rm_code ILIKE $%d OR rm_name ILIKE $%d)`, idx, idx)
		args = append(args, "%"+f.Search+"%")
	}
	return base, args
}

func buildCostListWhere(f rmcost.ListFilter) (string, []any) {
	base := `FROM cst_rm_cost WHERE 1=1`
	args := []any{}
	idx := 1
	if f.Period != "" {
		base += fmt.Sprintf(` AND period = $%d`, idx)
		args = append(args, f.Period)
		idx++
	}
	if f.RMType != "" {
		base += fmt.Sprintf(` AND rm_type = $%d`, idx)
		args = append(args, f.RMType.String())
		idx++
	}
	if f.GroupHeadID != nil {
		base += fmt.Sprintf(` AND group_head_id = $%d`, idx)
		args = append(args, *f.GroupHeadID)
		idx++
	}
	if f.Search != "" {
		base += fmt.Sprintf(` AND (rm_code ILIKE $%d OR rm_name ILIKE $%d)`, idx, idx)
		args = append(args, "%"+f.Search+"%")
	}
	return base, args
}

// ExistsForGroupHead reports whether any cst_rm_cost row currently references
// the given group head. Used to block deletion of group heads with cost data.
func (r *RMCostRepository) ExistsForGroupHead(ctx context.Context, groupHeadID uuid.UUID) (bool, error) {
	var exists bool
	err := r.db.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM cst_rm_cost WHERE group_head_id = $1)`,
		groupHeadID,
	).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check rm_cost exists for group head: %w", err)
	}
	return exists, nil
}

// ListDistinctPeriods returns the distinct periods (YYYYMM) that have cost rows,
// ordered newest first.
func (r *RMCostRepository) ListDistinctPeriods(ctx context.Context) ([]string, error) {
	rows, err := r.db.QueryContext(ctx,
		`SELECT DISTINCT period FROM cst_rm_cost ORDER BY period DESC`,
	)
	if err != nil {
		return nil, fmt.Errorf("list distinct cost periods: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var out []string
	for rows.Next() {
		var p string
		if scanErr := rows.Scan(&p); scanErr != nil {
			return nil, fmt.Errorf("scan period: %w", scanErr)
		}
		out = append(out, p)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		return nil, fmt.Errorf("iterate periods: %w", rowsErr)
	}
	return out, nil
}

// ListHistory returns history rows matching the filter, newest first.
func (r *RMCostRepository) ListHistory(ctx context.Context, filter rmcost.HistoryFilter) ([]rmcost.History, int64, error) {
	filter.Validate()
	base := `FROM aud_rm_cost_history WHERE 1=1`
	args := []any{}
	idx := 1
	if filter.Period != "" {
		base += fmt.Sprintf(` AND period = $%d`, idx)
		args = append(args, filter.Period)
		idx++
	}
	if filter.RMCode != "" {
		base += fmt.Sprintf(` AND rm_code = $%d`, idx)
		args = append(args, filter.RMCode)
		idx++
	}
	if filter.GroupHeadID != nil {
		base += fmt.Sprintf(` AND group_head_id = $%d`, idx)
		args = append(args, *filter.GroupHeadID)
		idx++
	}
	if filter.JobID != nil {
		base += fmt.Sprintf(` AND job_id = $%d`, idx)
		args = append(args, *filter.JobID)
		idx++
	}

	var total int64
	if err := r.db.QueryRowContext(ctx, `SELECT COUNT(*) `+base, args...).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count rm_cost history: %w", err)
	}

	query := historySelectColumnsSQL + " " + base +
		fmt.Sprintf(` ORDER BY calculated_at DESC LIMIT $%d OFFSET $%d`, idx, idx+1)
	args = append(args, filter.PageSize, filter.Offset())

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, 0, fmt.Errorf("list rm_cost history: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var out []rmcost.History
	for rows.Next() {
		h, err := scanHistoryRow(rows)
		if err != nil {
			return nil, 0, err
		}
		out = append(out, h)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, fmt.Errorf("iterate rm_cost history: %w", err)
	}
	return out, total, nil
}

// =============================================================================
// Cost scanning
// =============================================================================

const costSelectColumnsSQL = `
	SELECT rm_cost_id, period, rm_code, rm_type, group_head_id, item_code, rm_name, uom_code,
	       cons_rate, stores_rate, dept_rate, po_rate_1, po_rate_2, po_rate_3,
	       cost_val, cost_mark, cost_sim,
	       flag_valuation, flag_marketing, flag_simulation,
	       flag_valuation_used, flag_marketing_used, flag_simulation_used,
	       calculated_at, calculated_by, created_at, created_by, updated_at, updated_by`

const costSelectSQL = costSelectColumnsSQL + ` FROM cst_rm_cost`

type costDTO struct {
	ID                 uuid.UUID
	Period             string
	RMCode             string
	RMType             string
	GroupHeadID        uuid.NullUUID
	ItemCode           sql.NullString
	RMName             sql.NullString
	UOMCode            sql.NullString
	ConsRate           sql.NullFloat64
	StoresRate         sql.NullFloat64
	DeptRate           sql.NullFloat64
	PO1Rate            sql.NullFloat64
	PO2Rate            sql.NullFloat64
	PO3Rate            sql.NullFloat64
	CostVal            sql.NullFloat64
	CostMark           sql.NullFloat64
	CostSim            sql.NullFloat64
	FlagValuation      string
	FlagMarketing      string
	FlagSimulation     string
	FlagValuationUsed  string
	FlagMarketingUsed  string
	FlagSimulationUsed string
	CalculatedAt       sql.NullTime
	CalculatedBy       sql.NullString
	CreatedAt          time.Time
	CreatedBy          string
	UpdatedAt          sql.NullTime
	UpdatedBy          sql.NullString
}

func (d *costDTO) toEntity() *rmcost.Cost {
	rates := rmcost.StageRates{
		Cons:   nullFloatOrZero(d.ConsRate),
		Stores: nullFloatOrZero(d.StoresRate),
		Dept:   nullFloatOrZero(d.DeptRate),
		PO1:    nullFloatOrZero(d.PO1Rate),
		PO2:    nullFloatOrZero(d.PO2Rate),
		PO3:    nullFloatOrZero(d.PO3Rate),
	}
	var groupID *uuid.UUID
	if d.GroupHeadID.Valid {
		id := d.GroupHeadID.UUID
		groupID = &id
	}
	return rmcost.ReconstructCost(
		d.ID, d.Period, d.RMCode, rmcost.RMType(d.RMType), groupID,
		nullStringPtr(d.ItemCode), nullStringVal(d.RMName), nullStringVal(d.UOMCode),
		rates,
		nullFloatPtr(d.CostVal), nullFloatPtr(d.CostMark), nullFloatPtr(d.CostSim),
		rmcost.Stage(d.FlagValuation), rmcost.Stage(d.FlagMarketing), rmcost.Stage(d.FlagSimulation),
		rmcost.Stage(d.FlagValuationUsed), rmcost.Stage(d.FlagMarketingUsed), rmcost.Stage(d.FlagSimulationUsed),
		nullTimePtr(d.CalculatedAt), nullStringPtr(d.CalculatedBy),
		d.CreatedAt, d.CreatedBy,
		nullTimePtr(d.UpdatedAt), nullStringPtr(d.UpdatedBy),
	)
}

func (r *RMCostRepository) scanCost(row *sql.Row) (*rmcost.Cost, error) {
	var d costDTO
	err := scanCostInto(row.Scan, &d)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rmcost.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan rm_cost: %w", err)
	}
	return d.toEntity(), nil
}

func (r *RMCostRepository) scanCostRow(rows *sql.Rows) (*rmcost.Cost, error) {
	var d costDTO
	if err := scanCostInto(rows.Scan, &d); err != nil {
		return nil, fmt.Errorf("scan rm_cost row: %w", err)
	}
	return d.toEntity(), nil
}

type scanFn func(...any) error

func scanCostInto(scan scanFn, d *costDTO) error {
	return scan(
		&d.ID, &d.Period, &d.RMCode, &d.RMType, &d.GroupHeadID, &d.ItemCode, &d.RMName, &d.UOMCode,
		&d.ConsRate, &d.StoresRate, &d.DeptRate, &d.PO1Rate, &d.PO2Rate, &d.PO3Rate,
		&d.CostVal, &d.CostMark, &d.CostSim,
		&d.FlagValuation, &d.FlagMarketing, &d.FlagSimulation,
		&d.FlagValuationUsed, &d.FlagMarketingUsed, &d.FlagSimulationUsed,
		&d.CalculatedAt, &d.CalculatedBy, &d.CreatedAt, &d.CreatedBy, &d.UpdatedAt, &d.UpdatedBy,
	)
}

// =============================================================================
// History scanning
// =============================================================================

const historySelectColumnsSQL = `
	SELECT history_id, rm_cost_id, job_id, period, rm_code, rm_type, group_head_id,
	       cons_rate, stores_rate, dept_rate, po_rate_1, po_rate_2, po_rate_3,
	       cost_percentage, cost_per_kg,
	       flag_valuation, flag_marketing, flag_simulation,
	       init_val_valuation, init_val_marketing, init_val_simulation,
	       cost_val, cost_mark, cost_sim,
	       flag_valuation_used, flag_marketing_used, flag_simulation_used,
	       source_item_count, trigger_reason, calculated_at, calculated_by`

func scanHistoryRow(rows *sql.Rows) (rmcost.History, error) {
	var (
		h            rmcost.History
		rmCostID     uuid.NullUUID
		jobID        uuid.NullUUID
		groupHeadID  uuid.NullUUID
		cons         sql.NullFloat64
		stores       sql.NullFloat64
		dept         sql.NullFloat64
		po1          sql.NullFloat64
		po2          sql.NullFloat64
		po3          sql.NullFloat64
		initVal      sql.NullFloat64
		initMkt      sql.NullFloat64
		initSim      sql.NullFloat64
		costVal      sql.NullFloat64
		costMkt      sql.NullFloat64
		costSim      sql.NullFloat64
		rmType       string
		flagVal      string
		flagMkt      string
		flagSim      string
		flagValUsed  string
		flagMktUsed  string
		flagSimUsed  string
		reason       string
	)
	if err := rows.Scan(
		&h.ID, &rmCostID, &jobID, &h.Period, &h.RMCode, &rmType, &groupHeadID,
		&cons, &stores, &dept, &po1, &po2, &po3,
		&h.CostPercentage, &h.CostPerKg,
		&flagVal, &flagMkt, &flagSim,
		&initVal, &initMkt, &initSim,
		&costVal, &costMkt, &costSim,
		&flagValUsed, &flagMktUsed, &flagSimUsed,
		&h.SourceItemCount, &reason, &h.CalculatedAt, &h.CalculatedBy,
	); err != nil {
		return rmcost.History{}, err
	}
	if rmCostID.Valid {
		id := rmCostID.UUID
		h.RMCostID = &id
	}
	if jobID.Valid {
		id := jobID.UUID
		h.JobID = &id
	}
	if groupHeadID.Valid {
		id := groupHeadID.UUID
		h.GroupHeadID = &id
	}
	h.RMType = rmcost.RMType(rmType)
	h.Rates = rmcost.StageRates{
		Cons: nullFloatOrZero(cons), Stores: nullFloatOrZero(stores), Dept: nullFloatOrZero(dept),
		PO1: nullFloatOrZero(po1), PO2: nullFloatOrZero(po2), PO3: nullFloatOrZero(po3),
	}
	h.FlagValuation = rmcost.Stage(flagVal)
	h.FlagMarketing = rmcost.Stage(flagMkt)
	h.FlagSimulation = rmcost.Stage(flagSim)
	h.FlagValuationUsed = rmcost.Stage(flagValUsed)
	h.FlagMarketingUsed = rmcost.Stage(flagMktUsed)
	h.FlagSimulationUsed = rmcost.Stage(flagSimUsed)
	h.InitValValuation = nullFloatPtr(initVal)
	h.InitValMarketing = nullFloatPtr(initMkt)
	h.InitValSimulation = nullFloatPtr(initSim)
	h.CostValuation = nullFloatPtr(costVal)
	h.CostMarketing = nullFloatPtr(costMkt)
	h.CostSimulation = nullFloatPtr(costSim)
	h.TriggerReason = rmcost.HistoryTriggerReason(reason)
	return h, nil
}

// nullFloatOrZero returns the float64 value or 0 when the NullFloat64 is invalid.
func nullFloatOrZero(v sql.NullFloat64) float64 {
	if v.Valid {
		return v.Float64
	}
	return 0
}
