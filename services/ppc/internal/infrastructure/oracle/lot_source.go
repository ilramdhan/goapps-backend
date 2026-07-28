package oracle

import (
	"context"
	"database/sql"
	"fmt"
	"math"
	"strings"

	"github.com/rs/zerolog/log"
)

// LotRow is a raw row from the legacy Oracle lot master ASPAK.MMSMERGE.
//
// MMSMERGE has 101 columns; only the ~20 PPC plans and packs against are read.
// The rest are legacy spinning-process parameters (oil, jet, disc, spindle,
// nine machine slots, three chip-lot slots) that belong to the old ERP.
type LotRow struct {
	// Code is MERGE_CODE, the lot number and the sole unique key.
	Code string
	// ItemCode is MERGE_ITEM_CODE.
	ItemCode string
	// QCGrade is MERGE_QC_GRADE (AX, Aa, ...).
	QCGrade string
	// ProdType is MERGE_PROD_TYPE: PTY, POY or FOY.
	ProdType string
	// YarnType is YARN_TYPE, which overlaps ProdType but is not identical.
	YarnType string
	// Denier is MERGE_DENIER. VARCHAR2 in the source, kept as text.
	Denier string
	// Filament is MERGE_FILAMENT.
	Filament *int32
	// CrossSection is MERGE_CROSS_SEC: RND or TBL.
	CrossSection string
	// Description is MERGE_DESCR, the legacy spec shorthand.
	Description string
	// ShadeCode is SHADE_CODE. Null on every sampled row.
	ShadeCode string
	// ShadeColor is SHADE_COLOR, falling back in SQL to MERGE_COLOR then
	// YARN_COLOR, which is where the sampled rows actually carry the colour.
	ShadeColor string
	// TareBoxWeight is MERGE_TARE_BOX_WT (kg, empty carton).
	TareBoxWeight *float64
	// TareBobbinWeight is MERGE_TARE_BOBIN_WT (kg, empty bobbin).
	TareBobbinWeight *float64
	// BobbinsPerBox is NVL(MERGE_NOBOB, MERGE_BOX) — the carton fill count.
	BobbinsPerBox *int32
	// BobWeight is MERGE_BOB, provisionally kg of yarn per bobbin. Zero on the
	// POY/FOY samples, in which case it is reported as nil.
	BobWeight *float64
	// OrionItemCode is MERGE_ITEM_ORION.
	OrionItemCode string
	// MachineNo is MERGE_MACHINE.
	MachineNo string
	// EfficiencyPct is MERGE_EFF.
	EfficiencyPct *int32
	// Status is MERGE_STATUS verbatim.
	Status string
	// PakStatus is MERGE_PAK_STATUS verbatim. PPC_ORACLE_PROCEDURES.sql excludes
	// 'B' when aggregating packing, so the flag has to survive the import.
	PakStatus string
}

// mmsmergeQuery is the read-only projection of MMSMERGE the PPC lot sync
// consumes. SELECT only.
//
// Schema-qualified as ASPAK, not MGTDAT: PPC_ORACLE_PROCEDURES.sql joins
// ASPAK.MMSMERGE alongside ASPAK.PAKPKGDUP, so MMSMERGE is owned by ASPAK even
// though the ETL user connects as MGTDAT. An unqualified name would resolve
// against the caller's own schema and fail.
//
// The colour coalesce reflects the sample data: SHADE_COLOR and SHADE_CODE are
// empty on all ten sampled rows while MERGE_COLOR ('BLACK ORANGE',
// 'D-MARINER-03') and YARN_COLOR ('Natural') carry the real value.
const mmsmergeQuery = `
	SELECT MERGE_CODE,
	       MERGE_ITEM_CODE,
	       MERGE_QC_GRADE,
	       MERGE_PROD_TYPE,
	       YARN_TYPE,
	       MERGE_DENIER,
	       MERGE_FILAMENT,
	       MERGE_CROSS_SEC,
	       MERGE_DESCR,
	       SHADE_CODE,
	       NVL(NVL(SHADE_COLOR, MERGE_COLOR), YARN_COLOR),
	       MERGE_TARE_BOX_WT,
	       MERGE_TARE_BOBIN_WT,
	       NVL(MERGE_NOBOB, MERGE_BOX),
	       MERGE_BOB,
	       MERGE_ITEM_ORION,
	       MERGE_MACHINE,
	       MERGE_EFF,
	       MERGE_STATUS,
	       MERGE_PAK_STATUS
	FROM ASPAK.MMSMERGE
	WHERE MERGE_CODE IS NOT NULL`

