package erpintegration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"
)

// ADJ transaction codes in scope for the GoApps standard cost (design §2,
// DATA_MAPPING §2.1). Both demand sources filter on exactly this set.
const (
	TxnInvAdj     = "INVADJ"
	TxnMbInvAdj   = "MBINVADJ"
	TxnMbInvAdjRp = "MBINVADJRP"
)

// DemandSource selects where the ERP ADJ demand is read from (I-4).
type DemandSource string

// Demand sources. DemandSourceView is the contract (C-14); base_tables is a
// DEV-only fallback until M-ERP-1b deploys V_GOAPPS_ADJ_DEMAND.
const (
	DemandSourceView       DemandSource = "view"
	DemandSourceBaseTables DemandSource = "base_tables"
)

// Demand-source errors.
var (
	ErrInvalidDemandSource    = errors.New("erpintegration: invalid erp_integration.demand_source")
	ErrBaseTablesInProduction = errors.New("erpintegration: demand_source=base_tables is refused in production")
)

// ParseDemandSource normalizes a configured demand source. Empty means view.
func ParseDemandSource(s string) (DemandSource, error) {
	switch DemandSource(strings.ToLower(strings.TrimSpace(s))) {
	case "", DemandSourceView:
		return DemandSourceView, nil
	case DemandSourceBaseTables:
		return DemandSourceBaseTables, nil
	}
	return "", fmt.Errorf("%w: %q", ErrInvalidDemandSource, s)
}

// ErpDemandRow is one aggregated ERP ADJ demand line for a period, grouped by
// (txn, item, grade1, grade2), exactly as read (SELECT only) from Oracle. It
// is the raw source row; the load-demand step (P3-T4) maps it to the
// cst_erp_adj_demand snapshot. Text codes are trimmed; nothing is guessed.
type ErpDemandRow struct {
	Period        string
	TxnCode       string
	ItemCode      string
	ItemName      string
	GradeCode     string // ADJI_GRADE_CODE_1
	ShadeCode     string // ADJI_GRADE_CODE_2 (D-LINK: matched to cpm_shade_code)
	HeadCount     int64
	ItemCount     int64
	RateVariants  int64
	QtyKg         decimal.Decimal // SUM(ADJI_QTY_BU)/1000 (QTY_BU is grams)
	MinRate       decimal.NullDecimal
	MaxRate       decimal.NullDecimal
	AdjVal        decimal.NullDecimal
	ApprovedItems int64  // items whose head has ADJH_APPR_STATUS = 3
	PostedItems   int64  // items whose head has ADJH_POST_STATUS set (V-10)
	GoappsBatch   string // MAX(ADJI_FLEX_13)
	GoappsSource  string // MAX(ADJI_FLEX_14)
}

// AdjHeadCounts summarizes the ADJ heads of a period (in-scope txn codes).
// Eligible follows §S-R7: NVL(ADJH_APPR_STATUS,0) != 3 AND ADJH_POST_STATUS
// IS NULL. NullStatus surfaces heads the lead's package (`!= 3`) would skip.
type AdjHeadCounts struct {
	Heads      int64
	Posted     int64
	Approved   int64
	NullStatus int64
	Eligible   int64
}

// ErpDemandReader reads the ADJ demand of a period via the read-only user.
type ErpDemandReader interface {
	LoadAdjDemand(ctx context.Context, period string) ([]ErpDemandRow, error)
}

// ErpAdjHeadProber answers the read-only head probes used by V-10 and unlock.
type ErpAdjHeadProber interface {
	ProbeHeads(ctx context.Context, period string) (AdjHeadCounts, error)
	ProbePosted(ctx context.Context, period string) (int64, error)
}
