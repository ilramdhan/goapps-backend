package workorder

import (
	"context"
	"fmt"
)

// FormatLotNo renders a PPC lot number in the transcribable form
// {AREA}{SEQ:04d}-{YY} — e.g. SPG0042-26. Twelve characters at most for a
// five-digit sequence, comfortably inside work_order.wo_lot_no VARCHAR(30).
//
// The previous form was a nanosecond timestamp, which no operator can copy onto
// a doff card. The Oracle ETL joins production back to a work order by this
// exact string, and a mistyped lot surfaces only as a swallowed SYNC_FAILED.
func FormatLotNo(areaCode string, year, seq int) string {
	return fmt.Sprintf("%s%04d-%02d", areaCode, seq, year%100)
}

// LotUnfullWeightRatio estimates an unfull bobbin's standard weight as a
// fraction of the full standard weight.
//
// There is no master anywhere that carries a per-lot unfull weight: finance's
// mst_parameter has STD_WEIGHT but nothing for the unfull case. Grading (PRD
// §7) bands an unfull bobbin (grade A9) at 2.00 kg up to just under the full
// standard weight, so half the full weight sits inside that band for every
// realistic denier. The generated lot_master row is seeded with this estimate
// and flagged in its notes; PPC corrects it on the lot master screen once the
// real figure is known. The full weight is never estimated — it comes from the
// resolved STD_WEIGHT parameter, and a work order is rejected outright when
// that cannot be resolved.
const LotUnfullWeightRatio = 0.5

// GeneratedLotNote is written to a generated lot's lot_master notes so the
// estimated unfull weight is visibly provisional rather than authoritative.
const GeneratedLotNote = "Auto-generated at work-order creation. Unfull standard weight is an estimate — verify before relying on it for bobbin quantity."

// LotProvisionRequest carries what a generated lot needs in order to be
// registered in lot_master before the work order references it.
type LotProvisionRequest struct {
	AreaCode        string
	Year            int
	ItemCode        string
	ShadeCode       string
	StdWeightFull   float64
	StdWeightUnfull float64
	Notes           string
	CreatedBy       string
}

// Validate reports whether the request can produce a lot_master row. It mirrors
// the lot aggregate's own invariants (non-empty codes, positive weights) so a
// bad request fails before a sequence number is burned.
func (r LotProvisionRequest) Validate() error {
	if r.AreaCode == "" {
		return ErrInvalidArea
	}
	if r.ItemCode == "" || r.ShadeCode == "" {
		return ErrLotSpecUnavailable
	}
	if r.StdWeightFull <= 0 || r.StdWeightUnfull <= 0 {
		return ErrLotSpecUnavailable
	}
	return nil
}

// LotProvisioner mints the next lot number for an area, registers it in
// lot_master, and persists the work order built from it — all inside one
// transaction.
//
// The callback shape exists because the lot number is required before the
// aggregate can be constructed (a work order may not have an empty lot), while
// the sequence bump must share the work order's transaction so a rolled-back
// create burns no lot number.
type LotProvisioner interface {
	// CreateWithGeneratedLot mints a lot, calls build with it, and persists the
	// resulting work order. The entity returned by build carries its assigned ID
	// on success.
	CreateWithGeneratedLot(ctx context.Context, req LotProvisionRequest, build func(lotNo string) (*WorkOrder, error)) (*WorkOrder, error)
}
