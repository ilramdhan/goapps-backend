package postgres

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	cprapp "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
)

// ParamSummaryRepository implements cprapp.ParamSummaryRepository.
type ParamSummaryRepository struct {
	db *DB
}

// NewParamSummaryRepository constructs a ParamSummaryRepository.
func NewParamSummaryRepository(db *DB) *ParamSummaryRepository {
	return &ParamSummaryRepository{db: db}
}

var _ cprapp.ParamSummaryRepository = (*ParamSummaryRepository)(nil)

// paramSummaryRow is a flat row returned by the SQL query.
type paramSummaryRow struct {
	productSysID int64
	productCode  string
	productName  string
	routeLevel   int32
	taskStatus   string
	filledByUser string
	filledAt     string
	paramID      string
	paramCode    string
	paramName    string
	dataType     string
	uomCode      string
	isRequired   bool
	hasValue     bool
	valueNumeric string
	valueText    string
	valueFlag    bool
}

const paramSummarySQL = `
SELECT
    crs.crs_product_sys_id,
    COALESCE(cpm.cpm_product_code, '') AS product_code,
    COALESCE(cpm.cpm_product_name, '') AS product_name,
    crs.crs_route_level,
    COALESCE(cft.cft_status, 'ACTIVE')                                  AS task_status,
    COALESCE(cft.cft_claimed_by, '')                                     AS filled_by_user_id,
    COALESCE(to_char(cft.cft_filled_at AT TIME ZONE 'UTC',
             'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')                          AS filled_at,
    p.id::text                                                           AS param_id,
    p.param_code,
    p.param_name,
    p.data_type,
    COALESCE(u.uom_code, '')                                             AS uom_code,
    COALESCE(a.capp_is_required, FALSE)                                  AS is_required,
    CASE WHEN cpp.cpp_value_id IS NOT NULL THEN TRUE ELSE FALSE END      AS has_value,
    COALESCE(cpp.cpp_value_numeric::text, '')                            AS value_numeric,
    COALESCE(cpp.cpp_value_text, '')                                     AS value_text,
    COALESCE(cpp.cpp_value_flag, FALSE)                                  AS value_flag
FROM cost_product_request req
JOIN cost_route_head crh
    ON crh.crh_head_id = req.cpr_linked_route_head_id
JOIN cost_route_seq crs
    ON crs.crs_head_id = crh.crh_head_id
       AND crs.crs_deleted_at IS NULL
JOIN cost_product_master cpm
    ON cpm.cpm_product_sys_id = crs.crs_product_sys_id
LEFT JOIN cost_fill_task cft
    ON cft.cft_request_id = req.cpr_request_id
       AND cft.cft_route_level = crs.crs_route_level
JOIN cost_product_applicable_param a
    ON a.capp_product_sys_id = crs.crs_product_sys_id
JOIN mst_parameter p
    ON p.id = a.capp_param_id
       AND p.deleted_at IS NULL
       AND p.is_active = TRUE
LEFT JOIN mst_uom u
    ON u.uom_id = p.uom_id
       AND u.deleted_at IS NULL
LEFT JOIN cost_product_parameter cpp
    ON cpp.cpp_product_sys_id = crs.crs_product_sys_id
       AND cpp.cpp_param_id = p.id
WHERE req.cpr_request_id = $1
  AND req.cpr_deleted_at IS NULL
ORDER BY crs.crs_route_level, crs.crs_product_sys_id, p.param_code`

