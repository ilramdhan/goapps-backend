// Package oracle provides Oracle database connectivity using go-ora (pure Go driver).
package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/shade"
)

// ShadeRepository fetches shade rows from Oracle MGTDAT.OM_GRADE_CODE_2. This is
// a SELECT-only reader — R8 never writes to Oracle.
type ShadeRepository struct {
	client *Client
}

// NewShadeRepository creates a new repository instance.
func NewShadeRepository(client *Client) *ShadeRepository {
	return &ShadeRepository{client: client}
}

// Verify interface compliance at compile time.
var _ shade.Source = (*ShadeRepository)(nil)

// shadeOracleColumns lists the explicit columns to SELECT from Oracle (matches
// scanShade order). Column names verified against ALL_TAB_COLUMNS on
// MGTDAT.OM_GRADE_CODE_2 (2320 rows in production as of this writing).
//
// GRADE_BL_NAME / GRADE_BL_SHORT_NAME are deliberately NOT selected here — their
// meaning is unknown and is not guessed at (see migration 000493 for the same
// note; production measurement: GRADE_BL_NAME is empty on all 2320 rows,
// GRADE_BL_SHORT_NAME is populated on 1088/2320). GRADE_FRZ_FLAG_NUM is mapped
// to IsActive as NOT (flag = 1), ~~mirroring the ppc customer_is_active <-
// CUST_FRZ_FLAG_NUM convention; this is unverified against production and
// flagged in the report handed back with this change.~~
// ⭐ DIPERBARUI 2026-08-26 — see isActiveFromFrzFlag below for the measured
// production distribution and the still-open decision gate on value 2.
const shadeOracleColumns = `GRADE_CODE, GRADE_NAME, GRADE_SHORT_NAME, GRADE_FRZ_FLAG_NUM,
	GRADE_CR_UID, GRADE_CR_DT, GRADE_UPD_UID, GRADE_UPD_DT`

// ListShades fetches every shade row from Oracle.
func (r *ShadeRepository) ListShades(ctx context.Context) ([]shade.Sourced, error) {
	query := `SELECT ` + shadeOracleColumns + ` FROM MGTDAT.OM_GRADE_CODE_2`

	r.client.logger.Info().Str("query", query).Msg("Fetching shade master from Oracle")
	start := time.Now()

	rows, err := r.client.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("oracle query shade master: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.client.logger.Warn().Err(closeErr).Msg("failed to close oracle shade rows")
		}
	}()

	var items []shade.Sourced
	for rows.Next() {
		item, scanErr := scanShade(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan oracle shade row: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle shade rows iteration: %w", err)
	}

	r.client.logger.Info().
		Int("rows", len(items)).
		Dur("duration", time.Since(start)).
		Msg("Oracle shade master fetch completed")

	return items, nil
}

func scanShade(rows *sql.Rows) (shade.Sourced, error) {
	var (
		code          string
		name          sql.NullString
		shortName     sql.NullString
		frzFlag       sql.NullInt64
		crUID, updUID sql.NullString
		crDt, updDt   sql.NullTime
	)

	err := rows.Scan(&code, &name, &shortName, &frzFlag, &crUID, &crDt, &updUID, &updDt)
	if err != nil {
		return shade.Sourced{}, err
	}

	src := shade.Sourced{
		Code:     code,
		Name:     name.String,
		IsActive: isActiveFromFrzFlag(frzFlag),
	}
	if shortName.Valid {
		s := shortName.String
		src.ShortName = &s
	}
	if crUID.Valid {
		s := crUID.String
		src.SourceCreatedBy = &s
	}
	if updUID.Valid {
		s := updUID.String
		src.SourceUpdatedBy = &s
	}
	if crDt.Valid {
		t := crDt.Time
		src.SourceCreatedAt = &t
	}
	if updDt.Valid {
		t := updDt.Time
		src.SourceUpdatedAt = &t
	}
	return src, nil
}

// isActiveFromFrzFlag maps GRADE_FRZ_FLAG_NUM to Shade.IsActive: 1 = frozen ->
// inactive, mirroring the ppc customer_is_active <- CUST_FRZ_FLAG_NUM
// convention. Any other value (including NULL, which scans as 0) is treated as
// active. ~~This assumption is UNVERIFIED against production data — see the
// aggregate SQL in the R8 report that measures the real GRADE_FRZ_FLAG_NUM
// distribution before this mapping is trusted operationally.~~
//
// ⭐ DIPERBARUI 2026-08-26 — measured against the full production table
// (2320 rows): GRADE_FRZ_FLAG_NUM = 1 on 17 rows, NULL on 2301 rows, and = 2
// on exactly 2 rows. The "1 = frozen -> inactive" half of this mapping is now
// supported by real data (17 rows), not just the ppc analogy.
//
// The "2" case is NOT resolved: its business meaning is unknown, and this
// function currently treats it as active (falls into "any other value").
// That is a DEFAULT, not a verified fact — whether GRADE_FRZ_FLAG_NUM = 2
// should also mean inactive is an OPEN USER DECISION GATE (only 2 rows are
// affected in production, so the blast radius of guessing wrong is small, but
// it is still a guess this function does not make). Do not change the
// `!= 1` comparison below to `> 0` or similar without that decision — doing so
// would silently reclassify those 2 rows based on an unconfirmed assumption.
func isActiveFromFrzFlag(frzFlag sql.NullInt64) bool {
	return frzFlag.Int64 != 1
}
