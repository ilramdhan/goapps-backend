package erprule

import (
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func seedLikeInputs() ([]Rule, []Price, []GradeAssignment) {
	rules := []Rule{
		{RuleKey{"Type 1", ProdTypePOY, GradeGroupBC}, BasisSPPTY, d("0.5")},
		{RuleKey{"Type 1", ProdTypePOY, GradeGroupPOYA}, BasisCost, d("0")},
		{RuleKey{"Type 1", ProdTypePTY, GradeGroupNS}, BasisCost, d("0.05")},
		{RuleKey{"Type 10", ProdTypeITY, GradeGroupBB}, BasisSPITY, d("0.6")},
		{RuleKey{"Type 12", ProdTypePTY, GradeGroupBB}, BasisSPBSD, d("0.4")},
	}
	prices := []Price{
		{BasisSPBSD, d("1.4")},
		{BasisSPITY, d("1.5")},
		{BasisSPPTY, d("1.3")},
	}
	grades := []GradeAssignment{
		{"A9/A", GradeGroupNS},
		{"AX", GradeGroupAX},
		{"B1", GradeGroupBC},
		{"JLT", GradeGroupJLT},
	}
	return rules, prices, grades
}

func mustRuleSet(t *testing.T, r []Rule, p []Price, g []GradeAssignment) *RuleSet {
	t.Helper()
	rs, err := NewRuleSet(r, p, g)
	if err != nil {
		t.Fatal(err)
	}
	return rs
}

func TestRuleSetLoss(t *testing.T) {
	r0, p0, g0 := seedLikeInputs()
	rs := mustRuleSet(t, r0, p0, g0)
	basis, loss, err := rs.Loss("Type 12", ProdTypePTY, GradeGroupBB)
	if err != nil || basis != BasisSPBSD || !loss.Equal(d("0.4")) {
		t.Fatalf("Loss = %v %v %v", basis, loss, err)
	}
	if _, _, err := rs.Loss("Type 2", ProdTypePTY, GradeGroupBB); !errors.Is(err, ErrRuleMissing) {
		t.Errorf("missing err = %v", err)
	}
	if _, _, err := rs.Loss("Type 1", ProdTypePOY, GradeGroupAX); !errors.Is(err, ErrAxRule) {
		t.Errorf("AX err = %v", err)
	}
	if _, _, err := rs.Loss("", ProdTypePOY, GradeGroupBC); !errors.Is(err, ErrInvalidFgType) {
		t.Errorf("empty fg err = %v", err)
	}
}

func TestRuleSetAmbiguous(t *testing.T) {
	r, p, g := seedLikeInputs()
	r = append(r, Rule{RuleKey{"Type 1", ProdTypePOY, GradeGroupBC}, BasisCost, d("0.1")})
	rs := mustRuleSet(t, r, p, g)
	if _, _, err := rs.Loss("Type 1", ProdTypePOY, GradeGroupBC); !errors.Is(err, ErrRuleAmbiguous) {
		t.Fatalf("err = %v", err)
	}
	keys := rs.AmbiguousKeys()
	if len(keys) != 1 || keys[0] != (RuleKey{"Type 1", ProdTypePOY, GradeGroupBC}) {
		t.Fatalf("AmbiguousKeys = %v", keys)
	}
	if rs.RuleCount() != 6 {
		t.Fatalf("RuleCount = %d", rs.RuleCount())
	}
	all := rs.Rules()
	if all[0].Basis != BasisCost || all[1].Basis != BasisSPPTY {
		t.Fatalf("duplicates must be ordered by basis: %+v", all[:2])
	}
}

func TestRuleSetSellPriceAndGrades(t *testing.T) {
	r0, p0, g0 := seedLikeInputs()
	rs := mustRuleSet(t, r0, p0, g0)
	if p, err := rs.SellPrice(BasisSPPTY); err != nil || !p.Equal(d("1.3")) {
		t.Fatalf("SPPTY = %v %v", p, err)
	}
	if _, err := rs.SellPrice(BasisCost); !errors.Is(err, ErrInvalidBasis) {
		t.Errorf("COST err = %v", err)
	}
	r, _, g := seedLikeInputs()
	rs2 := mustRuleSet(t, r, []Price{{BasisSPPTY, d("1.3")}}, g)
	if _, err := rs2.SellPrice(BasisSPITY); !errors.Is(err, ErrSellPriceMissing) {
		t.Errorf("missing price err = %v", err)
	}
	missing := rs2.RulesMissingPrice()
	want := []RuleKey{{"Type 10", ProdTypeITY, GradeGroupBB}, {"Type 12", ProdTypePTY, GradeGroupBB}}
	if !reflect.DeepEqual(missing, want) {
		t.Fatalf("RulesMissingPrice = %v", missing)
	}
	if len(rs.RulesMissingPrice()) != 0 || len(rs.AmbiguousKeys()) != 0 {
		t.Fatal("complete set must have no gaps")
	}
	if grp, ok := rs.GradeGroupOf("A9/A"); !ok || grp != GradeGroupNS {
		t.Fatalf("A9/A = %v %v", grp, ok)
	}
	if _, ok := rs.GradeGroupOf("CI"); ok {
		t.Fatal("CI has no group")
	}
}

func TestRuleSetOrderingIsDeterministic(t *testing.T) {
	r, p, g := seedLikeInputs()
	base := mustRuleSet(t, r, p, g)
	rng := rand.New(rand.NewSource(42)) //nolint:gosec // test shuffle
	for i := 0; i < 20; i++ {
		rr := append([]Rule(nil), r...)
		pp := append([]Price(nil), p...)
		gg := append([]GradeAssignment(nil), g...)
		rng.Shuffle(len(rr), func(a, b int) { rr[a], rr[b] = rr[b], rr[a] })
		rng.Shuffle(len(pp), func(a, b int) { pp[a], pp[b] = pp[b], pp[a] })
		rng.Shuffle(len(gg), func(a, b int) { gg[a], gg[b] = gg[b], gg[a] })
		rs := mustRuleSet(t, rr, pp, gg)
		if !reflect.DeepEqual(rs.Rules(), base.Rules()) ||
			!reflect.DeepEqual(rs.Prices(), base.Prices()) ||
			!reflect.DeepEqual(rs.GradeAssignments(), base.GradeAssignments()) {
			t.Fatal("ordering must not depend on input order")
		}
	}
	rules := base.Rules()
	for i := 1; i < len(rules); i++ {
		if rules[i].Key.Less(rules[i-1].Key) {
			t.Fatalf("rules not sorted at %d", i)
		}
	}
	if base.Prices()[0].Basis != BasisSPBSD || base.GradeAssignments()[0].GradeCode != "A9/A" {
		t.Fatal("prices / grades not sorted")
	}
	if base.PriceCount() != 3 || base.GradeCount() != 4 || base.RuleCount() != 5 {
		t.Fatal("counts wrong")
	}
}

func TestRuleSetIsImmutable(t *testing.T) {
	r, p, g := seedLikeInputs()
	rs := mustRuleSet(t, r, p, g)
	r[0].ValLoss = d("0.9")
	p[2].Price = d("9")
	g[0].Group = GradeGroupBB
	rs.Rules()[0].Basis = BasisCost
	if _, loss, _ := rs.Loss("Type 1", ProdTypePOY, GradeGroupBC); !loss.Equal(d("0.5")) {
		t.Fatal("input mutation leaked into RuleSet")
	}
	if pr, _ := rs.SellPrice(BasisSPPTY); !pr.Equal(d("1.3")) {
		t.Fatal("price mutation leaked")
	}
	if grp, _ := rs.GradeGroupOf("A9/A"); grp != GradeGroupNS {
		t.Fatal("grade mutation leaked")
	}
}

func TestNewRuleSetRejects(t *testing.T) {
	okRule := Rule{RuleKey{"Type 1", ProdTypePOY, GradeGroupBC}, BasisSPPTY, d("0.5")}
	cases := []struct {
		name   string
		rules  []Rule
		prices []Price
		grades []GradeAssignment
		want   error
	}{
		{"ax rule", []Rule{{RuleKey{"Type 1", ProdTypePOY, GradeGroupAX}, BasisCost, d("0")}}, nil, nil, ErrAxRule},
		{"bad basis", []Rule{{okRule.Key, "X", d("0")}}, nil, nil, ErrInvalidBasis},
		{"bad loss", []Rule{{okRule.Key, BasisCost, d("-1")}}, nil, nil, ErrInvalidPercent},
		{"cost price", nil, []Price{{BasisCost, d("1")}}, nil, ErrInvalidBasis},
		{"zero price", nil, []Price{{BasisSPPTY, d("0")}}, nil, ErrInvalidPrice},
		{"dup price", nil, []Price{{BasisSPPTY, d("1")}, {BasisSPPTY, d("2")}}, nil, ErrDuplicateRule},
		{"empty grade", nil, nil, []GradeAssignment{{"", GradeGroupNS}}, ErrInvalidGradeCode},
		{"untrimmed grade", nil, nil, []GradeAssignment{{" A", GradeGroupNS}}, ErrInvalidGradeCode},
		{"bad group", nil, nil, []GradeAssignment{{"A", "ZZ"}}, ErrInvalidGradeGroup},
		{"dup grade", nil, nil, []GradeAssignment{{"A", GradeGroupNS}, {"A", GradeGroupBC}}, ErrDuplicateRule},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewRuleSet(c.rules, c.prices, c.grades); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
	rs, err := NewRuleSet(nil, nil, nil)
	if err != nil || rs.RuleCount() != 0 || len(rs.Rules()) != 0 {
		t.Fatalf("empty set: %v %v", rs, err)
	}
}

func TestVallossRuleFeedsRuleSet(t *testing.T) {
	k := mustKey(t, "Type 13", "PTY", "JLT")
	vr, err := NewVallossRule(k, BasisSPPTY, d("0.51"), "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := NewSellPrice(BasisSPPTY, d("1.3"), "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	gr := ReconstructGrade("JLT", "", true, nil)
	if _, err := gr.AssignGroup(GradeGroupJLT, "a"); err != nil {
		t.Fatal(err)
	}
	ga, _ := gr.Assignment()
	rs := mustRuleSet(t, []Rule{vr.Rule()}, []Price{sp.Value()}, []GradeAssignment{ga})
	grp, ok := rs.GradeGroupOf("JLT")
	if !ok {
		t.Fatal("grade group missing")
	}
	b, loss, err := rs.Loss("Type 13", ProdTypePTY, grp)
	if err != nil || b != BasisSPPTY || !loss.Equal(d("0.51")) {
		t.Fatalf("Loss = %v %v %v", b, loss, err)
	}
}
