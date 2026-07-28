package balanceforsale

import "testing"

func TestCompute_Formula(t *testing.T) {
	tests := []struct {
		name string
		row  Row
		want float64
	}{
		{
			name: "PRD example: stock + running + MTS - committed",
			row:  Row{CurrentStockAX: 1000, WORunningOutputEst: 500, MtsPlanQty: 300, CommittedContractQty: 800},
			want: 1000, // 1000 + 500 + 300 - 800
		},
		{
			name: "stubbed stock (0) still balances the rest",
			row:  Row{CurrentStockAX: 0, WORunningOutputEst: 1200, MtsPlanQty: 0, CommittedContractQty: 900},
			want: 300,
		},
		{
			name: "oversold negative balance",
			row:  Row{CurrentStockAX: 100, WORunningOutputEst: 50, MtsPlanQty: 0, CommittedContractQty: 500},
			want: -350,
		},
		{
			name: "all zero",
			row:  Row{},
			want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Compute(tt.row)
			if got.BalanceForSale != tt.want {
				t.Errorf("Compute().BalanceForSale = %g, want %g", got.BalanceForSale, tt.want)
			}
		})
	}
}

func TestBuildRow_CopiesComponents(t *testing.T) {
	c := Components{
		CpmProductSysID:      42,
		ProductCode:          "P1",
		ProductName:          "Poly 150D",
		CurrentStockAX:       200,
		WORunningOutputEst:   100,
		MtsPlanQty:           50,
		CommittedContractQty: 120,
	}
	r := BuildRow(c)
	if r.CpmProductSysID != 42 || r.ProductCode != "P1" || r.ProductName != "Poly 150D" {
		t.Errorf("identity fields not copied: %+v", r)
	}
	// 200 + 100 + 50 - 120 = 230.
	if r.BalanceForSale != 230 {
		t.Errorf("BalanceForSale = %g, want 230", r.BalanceForSale)
	}
}
