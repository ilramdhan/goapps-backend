package mbdozing

import "context"

// ImpactRow is one cost product whose frozen dozing came from a given MB spin.
//
// It is a READ-ONLY projection (user decision K-18): nothing in P7 writes to
// cost_product_master or cost_product_parameter. The row exists so the UI can
// warn the user how many products a dozing change would touch before P8
// actually applies anything.
type ImpactRow struct {
	// ProductSysID is cost_product_master.cpm_product_sys_id.
	ProductSysID int64
	// ProductCode is cost_product_master.cpm_product_code.
	ProductCode string
	// ProductName is cost_product_master.cpm_product_name.
	ProductName string
	// IsLocked mirrors cost_product_master.cpm_is_locked. A locked product
	// cannot be edited outside the MB recipe workflow.
	IsLocked bool
	// FrozenDozing is the MB_SP_DOZING value frozen onto the product, when one
	// is stored. Nil means the product has no frozen dozing row — it is NOT
	// defaulted to zero, because zero is a meaningful dozing value.
	FrozenDozing *float64
}

// Totals carries the un-truncated counts for an impact preview.
type Totals struct {
	// TotalAffected is the number of matching products before the limit applies.
	TotalAffected int
	// TotalLocked is how many of those are locked.
	TotalLocked int
}

// ImpactRepository reads which cost products are bound to an MB spin.
type ImpactRepository interface {
	// ImpactBySpin returns up to limit affected products for the given ORION
	// item code, together with the un-truncated totals. The returned slice is
	// capped by limit; Totals is not.
	ImpactBySpin(ctx context.Context, orionItemCode string, limit int) ([]ImpactRow, Totals, error)
}
