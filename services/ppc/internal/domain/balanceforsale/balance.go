// Package balanceforsale computes the real-time AX balance available for sale
// per commodity-watch product. Source: PRD page 10.
//
//	Balance for Sale (AX) = current_stock_AX
//	  + WO_running_output_estimated   (RUNNING WO production estimate)
//	  + MTS_plan_qty                  (confirmed MTS plan items)
//	  - committed_contract_qty        (IN_PRODUCTION + CONFIRMED demand)
package balanceforsale

// Row is the per-product balance-for-sale breakdown. All quantities are in kg.
type Row struct {
	CpmProductSysID      int64
	ProductCode          string
	ProductName          string
	CurrentStockAX       float64 // stubbed to 0 — no Orion inventory ETL in scope
	WORunningOutputEst   float64
	MtsPlanQty           float64
	CommittedContractQty float64
	BalanceForSale       float64
}

// Compute fills BalanceForSale from the four components per the PRD formula. It
// is a pure function so the formula is unit-testable in isolation.
func Compute(r Row) Row {
	r.BalanceForSale = r.CurrentStockAX + r.WORunningOutputEst + r.MtsPlanQty - r.CommittedContractQty
	return r
}

// Components carries the raw per-product component quantities gathered from the
// planning data before the balance is derived.
type Components struct {
	CpmProductSysID      int64
	ProductCode          string
	ProductName          string
	CurrentStockAX       float64
	WORunningOutputEst   float64
	MtsPlanQty           float64
	CommittedContractQty float64
}

// BuildRow derives a balance Row from its raw components.
func BuildRow(c Components) Row {
	return Compute(Row{
		CpmProductSysID:      c.CpmProductSysID,
		ProductCode:          c.ProductCode,
		ProductName:          c.ProductName,
		CurrentStockAX:       c.CurrentStockAX,
		WORunningOutputEst:   c.WORunningOutputEst,
		MtsPlanQty:           c.MtsPlanQty,
		CommittedContractQty: c.CommittedContractQty,
	})
}
