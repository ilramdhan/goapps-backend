package erpintegration

import (
	"strings"
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func rule(fg string, pt erprule.ProdType, g erprule.GradeGroup, b erprule.Basis, loss string) erprule.Rule {
	return erprule.Rule{Key: erprule.RuleKey{FgType: erprule.FgType(fg), ProdType: pt, GradeGroup: g}, Basis: b, ValLoss: d(loss)}
}

// testRuleSet is a small rule set covering every basis. SPBSD has no price
// (NO_SELL_PRICE path); grade AX2 is assigned to group AX (no rule possible).
func testRuleSet(t *testing.T, extra ...erprule.Rule) *erprule.RuleSet {
	t.Helper()
	rules := []erprule.Rule{
		rule("Type 1", erprule.ProdTypePOY, erprule.GradeGroupNS, erprule.BasisCost, "0.050000"),
		rule("Type 1", erprule.ProdTypePOY, erprule.GradeGroupBC, erprule.BasisSPPTY, "0.500000"),
		rule("Type 1", erprule.ProdTypePTY, erprule.GradeGroupNS, erprule.BasisSPITY, "0.123455"),
		rule("Type 1", erprule.ProdTypeITY, erprule.GradeGroupNS, erprule.BasisSPBSD, "0.200000"),
		rule("Type 1", erprule.ProdTypePOY, erprule.GradeGroupBB, erprule.BasisCost, "1.500000"),
		rule("Type 1", erprule.ProdTypePOY, erprule.GradeGroupJLT, erprule.BasisSPPTY, "2.000000"),
		rule("Type 1", erprule.ProdTypePOY, erprule.GradeGroupAE, erprule.BasisCost, "0"),
	}
	rules = append(rules, extra...)
	prices := []erprule.Price{
		{Basis: erprule.BasisSPPTY, Price: d("1.300000")},
		{Basis: erprule.BasisSPITY, Price: d("1.500000")},
	}
	grades := []erprule.GradeAssignment{
		{GradeCode: "A", Group: erprule.GradeGroupNS},
		{GradeCode: "B", Group: erprule.GradeGroupBC},
		{GradeCode: "BB", Group: erprule.GradeGroupBB},
		{GradeCode: "JLT", Group: erprule.GradeGroupJLT},
		{GradeCode: "AE", Group: erprule.GradeGroupAE},
		{GradeCode: "AX", Group: erprule.GradeGroupAX},
		{GradeCode: "AX2", Group: erprule.GradeGroupAX},
		{GradeCode: "C", Group: erprule.GradeGroupPOYA},
	}
	rs, err := erprule.NewRuleSet(rules, prices, grades)
	require.NoError(t, err)
	return rs
}

// baseAx: TotalRMCost 1.1234565 -> chp R5 1.12346 (tie, away from zero);
// ax_conv raw 1.2222224 -> 1.22222; ax_cost R5(1.12346+1.22222) = 2.34568.
func baseAx() *AxComponents {
	return &AxComponents{
		CostID: 11, Version: 2, ProductSysID: 7,
		CostPerUnit: d("2.3456789"), TotalRMCost: d("1.1234565"),
		FgType: " Type 1 ", ChpItemCode: "REG", ItemType: "POY",
	}
}

func key(item, grade, shade string) ErpKey {
	return ErpKey{ItemCode: item, GradeCode: grade, ShadeCode: shade}
}

type wantNums struct {
	chp, kg, axConv, conv, tier, sell, loss, axCost, std, pvl string
}

func assertNums(t *testing.T, r StdRow, w wantNums) {
	t.Helper()
	check := func(name string, got decimal.NullDecimal, want string) {
		t.Helper()
		require.Truef(t, got.Valid, "%s is NULL", name)
		assert.Truef(t, got.Decimal.Equal(d(want)), "%s = %s, want %s", name, got.Decimal, want)
	}
	check("chp_cost", r.ChpCost, w.chp)
	check("chp_con_kg", r.ChpConKg, w.kg)
	check("ax_conv_cost", r.AxConvCost, w.axConv)
	check("conv_cost", r.ConvCost, w.conv)
	check("conv_cost1", r.ConvCost1, w.tier)
	check("conv_cost2", r.ConvCost2, w.tier)
	check("conv_cost4", r.ConvCost4, w.tier)
	check("conv_cost5", r.ConvCost5, w.tier)
	check("selling_price", r.SellingPrice, w.sell)
	check("value_loss", r.ValueLoss, w.loss)
	check("ax_cost", r.AxCost, w.axCost)
	check("std_cost", r.StdCost, w.std)
	check("prod_value_loss", r.ProdValLoss, w.pvl)
}

func TestDeriveRow_OKBranches(t *testing.T) {
	t.Parallel()
	rs := testRuleSet(t)
	cases := []struct {
		name   string
		in     DeriveInput
		source StdSource
		basis  erprule.Basis
		group  erprule.GradeGroup
		prod   erprule.ProdType
		want   wantNums
	}{
		{
			name: "yarn AX: basis COST, zero loss/sell, pvl 0", in: DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: baseAx()},
			source: SourceAX, basis: erprule.BasisCost, group: erprule.GradeGroupAX,
			want: wantNums{"1.12346", "1", "1.22222", "1.22222", "1.22222", "0", "0", "2.34568", "2.34568", "0"},
		},
		{
			name: "derived COST: conv = R5(ax_conv - loss)", in: DeriveInput{Key: key("POY0001", "A", "NL"), Ax: baseAx()},
			source: SourceDerived, basis: erprule.BasisCost, group: erprule.GradeGroupNS, prod: erprule.ProdTypePOY,
			want: wantNums{"1.12346", "1", "1.22222", "1.17222", "1.17222", "0", "0.05", "2.34568", "2.29568", "-0.05"},
		},
		{
			name: "derived COST with zero loss equals AX", in: DeriveInput{Key: key("POY0001", "AE", "NL"), Ax: baseAx()},
			source: SourceDerived, basis: erprule.BasisCost, group: erprule.GradeGroupAE, prod: erprule.ProdTypePOY,
			want: wantNums{"1.12346", "1", "1.22222", "1.22222", "1.22222", "0", "0", "2.34568", "2.34568", "0"},
		},
		{
			name: "derived COST: loss above conv gives negative conv", in: DeriveInput{Key: key("POY0001", "BB", "NL"), Ax: baseAx()},
			source: SourceDerived, basis: erprule.BasisCost, group: erprule.GradeGroupBB, prod: erprule.ProdTypePOY,
			want: wantNums{"1.12346", "1", "1.22222", "-0.27778", "-0.27778", "0", "1.5", "2.34568", "0.84568", "-1.5"},
		},
		{
			name: "derived SPPTY: std = R5(sell - loss), conv 0", in: DeriveInput{Key: key("POY0001", "B", "NL"), Ax: baseAx()},
			source: SourceDerived, basis: erprule.BasisSPPTY, group: erprule.GradeGroupBC, prod: erprule.ProdTypePOY,
			want: wantNums{"1.12346", "1", "1.22222", "0", "0", "1.3", "0.5", "2.34568", "0.8", "-1.54568"},
		},
		{
			name: "derived SPPTY: loss above price gives negative std", in: DeriveInput{Key: key("POY0001", "JLT", "NL"), Ax: baseAx()},
			source: SourceDerived, basis: erprule.BasisSPPTY, group: erprule.GradeGroupJLT, prod: erprule.ProdTypePOY,
			want: wantNums{"1.12346", "1", "1.22222", "0", "0", "1.3", "2", "2.34568", "-0.7", "-3.04568"},
		},
		{
			name: "derived SPITY (PTY): loss 0.123455 rounds half away to 0.12346", in: DeriveInput{Key: key("PTY0001", "A", "NL"), Ax: baseAx()},
			source: SourceDerived, basis: erprule.BasisSPITY, group: erprule.GradeGroupNS, prod: erprule.ProdTypePTY,
			want: wantNums{"1.12346", "1", "1.22222", "0", "0", "1.5", "0.12346", "2.34568", "1.37654", "-0.96914"},
		},
		{
			name: "MB grade A: std = R5(CostPerUnit), pvl 0", in: DeriveInput{Key: key("CMB0001", "A", "NL"), Ax: &AxComponents{
				CostID: 3, Version: 1, ProductSysID: 9, CostPerUnit: d("3.1234550"), TotalRMCost: d("1.0000005"),
			}},
			source: SourceMB, basis: erprule.BasisCost, group: erprule.GradeGroupAX,
			// chp R5(1.0000005) = 1.00000; ax_conv R5(2.1234545) = 2.12345;
			// ax_cost 3.12345 while std is R5(3.123455) = 3.12346 (not chp+conv).
			want: wantNums{"1", "1", "2.12345", "2.12345", "2.12345", "0", "0", "3.12345", "3.12346", "0"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := DeriveRow(tc.in, rs)
			require.Equalf(t, DeriveOK, r.Status, "issues: %+v", r.Issues)
			assert.Empty(t, r.Issues)
			assert.False(t, r.HasErrors())
			assert.Equal(t, tc.source, r.Source)
			assert.Equal(t, tc.basis, r.Basis)
			assert.Equal(t, tc.group, r.GradeGroup)
			assert.Equal(t, tc.prod, r.ProdType)
			assertNums(t, r, tc.want)
		})
	}
}

