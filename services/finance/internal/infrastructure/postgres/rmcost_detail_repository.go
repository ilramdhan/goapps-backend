// Package postgres — V2 RM Cost detail repository.
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

// RMCostDetailRepository implements rmcost.CostDetailRepository.
type RMCostDetailRepository struct {
	db *DB
}

// NewRMCostDetailRepository constructs the repo.
func NewRMCostDetailRepository(db *DB) *RMCostDetailRepository {
	return &RMCostDetailRepository{db: db}
}

// Verify interface compliance at compile time.
var _ rmcost.CostDetailRepository = (*RMCostDetailRepository)(nil)

// UpsertAll wipes detail rows for one cost and reinserts the supplied set in
// a single transaction. Used after a fresh V2 calc pass.
func (r *RMCostDetailRepository) UpsertAll(ctx context.Context, costID uuid.UUID, details []*rmcost.CostDetail) error {
	return r.db.Transaction(ctx, func(tx *sql.Tx) error {
		if _, err := tx.ExecContext(ctx, `DELETE FROM cst_rm_cost_detail WHERE rm_cost_id = $1`, costID); err != nil {
			return fmt.Errorf("delete existing cost details: %w", err)
		}
		for _, d := range details {
			if err := insertCostDetail(ctx, tx, d); err != nil {
				return err
			}
		}
		return nil
	})
}

// GetByID returns one detail row.
func (r *RMCostDetailRepository) GetByID(ctx context.Context, id uuid.UUID) (*rmcost.CostDetail, error) {
	row := r.db.QueryRowContext(ctx, costDetailSelectSQL+` WHERE cost_detail_id = $1`, id)
	d, err := scanCostDetailRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, rmcost.ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("scan cost detail: %w", err)
	}
	return d, nil
}

