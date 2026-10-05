package erpintegration

// legacy_std.go holds the types and ports of the one-off ERP attribute
// backfill (plan P3-T6, design Part 2 §9.2, gap M-7/M-8, I-5). The legacy
// standard-cost table MGTDAT.OT_STD_COST_PRODUCTS_MGT is only ever read
// (SELECT through the read-only user); the backfill writes PostgreSQL only,
// fills NULL attributes of already-linked products and never creates links.

import (
	"context"
	"errors"
	"strings"
)

// LegacyStdRow is one OT_STD_COST_PRODUCTS_MGT row as read (SELECT only).
// Text values are trimmed; an empty string is NULL. Numeric values arrive as
// decimal text (never through a binary float).
type LegacyStdRow struct {
	ItemCode    string // FG_ITEM_CODE
	GradeCode   string // FG_ITEM_GRADE (AX base row; A for MB)
	ShadeCode   string // FG_ITEM_SHADE (D-LINK: matched to cpm_shade_code)
	ItemType    string // FG_ITEM_TYPE (SUBSTR(item,1,3), or CMB)
	FgType      string // FG_TYPE
	ChpItemCode string // FG_CHP_ITEM_CODE
	MsBatchItem string // FG_MS_BATCH_ITEM (VARCHAR2 or NUMBER, read as text)
	PrdPerDay   string // FG_PRD_PER_DAY decimal text ("" = NULL or not read)
}

// LegacyStdReader reads the legacy std table through the read-only querier.
// period is YYYYMM (rows created up to the end of that month) or "" for the
// whole table.
type LegacyStdReader interface {
	List(ctx context.Context, period string) ([]LegacyStdRow, error)
}

// AttrField names one of the five 000551 ERP attributes.
type AttrField string

// The backfilled attributes, in report order.
const (
	AttrFgType      AttrField = "fg_type"
	AttrChpItemCode AttrField = "chp_item_code"
	AttrMsBatchItem AttrField = "ms_batch_item"
	AttrItemType    AttrField = "item_type"
	AttrPrdPerDay   AttrField = "prd_per_day"
)

// AllAttrFields lists the attributes in report order.
func AllAttrFields() []AttrField {
	return []AttrField{AttrFgType, AttrChpItemCode, AttrMsBatchItem, AttrItemType, AttrPrdPerDay}
}

// AttrValues holds the five attributes as text; "" is NULL. PrdPerDay is the
// canonical decimal text.
type AttrValues struct {
	FgType      string
	ChpItemCode string
	MsBatchItem string
	ItemType    string
	PrdPerDay   string
}

// Get returns the value of f.
func (v AttrValues) Get(f AttrField) string {
	switch f {
	case AttrFgType:
		return v.FgType
	case AttrChpItemCode:
		return v.ChpItemCode
	case AttrMsBatchItem:
		return v.MsBatchItem
	case AttrItemType:
		return v.ItemType
	case AttrPrdPerDay:
		return v.PrdPerDay
	}
	return ""
}

// With returns a copy of v with f set to s (trimmed).
func (v AttrValues) With(f AttrField, s string) AttrValues {
	s = strings.TrimSpace(s)
	switch f {
	case AttrFgType:
		v.FgType = s
	case AttrChpItemCode:
		v.ChpItemCode = s
	case AttrMsBatchItem:
		v.MsBatchItem = s
	case AttrItemType:
		v.ItemType = s
	case AttrPrdPerDay:
		v.PrdPerDay = s
	}
	return v
}

// IsEmpty reports whether every attribute is NULL.
func (v AttrValues) IsEmpty() bool {
	return v == AttrValues{}
}

// LinkedProductAttrs is one active, linked AX product with its current ERP
// attributes, as listed for the backfill.
type LinkedProductAttrs struct {
	ProductSysID int64
	ProductCode  string
	ErpItemCode  string // trimmed cpm_erp_item_code (non-empty)
	ShadeCode    string // trimmed cpm_shade_code ("" when NULL)
	Attrs        AttrValues
}

// AttrFillWrite asks the repository to fill NULL attributes of one product.
// Only non-empty Values are candidates; a column that is no longer NULL at
// write time is left untouched. ErpItemCode and ShadeKey are the link key read
// at planning time: when the row no longer carries them (or is no longer an
// active AX product) nothing is written and the result is Stale.
type AttrFillWrite struct {
	ProductSysID int64
	ErpItemCode  string
	ShadeKey     string // upper-cased trimmed shade
	Values       AttrValues
	Actor        string
}

// AttrFillResult is what the repository actually did for one product.
type AttrFillResult struct {
	Stale  bool
	Filled []AttrField
	Before AttrValues
	After  AttrValues
}

// AttrBackfillRepository is the PostgreSQL side of the backfill.
type AttrBackfillRepository interface {
	// ListLinkedProducts returns active AX products with a non-blank
	// cpm_erp_item_code, ordered by cpm_product_sys_id.
	ListLinkedProducts(ctx context.Context) ([]LinkedProductAttrs, error)
	// FillNullAttributes fills NULL (or blank) attribute columns only, in one
	// transaction with the row locked. It never overwrites a value.
	FillNullAttributes(ctx context.Context, w AttrFillWrite) (AttrFillResult, error)
}

// ErrAttrBackfillNotConfigured is returned when the backfill has no legacy
// reader or repository (for example, no Oracle read connection configured).
var ErrAttrBackfillNotConfigured = errors.New("erpintegration: ERP attribute backfill not configured")
