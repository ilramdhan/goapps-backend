package erpintegration

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ItemKind separates yarn items from masterbatch (CMB…) items (design §5.2).
// It is stored in ced_item_kind / cec_item_kind (CHECK IN ('YARN','MB')).
type ItemKind string

// Item kinds.
const (
	ItemKindYarn ItemKind = "YARN"
	ItemKindMB   ItemKind = "MB"
)

// mbItemPrefix marks a masterbatch ERP item code (design §4.2: MB = prefix CMB).
const mbItemPrefix = "CMB"

// ItemKindForCode derives the kind from an ERP item code: CMB… is MB,
// everything else is YARN.
func ItemKindForCode(itemCode string) ItemKind {
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(itemCode)), mbItemPrefix) {
		return ItemKindMB
	}
	return ItemKindYarn
}

// ParseItemKind parses YARN or MB.
func ParseItemKind(s string) (ItemKind, error) {
	k := ItemKind(s)
	if k != ItemKindYarn && k != ItemKindMB {
		return "", fmt.Errorf("%w: item kind %q", ErrInvalidDemandLine, s)
	}
	return k, nil
}

// String returns the stored value.
func (k ItemKind) String() string { return string(k) }

// ErrInvalidDemandLine is returned for a demand line that breaks an invariant.
var ErrInvalidDemandLine = errors.New("erpintegration: invalid demand line")

// Column widths of cst_erp_adj_demand (design §4.2).
const (
	maxTxnCodeLen   = 12
	maxItemCodeLen  = 50
	maxItemNameLen  = 500
	maxGradeCodeLen = 20
	maxFlexLen      = 20
	maxSourceLen    = 16
)

// DemandLine is one row of the ADJ demand snapshot of a batch
// (cst_erp_adj_demand; design §4.2, §5.2). It is replaced as a whole on each
// (re)load. Kind is derived from the CMB prefix; quantities are ≥ 0.
type DemandLine struct {
	BatchID       int64
	Period        string
	TxnCode       string
	Kind          ItemKind
	ItemCode      string
	ItemName      string
	GradeCode     string
	ShadeCode     string
	ItemCount     int64
	RateVariants  int64
	QtyKg         decimal.Decimal
	MinRate       decimal.NullDecimal
	MaxRate       decimal.NullDecimal
	AdjVal        decimal.NullDecimal
	ApprovedItems int64
	PostedItems   int64
	GoappsBatch   string
	GoappsSource  string
	LoadedAt      time.Time
}

// NewDemandLineFromErpRow maps a raw ERP demand row to a DemandLine of the
// batch, trimming codes and deriving the kind, then validates it.
func NewDemandLineFromErpRow(batchID int64, row ErpDemandRow, loadedAt time.Time) (DemandLine, error) {
	item := strings.TrimSpace(row.ItemCode)
	l := DemandLine{
		BatchID:       batchID,
		Period:        strings.TrimSpace(row.Period),
		TxnCode:       strings.TrimSpace(row.TxnCode),
		Kind:          ItemKindForCode(item),
		ItemCode:      item,
		ItemName:      strings.TrimSpace(row.ItemName),
		GradeCode:     strings.TrimSpace(row.GradeCode),
		ShadeCode:     strings.TrimSpace(row.ShadeCode),
		ItemCount:     row.ItemCount,
		RateVariants:  row.RateVariants,
		QtyKg:         row.QtyKg,
		MinRate:       row.MinRate,
		MaxRate:       row.MaxRate,
		AdjVal:        row.AdjVal,
		ApprovedItems: row.ApprovedItems,
		PostedItems:   row.PostedItems,
		GoappsBatch:   strings.TrimSpace(row.GoappsBatch),
		GoappsSource:  strings.TrimSpace(row.GoappsSource),
		LoadedAt:      loadedAt,
	}
	if err := l.Validate(); err != nil {
		return DemandLine{}, err
	}
	return l, nil
}

// Validate checks the invariants and the column widths of the line.
func (l DemandLine) Validate() error {
	if err := ValidateBatchPeriod(l.Period); err != nil {
		return fmt.Errorf("%w: %w", ErrInvalidDemandLine, err)
	}
	switch l.TxnCode {
	case TxnInvAdj, TxnMbInvAdj, TxnMbInvAdjRp:
	default:
		return fmt.Errorf("%w: txn code %q", ErrInvalidDemandLine, l.TxnCode)
	}
	if _, err := ParseItemKind(string(l.Kind)); err != nil {
		return err
	}
	if l.ItemCode == "" {
		return fmt.Errorf("%w: empty item code", ErrInvalidDemandLine)
	}
	if l.Kind != ItemKindForCode(l.ItemCode) {
		return fmt.Errorf("%w: kind %s does not match item %q", ErrInvalidDemandLine, l.Kind, l.ItemCode)
	}
	if err := checkDemandWidths(l); err != nil {
		return err
	}
	if l.ItemCount < 0 || l.RateVariants < 0 || l.ApprovedItems < 0 || l.PostedItems < 0 {
		return fmt.Errorf("%w: negative count for %s/%s/%s", ErrInvalidDemandLine, l.ItemCode, l.GradeCode, l.ShadeCode)
	}
	if l.QtyKg.IsNegative() {
		return fmt.Errorf("%w: negative qty for %s/%s/%s", ErrInvalidDemandLine, l.ItemCode, l.GradeCode, l.ShadeCode)
	}
	return nil
}

func checkDemandWidths(l DemandLine) error {
	checks := []struct {
		name string
		v    string
		max  int
	}{
		{"txn code", l.TxnCode, maxTxnCodeLen},
		{"item code", l.ItemCode, maxItemCodeLen},
		{"item name", l.ItemName, maxItemNameLen},
		{"grade code", l.GradeCode, maxGradeCodeLen},
		{"shade code", l.ShadeCode, maxGradeCodeLen},
		{"goapps batch", l.GoappsBatch, maxFlexLen},
		{"goapps source", l.GoappsSource, maxSourceLen},
	}
	for _, c := range checks {
		if len([]rune(c.v)) > c.max {
			return fmt.Errorf("%w: %s longer than %d", ErrInvalidDemandLine, c.name, c.max)
		}
	}
	return nil
}

// DemandKey is the natural key of a demand line inside a batch (uk_ced).
type DemandKey struct {
	TxnCode   string
	ItemCode  string
	GradeCode string
	ShadeCode string
}

// Key returns the uk_ced key of the line.
func (l DemandLine) Key() DemandKey {
	return DemandKey{TxnCode: l.TxnCode, ItemCode: l.ItemCode, GradeCode: l.GradeCode, ShadeCode: l.ShadeCode}
}
