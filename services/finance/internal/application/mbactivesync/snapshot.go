package mbactivesync

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

// HeadSnap is one mst_mb_head row (deleted heads included so their SYSID is known).
type HeadSnap struct {
	ID            uuid.UUID
	SysID         string // mbh_oracle_sys_id, "" when NULL
	MBCosting     string
	OrionItemCode string
	IsActive      bool
	Deleted       bool
	CheckStatus   string
	EntryStatus   string
	SupersededBy  *uuid.UUID
	CostProductID *int64
	FGProductID   *int64
	ShadeCode     string
	ShadeName     string
	Code          string // mbh_code (Oracle CMBH_CODE), part of the head shade set (L-10 hshade)
	IsBoughtout   bool
	Denier        *float64
	Filament      *int
	CreatedAt     time.Time
}

// Live reports whether the head is non-deleted, active and not superseded.
func (h HeadSnap) Live() bool { return !h.Deleted && h.IsActive && h.SupersededBy == nil }

// SpinSnap is one non-deleted mst_mb_spin row.
type SpinSnap struct {
	ID            uuid.UUID
	HeadID        uuid.UUID
	ParentID      *uuid.UUID
	IsActive      bool
	SupersededBy  *uuid.UUID
	Denier        *float64
	Filament      *int
	CrossSection  string // COALESCE(NULLIF(cross_section,''),NULLIF(lesture,'')) (P2h-4b)
	ShadeCode     string
	ShadeName     string
	CC            string
	RunLDRPct     *float64
	LDRType       string
	LDRIsActual   bool
	LDRIsFixed    bool
	FGProductID   *int64
	FGLinkSource  string
	OrionItemCode string
	MGTName       string
	MBCosting     string
	CreatedAt     time.Time

	// Product references (MB_SP_CODE).
	PointerProducts int  // distinct products whose cpp_value_mb_spin_id = this spin
	TextProducts    int  // pointer-less products whose text equals SpinTextKey() (always filled, even if shared)
	TextShared      bool // key shared by >1 non-deleted spin AND TextProducts>0 (ambiguous text refs)
	// Planner has_refs rule (conservative): PointerProducts>0 || TextProducts>0.
	// A shared key with text rows therefore counts as has_refs; TextShared only
	// tells that those text rows cannot be attributed to this spin alone.
}

// Live reports whether the spin is active and not superseded.
func (s SpinSnap) Live() bool { return s.IsActive && s.SupersededBy == nil }

// SpinTextKey is UPPER(TRIM(COALESCE(NULLIF(orion,”), id))) — the key pointer-less
// MB_SP_CODE rows are matched on (same rule as the supersede repoint).
func (s SpinSnap) SpinTextKey() string {
	if v := strings.TrimSpace(s.OrionItemCode); v != "" {
		return strings.ToUpper(v)
	}
	return strings.ToUpper(s.ID.String())
}

// POYSnap is one cost_product_master row of type POY.
type POYSnap struct {
	SysID       int64
	ProductCode string
	ERPItemCode string
	ShadeCode   string // exact
	ShadeName   string
	IsActive    bool
}

// Snapshot is the full consistent read used by the planner.
type Snapshot struct {
	Heads []HeadSnap // ordered by created_at, id
	Spins []SpinSnap // ordered by created_at, id
	POYs  []POYSnap  // ordered by sys id

	// DozingSourceColumn is mst_parameter(MB_SP_DOZING).lookup_source_column ("" if absent).
	DozingSourceColumn string
}

// HeadsBySysID indexes heads by SYSID (a SYSID may map to several heads).
func (s *Snapshot) HeadsBySysID() map[string][]*HeadSnap {
	m := make(map[string][]*HeadSnap)
	for i := range s.Heads {
		if k := s.Heads[i].SysID; k != "" {
			m[k] = append(m[k], &s.Heads[i])
		}
	}
	return m
}

// SpinsByHead indexes spins by head id, preserving snapshot order.
func (s *Snapshot) SpinsByHead() map[uuid.UUID][]*SpinSnap {
	m := make(map[uuid.UUID][]*SpinSnap)
	for i := range s.Spins {
		m[s.Spins[i].HeadID] = append(m[s.Spins[i].HeadID], &s.Spins[i])
	}
	return m
}
