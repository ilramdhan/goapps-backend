package costproductmaster

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/shopspring/decimal"
)

// ERP link workflow (ERP cost integration P3-T5; design Part 1 §4.4, Part 2
// §7 V-04/V-12, D-LINK).
//
// The ERP link key is (cpm_erp_item_code, cpm_shade_code). The ERP shade
// (OM_GRADE_CODE_2) must equal the product's own cpm_shade_code; the link
// writes cpm_erp_item_code only and never cpm_erp_grade_code_1/2. A key that
// matches more than one active AX product is V-04 ambiguous and is refused,
// never guessed.

// Link workflow errors.
var (
	// ErrLinkInvalidItemCode is returned for an ERP item code that is too long
	// or not printable ASCII.
	ErrLinkInvalidItemCode = errors.New("invalid ERP item code (max 50 printable ASCII chars)")
	// ErrLinkInvalidShadeCode is returned for an ERP shade code that is too long.
	ErrLinkInvalidShadeCode = errors.New("invalid ERP shade code (max 50 chars)")
	// ErrLinkNotAxGrade is returned when the product's costing grade is not AX.
	ErrLinkNotAxGrade = errors.New("ERP link: only AX-grade products can be linked")
	// ErrLinkCmbRequiresMB is returned when a CMB… ERP item is linked to a
	// product that is not of type MB, or an MB product to a non-CMB item (V-12).
	ErrLinkCmbRequiresMB = errors.New("ERP link: CMB items link only to MB products and MB products only to CMB items (V-12)")
	// ErrLinkShadeMismatch is returned when the ERP shade differs from the
	// product's cpm_shade_code (D-LINK).
	ErrLinkShadeMismatch = errors.New("ERP link: ERP shade does not match the product shade")
	// ErrLinkProductHasNoShade is returned when a shaded ERP combo is linked to
	// a product whose cpm_shade_code is empty ("linked but no shade").
	ErrLinkProductHasNoShade = errors.New("ERP link: product has no shade; it cannot be linked to a shaded ERP combo")
	// ErrLinkDuplicate is returned when another active AX product already holds
	// the same (cpm_erp_item_code, cpm_shade_code) key (V-04 ambiguous).
	ErrLinkDuplicate = errors.New("ERP link: another active AX product already has this ERP item and shade (V-04)")
	// ErrLinkStale is returned when the product's link or shade changed
	// between the read and the write (optimistic concurrency miss).
	ErrLinkStale = errors.New("ERP link: product link or shade changed concurrently; reload and retry")
	// ErrInvalidErpAttributes is returned for an ERP attribute that fails validation.
	ErrInvalidErpAttributes = errors.New("invalid ERP attributes")
)

// Column limits (000106/000409/000551).
const (
	maxErpItemCodeLen    = 50 // cpm_erp_item_code VARCHAR(50)
	maxShadeCodeLen      = 50 // cpm_shade_code VARCHAR(50)
	maxErpFgTypeLen      = 15 // cpm_erp_fg_type VARCHAR(15)
	maxErpChpItemCodeLen = 50 // cpm_erp_chp_item_code VARCHAR(50)
	maxErpMsBatchItemLen = 12 // cpm_erp_ms_batch_item VARCHAR(12), FLEX_12 (C-11)
	maxErpItemTypeLen    = 20 // cpm_erp_item_type VARCHAR(20)
	prdPerDayScale       = 5  // cpm_erp_prd_per_day NUMERIC(20,5)
	prdPerDayIntDigits   = 15 // 20 - 5
)

// prdPerDayLimit is the exclusive upper bound of NUMERIC(20,5): 10^15.
var prdPerDayLimit = decimal.New(1, prdPerDayIntDigits)

// axGrade is the costing grade that may carry an ERP link.
const axGrade = "AX"

// cmbItemPrefix marks an ERP masterbatch item (V-12).
const cmbItemPrefix = "CMB"

// ErpAttributes are the five nullable 000551 ERP attributes. An empty string
// (or an invalid NullDecimal) is NULL.
type ErpAttributes struct {
	FgType      string              // cpm_erp_fg_type, GSC_FG_TYPE / FLEX_05
	ChpItemCode string              // cpm_erp_chp_item_code, GSC_CHP_ITEM_CODE / FLEX_04
	MsBatchItem string              // cpm_erp_ms_batch_item, FLEX_12
	ItemType    string              // cpm_erp_item_type (compat)
	PrdPerDay   decimal.NullDecimal // cpm_erp_prd_per_day (compat)
}

// ErpAttributesPatch changes some attributes. A nil field is left unchanged;
// a pointer to "" clears the column (NULL).
type ErpAttributesPatch struct {
	FgType      *string
	ChpItemCode *string
	MsBatchItem *string
	ItemType    *string
	PrdPerDay   *string // decimal text, "" clears
}

// IsEmpty reports whether the patch changes nothing.
func (p ErpAttributesPatch) IsEmpty() bool {
	return p.FgType == nil && p.ChpItemCode == nil && p.MsBatchItem == nil && p.ItemType == nil && p.PrdPerDay == nil
}