// GetParamSummary returns the full param summary nested by product → level.
func (r *ParamSummaryRepository) GetParamSummary(ctx context.Context, requestID int64) ([]cprapp.ProductSummaryRow, error) {
	rows, err := r.db.QueryContext(ctx, paramSummarySQL, requestID)
	if err != nil {
		return nil, fmt.Errorf("query param summary: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil {
			_ = cerr
		}
	}()

	flat := []paramSummaryRow{}
	for rows.Next() {
		var fr paramSummaryRow
		if scanErr := rows.Scan(
			&fr.productSysID, &fr.productCode, &fr.productName,
			&fr.routeLevel, &fr.taskStatus, &fr.filledByUser, &fr.filledAt,
			&fr.paramID, &fr.paramCode, &fr.paramName, &fr.dataType,
			&fr.uomCode, &fr.isRequired, &fr.hasValue,
			&fr.valueNumeric, &fr.valueText, &fr.valueFlag,
		); scanErr != nil {
			return nil, fmt.Errorf("scan param summary row: %w", scanErr)
		}
		flat = append(flat, fr)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate param summary rows: %w", err)
	}

	return nestParamSummary(flat), nil
}

// nestParamSummary converts flat rows → nested ProductSummaryRow slice.
// Insertion order is preserved because we use slice indices tracked via a seen map.
func nestParamSummary(flat []paramSummaryRow) []cprapp.ProductSummaryRow {
	type levelKey struct {
		sysID int64
		level int32
	}

	seenProduct := map[int64]int{}
	seenLevel := map[levelKey]int{}
	productRows := []cprapp.ProductSummaryRow{}

	for _, fr := range flat {
		pIdx, pOK := seenProduct[fr.productSysID]
		if !pOK {
			pIdx = len(productRows)
			seenProduct[fr.productSysID] = pIdx
			productRows = append(productRows, cprapp.ProductSummaryRow{
				ProductSysID: fr.productSysID,
				ProductCode:  fr.productCode,
				ProductName:  fr.productName,
			})
		}

		lk := levelKey{fr.productSysID, fr.routeLevel}
		lIdx, lOK := seenLevel[lk]
		if !lOK {
			lIdx = len(productRows[pIdx].Levels)
			seenLevel[lk] = lIdx
			productRows[pIdx].Levels = append(productRows[pIdx].Levels, cprapp.LevelSummaryRow{
				RouteLevel:     fr.routeLevel,
				TaskStatus:     fr.taskStatus,
				FilledByUserID: fr.filledByUser,
				FilledAt:       fr.filledAt,
			})
		}

		param := cprapp.ParamValueRow{
			ParamID:      fr.paramID,
			ParamCode:    fr.paramCode,
			ParamName:    fr.paramName,
			DataType:     fr.dataType,
			HasValue:     fr.hasValue,
			ValueNumeric: fr.valueNumeric,
			ValueText:    fr.valueText,
			ValueFlag:    fr.valueFlag,
			UOMCode:      fr.uomCode,
			IsRequired:   fr.isRequired,
		}
		productRows[pIdx].Levels[lIdx].Params = append(productRows[pIdx].Levels[lIdx].Params, param)
		productRows[pIdx].Levels[lIdx].TotalParams++
		if fr.hasValue {
			productRows[pIdx].Levels[lIdx].FilledParams++
		}
	}

	return productRows
}

// CountUnfilledParams counts unfilled required params across all levels for a route head.
// Used by the LockHandler param completeness checker.
func (r *ParamSummaryRepository) CountUnfilledParams(ctx context.Context, headID int64) (int, error) {
	const q = `
		SELECT COUNT(*)
		FROM cost_route_seq crs
		JOIN cost_product_applicable_param a
		    ON a.capp_product_sys_id = crs.crs_product_sys_id
		    AND a.capp_is_required = TRUE
		LEFT JOIN cost_product_parameter cpp
		    ON cpp.cpp_product_sys_id = crs.crs_product_sys_id
		    AND cpp.cpp_param_id = a.capp_param_id
		JOIN mst_parameter p
		    ON p.id = a.capp_param_id
		    AND p.deleted_at IS NULL
		    AND p.is_active = TRUE
		WHERE crs.crs_head_id = $1
		  AND crs.crs_deleted_at IS NULL
		  AND cpp.cpp_value_id IS NULL`
	var count int
	if err := r.db.QueryRowContext(ctx, q, headID).Scan(&count); err != nil {
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, fmt.Errorf("count unfilled params: %w", err)
		}
	}
	return count, nil
}
