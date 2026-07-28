package etl

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// spgProductionTable is the Oracle source table name and watermark key.
const spgProductionTable = "PPC_SPG_PRODUCTION"

// SpgProductionSource lists incremental SPG production rows from Oracle. A nil
// implementation (Oracle unavailable) is tolerated by the usecase.
type SpgProductionSource interface {
	ListSpgProduction(ctx context.Context, watermark time.Time) ([]oracle.SpgProductionRow, error)
}

// SpgProductionRepo is the write/read surface the SPG ETL needs. Implemented by
// postgres.ETLRepository.
type SpgProductionRepo interface {
	GetWatermark(ctx context.Context, table string) (time.Time, error)
	AdvanceWatermark(ctx context.Context, table string, ts time.Time) error
	MatchWO(ctx context.Context, lotNo, machineNo string) (woID int64, area string, ok bool, err error)
	LotStdWeights(ctx context.Context, lotNo string) (full, unfull float64, ok bool, err error)
	UpsertSpgProductionActual(ctx context.Context, in postgres.SpgProductionActualUpsert) error
}

// SpgProductionETL is the incremental SPG production ETL usecase. It pulls rows
// newer than the watermark, aggregates per (lot, line, date, shift), matches
// each to a WO, computes the dual doffed/transferred quantities, and upserts the
// SPG two-axis production-actual row.
type SpgProductionETL struct {
	source     SpgProductionSource
	repo       SpgProductionRepo
	bufferMins int
}

// NewSpgProductionETL builds the usecase. source may be nil (Oracle
// unavailable); bufferMins rewinds the advanced watermark so late TQM rows
// re-enter next run.
func NewSpgProductionETL(source SpgProductionSource, repo SpgProductionRepo, bufferMins int) *SpgProductionETL {
	if bufferMins < 0 {
		bufferMins = 0
	}
	return &SpgProductionETL{source: source, repo: repo, bufferMins: bufferMins}
}

// spgAggKey identifies one SPG production-actual row. SPG shift is not carried
// on the source row (doffing is per date), so all doffs of a lot+line+date roll
// up to a single row; shift defaults to "1".
type spgAggKey struct {
	lotNo   string
	machine string
	date    time.Time
}

// spgAggValue accumulates SPG bobbin counts and weighted doffed/transferred kg.
// Weight is applied per source row (per-doff DOFF_WT) so a shift's average is
// naturally reflected. weightedBobs tracks the bobbin base for a shift-average
// weight-per-bob on the persisted row.
type spgAggValue struct {
	grossBobbins     int
	transferredBobs  int
	cutBobbins       int
	notTransfer      int
	normalBobs       int
	downgradeBobs    int
	notCheckedBobs   int
	qtyDoffedKg      float64
	qtyTransferredKg float64
	weightBobs       int
	weightSum        float64
}

// spgShiftDefault is the shift assigned to SPG rows; the SPG summary is per doff
// per date, not per shift.
const spgShiftDefault = "1"

// Run executes one incremental SPG ETL pass. A nil source (Oracle unavailable)
// is a no-op (not an error). Unmatched aggregates are logged and skipped.
func (e *SpgProductionETL) Run(ctx context.Context) (Result, error) {
	res := Result{}
	if e.source == nil {
		log.Info().Msg("spg production ETL: oracle unavailable, skipping run")
		return res, nil
	}

	watermark, err := e.repo.GetWatermark(ctx, spgProductionTable)
	if err != nil {
		return res, err
	}

	rows, err := e.source.ListSpgProduction(ctx, watermark)
	if err != nil {
		return res, err
	}
	res.OracleUp = true
	res.Pulled = len(rows)
	if len(rows) == 0 {
		return res, nil
	}

	aggregated, maxSeen := aggregateSpgRows(rows)
	for key, val := range aggregated {
		if err := e.processSpgAggregate(ctx, key, val, &res); err != nil {
			return res, err
		}
	}

	advanced := maxSeen.Add(-time.Duration(e.bufferMins) * time.Minute)
	if err := e.repo.AdvanceWatermark(ctx, spgProductionTable, advanced); err != nil {
		return res, err
	}
	return res, nil
}

// aggregateSpgRows sums SPG bobbin counts and dual kg per key and returns the
// max LAST_UPDATED seen. Each source doff contributes its own weight-per-bob so
// dual quantities remain accurate across mixed-weight doffs.
func aggregateSpgRows(rows []oracle.SpgProductionRow) (map[spgAggKey]*spgAggValue, time.Time) {
	aggregated := make(map[spgAggKey]*spgAggValue)
	var maxSeen time.Time
	for i := range rows {
		row := rows[i]
		if row.LastUpdated.After(maxSeen) {
			maxSeen = row.LastUpdated
		}
		key := spgAggKey{lotNo: row.LotNo, machine: row.MachineLine, date: row.DoffDate}
		val, ok := aggregated[key]
		if !ok {
			val = &spgAggValue{}
			aggregated[key] = val
		}
		accumulateSpgRow(val, row)
	}
	return aggregated, maxSeen
}