// Apply returns a copy of a with the patch applied and validated.
func (p ErpAttributesPatch) Apply(a ErpAttributes) (ErpAttributes, error) {
	out := a
	if p.FgType != nil {
		out.FgType = strings.TrimSpace(*p.FgType)
	}
	if p.ChpItemCode != nil {
		out.ChpItemCode = strings.TrimSpace(*p.ChpItemCode)
	}
	if p.MsBatchItem != nil {
		out.MsBatchItem = strings.TrimSpace(*p.MsBatchItem)
	}
	if p.ItemType != nil {
		out.ItemType = strings.TrimSpace(*p.ItemType)
	}
	if p.PrdPerDay != nil {
		d, err := parsePrdPerDay(*p.PrdPerDay)
		if err != nil {
			return ErpAttributes{}, err
		}
		out.PrdPerDay = d
	}
	if err := out.Validate(); err != nil {
		return ErpAttributes{}, err
	}
	return out, nil
}

// parsePrdPerDay parses decimal text; blank is NULL.
func parsePrdPerDay(v string) (decimal.NullDecimal, error) {
	txt := strings.TrimSpace(v)
	if txt == "" {
		return decimal.NullDecimal{}, nil
	}
	d, err := decimal.NewFromString(txt)
	if err != nil {
		return decimal.NullDecimal{}, fmt.Errorf("%w: prd_per_day %q is not a decimal", ErrInvalidErpAttributes, txt)
	}
	return decimal.NullDecimal{Decimal: d, Valid: true}, nil
}

// Validate checks the column limits of every set attribute.
func (a ErpAttributes) Validate() error {
	checks := []struct {
		name, v string
		max     int
		ascii   bool
	}{
		{"fg_type", a.FgType, maxErpFgTypeLen, false},
		{"chp_item_code", a.ChpItemCode, maxErpChpItemCodeLen, true},
		{"ms_batch_item", a.MsBatchItem, maxErpMsBatchItemLen, true},
		{"item_type", a.ItemType, maxErpItemTypeLen, false},
	}
	for _, c := range checks {
		if len(c.v) > c.max {
			return fmt.Errorf("%w: %s longer than %d", ErrInvalidErpAttributes, c.name, c.max)
		}
		if c.ascii && !isPrintableASCII(c.v) {
			return fmt.Errorf("%w: %s must be printable ASCII", ErrInvalidErpAttributes, c.name)
		}
	}
	if a.PrdPerDay.Valid {
		d := a.PrdPerDay.Decimal
		if d.IsNegative() {
			return fmt.Errorf("%w: prd_per_day must be >= 0", ErrInvalidErpAttributes)
		}
		if !d.Equal(d.Truncate(prdPerDayScale)) {
			return fmt.Errorf("%w: prd_per_day has more than %d decimal places", ErrInvalidErpAttributes, prdPerDayScale)
		}
		if d.GreaterThanOrEqual(prdPerDayLimit) {
			return fmt.Errorf("%w: prd_per_day exceeds NUMERIC(20,5)", ErrInvalidErpAttributes)
		}
	}
	return nil
}

// ErpAttributes returns the product's 000551 ERP attributes.
func (p *CostProductMaster) ErpAttributes() ErpAttributes { return p.erpAttrs }

// RestoreErpAttributes sets the attributes loaded from persistence. It does
// not validate and does not touch the audit fields.
func (p *CostProductMaster) RestoreErpAttributes(a ErpAttributes) { p.erpAttrs = a }

// ApplyErpAttributes applies a validated patch. It returns whether anything
// changed. Inactive products are refused.
func (p *CostProductMaster) ApplyErpAttributes(patch ErpAttributesPatch, actor string, at time.Time) (bool, error) {
	if !p.isActive {
		return false, ErrInactive
	}
	next, err := patch.Apply(p.erpAttrs)
	if err != nil {
		return false, err
	}
	if erpAttributesEqual(next, p.erpAttrs) {
		return false, nil
	}
	p.erpAttrs = next
	p.updatedAt = at.UTC()
	p.updatedBy = actor
	return true, nil
}

func erpAttributesEqual(a, b ErpAttributes) bool {
	if a.FgType != b.FgType || a.ChpItemCode != b.ChpItemCode || a.MsBatchItem != b.MsBatchItem || a.ItemType != b.ItemType {
		return false
	}
	if a.PrdPerDay.Valid != b.PrdPerDay.Valid {
		return false
	}
	return !a.PrdPerDay.Valid || a.PrdPerDay.Decimal.Equal(b.PrdPerDay.Decimal)
}

// NormalizeErpItemCode trims an ERP item code and validates its length and
// charset. An empty result means "unlink".
func NormalizeErpItemCode(code string) (string, error) {
	c := strings.TrimSpace(code)
	if len(c) > maxErpItemCodeLen || !isPrintableASCII(c) {
		return "", fmt.Errorf("%w: %q", ErrLinkInvalidItemCode, code)
	}
	return c, nil
}

