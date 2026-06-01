package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/bi/factmetric"
)

const (
	misDefaultMetricName     = "VALUE"
	misDefaultMetricCategory = "VALUE"
	misDefaultAggMethod      = "SUM"
)

// dimensionKey builds the deduplication key used in bi_fact_metric upsert.
// Format: type|group_1|group_2|group_3|grain|YYYYMMDD|metric_name|scenario.
func dimensionKey(fmType, g1, g2, g3, grain string, periodDate time.Time, metricName, scenario string) string {
	return strings.Join([]string{
		fmType, g1, g2, g3, grain,
		periodDate.Format("20060102"),
		metricName, scenario,
	}, "|")
}

// BIMVRow holds one row from any of the three BI Oracle MVs.
// Fields absent in the MIS MV (no metric columns) carry zero values; callers
// supply defaults via ToFactMetric.
type BIMVRow struct {
	Type           string
	Group1         string
	Group2         string
	Group3         string
	Group1Order    int
	Group2Order    int
	Group3Order    int
	PeriodGrain    string
	PeriodDate     time.Time
	PeriodLabel    string
	Value          float64
	DisplayValue   float64
	UOM            string
	Scenario       string
	SourceCode     string
	MetricName     string // empty for MIS (no FM_METRIC_NAME column)
	MetricCategory string
	AggMethod      string
}

// ToFactMetric converts a BIMVRow to a factmetric.FactMetric domain value.
// MIS rows get default metric_name/category/agg_method='VALUE'/'VALUE'/'SUM'.
func (r BIMVRow) ToFactMetric(sourceID uuid.UUID) factmetric.FactMetric {
	mn := r.MetricName
	if mn == "" {
		mn = misDefaultMetricName
	}
	mc := r.MetricCategory
	if mc == "" {
		mc = misDefaultMetricCategory
	}
	am := r.AggMethod
	if am == "" {
		am = misDefaultAggMethod
	}
	typeNorm := strings.ToUpper(strings.TrimSpace(r.Type))
	dk := dimensionKey(typeNorm, r.Group1, r.Group2, r.Group3, r.PeriodGrain, r.PeriodDate, mn, r.Scenario)
	return factmetric.FactMetric{
		Type:           typeNorm,
		Group1:         r.Group1,
		Group2:         r.Group2,
		Group3:         r.Group3,
		Group1Order:    r.Group1Order,
		Group2Order:    r.Group2Order,
		Group3Order:    r.Group3Order,
		PeriodGrain:    r.PeriodGrain,
		PeriodDate:     r.PeriodDate,
		PeriodLabel:    r.PeriodLabel,
		Value:          r.Value,
		DisplayValue:   r.DisplayValue,
		UOM:            r.UOM,
		Scenario:       r.Scenario,
		SourceID:       sourceID, // use the job's registered source_id (env-specific UUID from bi_data_source)
		MetricName:     mn,
		MetricCategory: mc,
		AggMethod:      am,
		DimensionKey:   dk,
		IsActive:       true,
	}
}

// BIMVRepository fetches rows from the three Oracle BI materialized views.
type BIMVRepository struct {
	client *Client
}

// NewBIMVRepository constructs a BIMVRepository.
func NewBIMVRepository(client *Client) *BIMVRepository {
	return &BIMVRepository{client: client}
}

// FetchMIS fetches all rows from MGTDAT.MV_DASH_MIS_MGT.
// The MIS MV has no FM_METRIC_NAME / FM_METRIC_CATEGORY / FM_AGG_METHOD columns;
// defaults are applied in BIMVRow.ToFactMetric.
func (r *BIMVRepository) FetchMIS(ctx context.Context) ([]BIMVRow, error) {
	const q = `SELECT
		FM_TYPE, FM_GROUP_1, FM_GROUP_2, FM_GROUP_3,
		FM_GROUP_1_ORDER, FM_GROUP_2_ORDER, FM_GROUP_3_ORDER,
		FM_PERIODE_GRAIN, FM_PERIODE_DATE, FM_PERIODE_LABEL,
		FM_VALUE, FM_DISPLAY_VALUE, FM_UOM, FM_SCENARIO, FM_SOURCE_ID
	FROM MGTDAT.MV_DASH_MIS_MGT`
	rows, err := r.fetchRows(ctx, q, false)
	if err != nil {
		return nil, fmt.Errorf("fetch MIS MV: %w", err)
	}
	return rows, nil
}

