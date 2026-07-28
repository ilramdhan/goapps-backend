package etl

import (
	"context"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/oracle"
	"github.com/mutugading/goapps-backend/services/ppc/internal/infrastructure/postgres"
)

// gradeActualTable is the Oracle source table name and watermark key for the
// packing grade-actual ETL.
const gradeActualTable = "PPC_GRADE_ACTUAL"

// GradeActualSource lists incremental packing grade-actual rows from Oracle. A
// nil implementation (Oracle unavailable) is tolerated by the usecase.
type GradeActualSource interface {
	ListGradeActuals(ctx context.Context, watermark time.Time) ([]oracle.GradeActualRow, error)
}

// GradeActualRepo is the write/read surface the grade-actual ETL needs.
// Implemented by postgres.ETLRepository.
type GradeActualRepo interface {
	GetWatermark(ctx context.Context, table string) (time.Time, error)
	AdvanceWatermark(ctx context.Context, table string, ts time.Time) error
	MatchWOByLot(ctx context.Context, lotNo string) (woID int64, ok bool, err error)
	UpsertGradeActual(ctx context.Context, in postgres.GradeActualUpsert) error
}

// GradeActualETL is the incremental packing grade-actual ETL usecase. It pulls
// rows newer than the watermark from MGTDAT.PPC_GRADE_ACTUAL, matches each
// ORIGINAL_LOT_NO to a WO via wo_lot_no, and upserts wo_grade_actual by
// (wo, lot, grade). This confirms the suggest-chain P1 (PACKING_DONE) branch live.
type GradeActualETL struct {
	source     GradeActualSource
	repo       GradeActualRepo
	bufferMins int
}

// NewGradeActualETL builds the usecase. source may be nil (Oracle unavailable);
// bufferMins rewinds the advanced watermark so late rows re-enter next run.
func NewGradeActualETL(source GradeActualSource, repo GradeActualRepo, bufferMins int) *GradeActualETL {
	if bufferMins < 0 {
		bufferMins = 0
	}
	return &GradeActualETL{source: source, repo: repo, bufferMins: bufferMins}
}

// Run executes one incremental ETL pass. A nil source (Oracle unavailable) is a
// no-op (not an error). Unmatched lots are logged and skipped, not fatal.
func (e *GradeActualETL) Run(ctx context.Context) (Result, error) {
	res := Result{}
	if e.source == nil {
		log.Info().Msg("grade actual ETL: oracle unavailable, skipping run")
		return res, nil
	}

	watermark, err := e.repo.GetWatermark(ctx, gradeActualTable)
	if err != nil {
		return res, err
	}

	rows, err := e.source.ListGradeActuals(ctx, watermark)
	if err != nil {
		return res, err
	}
	res.OracleUp = true
	res.Pulled = len(rows)
	if len(rows) == 0 {
		return res, nil
	}

	var maxSeen time.Time
	for i := range rows {
		row := rows[i]
		if row.LastUpdated.After(maxSeen) {
			maxSeen = row.LastUpdated
		}
		if err := e.processRow(ctx, row, &res); err != nil {
			return res, err
		}
	}

	advanced := maxSeen.Add(-time.Duration(e.bufferMins) * time.Minute)
	if err := e.repo.AdvanceWatermark(ctx, gradeActualTable, advanced); err != nil {
		return res, err
	}
	return res, nil
}

// processRow matches one grade-actual row to a WO by its original lot and upserts
// the wo_grade_actual line. An unmatched lot is logged as SYNC_FAILED and skipped.
func (e *GradeActualETL) processRow(ctx context.Context, row oracle.GradeActualRow, res *Result) error {
	woID, ok, err := e.repo.MatchWOByLot(ctx, row.OriginalLotNo)
	if err != nil {
		return err
	}
	if !ok {
		res.Unmatched++
		log.Warn().
			Str("lot_no", row.OriginalLotNo).
			Str("grade", row.Grade).
			Str("dept", row.Dept).
			Msg("grade actual ETL: SYNC_FAILED, no matching WO")
		return nil
	}

	upsert := postgres.GradeActualUpsert{
		WOID:            woID,
		LotNo:           row.OriginalLotNo,
		Grade:           row.Grade,
		Dept:            row.Dept,
		TotalQtyKg:      row.TotalQtyKg,
		BobbinCount:     row.TotalBobbinCount,
		LastPackingDate: row.LastPackingDate,
	}
	if err := e.repo.UpsertGradeActual(ctx, upsert); err != nil {
		return err
	}
	res.Upserted++
	return nil
}