// ListLots reads the legacy lot master. Read-only (SELECT only).
//
// A nil client (Oracle unconfigured or unreachable at startup) yields no rows
// and no error, so the sync degrades to a no-op rather than failing the service
// — same contract as ListCustomers and ListTxtMachines.
func (c *Client) ListLots(ctx context.Context) ([]LotRow, error) {
	if c == nil || c.db == nil {
		return nil, nil
	}
	rows, err := c.db.QueryContext(ctx, mmsmergeQuery)
	if err != nil {
		return nil, fmt.Errorf("query MMSMERGE: %w", err)
	}
	defer func() {
		if closeErr := rows.Close(); closeErr != nil {
			log.Warn().Err(closeErr).Msg("oracle: failed to close MMSMERGE rows")
		}
	}()

	var result []LotRow
	for rows.Next() {
		row, scanErr := scanLotRow(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate MMSMERGE rows: %w", err)
	}
	return result, nil
}

// scanLotRow reads one MMSMERGE row defensively: every column is scanned
// through a nullable type, because the legacy master is overwhelmingly sparse.
func scanLotRow(rows *sql.Rows) (LotRow, error) {
	var (
		code, itemCode, qcGrade, prodType, yarnType        sql.NullString
		denier, crossSection, descr, shadeCode, shadeColor sql.NullString
		orionItem, machineNo, status, pakStatus            sql.NullString
		filament, bobbinsPerBox, efficiency                sql.NullFloat64
		tareBoxWeight, tareBobbinWeight, bobWeight         sql.NullFloat64
	)
	if err := rows.Scan(
		&code, &itemCode, &qcGrade, &prodType, &yarnType,
		&denier, &filament, &crossSection, &descr,
		&shadeCode, &shadeColor,
		&tareBoxWeight, &tareBobbinWeight, &bobbinsPerBox, &bobWeight,
		&orionItem, &machineNo, &efficiency, &status, &pakStatus,
	); err != nil {
		return LotRow{}, fmt.Errorf("scan MMSMERGE row: %w", err)
	}
	return LotRow{
		Code:             strings.TrimSpace(code.String),
		ItemCode:         strings.TrimSpace(itemCode.String),
		QCGrade:          strings.TrimSpace(qcGrade.String),
		ProdType:         strings.TrimSpace(prodType.String),
		YarnType:         strings.TrimSpace(yarnType.String),
		Denier:           strings.TrimSpace(denier.String),
		Filament:         optionalCount(filament),
		CrossSection:     strings.TrimSpace(crossSection.String),
		Description:      strings.TrimSpace(descr.String),
		ShadeCode:        strings.TrimSpace(shadeCode.String),
		ShadeColor:       strings.TrimSpace(shadeColor.String),
		TareBoxWeight:    optionalPositive(tareBoxWeight),
		TareBobbinWeight: optionalPositive(tareBobbinWeight),
		BobbinsPerBox:    optionalCount(bobbinsPerBox),
		BobWeight:        optionalPositive(bobWeight),
		OrionItemCode:    strings.TrimSpace(orionItem.String),
		MachineNo:        strings.TrimSpace(machineNo.String),
		EfficiencyPct:    optionalCount(efficiency),
		Status:           strings.TrimSpace(status.String),
		PakStatus:        strings.TrimSpace(pakStatus.String),
	}, nil
}

// optionalPositive maps a nullable Oracle NUMBER onto an optional float,
// treating a non-positive value as absent. The legacy master stores 0 to mean
// "not specified" (MERGE_BOB is 0.00 on every POY and FOY sample), and a zero
// would otherwise read downstream as a genuine weight of nothing.
func optionalPositive(v sql.NullFloat64) *float64 {
	if !v.Valid || v.Float64 <= 0 {
		return nil
	}
	f := v.Float64
	return &f
}

// optionalCount maps a nullable Oracle NUMBER onto an optional int32 count,
// treating a non-positive value as absent. Scanned as a float because go-ora
// surfaces NUMBER(n) columns with a scale, and clamped to int32 range so a
// corrupt legacy value cannot overflow (golangci-lint gosec G115).
func optionalCount(v sql.NullFloat64) *int32 {
	if !v.Valid || v.Float64 <= 0 {
		return nil
	}
	if v.Float64 > math.MaxInt32 {
		clamped := int32(math.MaxInt32)
		return &clamped
	}
	n := int32(v.Float64) //nolint:gosec // bounds checked above
	return &n
}
