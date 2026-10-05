package erprule

import (
	"errors"
	"math/rand"
	"reflect"
	"testing"
)

func TestDiff(t *testing.T) {
	r0, p0, g0 := seedLikeInputs()
	base := mustRuleSet(t, r0, p0, g0)
	kPOYBC := RuleKey{"Type 1", ProdTypePOY, GradeGroupBC}
	kNew := RuleKey{"Type 13", ProdTypeITY, GradeGroupJLT}

	// mutate returns a copy of the seed inputs changed by fn.
	mutate := func(fn func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment)) *RuleSet {
		r, p, g := fn(seedLikeInputs())
		return mustRuleSet(t, r, p, g)
	}

	tests := []struct {
		name string
		prev *RuleSet
		cur  *RuleSet
		want []RuleChange
	}{
		{name: "identical", prev: base, cur: base, want: nil},
		{name: "same value different scale", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			r[2].ValLoss = d("0.050000")
			p[0].Price = d("1.400000")
			return r, p, g
		}), want: nil},
		{name: "rule added", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return append(r, Rule{kNew, BasisCost, d("0.1")}), p, g
		}), want: []RuleChange{{
			Subject: SubjectValLossRule, Kind: ChangeAdded, Key: "Type 13/ITY/JLT", RuleKey: kNew,
			After: []ChangeValue{{Basis: BasisCost, Value: d("0.1")}},
		}}},
		{name: "rule removed", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return r[1:], p, g
		}), want: []RuleChange{{
			Subject: SubjectValLossRule, Kind: ChangeRemoved, Key: "Type 1/POY/BC", RuleKey: kPOYBC,
			Before: []ChangeValue{{Basis: BasisSPPTY, Value: d("0.5")}},
		}}},
		{name: "rule val_loss changed by 1e-6", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			r[0].ValLoss = d("0.500001")
			return r, p, g
		}), want: []RuleChange{{
			Subject: SubjectValLossRule, Kind: ChangeChanged, Key: "Type 1/POY/BC", RuleKey: kPOYBC,
			Before: []ChangeValue{{Basis: BasisSPPTY, Value: d("0.5")}},
			After:  []ChangeValue{{Basis: BasisSPPTY, Value: d("0.500001")}},
		}}},
		{name: "rule basis changed", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			r[0].Basis = BasisCost
			return r, p, g
		}), want: []RuleChange{{
			Subject: SubjectValLossRule, Kind: ChangeChanged, Key: "Type 1/POY/BC", RuleKey: kPOYBC,
			Before: []ChangeValue{{Basis: BasisSPPTY, Value: d("0.5")}},
			After:  []ChangeValue{{Basis: BasisCost, Value: d("0.5")}},
		}}},
		{name: "duplicate makes key ambiguous", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return append(r, Rule{kPOYBC, BasisCost, d("0.2")}), p, g
		}), want: []RuleChange{{
			Subject: SubjectValLossRule, Kind: ChangeChanged, Key: "Type 1/POY/BC", RuleKey: kPOYBC,
			Before: []ChangeValue{{Basis: BasisSPPTY, Value: d("0.5")}},
			After:  []ChangeValue{{Basis: BasisCost, Value: d("0.2")}, {Basis: BasisSPPTY, Value: d("0.5")}},
		}}},
		{name: "price changed", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			p[2].Price = d("1.35")
			return r, p, g
		}), want: []RuleChange{{
			Subject: SubjectSellPrice, Kind: ChangeChanged, Key: "SPPTY", Basis: BasisSPPTY,
			Before: []ChangeValue{{Basis: BasisSPPTY, Value: d("1.3")}},
			After:  []ChangeValue{{Basis: BasisSPPTY, Value: d("1.35")}},
		}}},
		{name: "price removed", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return r, p[1:], g
		}), want: []RuleChange{{
			Subject: SubjectSellPrice, Kind: ChangeRemoved, Key: "SPBSD", Basis: BasisSPBSD,
			Before: []ChangeValue{{Basis: BasisSPBSD, Value: d("1.4")}},
		}}},
		{name: "grade added, changed and removed", prev: base, cur: mutate(func(r []Rule, p []Price, g []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			g[2].Group = GradeGroupBB                              // B1 BC -> BB
			g = append(g[:3], GradeAssignment{"C1", GradeGroupAE}) // drop JLT, add C1
			return r, p, g
		}), want: []RuleChange{
			{Subject: SubjectGradeGroup, Kind: ChangeChanged, Key: "B1", GradeCode: "B1",
				Before: []ChangeValue{{Group: GradeGroupBC}}, After: []ChangeValue{{Group: GradeGroupBB}}},
			{Subject: SubjectGradeGroup, Kind: ChangeAdded, Key: "C1", GradeCode: "C1",
				After: []ChangeValue{{Group: GradeGroupAE}}},
			{Subject: SubjectGradeGroup, Kind: ChangeRemoved, Key: "JLT", GradeCode: "JLT",
				Before: []ChangeValue{{Group: GradeGroupJLT}}},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := Diff(tc.prev, tc.cur)
			assertChanges(t, got, tc.want)
		})
	}
}

func assertChanges(t *testing.T, got, want []RuleChange) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %d changes %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		g, w := got[i], want[i]
		if g.Subject != w.Subject || g.Kind != w.Kind || g.Key != w.Key || g.RuleKey != w.RuleKey ||
			g.Basis != w.Basis || g.GradeCode != w.GradeCode ||
			!equalValuesNil(g.Before, w.Before) || !equalValuesNil(g.After, w.After) {
			t.Errorf("change %d:\n got  %s (%+v)\n want %s (%+v)", i, g, g, w, w)
		}
	}
}