// NormalizeShadeKey is the comparison form of a shade code (trimmed, upper
// case), mirroring shade.NormalizeCode.
func NormalizeShadeKey(code string) string {
	return strings.ToUpper(strings.TrimSpace(code))
}

// IsCmbItem reports whether an ERP item code is a masterbatch (CMB…) item.
func IsCmbItem(itemCode string) bool {
	return strings.HasPrefix(strings.ToUpper(strings.TrimSpace(itemCode)), cmbItemPrefix)
}

// IsAxGrade reports whether the costing grade counts as AX. An empty grade is
// AX, matching the SQL predicate that maps a NULL or blank grade to AX (design §4.9).
func IsAxGrade(grade string) bool {
	g := strings.TrimSpace(grade)
	return g == "" || strings.EqualFold(g, axGrade)
}

// CheckErpLink runs the pure link validations of P3-T5 against a loaded
// product (the duplicate check needs I/O and is done by the caller):
//   - the product is active and of grade AX;
//   - CMB items link only to MB products, and MB products only to CMB items (V-12);
//   - the ERP shade equals the product's cpm_shade_code (D-LINK).
//
// itemCode must already be normalized and non-empty.
func (p *CostProductMaster) CheckErpLink(itemCode, erpShadeCode string, isMBProduct bool) error {
	if !p.isActive {
		return ErrInactive
	}
	if !IsAxGrade(p.gradeCode) {
		return fmt.Errorf("%w (grade %q)", ErrLinkNotAxGrade, p.gradeCode)
	}
	if len(strings.TrimSpace(erpShadeCode)) > maxShadeCodeLen {
		return fmt.Errorf("%w: %q", ErrLinkInvalidShadeCode, erpShadeCode)
	}
	if IsCmbItem(itemCode) != isMBProduct {
		return ErrLinkCmbRequiresMB
	}
	erpShade := NormalizeShadeKey(erpShadeCode)
	own := NormalizeShadeKey(p.shadeCode)
	if erpShade != "" && own == "" {
		return ErrLinkProductHasNoShade
	}
	if erpShade != own {
		return fmt.Errorf("%w (ERP %q, product %q)", ErrLinkShadeMismatch, erpShade, own)
	}
	return nil
}

// LinkErpItem sets cpm_erp_item_code only (D-LINK); cpm_erp_grade_code_1/2
// are left exactly as they are. An empty itemCode unlinks. It returns whether
// the link changed. Validation is CheckErpLink's job.
func (p *CostProductMaster) LinkErpItem(itemCode, actor string, at time.Time) bool {
	c := strings.TrimSpace(itemCode)
	if c == p.erpItemCode {
		return false
	}
	at = at.UTC()
	p.erpItemCode = c
	if c == "" {
		p.erpLinkedAt = nil
		p.erpLinkedBy = ""
	} else {
		p.erpLinkedAt = &at
		p.erpLinkedBy = actor
	}
	p.updatedAt = at
	p.updatedBy = actor
	return true
}

func isPrintableASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] > 0x7e {
			return false
		}
	}
	return true
}

// ErpLinkWrite is one link change persisted by ErpLinkRepository.SaveErpLink.
type ErpLinkWrite struct {
	ProductSysID int64
	// PrevItemCode and ShadeKey are the values read before the change; the
	// write is refused with ErrLinkStale when the row no longer has them.
	PrevItemCode string
	ShadeKey     string // NormalizeShadeKey(product shade)
	NewItemCode  string // "" unlinks
	LinkedAt     *time.Time
	LinkedBy     string
	UpdatedAt    time.Time
	UpdatedBy    string
}

// ErpAttributesWrite is one attribute change persisted by
// ErpLinkRepository.SaveErpAttributes.
type ErpAttributesWrite struct {
	ProductSysID int64
	Attributes   ErpAttributes
	UpdatedAt    time.Time
	UpdatedBy    string
}

// ErpLinkRepository persists the ERP link and the 000551 attributes. It is a
// separate port from Repository so existing implementations stay unchanged.
type ErpLinkRepository interface {
	// ListActiveAxByErpKey returns the sys ids of active AX products (other
	// than excludeSysID) whose (cpm_erp_item_code, cpm_shade_code) equals the
	// key, compared trimmed and case-insensitively; a NULL shade equals "".
	ListActiveAxByErpKey(ctx context.Context, itemCode, shadeKey string, excludeSysID int64) ([]int64, error)
	// SaveErpLink writes cpm_erp_item_code, cpm_erp_linked_at/by and
	// cpm_updated_at/by only. For a non-empty NewItemCode it re-checks the
	// duplicate key inside the transaction (ErrLinkDuplicate) under a
	// per-key advisory lock. It returns ErrLinkStale when the row's item or
	// shade no longer matches, and ErrNotFound for an unknown product.
	SaveErpLink(ctx context.Context, w ErpLinkWrite) error
	// SaveErpAttributes writes the five 000551 columns and cpm_updated_at/by.
	SaveErpAttributes(ctx context.Context, w ErpAttributesWrite) error
}
