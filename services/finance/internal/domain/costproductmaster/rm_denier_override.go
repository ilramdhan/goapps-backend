package costproductmaster

import (
	"context"
	"errors"
	"math"
	"strings"
	"time"
)

// RM types an RM denier override can target (cost_route_rm.crm_rm_type).
// ITEM lines have no denier and cannot be overridden.
const (
	RMTypeProduct = "PRODUCT"
	RMTypeGroup   = "GROUP"
)

var (
	// ErrInvalidOverrideRMType is returned when rm_type is not PRODUCT or GROUP.
	ErrInvalidOverrideRMType = errors.New("invalid rm_type (must be PRODUCT or GROUP)")
	// ErrInvalidOverrideRMIdentity is returned when the RM identity does not match rm_type:
	// PRODUCT needs exactly rm_product_sys_id (not the owner product itself), GROUP needs
	// exactly rm_group_code.
	ErrInvalidOverrideRMIdentity = errors.New("invalid RM identity (PRODUCT needs rm_product_sys_id, GROUP needs rm_group_code, exactly one)")
	// ErrInvalidOverrideDenier is returned when the override denier is not > 0.
	ErrInvalidOverrideDenier = errors.New("invalid denier override (must be > 0 and < 1e12)")
	// ErrInvalidOverrideReason is returned when the reason is blank.
	ErrInvalidOverrideReason = errors.New("invalid override reason (must not be blank)")
	// ErrOverrideRMProductNotFound is returned when the overridden RM product does not exist.
	ErrOverrideRMProductNotFound = errors.New("override RM product not found")
	// ErrOverrideRMGroupNotFound is returned when the overridden RM group does not exist.
	ErrOverrideRMGroupNotFound = errors.New("override RM group not found")
)

// maxOverrideDenier is the exclusive upper bound of cpdo_denier NUMERIC(18,6).
const maxOverrideDenier = 1e12

// RMDenierOverride is a product-owned override of the denier used for one RM line
// (identified by RM product or RM group) when computing THAT product's DENIER RM ratio.
// Denier is the FINAL line denier: for spandex groups it is not divided by SPANDEX_DRAFT.
type RMDenierOverride struct {
	id             int64
	productSysID   int64
	rmType         string
	rmProductSysID int64
	rmGroupCode    string
	denier         float64
	reason         string

	// Read-only display fields (joined on read).
	rmProductCode string
	rmProductName string
	rmGroupName   string

	createdAt time.Time
	createdBy string
	updatedAt time.Time
	updatedBy string
}

// RMDenierOverrideInput holds the user-editable fields of an override.
type RMDenierOverrideInput struct {
	ProductSysID   int64
	RMType         string
	RMProductSysID int64
	RMGroupCode    string
	Denier         float64
	Reason         string
	ActorUserID    string
}

// NewRMDenierOverride validates the input and builds an override ready to upsert.
func NewRMDenierOverride(in RMDenierOverrideInput) (*RMDenierOverride, error) {
	if in.ProductSysID <= 0 {
		return nil, ErrNotFound
	}
	rmType, rmProductSysID, rmGroupCode, err := ValidateRMIdentity(in.ProductSysID, in.RMType, in.RMProductSysID, in.RMGroupCode)
	if err != nil {
		return nil, err
	}
	// NUMERIC(18,6) holds < 1e12; NaN fails the > 0 test.
	if !(in.Denier > 0) || math.IsInf(in.Denier, 0) || in.Denier >= maxOverrideDenier {
		return nil, ErrInvalidOverrideDenier
	}
	reason := strings.TrimSpace(in.Reason)
	if reason == "" {
		return nil, ErrInvalidOverrideReason
	}
	now := time.Now().UTC()
	return &RMDenierOverride{
		productSysID:   in.ProductSysID,
		rmType:         rmType,
		rmProductSysID: rmProductSysID,
		rmGroupCode:    rmGroupCode,
		denier:         in.Denier,
		reason:         reason,
		createdAt:      now,
		createdBy:      in.ActorUserID,
		updatedAt:      now,
		updatedBy:      in.ActorUserID,
	}, nil
}

// ValidateRMIdentity normalizes and validates an override's RM identity: rm_type is
// upper-cased, PRODUCT keeps only rmProductSysID (> 0 and not the owner product),
// GROUP keeps only the trimmed rmGroupCode. It is shared by create and delete.
func ValidateRMIdentity(productSysID int64, rmType string, rmProductSysID int64, rmGroupCode string) (string, int64, string, error) {
	t := strings.ToUpper(strings.TrimSpace(rmType))
	code := strings.TrimSpace(rmGroupCode)
	switch t {
	case RMTypeProduct:
		if rmProductSysID <= 0 || code != "" || rmProductSysID == productSysID {
			return "", 0, "", ErrInvalidOverrideRMIdentity
		}
		return t, rmProductSysID, "", nil
	case RMTypeGroup:
		if code == "" || rmProductSysID != 0 || len(code) > 30 {
			return "", 0, "", ErrInvalidOverrideRMIdentity
		}
		return t, 0, code, nil
	default:
		return "", 0, "", ErrInvalidOverrideRMType
	}
}