// FetchDeliveryMargin fetches all rows from MGTDAT.MV_DASH_DELMAR_MGT.
// This MV includes metric columns (FM_METRIC_NAME, FM_METRIC_CATEGORY, FM_AGG_METHOD)
// and uses FM_SOURCE_CODE (CHAR 10) instead of FM_SOURCE_ID.
func (r *BIMVRepository) FetchDeliveryMargin(ctx context.Context) ([]BIMVRow, error) {
	const q = `SELECT
		FM_TYPE, FM_GROUP_1, FM_GROUP_2, FM_GROUP_3,
		FM_GROUP_1_ORDER, FM_GROUP_2_ORDER, FM_GROUP_3_ORDER,
		FM_PERIODE_GRAIN, FM_PERIODE_DATE, FM_PERIODE_LABEL,
		FM_VALUE, FM_DISPLAY_VALUE, FM_UOM, FM_SCENARIO, FM_SOURCE_CODE,
		FM_METRIC_NAME, FM_METRIC_CATEGORY, FM_AGG_METHOD
	FROM MGTDAT.MV_DASH_DELMAR_MGT`
	rows, err := r.fetchRows(ctx, q, true)
	if err != nil {
		return nil, fmt.Errorf("fetch DELMAR MV: %w", err)
	}
	return rows, nil
}

// FetchSales fetches all rows from MGTDAT.MV_DASH_SALES_MGT.
// This MV includes metric columns and uses FM_SOURCE_CODE.
func (r *BIMVRepository) FetchSales(ctx context.Context) ([]BIMVRow, error) {
	const q = `SELECT
		FM_TYPE, FM_GROUP_1, FM_GROUP_2, FM_GROUP_3,
		FM_GROUP_1_ORDER, FM_GROUP_2_ORDER, FM_GROUP_3_ORDER,
		FM_PERIODE_GRAIN, FM_PERIODE_DATE, FM_PERIODE_LABEL,
		FM_VALUE, FM_DISPLAY_VALUE, FM_UOM, FM_SCENARIO, FM_SOURCE_CODE,
		FM_METRIC_NAME, FM_METRIC_CATEGORY, FM_AGG_METHOD
	FROM MGTDAT.MV_DASH_SALES_MGT`
	rows, err := r.fetchRows(ctx, q, true)
	if err != nil {
		return nil, fmt.Errorf("fetch SALES MV: %w", err)
	}
	return rows, nil
}

// fetchRows runs query and scans rows.
// hasMetric=true means the SELECT includes FM_METRIC_NAME/CATEGORY/AGG_METHOD columns.
func (r *BIMVRepository) fetchRows(ctx context.Context, query string, hasMetric bool) ([]BIMVRow, error) {
	rows, err := r.client.db.QueryContext(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("oracle MV query: %w", err)
	}
	defer func() {
		if ce := rows.Close(); ce != nil {
			r.client.logger.Warn().Err(ce).Msg("close oracle MV rows")
		}
	}()

	var result []BIMVRow
	for rows.Next() {
		row, scanErr := r.scanRow(rows, hasMetric)
		if scanErr != nil {
			return nil, fmt.Errorf("scan MV row: %w", scanErr)
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MV rows: %w", err)
	}
	return result, nil
}

// scanRow scans one row from an Oracle MV query into a BIMVRow.
func (r *BIMVRepository) scanRow(rows *sql.Rows, hasMetric bool) (BIMVRow, error) {
	var row BIMVRow
	var periodeDate sql.NullTime
	var g1o, g2o, g3o sql.NullInt64
	var srcCode sql.NullString

	if hasMetric {
		var mn, mc, am sql.NullString
		err := rows.Scan(
			&row.Type, &row.Group1, &row.Group2, &row.Group3,
			&g1o, &g2o, &g3o,
			&row.PeriodGrain, &periodeDate, &row.PeriodLabel,
			&row.Value, &row.DisplayValue, &row.UOM, &row.Scenario, &srcCode,
			&mn, &mc, &am,
		)
		if err != nil {
			return BIMVRow{}, err
		}
		row.MetricName = mn.String
		row.MetricCategory = mc.String
		row.AggMethod = am.String
	} else {
		err := rows.Scan(
			&row.Type, &row.Group1, &row.Group2, &row.Group3,
			&g1o, &g2o, &g3o,
			&row.PeriodGrain, &periodeDate, &row.PeriodLabel,
			&row.Value, &row.DisplayValue, &row.UOM, &row.Scenario, &srcCode,
		)
		if err != nil {
			return BIMVRow{}, err
		}
	}

	if periodeDate.Valid {
		row.PeriodDate = periodeDate.Time
	}
	if g1o.Valid {
		row.Group1Order = int(g1o.Int64) //nolint:gosec // Oracle order values are small ints, safe narrowing
	}
	if g2o.Valid {
		row.Group2Order = int(g2o.Int64) //nolint:gosec // Oracle order values are small ints, safe narrowing
	}
	if g3o.Valid {
		row.Group3Order = int(g3o.Int64) //nolint:gosec // Oracle order values are small ints, safe narrowing
	}
	row.SourceCode = strings.TrimSpace(srcCode.String)
	return row, nil
}
