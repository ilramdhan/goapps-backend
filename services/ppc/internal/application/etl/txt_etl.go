package etl

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/shared"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// txtProductionTable is the Oracle source table name and watermark key.
const txtProductionTable = "PPC_TXT_PRODUCTION"

// TxtProductionSource lists incremental TXT/TWT production rows from Oracle. A nil
// implementation (Oracle unavailable) is tolerated by the usecase.
type TxtProductionSource interface {
	ListTxtProduction(ctx context.Context, watermark time.Time) ([]oracle.TxtProductionRow, error)
}

// TxtProductionRepo is the write/read surface the TXT ETL needs. Implemented by
// postgres.ETLRepository.
type TxtProductionRepo interface {
	GetWatermark(ctx context.Context, table string) (time.Time, error)
	AdvanceWatermark(ctx context.Context, table string, ts time.Time) error
	MatchWO(ctx context.Context, lotNo, machineNo string) (woID int64, area string, ok bool, err error)
	LotStdWeights(ctx context.Context, lotNo string) (full, unfull float64, ok bool, err error)
	UpsertProductionActual(ctx context.Context, in postgres.ProductionActualUpsert) error
}

// Result summarizes a TXT ETL run.
type Result struct {
	Pulled    int
	Upserted  int
	Unmatched int
	OracleUp  bool
}

// TxtProductionETL is the incremental TXT/TWT production ETL usecase. It pulls
// rows newer than the watermark, aggregates per (lot, machine, date, shift),
// matches each to a WO, computes the bobbin quantity, and upserts the two-axis
// production-actual row.
type TxtProductionETL struct {
	source     TxtProductionSource
	repo       TxtProductionRepo
	bufferMins int
}

// NewTxtProductionETL builds the usecase. source may be nil (Oracle unavailable);
// bufferMins rewinds the advanced watermark so late TQM rows re-enter next run.
func NewTxtProductionETL(source TxtProductionSource, repo TxtProductionRepo, bufferMins int) *TxtProductionETL {
	if bufferMins < 0 {
		bufferMins = 0
	}
	return &TxtProductionETL{source: source, repo: repo, bufferMins: bufferMins}
}

// aggKey identifies one production-actual row (a WO+date+shift may span multiple
// DOFF_NO rows in Oracle).
type aggKey struct {
	lotNo     string
	machineNo string
	date      time.Time
	shift     string
}

// aggValue accumulates bobbin counts and tracks the max LAST_UPDATED for a key.
type aggValue struct {
	area          string
	totalBobbins  int
	fullBobbins   int
	unfullBobbins int
	normalBobs    int
	downgradeBobs int
	pendingBobs   int
	packCekBobs   int
}

// Run executes one incremental ETL pass. A nil source (Oracle unavailable) is a
// no-op (not an error). Unmatched aggregates are logged and skipped, not fatal.
func (e *TxtProductionETL) Run(ctx context.Context) (Result, error) {
	res := Result{}
	if e.source == nil {
		log.Info().Msg("txt production ETL: oracle unavailable, skipping run")
		return res, nil
	}

	watermark, err := e.repo.GetWatermark(ctx, txtProductionTable)
	if err != nil {
		return res, err
	}

	rows, err := e.source.ListTxtProduction(ctx, watermark)
	if err != nil {
		return res, err
	}
	res.OracleUp = true
	res.Pulled = len(rows)
	if len(rows) == 0 {
		return res, nil
	}

	aggregated, maxSeen := aggregateRows(rows)
	for key, val := range aggregated {
		if err := e.processAggregate(ctx, key, val, &res); err != nil {
			return res, err
		}
	}

	advanced := maxSeen.Add(-time.Duration(e.bufferMins) * time.Minute)
	if err := e.repo.AdvanceWatermark(ctx, txtProductionTable, advanced); err != nil {
		return res, err
	}
	return res, nil
}

// aggregateRows sums bobbin counts per key and returns the max LAST_UPDATED seen.
func aggregateRows(rows []oracle.TxtProductionRow) (map[aggKey]*aggValue, time.Time) {
	aggregated := make(map[aggKey]*aggValue)
	var maxSeen time.Time
	for i := range rows {
		row := rows[i]
		if row.LastUpdated.After(maxSeen) {
			maxSeen = row.LastUpdated
		}
		key := aggKey{lotNo: row.LotNo, machineNo: row.MachineNo, date: row.TrnDate, shift: row.TrnShift}
		val, ok := aggregated[key]
		if !ok {
			val = &aggValue{area: row.Area}
			aggregated[key] = val
		}
		val.totalBobbins += row.TotalBobbins
		val.fullBobbins += row.FullBobbins
		val.unfullBobbins += row.UnfullBobbins
		val.normalBobs += row.NormalBobs
		val.downgradeBobs += row.DowngradeBobs
		val.pendingBobs += row.PendingBobs
		val.packCekBobs += row.PackCekBobs
	}
	return aggregated, maxSeen
}

// processAggregate matches one aggregate to a WO and upserts its production-actual
// row, computing the bobbin quantity from the lot's standard weights.
func (e *TxtProductionETL) processAggregate(ctx context.Context, key aggKey, val *aggValue, res *Result) error {
	woID, area, ok, err := e.repo.MatchWO(ctx, key.lotNo, key.machineNo)
	if err != nil {
		return err
	}
	if !ok {
		res.Unmatched++
		log.Warn().
			Str("lot_no", key.lotNo).
			Str("machine_no", key.machineNo).
			Str("shift", key.shift).
			Msg("txt production ETL: SYNC_FAILED, no matching WO")
		return nil
	}

	stdFull, stdUnfull, hasLot, err := e.repo.LotStdWeights(ctx, key.lotNo)
	if err != nil {
		return err
	}
	var qtyBobbin float64
	if hasLot {
		qtyBobbin = shared.QtyTXT(val.fullBobbins, val.unfullBobbins, stdFull, stdUnfull)
	}

	if area == "" {
		area = val.area
	}
	upsert := postgres.ProductionActualUpsert{
		WOID:          woID,
		Date:          key.date,
		Shift:         key.shift,
		Area:          area,
		TotalBobbins:  val.totalBobbins,
		FullBobbins:   val.fullBobbins,
		UnfullBobbins: val.unfullBobbins,
		NormalBobs:    val.normalBobs,
		DowngradeBobs: val.downgradeBobs,
		PendingBobs:   val.pendingBobs,
		PackCekBobs:   val.packCekBobs,
		QtyBobbin:     qtyBobbin,
	}
	if err := e.repo.UpsertProductionActual(ctx, upsert); err != nil {
		return err
	}
	res.Upserted++
	return nil
}
