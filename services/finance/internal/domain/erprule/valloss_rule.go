package erprule

import (
	"time"

	"github.com/shopspring/decimal"
)

// VallossRule is the aggregate for one cst_erp_valloss_rule row: the basis
// and value loss applied to a derived (non-AX) grade for one
// (FG type, prod type, grade group) key (design Part 1 §4.1, Part 2 §6.2).
//
// Deletion is soft (is_active=false). Only one active rule may exist per key
// (uk_cevr_key); inactive history rows may repeat the key.
type VallossRule struct {
	id        int64
	key       RuleKey
	basis     Basis
	valLoss   decimal.Decimal
	isActive  bool
	createdAt time.Time
	createdBy string
	updatedAt *time.Time
	updatedBy string
}

// NewVallossRule validates and creates a new, active rule (id 0 until the
// repository assigns cevr_id).
func NewVallossRule(key RuleKey, basis Basis, valLoss decimal.Decimal, user string, now time.Time) (*VallossRule, error) {
	if err := validateKey(key); err != nil {
		return nil, err
	}
	if err := validateBasis(basis); err != nil {
		return nil, err
	}
	if err := ValidateValLoss(valLoss); err != nil {
		return nil, err
	}
	u, err := validateUser(user)
	if err != nil {
		return nil, err
	}
	return &VallossRule{
		key:       key,
		basis:     basis,
		valLoss:   valLoss,
		isActive:  true,
		createdAt: now,
		createdBy: u,
	}, nil
}

// ReconstructVallossRule rebuilds a rule from persistence without validation.
func ReconstructVallossRule(
	id int64, key RuleKey, basis Basis, valLoss decimal.Decimal, isActive bool,
	createdAt time.Time, createdBy string, updatedAt *time.Time, updatedBy string,
) *VallossRule {
	return &VallossRule{
		id:        id,
		key:       key,
		basis:     basis,
		valLoss:   valLoss,
		isActive:  isActive,
		createdAt: createdAt,
		createdBy: createdBy,
		updatedAt: copyTime(updatedAt),
		updatedBy: updatedBy,
	}
}

// Update changes the basis and value loss of an active rule. The key is
// immutable: a key change is a delete plus a create.
func (r *VallossRule) Update(basis Basis, valLoss decimal.Decimal, user string, now time.Time) error {
	if !r.isActive {
		return ErrRuleInactive
	}
	if err := validateBasis(basis); err != nil {
		return err
	}
	if err := ValidateValLoss(valLoss); err != nil {
		return err
	}
	u, err := validateUser(user)
	if err != nil {
		return err
	}
	r.basis = basis
	r.valLoss = valLoss
	r.touch(u, now)
	return nil
}

// Deactivate soft-deletes the rule. The ErrRuleInUse check (a non-terminal
// batch uses the current rule hash) belongs to the application layer, which
// owns the batch query port.
func (r *VallossRule) Deactivate(user string, now time.Time) error {
	if !r.isActive {
		return ErrRuleInactive
	}
	u, err := validateUser(user)
	if err != nil {
		return err
	}
	r.isActive = false
	r.touch(u, now)
	return nil
}

func (r *VallossRule) touch(user string, now time.Time) {
	t := now
	r.updatedAt = &t
	r.updatedBy = user
}

// ID returns cevr_id (0 before insert).
func (r *VallossRule) ID() int64 { return r.id }

// Key returns the (FG type, prod type, grade group) key.
func (r *VallossRule) Key() RuleKey { return r.key }

// Basis returns the valuation basis.
func (r *VallossRule) Basis() Basis { return r.basis }

// ValLoss returns the value loss as stored (6 dp).
func (r *VallossRule) ValLoss() decimal.Decimal { return r.valLoss }

// IsActive reports whether the rule is active (not soft-deleted).
func (r *VallossRule) IsActive() bool { return r.isActive }

// CreatedAt returns the creation time.
func (r *VallossRule) CreatedAt() time.Time { return r.createdAt }

// CreatedBy returns the creating user.
func (r *VallossRule) CreatedBy() string { return r.createdBy }

// UpdatedAt returns the last update time, or nil.
func (r *VallossRule) UpdatedAt() *time.Time { return copyTime(r.updatedAt) }

// UpdatedBy returns the last updating user, or "".
func (r *VallossRule) UpdatedBy() string { return r.updatedBy }

// Rule returns the immutable value used by RuleSet.
func (r *VallossRule) Rule() Rule {
	return Rule{Key: r.key, Basis: r.basis, ValLoss: r.valLoss}
}

// validateKey re-parses a key built outside NewRuleKey and rejects any value
// that parsing would change (untrimmed text, unknown enum, AX).
func validateKey(k RuleKey) error {
	parsed, err := NewRuleKey(string(k.FgType), string(k.ProdType), string(k.GradeGroup))
	if err != nil {
		return err
	}
	if parsed != k {
		return ErrInvalidFgType
	}
	return nil
}

func validateBasis(b Basis) error {
	parsed, err := ParseBasis(string(b))
	if err != nil {
		return err
	}
	if parsed != b {
		return ErrInvalidBasis
	}
	return nil
}

func copyTime(t *time.Time) *time.Time {
	if t == nil {
		return nil
	}
	c := *t
	return &c
}
