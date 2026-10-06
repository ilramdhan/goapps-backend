package erprule

import (
	"time"

	"github.com/shopspring/decimal"
)

// SellPrice is the aggregate for one cst_erp_sell_price row: the reference
// selling price (USD/kg) for a selling-price basis (design Part 1 §4.1).
type SellPrice struct {
	basis     Basis
	price     decimal.Decimal
	isActive  bool
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy string
}

// NewSellPrice validates and creates a new, active sell price.
func NewSellPrice(basis Basis, price decimal.Decimal, user string, now time.Time) (*SellPrice, error) {
	if err := validateSellPriceBasis(basis); err != nil {
		return nil, err
	}
	if err := ValidatePrice(price); err != nil {
		return nil, err
	}
	u, err := validateUser(user)
	if err != nil {
		return nil, err
	}
	return &SellPrice{basis: basis, price: price, isActive: true, createdAt: now, createdBy: u}, nil
}

// ReconstructSellPrice rebuilds a sell price from persistence without
// validation.
func ReconstructSellPrice(
	basis Basis, price decimal.Decimal, isActive bool,
	createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy string,
) *SellPrice {
	return &SellPrice{
		basis:     basis,
		price:     price,
		isActive:  isActive,
		createdAt: createdAt,
		createdBy: createdBy,
		updatedAt: copyTime(updatedAt),
		updatedBy: updatedBy,
	}
}

// UpdatePrice changes the price. Updating an inactive price re-activates it
// (the upsert semantics of UpsertSellPrice, design Part 2 §8.2).
func (p *SellPrice) UpdatePrice(price decimal.Decimal, user string, now time.Time) error {
	if err := ValidatePrice(price); err != nil {
		return err
	}
	u, err := validateUser(user)
	if err != nil {
		return err
	}
	p.price = price
	p.isActive = true
	t := now
	p.updatedAt = &t
	p.updatedBy = u
	return nil
}

// Basis returns the selling-price basis (the primary key).
func (p *SellPrice) Basis() Basis { return p.basis }

// Price returns the price as stored (6 dp).
func (p *SellPrice) Price() decimal.Decimal { return p.price }

// IsActive reports whether the price is active.
func (p *SellPrice) IsActive() bool { return p.isActive }

// CreatedAt returns the creation time.
func (p *SellPrice) CreatedAt() time.Time { return p.createdAt }

// CreatedBy returns the creating user.
func (p *SellPrice) CreatedBy() string { return p.createdBy }

// UpdatedAt returns the last update time, or nil.
func (p *SellPrice) UpdatedAt() *time.Time { return copyTime(p.updatedAt) }

// UpdatedBy returns the last updating user, or "".
func (p *SellPrice) UpdatedBy() string { return p.updatedBy }

// Value returns the immutable value used by RuleSet.
func (p *SellPrice) Value() Price { return Price{Basis: p.basis, Price: p.price} }

func validateSellPriceBasis(b Basis) error {
	parsed, err := ParseSellPriceBasis(string(b))
	if err != nil {
		return err
	}
	if parsed != b {
		return ErrInvalidBasis
	}
	return nil
}
