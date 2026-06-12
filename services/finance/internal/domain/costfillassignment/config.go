package costfillassignment

import "strings"

// Actor types.
const (
	ActorUser = "USER"
	ActorDept = "DEPT"
)

// ActorRef is an immutable filler/approver reference (type + value).
type ActorRef struct {
	typ   string
	value string
}

// NewActorRef validates and constructs an ActorRef.
func NewActorRef(typ, value string) (ActorRef, error) {
	if typ != ActorUser && typ != ActorDept {
		return ActorRef{}, ErrInvalidActorType
	}
	v := strings.TrimSpace(value)
	if v == "" {
		return ActorRef{}, ErrEmptyActorValue
	}
	return ActorRef{typ: typ, value: v}, nil
}

// Type returns USER or DEPT.
func (a ActorRef) Type() string { return a.typ }

// Value returns the user_id or dept_code.
func (a ActorRef) Value() string { return a.value }

// IsZero reports whether the ref is unset.
func (a ActorRef) IsZero() bool { return a.typ == "" }

// ConfigTier identifies which override level a row belongs to.
type ConfigTier string

// Config tiers.
const (
	TierGlobal  ConfigTier = "GLOBAL"
	TierProduct ConfigTier = "PRODUCT"
	TierRequest ConfigTier = "REQUEST"
)

// Config is one assignment-config row. For PRODUCT/REQUEST tiers any pointer
// field may be nil, meaning "inherit from the lower tier" (field-level merge).
type Config struct {
	ConfigID          int64
	Tier              ConfigTier
	RouteLevel        int32
	ProductSysID      int64
	RequestID         int64
	FillerType        *string
	FillerValue       *string
	ApproverType      *string
	ApproverValue     *string
	ReapproveOnChange *bool
	SLAFillHours      *int32
	SLAApproveHours   *int32
}
