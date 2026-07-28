// Package commonlot provides domain logic for common lots (Phase 3, PRD v1.2
// §12): combining leftover bobbins from several original lots — potentially of
// different shades — into a new ERP identity. Each original lot folded in is
// tracked as a component so its provenance is preserved.
package commonlot

import (
	"strings"
	"time"
)

// Component is one original lot folded into a common lot.
type Component struct {
	id                int64
	commonLotID       int64
	originalLotNo     string
	originalShadeCode string
	bobbinCount       int32
	qtyKg             float64
}

// NewComponent builds and validates a component line for a common lot.
func NewComponent(originalLotNo, originalShadeCode string, bobbinCount int32, qtyKg float64) (Component, error) {
	lot := strings.TrimSpace(originalLotNo)
	if lot == "" {
		return Component{}, ErrEmptyOriginalLot
	}
	if bobbinCount < 0 {
		return Component{}, ErrNegativeBobbin
	}
	if qtyKg < 0 {
		return Component{}, ErrNegativeQty
	}
	return Component{
		originalLotNo:     lot,
		originalShadeCode: strings.TrimSpace(originalShadeCode),
		bobbinCount:       bobbinCount,
		qtyKg:             qtyKg,
	}, nil
}

// ReconstructComponent rebuilds a Component from persistence (no validation).
func ReconstructComponent(id, commonLotID int64, originalLotNo, originalShadeCode string, bobbinCount int32, qtyKg float64) Component {
	return Component{
		id: id, commonLotID: commonLotID, originalLotNo: originalLotNo,
		originalShadeCode: originalShadeCode, bobbinCount: bobbinCount, qtyKg: qtyKg,
	}
}

// ID returns the component id.
func (c Component) ID() int64 { return c.id }

// CommonLotID returns the parent common-lot id.
func (c Component) CommonLotID() int64 { return c.commonLotID }

// OriginalLotNo returns the original lot number.
func (c Component) OriginalLotNo() string { return c.originalLotNo }

// OriginalShadeCode returns the original shade code.
func (c Component) OriginalShadeCode() string { return c.originalShadeCode }

// BobbinCount returns the bobbin count folded in from this lot.
func (c Component) BobbinCount() int32 { return c.bobbinCount }

// QtyKg returns the quantity in kilograms folded in from this lot.
func (c Component) QtyKg() float64 { return c.qtyKg }

// CommonLot is the aggregate root: a consolidated ERP lot combining leftover
// bobbins from several original lots under a new ERP lot number.
type CommonLot struct {
	id           int64
	lotNo        string
	itemCode     string
	shadeCode    string
	erpGradeCode string
	createdAt    time.Time
	components   []Component
}

// NewCommonLot builds and validates a common lot from its components. At least
// one component is required.
func NewCommonLot(lotNo, itemCode, shadeCode, erpGradeCode string, components []Component) (*CommonLot, error) {
	lot := strings.TrimSpace(lotNo)
	if lot == "" {
		return nil, ErrEmptyLotNo
	}
	if len(components) == 0 {
		return nil, ErrNoComponents
	}
	return &CommonLot{
		lotNo:        lot,
		itemCode:     strings.TrimSpace(itemCode),
		shadeCode:    strings.TrimSpace(shadeCode),
		erpGradeCode: strings.TrimSpace(erpGradeCode),
		components:   components,
	}, nil
}

// ReconstructCommonLot rebuilds a CommonLot from persistence (no validation).
func ReconstructCommonLot(id int64, lotNo, itemCode, shadeCode, erpGradeCode string, createdAt time.Time, components []Component) *CommonLot {
	return &CommonLot{
		id: id, lotNo: lotNo, itemCode: itemCode, shadeCode: shadeCode,
		erpGradeCode: erpGradeCode, createdAt: createdAt, components: components,
	}
}

// ID returns the common-lot id.
func (l *CommonLot) ID() int64 { return l.id }

// LotNo returns the new ERP lot number.
func (l *CommonLot) LotNo() string { return l.lotNo }

// ItemCode returns the item code.
func (l *CommonLot) ItemCode() string { return l.itemCode }

// ShadeCode returns the shade code.
func (l *CommonLot) ShadeCode() string { return l.shadeCode }

// ErpGradeCode returns the ERP grade code.
func (l *CommonLot) ErpGradeCode() string { return l.erpGradeCode }

// CreatedAt returns the creation timestamp.
func (l *CommonLot) CreatedAt() time.Time { return l.createdAt }

// Components returns the folded-in original-lot components.
func (l *CommonLot) Components() []Component { return l.components }

// SetID sets the persisted id after insert (used by the repository).
func (l *CommonLot) SetID(id int64) { l.id = id }

// TotalBobbins sums the bobbin counts across all components.
func (l *CommonLot) TotalBobbins() int32 {
	var total int32
	for i := range l.components {
		total += l.components[i].bobbinCount
	}
	return total
}

// TotalQtyKg sums the quantities across all components.
func (l *CommonLot) TotalQtyKg() float64 {
	var total float64
	for i := range l.components {
		total += l.components[i].qtyKg
	}
	return total
}
