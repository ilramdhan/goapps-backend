package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// SpgProductionRow is a raw row from MGTDAT.PPC_SPG_PRODUCTION used by the SPG
// production ETL. Natural key: LOT_NO+MACHINE_LINE+DOFF_DATE+POSITION_NO+DOFF_NO.
// DOFF_OPTION SPG convention: 1=Full, 2=Unfull (INVERSE of TXT TRN_STS).
// Sanity: GROSS = TRANSFERRED + CUT + NOT_TRANSFER;
// TRANSFERRED = NORMAL + DOWNGRADE + NOT_CHECKED.
type SpgProductionRow struct {
	LotNo          string
	MachineLine    string
	DoffDate       time.Time
	PositionNo     int
	DoffNo         int
	DoffOption     int
	GrossBobbins   int
	TransferredBob int
	CutBobbins     int
	NotTransfer    int
	NormalBobs     int
	DowngradeBobs  int
	NotCheckedBobs int
	WeightPerBob   float64
	LastUpdated    time.Time
}

const spgProductionQuery = `SELECT LOT_NO, MACHINE_LINE, DOFF_DATE, POSITION_NO, DOFF_NO, ` +
	`DOFF_OPTION, GROSS_BOBBINS, TRANSFERRED_BOBS, CUT_BOBBINS, NOT_TRANSFER, ` +
	`NORMAL_BOBS, DOWNGRADE_BOBS, NOT_CHECKED_BOBS, WEIGHT_PER_BOB, LAST_UPDATED ` +
	`FROM MGTDAT.PPC_SPG_PRODUCTION WHERE LAST_UPDATED > :1 ORDER BY LAST_UPDATED`

// ListSpgProduction reads incremental SPG production rows whose LAST_UPDATED is
// strictly greater than the watermark. A nil client or nil pool (Oracle
// unavailable) yields (nil, nil) so callers degrade gracefully. Read-only.
func (c *Client) ListSpgProduction(ctx context.Context, watermark time.Time) ([]SpgProductionRow, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, spgProductionQuery, watermark)
	if err != nil {
		return nil, fmt.Errorf("query PPC_SPG_PRODUCTION: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close PPC_SPG_PRODUCTION rows")
		}
	}()

	var result []SpgProductionRow
	for rows.Next() {
		row, scanErr := scanSpgProductionRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PPC_SPG_PRODUCTION rows: %w", err)
	}
	return result, nil
}

// scanSpgProductionRow scans one SPG production row, coalescing nullable numerics
// to zero and trimming string columns.
func scanSpgProductionRow(rows *sql.Rows) (SpgProductionRow, error) {
	var (
		lotNo, machineLine                       string
		positionNo, doffNo, doffOption           sql.NullInt64
		grossBobbins, transferredBob, cutBobbins sql.NullInt64
		notTransfer, normalBobs, downgradeBobs   sql.NullInt64
		notCheckedBobs                           sql.NullInt64
		weightPerBob                             sql.NullFloat64
		doffDate, lastUpdated                    time.Time
	)
	if err := rows.Scan(
		&lotNo, &machineLine, &doffDate, &positionNo, &doffNo,
		&doffOption, &grossBobbins, &transferredBob, &cutBobbins, &notTransfer,
		&normalBobs, &downgradeBobs, &notCheckedBobs, &weightPerBob, &lastUpdated,
	); err != nil {
		return SpgProductionRow{}, fmt.Errorf("scan PPC_SPG_PRODUCTION row: %w", err)
	}
	return SpgProductionRow{
		LotNo:          strings.TrimSpace(lotNo),
		MachineLine:    strings.TrimSpace(machineLine),
		DoffDate:       doffDate,
		PositionNo:     int(positionNo.Int64),
		DoffNo:         int(doffNo.Int64),
		DoffOption:     int(doffOption.Int64),
		GrossBobbins:   int(grossBobbins.Int64),
		TransferredBob: int(transferredBob.Int64),
		CutBobbins:     int(cutBobbins.Int64),
		NotTransfer:    int(notTransfer.Int64),
		NormalBobs:     int(normalBobs.Int64),
		DowngradeBobs:  int(downgradeBobs.Int64),
		NotCheckedBobs: int(notCheckedBobs.Int64),
		WeightPerBob:   weightPerBob.Float64,
		LastUpdated:    lastUpdated,
	}, nil
}