func TestDeriveRow_AXAttributesAndIDs(t *testing.T) {
	t.Parallel()
	ax := baseAx()
	ppd := d("123.4567891")
	ax.PrdPerDay = &ppd
	r := DeriveRow(DeriveInput{Key: key("POY0001", "A", "NL"), Ax: ax, ItemName: "  Yarn  ", ShadeName: strings.Repeat("é", 300)}, testRuleSet(t))
	require.Equal(t, DeriveOK, r.Status)
	assert.Equal(t, "Type 1", r.FgType)
	assert.Equal(t, "REG", r.ChpItemCode)
	assert.Equal(t, "POY", r.ItemType)
	assert.Equal(t, "0", r.MsBatchItem, "derived rows NVL the MS batch item to 0")
	assert.Equal(t, "Yarn", r.ItemName)
	assert.Len(t, []rune(r.ShadeName), 240)
	require.NotNil(t, r.AxCostSysID)
	assert.Equal(t, int64(11), *r.AxCostSysID)
	assert.Equal(t, int32(2), *r.AxCostVersion)
	assert.Equal(t, int64(7), *r.ProductSysID)
	assert.True(t, r.PrdPerDay.Decimal.Equal(ppd), "prd_per_day passes through unrounded")
	assert.Equal(t, ItemKindYarn, r.Kind)

	// AX keeps an empty MS batch item; item type defaults to the prefix.
	ax2 := baseAx()
	ax2.ItemType, ax2.MsBatchItem = "", ""
	ra := DeriveRow(DeriveInput{Key: key("TTY0001", "AX", "NL"), Ax: ax2}, nil)
	require.Equal(t, DeriveOK, ra.Status)
	assert.Empty(t, ra.MsBatchItem)
	assert.Equal(t, "TTY", ra.ItemType)
	assert.False(t, ra.PrdPerDay.Valid)
}