// RMDenierOverrideRecord is the persisted shape used by ReconstructRMDenierOverride.
type RMDenierOverrideRecord struct {
	ID             int64
	ProductSysID   int64
	RMType         string
	RMProductSysID int64
	RMProductCode  string
	RMProductName  string
	RMGroupCode    string
	RMGroupName    string
	Denier         float64
	Reason         string
	CreatedAt      time.Time
	CreatedBy      string
	UpdatedAt      time.Time
	UpdatedBy      string
}

// ReconstructRMDenierOverride rebuilds an override from persistence (no validation).
func ReconstructRMDenierOverride(r RMDenierOverrideRecord) *RMDenierOverride {
	return &RMDenierOverride{
		id: r.ID, productSysID: r.ProductSysID, rmType: r.RMType,
		rmProductSysID: r.RMProductSysID, rmGroupCode: r.RMGroupCode,
		denier: r.Denier, reason: r.Reason,
		rmProductCode: r.RMProductCode, rmProductName: r.RMProductName, rmGroupName: r.RMGroupName,
		createdAt: r.CreatedAt, createdBy: r.CreatedBy, updatedAt: r.UpdatedAt, updatedBy: r.UpdatedBy,
	}
}

// ID returns the row id (0 before insert).
func (o *RMDenierOverride) ID() int64 { return o.id }

// ProductSysID returns the owner product.
func (o *RMDenierOverride) ProductSysID() int64 { return o.productSysID }

// RMType returns PRODUCT or GROUP.
func (o *RMDenierOverride) RMType() string { return o.rmType }

// RMProductSysID returns the overridden RM product (PRODUCT only).
func (o *RMDenierOverride) RMProductSysID() int64 { return o.rmProductSysID }

// RMProductCode returns the RM product code (read only).
func (o *RMDenierOverride) RMProductCode() string { return o.rmProductCode }

// RMProductName returns the RM product name (read only).
func (o *RMDenierOverride) RMProductName() string { return o.rmProductName }

// RMGroupCode returns the overridden RM group code (GROUP only).
func (o *RMDenierOverride) RMGroupCode() string { return o.rmGroupCode }

// RMGroupName returns the RM group name (read only).
func (o *RMDenierOverride) RMGroupName() string { return o.rmGroupName }

// Denier returns the final line denier override.
func (o *RMDenierOverride) Denier() float64 { return o.denier }

// Reason returns the override reason.
func (o *RMDenierOverride) Reason() string { return o.reason }

// CreatedAt returns the creation time.
func (o *RMDenierOverride) CreatedAt() time.Time { return o.createdAt }

// CreatedBy returns the creator.
func (o *RMDenierOverride) CreatedBy() string { return o.createdBy }

// UpdatedAt returns the last update time.
func (o *RMDenierOverride) UpdatedAt() time.Time { return o.updatedAt }

// UpdatedBy returns the last updater.
func (o *RMDenierOverride) UpdatedBy() string { return o.updatedBy }

// DuplicateRef is a direct copy of a product (cpm_duplicated_from_sys_id = source).
type DuplicateRef struct {
	ProductSysID int64
	ProductCode  string
	ProductName  string
}

// DeleteRMDenierOverrideInput identifies the override to delete.
type DeleteRMDenierOverrideInput struct {
	ProductSysID      int64
	RMType            string
	RMProductSysID    int64
	RMGroupCode       string
	ApplyToDuplicates bool
	ActorUserID       string
}

// RMDenierOverrideRepository persists per-product RM denier overrides. It is a separate
// interface (implemented by the same postgres CostProductMasterRepository) so existing
// Repository consumers and fakes are untouched.
type RMDenierOverrideRepository interface {
	// ListRMDenierOverrides returns the product's overrides with RM product code/name and
	// RM group name joined, ordered by rm_type then RM code.
	ListRMDenierOverrides(ctx context.Context, productSysID int64) ([]*RMDenierOverride, error)
	// ListDirectDuplicates returns the products whose cpm_duplicated_from_sys_id is
	// productSysID (direct copies only, not transitive).
	ListDirectDuplicates(ctx context.Context, productSysID int64) ([]DuplicateRef, error)
	// UpsertRMDenierOverride inserts or updates o (keyed by owner product + RM identity).
	// With applyToDuplicates the same override is written to every direct copy in the same
	// transaction; appliedProductSysIDs lists those copies (never the owner).
	// Returns ErrNotFound (owner), ErrOverrideRMProductNotFound or ErrOverrideRMGroupNotFound.
	UpsertRMDenierOverride(ctx context.Context, o *RMDenierOverride, applyToDuplicates bool, actor string) (saved *RMDenierOverride, appliedProductSysIDs []int64, err error)
	// DeleteRMDenierOverride hard-deletes the override (and, with ApplyToDuplicates, the
	// same RM identity on every direct copy) in one transaction. deleted counts all rows
	// removed; appliedProductSysIDs lists the copies that actually had a row.
	DeleteRMDenierOverride(ctx context.Context, in DeleteRMDenierOverrideInput) (deleted int, appliedProductSysIDs []int64, err error)
}