func equalValuesNil(a, b []ChangeValue) bool {
	if (a == nil) != (b == nil) {
		return false
	}
	return equalValues(a, b)
}

func TestDiffOrderAndMixedSubjects(t *testing.T) {
	r0, p0, g0 := seedLikeInputs()
	prev := mustRuleSet(t, r0, p0, g0)
	r, p, g := seedLikeInputs()
	r[4].ValLoss = d("0.45") // Type 12/PTY/BB
	r[0].ValLoss = d("0.55") // Type 1/POY/BC
	p[1].Price = d("1.55")   // SPITY
	g[0].Group = GradeGroupAE
	cur := mustRuleSet(t, r, p, g)

	got := Diff(prev, cur)
	keys := make([]string, len(got))
	for i, c := range got {
		keys[i] = string(c.Subject) + ":" + c.Key
	}
	want := []string{
		"VALLOSS_RULE:Type 1/POY/BC", "VALLOSS_RULE:Type 12/PTY/BB",
		"SELL_PRICE:SPITY", "GRADE_GROUP:A9/A",
	}
	if !reflect.DeepEqual(keys, want) {
		t.Fatalf("order = %v, want %v", keys, want)
	}

	// Input order never changes the result.
	for i := 0; i < 20; i++ {
		rng := rand.New(rand.NewSource(int64(i)))
		rr, pp, gg := seedLikeInputs()
		rr[4].ValLoss, rr[0].ValLoss, pp[1].Price, gg[0].Group = d("0.45"), d("0.55"), d("1.55"), GradeGroupAE
		rng.Shuffle(len(rr), func(a, b int) { rr[a], rr[b] = rr[b], rr[a] })
		rng.Shuffle(len(pp), func(a, b int) { pp[a], pp[b] = pp[b], pp[a] })
		rng.Shuffle(len(gg), func(a, b int) { gg[a], gg[b] = gg[b], gg[a] })
		assertChanges(t, Diff(prev, mustRuleSet(t, rr, pp, gg)), got)
	}

	// Reverse diff swaps kinds' sides.
	rev := Diff(cur, prev)
	if len(rev) != len(got) {
		t.Fatalf("reverse len %d, want %d", len(rev), len(got))
	}
	for i := range rev {
		if !equalValues(rev[i].Before, got[i].After) || !equalValues(rev[i].After, got[i].Before) {
			t.Errorf("reverse %d not mirrored: %s vs %s", i, rev[i], got[i])
		}
	}
}

func TestDiffNilSets(t *testing.T) {
	r0, p0, g0 := seedLikeInputs()
	rs := mustRuleSet(t, r0, p0, g0)
	total := rs.RuleCount() + rs.PriceCount() + rs.GradeCount()

	added := Diff(nil, rs)
	if len(added) != total {
		t.Fatalf("nil prev: %d changes, want %d", len(added), total)
	}
	for _, c := range added {
		if c.Kind != ChangeAdded || c.Before != nil || len(c.After) != 1 {
			t.Errorf("nil prev: %s", c)
		}
	}
	removed := Diff(rs, nil)
	if len(removed) != total {
		t.Fatalf("nil cur: %d changes, want %d", len(removed), total)
	}
	for _, c := range removed {
		if c.Kind != ChangeRemoved || c.After != nil {
			t.Errorf("nil cur: %s", c)
		}
	}
	if got := Diff(nil, nil); got != nil {
		t.Errorf("nil/nil = %v", got)
	}
}

func TestDiffSnapshots(t *testing.T) {
	r0, p0, g0 := seedLikeInputs()
	prev := mustRuleSet(t, r0, p0, g0)
	r, p, g := seedLikeInputs()
	r[0].ValLoss = d("0.55")
	cur := mustRuleSet(t, r, p, g)

	got, err := DiffSnapshots(prev.Canonical(), cur.Canonical())
	if err != nil {
		t.Fatal(err)
	}
	assertChanges(t, got, Diff(prev, cur))

	same, err := DiffSnapshots(prev.Canonical(), prev.Canonical())
	if err != nil || same != nil {
		t.Fatalf("same snapshot: %v, %v", same, err)
	}
	first, err := DiffSnapshots(nil, cur.Canonical())
	if err != nil || len(first) != cur.RuleCount()+cur.PriceCount()+cur.GradeCount() {
		t.Fatalf("first-seen: %d changes, %v", len(first), err)
	}

	if _, err := DiffSnapshots([]byte("{bad"), cur.Canonical()); !errors.Is(err, ErrInvalidSnapshot) {
		t.Errorf("bad prev: %v", err)
	}
	if _, err := DiffSnapshots(prev.Canonical(), []byte(`{"version":2}`)); !errors.Is(err, ErrInvalidSnapshot) {
		t.Errorf("bad cur: %v", err)
	}
}

func TestRuleChangeString(t *testing.T) {
	c := RuleChange{
		Subject: SubjectValLossRule, Kind: ChangeChanged, Key: "Type 1/POY/BC",
		Before: []ChangeValue{{Basis: BasisSPPTY, Value: d("0.5")}},
		After:  []ChangeValue{{Basis: BasisCost, Value: d("0.2")}, {Basis: BasisSPPTY, Value: d("0.5")}},
	}
	want := "VALLOSS_RULE CHANGED Type 1/POY/BC: SPPTY@0.500000 -> COST@0.200000,SPPTY@0.500000"
	if got := c.String(); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
	g := RuleChange{Subject: SubjectGradeGroup, Kind: ChangeAdded, Key: "C1", After: []ChangeValue{{Group: GradeGroupAE}}}
	if got, want := g.String(), "GRADE_GROUP ADDED C1: - -> AE"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}
