package costproductrequest

import (
	"strings"
	"time"
)

// Allowed enum values.
const (
	RawMatPOYBoughtout = "POY_BOUGHTOUT"
	RawMatChipsSD      = "CHIPS_SD"
	RawMatChipsBRT     = "CHIPS_BRT"
	RawMatChipsRecycle = "CHIPS_RECYCLE"

	BoxTypeJumbo  = "JUMBO"
	BoxTypeNormal = "NORMAL"
	BoxTypePallet = "PALLET"
)

var (
	allowedRawMat  = map[string]struct{}{RawMatPOYBoughtout: {}, RawMatChipsSD: {}, RawMatChipsBRT: {}, RawMatChipsRecycle: {}}
	allowedBoxType = map[string]struct{}{BoxTypeJumbo: {}, BoxTypeNormal: {}, BoxTypePallet: {}}
)

// Spec is the embedded product specification for a request with classification=new.
type Spec struct {
	SpecID             int64
	RawMaterialType    string
	ProductDescription string
	ShadeID            *int32
	ShadeCustomText    string
	PaperTubeTypeID    int32
	WeightPerBobbinKg  string // decimal stringified
	BoxType            string
	CreatedAt          time.Time
	CreatedBy          string
}

// SpecInput is the user-facing shape passed to create/update.
type SpecInput struct {
	RawMaterialType    string
	ProductDescription string
	ShadeID            int32 // 0 = unset
	ShadeCustomText    string
	PaperTubeTypeID    int32
	WeightPerBobbinKg  string
	BoxType            string
}

// Validate enforces the FR-1 spec rules + DB CHECK constraints (chk_cps_*).
func (in SpecInput) Validate() error {
	if _, ok := allowedRawMat[in.RawMaterialType]; !ok {
		return ErrInvalidSpec
	}
	if strings.TrimSpace(in.ProductDescription) == "" {
		return ErrInvalidSpec
	}
	if in.PaperTubeTypeID <= 0 {
		return ErrInvalidSpec
	}
	if strings.TrimSpace(in.WeightPerBobbinKg) == "" {
		return ErrInvalidSpec
	}
	if _, ok := allowedBoxType[in.BoxType]; !ok {
		return ErrInvalidSpec
	}
	// shade_id OR shade_custom_text must be present.
	if in.ShadeID <= 0 && strings.TrimSpace(in.ShadeCustomText) == "" {
		return ErrInvalidSpec
	}
	return nil
}

// ToSpec materializes a Spec (sans SpecID / CreatedAt — set by repo).
func (in SpecInput) ToSpec(actor string) Spec {
	s := Spec{
		RawMaterialType:    in.RawMaterialType,
		ProductDescription: strings.TrimSpace(in.ProductDescription),
		ShadeCustomText:    strings.TrimSpace(in.ShadeCustomText),
		PaperTubeTypeID:    in.PaperTubeTypeID,
		WeightPerBobbinKg:  strings.TrimSpace(in.WeightPerBobbinKg),
		BoxType:            in.BoxType,
		CreatedAt:          time.Now().UTC(),
		CreatedBy:          actor,
	}
	if in.ShadeID > 0 {
		v := in.ShadeID
		s.ShadeID = &v
	}
	return s
}
