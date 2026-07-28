package postgres

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// LotProvisioner mints a transcribable lot number, registers it in lot_master,
// and inserts the work order that references it — all in one transaction.
//
// Doing all three together is the point: the lot sequence bump takes a row lock
// that serializes concurrent creates, and a work order that fails validation
// after the lot was minted rolls the counter back instead of burning a number.
// It also closes the hole where an auto-generated lot never reached lot_master,
// leaving the ETL unable to compute a bobbin quantity for it.
type LotProvisioner struct {
	db *DB
}

// NewLotProvisioner builds a lot provisioner over the DB.
func NewLotProvisioner(db *DB) *LotProvisioner {
	return &LotProvisioner{db: db}
}

var _ workorder.LotProvisioner = (*LotProvisioner)(nil)

// CreateWithGeneratedLot mints the next lot number for the area+year, registers
// it in lot_master, builds the work order from it and persists the work order.
func (p *LotProvisioner) CreateWithGeneratedLot(
	ctx context.Context,
	req workorder.LotProvisionRequest,
	build func(lotNo string) (*workorder.WorkOrder, error),
) (*workorder.WorkOrder, error) {
	var entity *workorder.WorkOrder
	err := p.db.Transaction(ctx, func(tx *sql.Tx) error {
		seq, err := nextLotSeqTx(ctx, tx, req.AreaCode, req.Year)
		if err != nil {
			return err
		}
		lotNo := workorder.FormatLotNo(req.AreaCode, req.Year, seq)
		if err := insertLotMasterTx(ctx, tx, lotNo, req); err != nil {
			return err
		}
		built, err := build(lotNo)
		if err != nil {
			return err
		}
		if err := insertWorkOrderTx(ctx, tx, built); err != nil {
			return err
		}
		entity = built
		return nil
	})
	if err != nil {
		return nil, err
	}
	return entity, nil
}

// nextLotSeqTx bumps and returns the area+year counter. The upsert takes a row
// lock, so concurrent work-order creates queue rather than collide.
func nextLotSeqTx(ctx context.Context, tx *sql.Tx, areaCode string, year int) (int, error) {
	const query = `
		INSERT INTO lot_sequence (ls_area_code, ls_year, ls_last_seq)
		VALUES ($1, $2, 1)
		ON CONFLICT (ls_area_code, ls_year)
		DO UPDATE SET ls_last_seq = lot_sequence.ls_last_seq + 1
		RETURNING ls_last_seq`
	var seq int
	if err := tx.QueryRowContext(ctx, query, areaCode, year).Scan(&seq); err != nil {
		return 0, fmt.Errorf("failed to allocate lot sequence: %w", err)
	}
	return seq, nil
}

// insertLotMasterTx registers a generated lot so every work-order lot exists in
// lot_master regardless of how it was created.
func insertLotMasterTx(ctx context.Context, tx *sql.Tx, lotNo string, req workorder.LotProvisionRequest) error {
	const query = `
		INSERT INTO lot_master (
			lm_lot_no, lm_item_code, lm_shade_code,
			lm_std_weight_full, lm_std_weight_unfull, lm_notes, lm_created_by
		) VALUES ($1, $2, $3, $4, $5, NULLIF($6, ''), $7)`
	_, err := tx.ExecContext(ctx, query,
		lotNo, req.ItemCode, req.ShadeCode,
		req.StdWeightFull, req.StdWeightUnfull, req.Notes, req.CreatedBy,
	)
	if err != nil {
		return fmt.Errorf("failed to register generated lot %s: %w", lotNo, err)
	}
	return nil
}
