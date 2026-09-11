package rmcost_test

import (
	"testing"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
)

// TestSelectValuationWithFlag_ExplicitFlagEchoes pins that a configured tier is
// returned verbatim — the cascade must NOT second-guess an explicit choice even when
// the chosen tier's rate is zero.
func TestSelectValuationWithFlag_ExplicitFlagEchoes(t *testing.T) {
	tot := rmcost.ValuationTotals{CR: 1, SR: 2, PR: 3, CL: 4, SL: 5, FL: 6}
	for _, tc := range []struct {
		flag string
		want float64
	}{
		{"CR", 1}, {"SR", 2}, {"PR", 3}, {"CL", 4}, {"SL", 5}, {"FL", 6},
	} {
		got, label := rmcost.SelectValuationWithFlag(tot, tc.flag)
		if got != tc.want || label != tc.flag {
			t.Errorf("flag %s: got (%v,%q), want (%v,%q)", tc.flag, got, label, tc.want, tc.flag)
		}
	}
}

// TestSelectValuationWithFlag_AutoCascadesCLSLFLPR pins the AUTO cascade order, which
// is what the MB cost-calc-detail export re-derives a persisted row's tier from.
func TestSelectValuationWithFlag_AutoCascadesCLSLFLPR(t *testing.T) {
	for _, tc := range []struct {
		name  string
		tot   rmcost.ValuationTotals
		want  float64
		label string
	}{
		{"CL wins first", rmcost.ValuationTotals{CL: 4, SL: 5, FL: 6, PR: 3}, 4, "CL"},
		{"SL when CL zero", rmcost.ValuationTotals{SL: 5, FL: 6, PR: 3}, 5, "SL"},
		{"FL when CL+SL zero", rmcost.ValuationTotals{FL: 6, PR: 3}, 6, "FL"},
		{"PR last", rmcost.ValuationTotals{PR: 3}, 3, "PR"},
		// ⛔ CR/SR are NOT in the cascade — only an explicit flag selects them.
		{"CR/SR never cascade", rmcost.ValuationTotals{CR: 9, SR: 9}, 0, rmcost.FlagNone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, label := rmcost.SelectValuationWithFlag(tc.tot, rmcost.FlagAuto)
			if got != tc.want || label != tc.label {
				t.Errorf("got (%v,%q), want (%v,%q)", got, label, tc.want, tc.label)
			}
		})
	}
}

// TestSelectValuationWithFlag_AllZeroResolvesToNone pins the honest "no price source
// existed" marker. The MB cost-calc-detail export renders this as a BLANK cell rather
// than leaking the internal "NONE" string into the dump.
func TestSelectValuationWithFlag_AllZeroResolvesToNone(t *testing.T) {
	got, label := rmcost.SelectValuationWithFlag(rmcost.ValuationTotals{}, rmcost.FlagAuto)
	if got != 0 || label != rmcost.FlagNone {
		t.Errorf("got (%v,%q), want (0,%q)", got, label, rmcost.FlagNone)
	}
	// An unrecognized flag falls into the same cascade as AUTO.
	if _, l := rmcost.SelectValuationWithFlag(rmcost.ValuationTotals{}, "WAT"); l != rmcost.FlagNone {
		t.Errorf("unknown flag: got %q, want %q", l, rmcost.FlagNone)
	}
}

// TestSelectMarketingWithFlag_Cascade pins the SP→PP→FP marketing cascade.
func TestSelectMarketingWithFlag_Cascade(t *testing.T) {
	if v, l := rmcost.SelectMarketingWithFlag(rmcost.MarketingTotals{PP: 2, FP: 3}, rmcost.FlagAuto); v != 2 || l != "PP" {
		t.Errorf("got (%v,%q), want (2,\"PP\")", v, l)
	}
	if v, l := rmcost.SelectMarketingWithFlag(rmcost.MarketingTotals{SP: 7}, "FP"); v != 0 || l != "FP" {
		t.Errorf("explicit FP must echo back: got (%v,%q)", v, l)
	}
	if _, l := rmcost.SelectMarketingWithFlag(rmcost.MarketingTotals{}, rmcost.FlagAuto); l != rmcost.FlagNone {
		t.Errorf("all-zero: got %q, want %q", l, rmcost.FlagNone)
	}
}

// TestFirstNonZeroWithLabel_UsesStrictlyPositive pins that a NEGATIVE rate is treated
// as "no source" — the semantics the V2 engine has always had. ⛔ Not `!= 0`.
func TestFirstNonZeroWithLabel_UsesStrictlyPositive(t *testing.T) {
	v, l := rmcost.FirstNonZeroWithLabel([]rmcost.LabeledRate{{-5, "CL"}, {3, "SL"}})
	if v != 3 || l != "SL" {
		t.Errorf("negative must be skipped: got (%v,%q), want (3,\"SL\")", v, l)
	}
	if v, l := rmcost.FirstNonZeroWithLabel(nil); v != 0 || l != "" {
		t.Errorf("empty slice: got (%v,%q), want (0,\"\")", v, l)
	}
}
