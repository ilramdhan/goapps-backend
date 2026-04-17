package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/syncdata"
)

// ItemConsStockPORepository fetches data from Oracle MGTDAT.MGT_ITEM_CONS_STK_PO.
type ItemConsStockPORepository struct {
	client *Client
}

// NewItemConsStockPORepository creates a new repository instance.
func NewItemConsStockPORepository(client *Client) *ItemConsStockPORepository {
	return &ItemConsStockPORepository{client: client}
}

// Verify interface compliance at compile time.
var _ syncdata.OracleSourceRepository = (*ItemConsStockPORepository)(nil)

// ExecuteProcedure delegates to the Oracle client.
func (r *ItemConsStockPORepository) ExecuteProcedure(ctx context.Context, schema, procedure string) error {
	return r.client.ExecuteProcedure(ctx, schema, procedure)
}

// ExecuteProcedureWithParam delegates to the Oracle client.
func (r *ItemConsStockPORepository) ExecuteProcedureWithParam(ctx context.Context, schema, procedure, param string) error {
	return r.client.ExecuteProcedureWithParam(ctx, schema, procedure, param)
}

// oracleColumns lists the explicit columns to SELECT from Oracle (matches scanItemConsStockPO order).
const oracleColumns = `MICSP_PERIOD, MICSP_ITEM_CODE, MICSP_GRADE_CODE_2,
	MICSP_GRADE_NAME, MICSP_ITEM_NAME, MICSP_UOM,
	MICSP_CONS_QTY, MICSP_CONS_VAL, MICSP_CONS_RATE,
	MICSP_STORES_QTY, MICSP_STORES_VAL, MICSP_STORES_RATE,
	MICSP_DEPT_QTY, MICSP_DEPT_VAL, MICSP_DEPT_RATE,
	MICSP_LAST_PO_QTY1, MICSP_LAST_PO_VAL1, MICSP_LAST_PO_RATE1, MICSP_LAST_PO_DT1,
	MICSP_LAST_PO_QTY2, MICSP_LAST_PO_VAL2, MICSP_LAST_PO_RATE2, MICSP_LAST_PO_DT2,
	MICSP_LAST_PO_QTY3, MICSP_LAST_PO_VAL3, MICSP_LAST_PO_RATE3, MICSP_LAST_PO_DT3`

// FetchItemConsStockPO fetches records for a specific period.
func (r *ItemConsStockPORepository) FetchItemConsStockPO(ctx context.Context, period string) ([]*syncdata.ItemConsStockPO, error) {
	query := `SELECT ` + oracleColumns + ` FROM MGTDAT.MGT_ITEM_CONS_STK_PO WHERE MICSP_PERIOD = :1`
	return r.fetchRows(ctx, query, period)
}

// FetchAllItemConsStockPO fetches all records.
func (r *ItemConsStockPORepository) FetchAllItemConsStockPO(ctx context.Context) ([]*syncdata.ItemConsStockPO, error) {
	query := `SELECT ` + oracleColumns + ` FROM MGTDAT.MGT_ITEM_CONS_STK_PO`
	return r.fetchRows(ctx, query)
}

func (r *ItemConsStockPORepository) fetchRows(ctx context.Context, query string, args ...any) ([]*syncdata.ItemConsStockPO, error) {
	r.client.logger.Info().Str("query", query).Msg("Fetching data from Oracle")
	start := time.Now()

	rows, err := r.client.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("oracle query: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			r.client.logger.Warn().Err(closeErr).Msg("failed to close oracle rows")
		}
	}()

	var items []*syncdata.ItemConsStockPO
	for rows.Next() {
		item, scanErr := scanItemConsStockPO(rows)
		if scanErr != nil {
			return nil, fmt.Errorf("scan oracle row: %w", scanErr)
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("oracle rows iteration: %w", err)
	}

	r.client.logger.Info().
		Int("rows", len(items)).
		Dur("duration", time.Since(start)).
		Msg("Oracle data fetch completed")

	return items, nil
}

func scanItemConsStockPO(rows *sql.Rows) (*syncdata.ItemConsStockPO, error) {
	var (
		period    string
		itemCode  string
		gradeCode string
		gradeName sql.NullString
		itemName  sql.NullString
		uom       sql.NullString

		consQty  sql.NullFloat64
		consVal  sql.NullFloat64
		consRate sql.NullFloat64

		storesQty  sql.NullFloat64
		storesVal  sql.NullFloat64
		storesRate sql.NullFloat64

		deptQty  sql.NullFloat64
		deptVal  sql.NullFloat64
		deptRate sql.NullFloat64

		lastPOQty1  sql.NullFloat64
		lastPOVal1  sql.NullFloat64
		lastPORate1 sql.NullFloat64
		lastPODt1   sql.NullTime

		lastPOQty2  sql.NullFloat64
		lastPOVal2  sql.NullFloat64
		lastPORate2 sql.NullFloat64
		lastPODt2   sql.NullTime

		lastPOQty3  sql.NullFloat64
		lastPOVal3  sql.NullFloat64
		lastPORate3 sql.NullFloat64
		lastPODt3   sql.NullTime
	)

	err := rows.Scan(
		&period, &itemCode, &gradeCode,
		&gradeName, &itemName, &uom,
		&consQty, &consVal, &consRate,
		&storesQty, &storesVal, &storesRate,
		&deptQty, &deptVal, &deptRate,
		&lastPOQty1, &lastPOVal1, &lastPORate1, &lastPODt1,
		&lastPOQty2, &lastPOVal2, &lastPORate2, &lastPODt2,
		&lastPOQty3, &lastPOVal3, &lastPORate3, &lastPODt3,
	)
	if err != nil {
		return nil, err
	}

	return &syncdata.ItemConsStockPO{
		Period:    period,
		ItemCode:  itemCode,
		GradeCode: gradeCode,
		GradeName: gradeName.String,
		ItemName:  itemName.String,
		UOM:       uom.String,

		ConsQty:  nullFloat(consQty),
		ConsVal:  nullFloat(consVal),
		ConsRate: nullFloat(consRate),

		StoresQty:  nullFloat(storesQty),
		StoresVal:  nullFloat(storesVal),
		StoresRate: nullFloat(storesRate),

		DeptQty:  nullFloat(deptQty),
		DeptVal:  nullFloat(deptVal),
		DeptRate: nullFloat(deptRate),

		LastPOQty1:  nullFloat(lastPOQty1),
		LastPOVal1:  nullFloat(lastPOVal1),
		LastPORate1: nullFloat(lastPORate1),
		LastPODt1:   nullTime(lastPODt1),

		LastPOQty2:  nullFloat(lastPOQty2),
		LastPOVal2:  nullFloat(lastPOVal2),
		LastPORate2: nullFloat(lastPORate2),
		LastPODt2:   nullTime(lastPODt2),

		LastPOQty3:  nullFloat(lastPOQty3),
		LastPOVal3:  nullFloat(lastPOVal3),
		LastPORate3: nullFloat(lastPORate3),
		LastPODt3:   nullTime(lastPODt3),
	}, nil
}

func nullFloat(nf sql.NullFloat64) *float64 {
	if !nf.Valid {
		return nil
	}
	return &nf.Float64
}

func nullTime(nt sql.NullTime) *time.Time {
	if !nt.Valid {
		return nil
	}
	return &nt.Time
}