func TestDeriveRow_FailureStatuses(t *testing.T) {
	t.Parallel()
	ambiguous := rule("Type 3", erprule.ProdTypePOY, erprule.GradeGroupNS, erprule.BasisCost, "0.1")
	rs := testRuleSet(t, ambiguous, rule("Type 3", erprule.ProdTypePOY, erprule.GradeGroupNS, erprule.BasisSPPTY, "0.2"))
	noFg := baseAx()
	noFg.FgType = "  "
	type3 := baseAx()
	type3.FgType = "Type 3"
	type9 := baseAx()
	type9.FgType = "Type 9"
	longFg := baseAx()
	longFg.FgType = strings.Repeat("x", 16)
	cases := []struct {
		name   string
		in     DeriveInput
		rs     *erprule.RuleSet
		status DeriveStatus
		code   IssueCode
		msg    string
	}{
		{"no AX (yarn derived)", DeriveInput{Key: key("POY0001", "A", "NL")}, rs, DeriveNoAX, IssueV08, "no active approved AX"},
		{"no AX (AX grade)", DeriveInput{Key: key("POY0001", "AX", "NL")}, rs, DeriveNoAX, IssueV08, "no active approved AX"},
		{"no AX (MB)", DeriveInput{Key: key("CMB0001", "A", "NL")}, rs, DeriveNoAX, IssueV08, "no active approved AX"},
		{"MB grade not A", DeriveInput{Key: key("CMB0001", "B", "NL"), Ax: baseAx()}, rs, DeriveNoRule, IssueV08, "grade A only"},
		{"grade without group", DeriveInput{Key: key("POY0001", "ZZ", "NL"), Ax: baseAx()}, rs, DeriveNoGradeGroup, IssueV08, `grade "ZZ"`},
		{"nil rule set", DeriveInput{Key: key("POY0001", "A", "NL"), Ax: baseAx()}, nil, DeriveNoGradeGroup, IssueV08, "no rule set"},
		{"empty FG type", DeriveInput{Key: key("POY0001", "A", "NL"), Ax: noFg}, rs, DeriveNoFgType, IssueV08, "no FG type"},
		{"no rule for key", DeriveInput{Key: key("POY0001", "A", "NL"), Ax: type9}, rs, DeriveNoRule, IssueV08, "no valloss rule for Type 9/POY/NS"},
		{"no rule for group", DeriveInput{Key: key("POY0001", "C", "NL"), Ax: baseAx()}, rs, DeriveNoRule, IssueV08, "Type 1/POY/POYA"},
		{"grade in group AX", DeriveInput{Key: key("POY0001", "AX2", "NL"), Ax: baseAx()}, rs, DeriveNoRule, IssueV08, "grade group AX"},
		{"ambiguous rule", DeriveInput{Key: key("POY0001", "A", "NL"), Ax: type3}, rs, DeriveNoRule, IssueV08, "more than one"},
		{"FG type too long", DeriveInput{Key: key("POY0001", "A", "NL"), Ax: longFg}, rs, DeriveNoRule, IssueV08, "cannot match"},
		{"sell price missing", DeriveInput{Key: key("ITY0001", "A", "NL"), Ax: baseAx()}, rs, DeriveNoSellPrice, IssueV08, "SPBSD"},
		{"invalid kind", DeriveInput{Key: key("POY0001", "AX", "NL"), Kind: "FOO", Ax: baseAx()}, rs, DeriveInvalid, IssueV06, "item kind"},
		{"item code too long", DeriveInput{Key: key("POY0001234567", "AX", "NL"), Ax: baseAx()}, rs, DeriveInvalid, IssueV06, "push-safe"},
		{"empty shade", DeriveInput{Key: key("POY0001", "AX", ""), Ax: baseAx()}, rs, DeriveInvalid, IssueV06, "push-safe"},
		{"non-ASCII grade", DeriveInput{Key: key("POY0001", "Aé", "NL"), Ax: baseAx()}, rs, DeriveNoGradeGroup, IssueV08, "grade"},
		{"std overflow", DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: &AxComponents{CostPerUnit: d("1000.5"), TotalRMCost: d("1")}}, rs, DeriveInvalid, IssueV03, "ax_cost 1000.5"},
		{"negative overflow", DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: &AxComponents{CostPerUnit: d("-1000"), TotalRMCost: d("0")}}, rs, DeriveInvalid, IssueV03, "ax_conv_cost"},
		{"rounds up into overflow", DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: &AxComponents{CostPerUnit: d("999.999995"), TotalRMCost: d("0")}}, rs, DeriveInvalid, IssueV03, "ax_conv_cost"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			r := DeriveRow(tc.in, tc.rs)
			assert.Equal(t, tc.status, r.Status)
			require.NotEmpty(t, r.Issues)
			last := r.Issues[len(r.Issues)-1]
			assert.Equal(t, tc.code, last.Code)
			assert.Equal(t, SeverityError, last.Severity)
			assert.Equal(t, tc.in.Key, last.Key)
			assert.Contains(t, last.Message, tc.msg)
			assert.True(t, strings.HasPrefix(last.Message, string(tc.status)+": "))
			assert.True(t, r.HasErrors())
		})
	}
}

