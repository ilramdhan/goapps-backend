package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// GradeActualRow is a raw row from MGTDAT.PPC_GRADE_ACTUAL used by the packing
// grade-actual ETL (Phase 3). Natural key: ORIGINAL_LOT_NO+GRADE+DEPT.
type GradeActualRow struct {
	OriginalLotNo    string
	Grade            string
	Dept             string
	TotalQtyKg       float64
	TotalBobbinCount int
	LastPackingDate  time.Time
	LastUpdated      time.Time
}

const gradeActualQuery = `SELECT ORIGINAL_LOT_NO, GRADE, DEPT, TOTAL_QTY_KG, ` +
	`TOTAL_BOBBIN_COUNT, LAST_PACKING_DATE, LAST_UPDATED ` +
	`FROM MGTDAT.PPC_GRADE_ACTUAL WHERE LAST_UPDATED > :1 ORDER BY LAST_UPDATED`

// ListGradeActuals reads incremental packing grade-actual rows whose LAST_UPDATED
// is strictly greater than the watermark. A nil client or nil pool (Oracle
// unavailable) yields (nil, nil) so callers degrade gracefully. Read-only.
func (c *Client) ListGradeActuals(ctx context.Context, watermark time.Time) ([]GradeActualRow, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, gradeActualQuery, watermark)
	if err != nil {
		return nil, fmt.Errorf("query PPC_GRADE_ACTUAL: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close PPC_GRADE_ACTUAL rows")
		}
	}()

	var result []GradeActualRow
	for rows.Next() {
		row, scanErr := scanGradeActualRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PPC_GRADE_ACTUAL rows: %w", err)
	}
	return result, nil
}

// scanGradeActualRow scans one grade-actual row, coalescing nullable numerics to
// zero and trimming string columns.
func scanGradeActualRow(rows *sql.Rows) (GradeActualRow, error) {
	var (
		originalLotNo, grade, dept string
		totalQtyKg                 sql.NullFloat64
		totalBobbinCount           sql.NullInt64
		lastPackingDate            sql.NullTime
		lastUpdated                time.Time
	)
	if err := rows.Scan(
		&originalLotNo, &grade, &dept, &totalQtyKg,
		&totalBobbinCount, &lastPackingDate, &lastUpdated,
	); err != nil {
		return GradeActualRow{}, fmt.Errorf("scan PPC_GRADE_ACTUAL row: %w", err)
	}
	return GradeActualRow{
		OriginalLotNo:    strings.TrimSpace(originalLotNo),
		Grade:            strings.TrimSpace(grade),
		Dept:             strings.TrimSpace(dept),
		TotalQtyKg:       totalQtyKg.Float64,
		TotalBobbinCount: int(totalBobbinCount.Int64),
		LastPackingDate:  lastPackingDate.Time,
		LastUpdated:      lastUpdated,
	}, nil
}