// ListByCostID returns all detail rows for one cost row.
func (r *RMCostDetailRepository) ListByCostID(ctx context.Context, costID uuid.UUID) ([]*rmcost.CostDetail, error) {
	rows, err := r.db.QueryContext(ctx,
		costDetailSelectSQL+` WHERE rm_cost_id = $1 ORDER BY item_code ASC, COALESCE(grade_code,'') ASC`,
		costID)
	if err != nil {
		return nil, fmt.Errorf("list cost details: %w", err)
	}
	defer closeRows(rows)
	var out []*rmcost.CostDetail
	for rows.Next() {
		d, err := scanCostDetailRow(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("scan cost detail row: %w", err)
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate cost details: %w", err)
	}
	return out, nil
}

// UpdateSnapshot writes the per-stage values for one detail row (after a
// fix_rate edit recompute).
func (r *RMCostDetailRepository) UpdateSnapshot(ctx context.Context, detail *rmcost.CostDetail) error {
	snap := detail.Snapshot()
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `
		UPDATE cst_rm_cost_detail SET
			freight_rate = $2, anti_dumping_pct = $3, duty_pct = $4, transport_rate = $5, valuation_default_value = $6,
			cons_val = $7, cons_qty = $8, cons_rate = $9, cons_freight_val = $10, cons_val_based = $11,
			cons_rate_based = $12, cons_anti_dumping_val = $13, cons_anti_dumping_rate = $14,
			cons_duty_val = $15, cons_duty_rate = $16, cons_transport_val = $17, cons_transport_rate = $18,
			cons_landed_cost = $19,
			stock_val = $20, stock_qty = $21, stock_rate = $22, stock_freight_val = $23, stock_val_based = $24,
			stock_rate_based = $25, stock_anti_dumping_val = $26, stock_anti_dumping_rate = $27,
			stock_duty_val = $28, stock_duty_rate = $29, stock_transport_val = $30, stock_transport_rate = $31,
			stock_landed_cost = $32,
			po_val = $33, po_qty = $34, po_rate = $35,
			fix_rate = $36, fix_freight_rate = $37, fix_rate_based = $38, fix_anti_dumping_rate = $39,
			fix_duty_rate = $40, fix_transport_rate = $41, fix_landed_cost = $42,
			updated_at = $43, updated_by = $44
		WHERE cost_detail_id = $1
	`,
		detail.ID(),
		snap.FreightRate, snap.AntiDumpingPct, snap.DutyPct, snap.TransportRate, snap.ValuationDefaultValue,
		snap.ConsVal, snap.ConsQty, snap.ConsRate, snap.ConsFreightVal, snap.ConsValBased,
		snap.ConsRateBased, snap.ConsAntiDumpingVal, snap.ConsAntiDumpingRate,
		snap.ConsDutyVal, snap.ConsDutyRate, snap.ConsTransportVal, snap.ConsTransportRate,
		snap.ConsLandedCost,
		snap.StockVal, snap.StockQty, snap.StockRate, snap.StockFreightVal, snap.StockValBased,
		snap.StockRateBased, snap.StockAntiDumpingVal, snap.StockAntiDumpingRate,
		snap.StockDutyVal, snap.StockDutyRate, snap.StockTransportVal, snap.StockTransportRate,
		snap.StockLandedCost,
		snap.POVal, snap.POQty, snap.PORate,
		snap.FixRate, snap.FixFreightRate, snap.FixRateBased, snap.FixAntiDumpingRate,
		snap.FixDutyRate, snap.FixTransportRate, snap.FixLandedCost,
		now, derefString(detail.UpdatedBy()),
	)
	if err != nil {
		return fmt.Errorf("update cost detail snapshot: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return rmcost.ErrNotFound
	}
	return nil
}

// DeleteByCostID removes every detail belonging to one cost row.
func (r *RMCostDetailRepository) DeleteByCostID(ctx context.Context, costID uuid.UUID) error {
	if _, err := r.db.ExecContext(ctx, `DELETE FROM cst_rm_cost_detail WHERE rm_cost_id = $1`, costID); err != nil {
		return fmt.Errorf("delete cost details: %w", err)
	}
	return nil
}

// =============================================================================
// V2Inputs writes (CostInputsRepository)
// =============================================================================

// RMCostInputsRepository implements rmcost.CostInputsRepository.
type RMCostInputsRepository struct {
	db *DB
}

// NewRMCostInputsRepository constructs the repo.
func NewRMCostInputsRepository(db *DB) *RMCostInputsRepository {
	return &RMCostInputsRepository{db: db}
}

// Verify interface compliance at compile time.
var _ rmcost.CostInputsRepository = (*RMCostInputsRepository)(nil)

// UpdateInputs persists V2 marketing snapshot + simulation rate + flags onto
// a cost row, with recomputed cost_marketing / cost_simulation.
func (r *RMCostInputsRepository) UpdateInputs(
	ctx context.Context,
	costID uuid.UUID,
	in rmcost.V2Inputs,
	rates rmcost.V2Rates,
	costMkt, costSim float64,
	updatedBy string,
) error {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `
		UPDATE cst_rm_cost SET
			marketing_freight_rate = $2,
			marketing_anti_dumping_pct = $3,
			marketing_duty_pct = $4,
			marketing_transport_rate = $5,
			marketing_default_value = $6,
			simulation_rate = $7,
			valuation_flag_v2 = $8,
			marketing_flag_v2 = $9,
			sp_rate = $10, pp_rate = $11, fp_rate = $12,
			cost_mark = $13, cost_sim = $14,
			updated_at = $15, updated_by = $16
		WHERE rm_cost_id = $1
	`,
		costID,
		in.MarketingFreightRate, in.MarketingAntiDumpingPct, in.MarketingDutyPct,
		in.MarketingTransportRate, in.MarketingDefaultValue, in.SimulationRate,
		nullableFlagString(in.ValuationFlag), nullableFlagString(in.MarketingFlag),
		rates.SP, rates.PP, rates.FP,
		costMkt, costSim,
		now, updatedBy,
	)
	if err != nil {
		return fmt.Errorf("update cost inputs: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return rmcost.ErrNotFound
	}
	return nil
}

// UpdateFLAndCostVal refreshes fl_rate and cost_val on a cost row after a
// per-detail fix_rate change shifts the MAX.
func (r *RMCostInputsRepository) UpdateFLAndCostVal(
	ctx context.Context,
	costID uuid.UUID,
	flRate float64,
	costVal *float64,
	updatedBy string,
) error {
	now := time.Now()
	res, err := r.db.ExecContext(ctx, `
		UPDATE cst_rm_cost SET
			fl_rate = $2, cost_val = $3, updated_at = $4, updated_by = $5
		WHERE rm_cost_id = $1
	`, costID, flRate, costVal, now, updatedBy)
	if err != nil {
		return fmt.Errorf("update fl_rate: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("rows affected: %w", err)
	}
	if n == 0 {
		return rmcost.ErrNotFound
	}
	return nil
}

// =============================================================================
// Cost-detail SELECT + scan
// =============================================================================

const costDetailSelectSQL = `
	SELECT cost_detail_id, rm_cost_id, period, group_head_id, group_detail_id,
	       item_code, item_name, grade_code,
	       freight_rate, anti_dumping_pct, duty_pct, transport_rate, valuation_default_value,
	       cons_val, cons_qty, cons_rate, cons_freight_val, cons_val_based,
	       cons_rate_based, cons_anti_dumping_val, cons_anti_dumping_rate,
	       cons_duty_val, cons_duty_rate, cons_transport_val, cons_transport_rate, cons_landed_cost,
	       stock_val, stock_qty, stock_rate, stock_freight_val, stock_val_based,
	       stock_rate_based, stock_anti_dumping_val, stock_anti_dumping_rate,
	       stock_duty_val, stock_duty_rate, stock_transport_val, stock_transport_rate, stock_landed_cost,
	       po_val, po_qty, po_rate,
	       fix_rate, fix_freight_rate, fix_rate_based, fix_anti_dumping_rate,
	       fix_duty_rate, fix_transport_rate, fix_landed_cost,
	       created_at, created_by, updated_at, updated_by
	FROM cst_rm_cost_detail`

//nolint:gocyclo,gocognit // Wide scan mirrors persistence row 1:1.
func scanCostDetailRow(scan scanFn) (*rmcost.CostDetail, error) {
	var (
		id, costID, headID                            uuid.UUID
		groupDetailID                                 uuid.NullUUID
		period, itemCode                              string
		itemName                                      sql.NullString
		gradeCode                                     sql.NullString
		freight, anti, duty, transport, valDefault    sql.NullFloat64
		cv, cq, cr, cfv, cvb, crb                     sql.NullFloat64
		cav, car, cdv, cdr, ctv, ctr, cl              sql.NullFloat64
		sv, sq, sr2, sfv, svb, srb                    sql.NullFloat64
		sav, sar, sdv, sdr, stv, str2, sl             sql.NullFloat64
		pv, pq, pr                                    sql.NullFloat64
		fr, ffr, frb, far, fdr, ftr, fl2              sql.NullFloat64
		createdAt                                     time.Time
		createdBy                                     string
		updatedAt                                     sql.NullTime
		updatedBy                                     sql.NullString
	)
	err := scan(
		&id, &costID, &period, &headID, &groupDetailID,
		&itemCode, &itemName, &gradeCode,
		&freight, &anti, &duty, &transport, &valDefault,
		&cv, &cq, &cr, &cfv, &cvb,
		&crb, &cav, &car,
		&cdv, &cdr, &ctv, &ctr, &cl,
		&sv, &sq, &sr2, &sfv, &svb,
		&srb, &sav, &sar,
		&sdv, &sdr, &stv, &str2, &sl,
		&pv, &pq, &pr,
		&fr, &ffr, &frb, &far,
		&fdr, &ftr, &fl2,
		&createdAt, &createdBy, &updatedAt, &updatedBy,
	)
	if err != nil {
		return nil, err
	}
	var gdID *uuid.UUID
	if groupDetailID.Valid {
		v := groupDetailID.UUID
		gdID = &v
	}
	d := rmcost.ReconstructCostDetail(
		id, costID, headID,
		period, itemCode, nullStringVal(itemName), nullStringVal(gradeCode),
		gdID,
		createdAt, createdBy,
		nullTimePtr(updatedAt), nullStringPtr(updatedBy),
	)
	d.AttachSnapshot(rmcost.CostDetailSnapshot{
		FreightRate: nullFloatPtr(freight), AntiDumpingPct: nullFloatPtr(anti),
		DutyPct: nullFloatPtr(duty), TransportRate: nullFloatPtr(transport),
		ValuationDefaultValue: nullFloatPtr(valDefault),
		ConsVal:               nullFloatPtr(cv), ConsQty: nullFloatPtr(cq), ConsRate: nullFloatPtr(cr),
		ConsFreightVal: nullFloatPtr(cfv), ConsValBased: nullFloatPtr(cvb),
		ConsRateBased:  nullFloatPtr(crb),
		ConsAntiDumpingVal: nullFloatPtr(cav), ConsAntiDumpingRate: nullFloatPtr(car),
		ConsDutyVal: nullFloatPtr(cdv), ConsDutyRate: nullFloatPtr(cdr),
		ConsTransportVal: nullFloatPtr(ctv), ConsTransportRate: nullFloatPtr(ctr),
		ConsLandedCost: nullFloatPtr(cl),
		StockVal:       nullFloatPtr(sv), StockQty: nullFloatPtr(sq), StockRate: nullFloatPtr(sr2),
		StockFreightVal: nullFloatPtr(sfv), StockValBased: nullFloatPtr(svb),
		StockRateBased:  nullFloatPtr(srb),
		StockAntiDumpingVal: nullFloatPtr(sav), StockAntiDumpingRate: nullFloatPtr(sar),
		StockDutyVal: nullFloatPtr(sdv), StockDutyRate: nullFloatPtr(sdr),
		StockTransportVal: nullFloatPtr(stv), StockTransportRate: nullFloatPtr(str2),
		StockLandedCost: nullFloatPtr(sl),
		POVal:           nullFloatPtr(pv), POQty: nullFloatPtr(pq), PORate: nullFloatPtr(pr),
		FixRate:         nullFloatPtr(fr), FixFreightRate: nullFloatPtr(ffr),
		FixRateBased:    nullFloatPtr(frb), FixAntiDumpingRate: nullFloatPtr(far),
		FixDutyRate: nullFloatPtr(fdr), FixTransportRate: nullFloatPtr(ftr),
		FixLandedCost: nullFloatPtr(fl2),
	})
	return d, nil
}

func insertCostDetail(ctx context.Context, tx *sql.Tx, d *rmcost.CostDetail) error {
	snap := d.Snapshot()
	var groupDetailID uuid.NullUUID
	if d.GroupDetailID() != nil {
		groupDetailID = uuid.NullUUID{UUID: *d.GroupDetailID(), Valid: true}
	}
	_, err := tx.ExecContext(ctx, `
		INSERT INTO cst_rm_cost_detail (
			cost_detail_id, rm_cost_id, period, group_head_id, group_detail_id,
			item_code, item_name, grade_code,
			freight_rate, anti_dumping_pct, duty_pct, transport_rate, valuation_default_value,
			cons_val, cons_qty, cons_rate, cons_freight_val, cons_val_based,
			cons_rate_based, cons_anti_dumping_val, cons_anti_dumping_rate,
			cons_duty_val, cons_duty_rate, cons_transport_val, cons_transport_rate, cons_landed_cost,
			stock_val, stock_qty, stock_rate, stock_freight_val, stock_val_based,
			stock_rate_based, stock_anti_dumping_val, stock_anti_dumping_rate,
			stock_duty_val, stock_duty_rate, stock_transport_val, stock_transport_rate, stock_landed_cost,
			po_val, po_qty, po_rate,
			fix_rate, fix_freight_rate, fix_rate_based, fix_anti_dumping_rate,
			fix_duty_rate, fix_transport_rate, fix_landed_cost,
			created_at, created_by
		) VALUES (
			$1,$2,$3,$4,$5,$6,$7,$8,
			$9,$10,$11,$12,$13,
			$14,$15,$16,$17,$18,
			$19,$20,$21,
			$22,$23,$24,$25,$26,
			$27,$28,$29,$30,$31,
			$32,$33,$34,
			$35,$36,$37,$38,$39,
			$40,$41,$42,
			$43,$44,$45,$46,
			$47,$48,$49,
			$50,$51
		)
	`,
		d.ID(), d.CostID(), d.Period(), d.GroupHeadID(), groupDetailID,
		d.ItemCode(), nullableString(d.ItemName()), nullableString(d.GradeCode()),
		snap.FreightRate, snap.AntiDumpingPct, snap.DutyPct, snap.TransportRate, snap.ValuationDefaultValue,
		snap.ConsVal, snap.ConsQty, snap.ConsRate, snap.ConsFreightVal, snap.ConsValBased,
		snap.ConsRateBased, snap.ConsAntiDumpingVal, snap.ConsAntiDumpingRate,
		snap.ConsDutyVal, snap.ConsDutyRate, snap.ConsTransportVal, snap.ConsTransportRate, snap.ConsLandedCost,
		snap.StockVal, snap.StockQty, snap.StockRate, snap.StockFreightVal, snap.StockValBased,
		snap.StockRateBased, snap.StockAntiDumpingVal, snap.StockAntiDumpingRate,
		snap.StockDutyVal, snap.StockDutyRate, snap.StockTransportVal, snap.StockTransportRate, snap.StockLandedCost,
		snap.POVal, snap.POQty, snap.PORate,
		snap.FixRate, snap.FixFreightRate, snap.FixRateBased, snap.FixAntiDumpingRate,
		snap.FixDutyRate, snap.FixTransportRate, snap.FixLandedCost,
		d.CreatedAt(), d.CreatedBy(),
	)
	if err != nil {
		return fmt.Errorf("insert cost detail: %w", err)
	}
	return nil
}