func TestDeriveRow_FailedRowHasNoStd(t *testing.T) {
	t.Parallel()
	r := DeriveRow(DeriveInput{Key: key("POY0001", "ZZ", "NL"), Ax: baseAx()}, testRuleSet(t))
	require.Equal(t, DeriveNoGradeGroup, r.Status)
	assert.False(t, r.StdCost.Valid)
	assert.False(t, r.ConvCost.Valid)
	assert.False(t, r.ValueLoss.Valid)
	assert.True(t, r.AxCost.Valid, "AX components are still reported for diagnosis")

	n := DeriveRow(DeriveInput{Key: key("POY0001", "A", "NL")}, testRuleSet(t))
	assert.Nil(t, n.AxCostSysID)
	assert.False(t, n.AxCost.Valid)
	assert.Equal(t, SourceDerived, n.Source)
}

func TestDeriveRow_ProdTypeFallbackWarning(t *testing.T) {
	t.Parallel()
	rs := testRuleSet(t)
	for _, item := range []string{"ACY0001", "TTY0001", "HOY0001", "MM"} {
		t.Run(item, func(t *testing.T) {
			t.Parallel()
			r := DeriveRow(DeriveInput{Key: key(item, "A", "NL"), Ax: baseAx()}, rs)
			require.Equal(t, DeriveOK, r.Status, "falls back to PTY -> SPITY rule")
			assert.Equal(t, erprule.ProdTypePTY, r.ProdType)
			assert.Equal(t, erprule.BasisSPITY, r.Basis)
			require.Len(t, r.Issues, 1)
			assert.Equal(t, IssueV08w, r.Issues[0].Code)
			assert.Equal(t, SeverityWarning, r.Issues[0].Severity)
			assert.Equal(t, key(item, "A", "NL"), r.Issues[0].Key)
			assert.False(t, r.HasErrors())
		})
	}
	// The warning is kept alongside a later error.
	r := DeriveRow(DeriveInput{Key: key("ACY0001", "ZZ", "NL"), Ax: baseAx()}, rs)
	require.Len(t, r.Issues, 2)
	assert.Equal(t, IssueV08w, r.Issues[0].Code)
	assert.Equal(t, IssueV08, r.Issues[1].Code)
	// AX rows never go through the rule, so no warning.
	ax := DeriveRow(DeriveInput{Key: key("ACY0001", "AX", "NL"), Ax: baseAx()}, rs)
	assert.Empty(t, ax.Issues)
}