// accumulateSpgRow folds one Oracle doff row into the running aggregate. The
// doffed basis uses GROSS_BOBBINS × weight (efficiency); the transferred basis
// uses TRANSFERRED_BOBS × weight (fulfillment). DOFF_OPTION selects the standard
// fallback weight (1=Full) only when a measured DOFF_WT is absent.
func accumulateSpgRow(val *spgAggValue, row oracle.SpgProductionRow) {
	val.grossBobbins += row.GrossBobbins
	val.transferredBobs += row.TransferredBob
	val.cutBobbins += row.CutBobbins
	val.notTransfer += row.NotTransfer
	val.normalBobs += row.NormalBobs
	val.downgradeBobs += row.DowngradeBobs
	val.notCheckedBobs += row.NotCheckedBobs
	// Per-doff weighted quantities; std weights are 0 here (fallback handled at
	// persist time via measured DOFF_WT which the summary always carries).
	val.qtyDoffedKg += shared.QtySPG(row.GrossBobbins, row.DoffOption, row.WeightPerBob, 0, 0)
	val.qtyTransferredKg += shared.QtySPG(row.TransferredBob, row.DoffOption, row.WeightPerBob, 0, 0)
	if row.WeightPerBob > 0 {
		val.weightBobs += row.GrossBobbins
		val.weightSum += row.WeightPerBob * float64(row.GrossBobbins)
	}
}

// processSpgAggregate matches one aggregate to a WO and upserts its SPG
// production-actual row. When no measured weight was present, standard lot
// weights back-fill the dual quantities. A failed sanity check
// (GROSS != TRANSFERRED + CUT + NOT_TRANSFER) is logged but not fatal.
func (e *SpgProductionETL) processSpgAggregate(ctx context.Context, key spgAggKey, val *spgAggValue, res *Result) error {
	woID, area, ok, err := e.repo.MatchWO(ctx, key.lotNo, key.machine)
	if err != nil {
		return err
	}
	if !ok {
		res.Unmatched++
		log.Warn().
			Str("lot_no", key.lotNo).
			Str("machine_line", key.machine).
			Msg("spg production ETL: SYNC_FAILED, no matching WO")
		return nil
	}

	e.backfillWeights(ctx, key.lotNo, val)
	logSpgSanity(key, val)

	if area == "" {
		area = "SPG"
	}
	weightPerBob := 0.0
	if val.weightBobs > 0 {
		weightPerBob = val.weightSum / float64(val.weightBobs)
	}
	upsert := postgres.SpgProductionActualUpsert{
		WOID:             woID,
		Date:             key.date,
		Shift:            spgShiftDefault,
		Area:             area,
		GrossBobbins:     val.grossBobbins,
		TransferredBobs:  val.transferredBobs,
		CutBobbins:       val.cutBobbins,
		NotTransfer:      val.notTransfer,
		NormalBobsSpg:    val.normalBobs,
		DowngradeBobsSpg: val.downgradeBobs,
		NotCheckedBobs:   val.notCheckedBobs,
		WeightPerBob:     weightPerBob,
		QtyDoffedKg:      val.qtyDoffedKg,
		QtyTransferredKg: val.qtyTransferredKg,
	}
	if err := e.repo.UpsertSpgProductionActual(ctx, upsert); err != nil {
		return err
	}
	res.Upserted++
	return nil
}

// backfillWeights fills dual quantities from lot standard weights when the
// Oracle summary carried no measured per-bobbin weight (both dual sums are 0
// despite non-zero bobbins). Full doffs use the standard full weight.
func (e *SpgProductionETL) backfillWeights(ctx context.Context, lotNo string, val *spgAggValue) {
	if val.qtyDoffedKg > 0 || val.grossBobbins == 0 {
		return
	}
	stdFull, stdUnfull, hasLot, err := e.repo.LotStdWeights(ctx, lotNo)
	if err != nil {
		log.Warn().Err(err).Str("lot_no", lotNo).Msg("spg production ETL: lot std weights lookup failed")
		return
	}
	if !hasLot {
		return
	}
	// Bobbin fill state is unknown at aggregate level; treat as full (the
	// dominant case) for the standard-weight fallback.
	val.qtyDoffedKg = shared.QtySPG(val.grossBobbins, shared.SpgDoffOptionFull, 0, stdFull, stdUnfull)
	val.qtyTransferredKg = shared.QtySPG(val.transferredBobs, shared.SpgDoffOptionFull, 0, stdFull, stdUnfull)
}

// logSpgSanity warns when the SPG bobbin identity GROSS = TRANSFERRED + CUT +
// NOT_TRANSFER does not hold; the row is still persisted (data-quality signal).
func logSpgSanity(key spgAggKey, val *spgAggValue) {
	if val.grossBobbins != val.transferredBobs+val.cutBobbins+val.notTransfer {
		log.Warn().
			Str("lot_no", key.lotNo).
			Str("machine_line", key.machine).
			Int("gross", val.grossBobbins).
			Int("transferred", val.transferredBobs).
			Int("cut", val.cutBobbins).
			Int("not_transfer", val.notTransfer).
			Msg("spg production ETL: sanity check failed (GROSS != TRANSFERRED + CUT + NOT_TRANSFER)")
	}
}
