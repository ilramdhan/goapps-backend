package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// TxtProductionRow is a raw row from MGTDAT.PPC_TXT_PRODUCTION used by the TXT/TWT
// production ETL. Natural key: LOT_NO+MACHINE_NO+TRN_DATE+TRN_SHIFT+DOFF_NO.
// TRN_STS TXT convention: 0=Full (FULL_BOBBINS), 1=Unfull (UNFULL_BOBBINS).
type TxtProductionRow struct {
	LotNo         string
	MachineNo     string
	Area          string
	TrnDate       time.Time
	TrnShift      string
	DoffNo        int
	TotalBobbins  int
	FullBobbins   int
	UnfullBobbins int
	NormalBobs    int
	DowngradeBobs int
	PendingBobs   int
	PackCekBobs   int
	LastUpdated   time.Time
}

const txtProductionQuery = `SELECT LOT_NO, MACHINE_NO, AREA, TRN_DATE, TRN_SHIFT, ` +
	`DOFF_NO, TOTAL_BOBBINS, FULL_BOBBINS, UNFULL_BOBBINS, NORMAL_BOBS, ` +
	`DOWNGRADE_BOBS, PENDING_BOBS, PACK_CEK_BOBS, LAST_UPDATED ` +
	`FROM MGTDAT.PPC_TXT_PRODUCTION WHERE LAST_UPDATED > :1 ORDER BY LAST_UPDATED`

// ListTxtProduction reads incremental TXT/TWT production rows whose LAST_UPDATED
// is strictly greater than the watermark. A nil client or nil pool (Oracle
// unavailable) yields (nil, nil) so callers degrade gracefully. Read-only.
func (c *Client) ListTxtProduction(ctx context.Context, watermark time.Time) ([]TxtProductionRow, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, txtProductionQuery, watermark)
	if err != nil {
		return nil, fmt.Errorf("query PPC_TXT_PRODUCTION: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close PPC_TXT_PRODUCTION rows")
		}
	}()

	var result []TxtProductionRow
	for rows.Next() {
		row, scanErr := scanTxtProductionRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate PPC_TXT_PRODUCTION rows: %w", err)
	}
	return result, nil
}

// scanTxtProductionRow scans one production row, coalescing nullable numerics to
// zero and trimming string columns.
func scanTxtProductionRow(rows *sql.Rows) (TxtProductionRow, error) {
	var (
		lotNo, machineNo, area, trnShift         string
		doffNo, totalBobbins, fullBobbins        sql.NullInt64
		unfullBobbins, normalBobs, downgradeBobs sql.NullInt64
		pendingBobs, packCekBobs                 sql.NullInt64
		trnDate, lastUpdated                     time.Time
	)
	if err := rows.Scan(
		&lotNo, &machineNo, &area, &trnDate, &trnShift,
		&doffNo, &totalBobbins, &fullBobbins, &unfullBobbins, &normalBobs,
		&downgradeBobs, &pendingBobs, &packCekBobs, &lastUpdated,
	); err != nil {
		return TxtProductionRow{}, fmt.Errorf("scan PPC_TXT_PRODUCTION row: %w", err)
	}
	return TxtProductionRow{
		LotNo:         strings.TrimSpace(lotNo),
		MachineNo:     strings.TrimSpace(machineNo),
		Area:          strings.TrimSpace(area),
		TrnDate:       trnDate,
		TrnShift:      strings.TrimSpace(trnShift),
		DoffNo:        int(doffNo.Int64),
		TotalBobbins:  int(totalBobbins.Int64),
		FullBobbins:   int(fullBobbins.Int64),
		UnfullBobbins: int(unfullBobbins.Int64),
		NormalBobs:    int(normalBobs.Int64),
		DowngradeBobs: int(downgradeBobs.Int64),
		PendingBobs:   int(pendingBobs.Int64),
		PackCekBobs:   int(packCekBobs.Int64),
		LastUpdated:   lastUpdated,
	}, nil
}