func TestProdTypeForItem(t *testing.T) {
	t.Parallel()
	cases := []struct {
		item     string
		want     erprule.ProdType
		fallback bool
	}{
		{"POY123", erprule.ProdTypePOY, false},
		{" ITY9 ", erprule.ProdTypeITY, false},
		{"PTY", erprule.ProdTypePTY, false},
		{"poy123", erprule.ProdTypePTY, true}, // SUBSTR is case-sensitive
		{"ACY1", erprule.ProdTypePTY, true},
		{"CMB1", erprule.ProdTypePTY, true},
		{"", erprule.ProdTypePTY, true},
	}
	for _, tc := range cases {
		got, fb := ProdTypeForItem(tc.item)
		assert.Equalf(t, tc.want, got, "item %q", tc.item)
		assert.Equalf(t, tc.fallback, fb, "item %q", tc.item)
	}
}

func TestDeriveRow_ZeroAndNegativeInputs(t *testing.T) {
	t.Parallel()
	rs := testRuleSet(t)
	zero := &AxComponents{CostPerUnit: decimal.Zero, TotalRMCost: decimal.Zero, FgType: "Type 1"}
	r := DeriveRow(DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: zero}, rs)
	require.Equal(t, DeriveOK, r.Status, "std = 0 is caught by V-01 at validate, not by derive")
	assertNums(t, r, wantNums{"0", "1", "0", "0", "0", "0", "0", "0", "0", "0"})

	rd := DeriveRow(DeriveInput{Key: key("POY0001", "A", "NL"), Ax: zero}, rs)
	require.Equal(t, DeriveOK, rd.Status)
	assertNums(t, rd, wantNums{"0", "1", "0", "-0.05", "-0.05", "0", "0.05", "0", "-0.05", "-0.05"})

	// Cost per unit below the RM cost: negative conversion, ties away from zero.
	neg := &AxComponents{CostPerUnit: d("1.0000000"), TotalRMCost: d("1.0000150"), FgType: "Type 1"}
	rn := DeriveRow(DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: neg}, rs)
	require.Equal(t, DeriveOK, rn.Status)
	// chp R5(1.000015) = 1.00002; ax_conv R5(-0.000015) = -0.00002; std R5(1.00002-0.00002) = 1.
	assertNums(t, rn, wantNums{"1.00002", "1", "-0.00002", "-0.00002", "-0.00002", "0", "0", "1", "1", "0"})
}

func TestDeriveRow_RoundingIsHalfAwayFromZeroNotBankers(t *testing.T) {
	t.Parallel()
	// ax_conv raw = 0.000025: half-away gives 0.00003, banker's 0.00002.
	ax := &AxComponents{CostPerUnit: d("1.000025"), TotalRMCost: d("1"), FgType: "Type 1"}
	r := DeriveRow(DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: ax}, testRuleSet(t))
	require.Equal(t, DeriveOK, r.Status)
	assert.True(t, r.AxConvCost.Decimal.Equal(d("0.00003")))
	assert.False(t, r.AxConvCost.Decimal.Equal(d("0.000025").RoundBank(ScaleR5)))
	assert.True(t, r.StdCost.Decimal.Equal(d("1.00003")))

	neg := &AxComponents{CostPerUnit: d("1"), TotalRMCost: d("1.000025"), FgType: "Type 1"}
	rn := DeriveRow(DeriveInput{Key: key("POY0001", "AX", "NL"), Ax: neg}, testRuleSet(t))
	require.Equal(t, DeriveOK, rn.Status)
	// chp R5(1.000025) = 1.00003; ax_conv R5(-0.000025) = -0.00003.
	assert.True(t, rn.ChpCost.Decimal.Equal(d("1.00003")))
	assert.True(t, rn.AxConvCost.Decimal.Equal(d("-0.00003")))
	assert.True(t, rn.StdCost.Decimal.Equal(d("1")))
}

