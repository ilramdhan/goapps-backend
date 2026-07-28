package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

// SoPendingRow is a raw row from MGTDAT.MGT_SO_PENDING_WEB (sales-order pending
// backlog). Full-replace source: pulled in its entirety each sync (no watermark).
type SoPendingRow struct {
	CustomerCode  string
	CustomerName  string
	ContractNo    string
	ContractDate  time.Time
	ContractSysID int64
	ItemCode      string
	GradeCode     string
	ShadeCode     string
	QtyRemaining  float64
	QtyOrdered    float64
	QtyDelivered  float64
	Deadline      time.Time
	MergeNo       string
	Term          string
	Rate          float64
	BlockedStatus string
	Currency      string
	OutstandingAR float64
}

const soPendingQuery = `SELECT PEND_CUST_CODE, PEND_CUST_NAME, PEND_CONTRACT_NO, ` +
	`PEND_CONTRACT_DT, PEND_CTRT_SYS_ID, PEND_ITEM_CODE, PEND_GRADE_CODE_1, ` +
	`PEND_GRADE_CODE_2, PEND_QTY, PEND_SO_QTY, PEND_DEL_QTY, PEND_DEL_DT, ` +
	`PEND_MERGE_NO, PEND_TERM, PEND_RATE, PEND_STS, PEND_CURR_CODE, PEND_OUTSTANDING ` +
	`FROM MGTDAT.MGT_SO_PENDING_WEB`

// ListSoPending reads the full pending sales-order backlog. A nil client or nil
// pool (Oracle unavailable) yields (nil, nil). Nullable numeric/date columns are
// coalesced to their zero value. Read-only.
func (c *Client) ListSoPending(ctx context.Context) ([]SoPendingRow, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, soPendingQuery)
	if err != nil {
		return nil, fmt.Errorf("query MGT_SO_PENDING_WEB: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close MGT_SO_PENDING_WEB rows")
		}
	}()

	var result []SoPendingRow
	for rows.Next() {
		row, scanErr := scanSoPendingRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MGT_SO_PENDING_WEB rows: %w", err)
	}
	return result, nil
}

// scanSoPendingRow scans one pending SO row, coalescing nullable columns to zero
// values and trimming string columns.
//
// MGT_SO_PENDING_WEB is a pre-existing external table (NOT created by the PPC
// DDL), so it carries no NOT-NULL guarantees. Confirmed NULL columns on live
// data: PEND_MERGE_NO, PEND_TERM, PEND_STS. Every string column is therefore
// scanned via sql.NullString defensively — a NULL in any of them must degrade to
// "" rather than abort the whole ETL sync with a scan error.
func scanSoPendingRow(rows *sql.Rows) (SoPendingRow, error) {
	var (
		custCode, custName, contractNo, itemCode sql.NullString
		gradeCode, shadeCode, mergeNo, term      sql.NullString
		blockedStatus, currency                  sql.NullString
		contractSysID                            sql.NullInt64
		qtyRemaining, qtyOrdered, qtyDelivered   sql.NullFloat64
		rate, outstanding                        sql.NullFloat64
		contractDate, deadline                   sql.NullTime
	)
	if err := rows.Scan(
		&custCode, &custName, &contractNo, &contractDate, &contractSysID,
		&itemCode, &gradeCode, &shadeCode, &qtyRemaining, &qtyOrdered,
		&qtyDelivered, &deadline, &mergeNo, &term, &rate,
		&blockedStatus, &currency, &outstanding,
	); err != nil {
		return SoPendingRow{}, fmt.Errorf("scan MGT_SO_PENDING_WEB row: %w", err)
	}
	return SoPendingRow{
		CustomerCode:  strings.TrimSpace(custCode.String),
		CustomerName:  strings.TrimSpace(custName.String),
		ContractNo:    strings.TrimSpace(contractNo.String),
		ContractDate:  contractDate.Time,
		ContractSysID: contractSysID.Int64,
		ItemCode:      strings.TrimSpace(itemCode.String),
		GradeCode:     strings.TrimSpace(gradeCode.String),
		ShadeCode:     strings.TrimSpace(shadeCode.String),
		QtyRemaining:  qtyRemaining.Float64,
		QtyOrdered:    qtyOrdered.Float64,
		QtyDelivered:  qtyDelivered.Float64,
		Deadline:      deadline.Time,
		MergeNo:       strings.TrimSpace(mergeNo.String),
		Term:          strings.TrimSpace(term.String),
		Rate:          rate.Float64,
		BlockedStatus: strings.TrimSpace(blockedStatus.String),
		Currency:      strings.TrimSpace(currency.String),
		OutstandingAR: outstanding.Float64,
	}, nil
}