func TestDeriveRow_AllComponentsAtMost5dp(t *testing.T) {
	t.Parallel()
	rs := testRuleSet(t)
	ax := &AxComponents{CostPerUnit: d("2.123456789123"), TotalRMCost: d("0.987654321987"), FgType: "Type 1"}
	for _, g := range []string{"AX", "A", "B", "BB", "JLT", "AE"} {
		r := DeriveRow(DeriveInput{Key: key("POY0001", g, "NL"), Ax: ax}, rs)
		require.Equal(t, DeriveOK, r.Status, g)
		for _, c := range r.NumericComponents() {
			require.True(t, c.Value.Valid, "%s/%s", g, c.Name)
			assert.Truef(t, c.Value.Decimal.Equal(Round5(c.Value.Decimal)), "%s/%s = %s has more than 5 dp", g, c.Name, c.Value.Decimal)
			_, err := FormatFlexNull(c.Value)
			assert.NoError(t, err)
		}
	}
}

func TestDerive_BatchKeepsOrderAndCollectsIssues(t *testing.T) {
	t.Parallel()
	inputs := []DeriveInput{
		{Key: key("POY0001", "AX", "NL"), Ax: baseAx()},
		{Key: key("POY0001", "ZZ", "NL"), Ax: baseAx()},
		{Key: key("ACY0001", "A", "NL"), Ax: baseAx()},
		{Key: key("CMB0001", "A", "NL"), Kind: ItemKindMB, Ax: baseAx()},
	}
	rows, issues := Derive(inputs, testRuleSet(t))
	require.Len(t, rows, len(inputs))
	for i, r := range rows {
		assert.Equal(t, inputs[i].Key, r.Key)
	}
	assert.Equal(t, []DeriveStatus{DeriveOK, DeriveNoGradeGroup, DeriveOK, DeriveOK},
		[]DeriveStatus{rows[0].Status, rows[1].Status, rows[2].Status, rows[3].Status})
	require.Len(t, issues, 2)
	assert.Equal(t, IssueV08, issues[0].Code)
	assert.Equal(t, IssueV08w, issues[1].Code)

	empty, none := Derive(nil, nil)
	assert.Empty(t, empty)
	assert.Empty(t, none)
}

func TestDerive_Deterministic(t *testing.T) {
	t.Parallel()
	rs := testRuleSet(t)
	in := []DeriveInput{{Key: key("POY0001", "B", "NL"), Ax: baseAx()}, {Key: key("PTY0001", "A", "NL"), Ax: baseAx()}}
	a, _ := Derive(in, rs)
	b, _ := Derive(in, rs)
	assert.Equal(t, a, b)
}

func TestParseDeriveStatus(t *testing.T) {
	t.Parallel()
	for _, s := range allDeriveStatuses {
		got, err := ParseDeriveStatus(s.String())
		require.NoError(t, err)
		assert.Equal(t, s, got)
	}
	_, err := ParseDeriveStatus("ok")
	require.ErrorIs(t, err, ErrInvalidDeriveStatus)
	_, err = ParseDeriveStatus("")
	require.ErrorIs(t, err, ErrInvalidDeriveStatus)
}

func TestErpKey_PushSafe(t *testing.T) {
	t.Parallel()
	cases := []struct {
		k    ErpKey
		want bool
	}{
		{key("POY000000001", "AX", "NL"), true},
		{key("POY0000000012", "AX", "NL"), false},
		{key("POY1", "", "NL"), false},
		{key("POY1", "AX", " NL"), false},
		{key("POY1", "AX", "N\tL"), false},
		{key("POY1", "Aé", "NL"), false},
		{key("POY1", "A9/A", "NL-01"), true},
	}
	for _, tc := range cases {
		assert.Equalf(t, tc.want, tc.k.PushSafe(), "%s", tc.k)
	}
	assert.Equal(t, "POY1/AX/NL", key("POY1", "AX", "NL").String())
	assert.Equal(t, "GOAPPS_AX", SourceAX.String())
}
