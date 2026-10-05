package erprule

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func mustKey(t *testing.T, f, p, g string) RuleKey {
	t.Helper()
	k, err := NewRuleKey(f, p, g)
	if err != nil {
		t.Fatal(err)
	}
	return k
}

func TestNewVallossRule(t *testing.T) {
	k := mustKey(t, "Type 1", "POY", "BC")
	r, err := NewVallossRule(k, BasisSPPTY, d("0.5"), " finance ", t0)
	if err != nil {
		t.Fatal(err)
	}
	if r.ID() != 0 || r.Key() != k || r.Basis() != BasisSPPTY || !r.ValLoss().Equal(d("0.5")) ||
		!r.IsActive() || !r.CreatedAt().Equal(t0) || r.CreatedBy() != "finance" ||
		r.UpdatedAt() != nil || r.UpdatedBy() != "" {
		t.Fatalf("unexpected rule %+v", r)
	}
	if got := r.Rule(); got.Key != k || got.Basis != BasisSPPTY || !got.ValLoss.Equal(d("0.5")) {
		t.Fatalf("Rule() = %+v", got)
	}
}

func TestNewVallossRuleInvalid(t *testing.T) {
	good := mustKey(t, "Type 1", "POY", "BC")
	cases := []struct {
		name  string
		key   RuleKey
		basis Basis
		loss  string
		user  string
		want  error
	}{
		{"ax key", RuleKey{"Type 1", ProdTypePOY, GradeGroupAX}, BasisCost, "0", "u", ErrAxRule},
		{"empty fg", RuleKey{"", ProdTypePOY, GradeGroupBC}, BasisCost, "0", "u", ErrInvalidFgType},
		{"untrimmed fg", RuleKey{" Type 1", ProdTypePOY, GradeGroupBC}, BasisCost, "0", "u", ErrInvalidFgType},
		{"bad prod", RuleKey{"Type 1", "ACY", GradeGroupBC}, BasisCost, "0", "u", ErrInvalidProdType},
		{"bad basis", good, "SP", "0", "u", ErrInvalidBasis},
		{"untrimmed basis", good, " COST", "0", "u", ErrInvalidBasis},
		{"negative", good, BasisCost, "-0.1", "u", ErrInvalidPercent},
		{"too many dp", good, BasisCost, "0.1234567", "u", ErrInvalidPercent},
		{"no user", good, BasisCost, "0", " ", ErrUserRequired},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := NewVallossRule(c.key, c.basis, d(c.loss), c.user, t0); !errors.Is(err, c.want) {
				t.Fatalf("err = %v, want %v", err, c.want)
			}
		})
	}
}

func TestVallossRuleUpdateAndDeactivate(t *testing.T) {
	k := mustKey(t, "Type 12", "PTY", "BB")
	r, err := NewVallossRule(k, BasisSPBSD, d("0.4"), "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	t1 := t0.Add(time.Hour)
	if err := r.Update(BasisCost, d("0.05"), "b", t1); err != nil {
		t.Fatal(err)
	}
	if r.Basis() != BasisCost || !r.ValLoss().Equal(d("0.05")) || r.UpdatedBy() != "b" || !r.UpdatedAt().Equal(t1) {
		t.Fatalf("after update %+v", r)
	}
	if err := r.Update("X", d("0"), "b", t1); !errors.Is(err, ErrInvalidBasis) {
		t.Errorf("bad basis err = %v", err)
	}
	if err := r.Update(BasisCost, d("1000"), "b", t1); !errors.Is(err, ErrInvalidPercent) {
		t.Errorf("bad loss err = %v", err)
	}
	if err := r.Update(BasisCost, d("1"), "", t1); !errors.Is(err, ErrUserRequired) {
		t.Errorf("no user err = %v", err)
	}
	if !r.ValLoss().Equal(d("0.05")) {
		t.Error("failed update must not change state")
	}
	if err := r.Deactivate("", t1); !errors.Is(err, ErrUserRequired) {
		t.Errorf("deactivate no user err = %v", err)
	}
	t2 := t1.Add(time.Hour)
	if err := r.Deactivate("c", t2); err != nil {
		t.Fatal(err)
	}
	if r.IsActive() || r.UpdatedBy() != "c" || !r.UpdatedAt().Equal(t2) {
		t.Fatalf("after deactivate %+v", r)
	}
	if err := r.Deactivate("c", t2); !errors.Is(err, ErrRuleInactive) {
		t.Errorf("double deactivate err = %v", err)
	}
	if err := r.Update(BasisCost, d("0"), "c", t2); !errors.Is(err, ErrRuleInactive) {
		t.Errorf("update inactive err = %v", err)
	}
}

func TestReconstructVallossRuleCopiesTime(t *testing.T) {
	u := t0
	k := RuleKey{"Type 3", ProdTypeITY, GradeGroupNS}
	r := ReconstructVallossRule(7, k, BasisCost, d("0.05"), false, t0, "m", &u, "x")
	u = u.Add(time.Hour)
	if !r.UpdatedAt().Equal(t0) {
		t.Fatal("reconstruct must copy updatedAt")
	}
	got := r.UpdatedAt()
	*got = got.Add(time.Hour)
	if !r.UpdatedAt().Equal(t0) {
		t.Fatal("accessor must return a copy")
	}
	if r.ID() != 7 || r.IsActive() || r.UpdatedBy() != "x" {
		t.Fatalf("reconstruct %+v", r)
	}
}

func TestSellPrice(t *testing.T) {
	p, err := NewSellPrice(BasisSPITY, d("1.5"), "a", t0)
	if err != nil {
		t.Fatal(err)
	}
	if p.Basis() != BasisSPITY || !p.Price().Equal(d("1.5")) || !p.IsActive() ||
		!p.CreatedAt().Equal(t0) || p.CreatedBy() != "a" || p.UpdatedAt() != nil || p.UpdatedBy() != "" {
		t.Fatalf("unexpected %+v", p)
	}
	if v := p.Value(); v.Basis != BasisSPITY || !v.Price.Equal(d("1.5")) {
		t.Fatalf("Value() = %+v", v)
	}
	cases := []struct {
		basis Basis
		price string
		user  string
		want  error
	}{
		{BasisCost, "1", "a", ErrInvalidBasis},
		{" SPITY", "1", "a", ErrInvalidBasis},
		{"X", "1", "a", ErrInvalidBasis},
		{BasisSPPTY, "0", "a", ErrInvalidPrice},
		{BasisSPPTY, "1", "", ErrUserRequired},
	}
	for _, c := range cases {
		if _, err := NewSellPrice(c.basis, d(c.price), c.user, t0); !errors.Is(err, c.want) {
			t.Errorf("NewSellPrice(%q,%s,%q) err = %v, want %v", c.basis, c.price, c.user, err, c.want)
		}
	}
}

func TestSellPriceUpdate(t *testing.T) {
	p := ReconstructSellPrice(BasisSPPTY, d("1.3"), false, t0, "m", nil, "")
	t1 := t0.Add(time.Hour)
	if err := p.UpdatePrice(d("-1"), "a", t1); !errors.Is(err, ErrInvalidPrice) {
		t.Errorf("err = %v", err)
	}
	if err := p.UpdatePrice(d("1.35"), "", t1); !errors.Is(err, ErrUserRequired) {
		t.Errorf("err = %v", err)
	}
	if err := p.UpdatePrice(d("1.35"), "a", t1); err != nil {
		t.Fatal(err)
	}
	if !p.Price().Equal(d("1.35")) || !p.IsActive() || p.UpdatedBy() != "a" || !p.UpdatedAt().Equal(t1) {
		t.Fatalf("after update %+v", p)
	}
}

func TestGradeAssignGroup(t *testing.T) {
	g := ReconstructGrade("B1", "Grade B1", true, nil)
	if g.Code() != "B1" || g.Name() != "Grade B1" || !g.IsActive() || g.HasGroup() || g.Group() != nil || g.AssignedBy() != "" {
		t.Fatalf("unexpected %+v", g)
	}
	if _, ok := g.Assignment(); ok {
		t.Fatal("unassigned grade must have no assignment")
	}
	prev, err := g.AssignGroup(GradeGroupBC, "costing")
	if err != nil || prev != nil {
		t.Fatalf("prev=%v err=%v", prev, err)
	}
	if !g.HasGroup() || *g.Group() != GradeGroupBC || g.AssignedBy() != "costing" {
		t.Fatalf("after assign %+v", g)
	}
	a, ok := g.Assignment()
	if !ok || a.GradeCode != "B1" || a.Group != GradeGroupBC {
		t.Fatalf("assignment %+v %v", a, ok)
	}
	prev, err = g.AssignGroup(GradeGroupAX, "costing")
	if err != nil || prev == nil || *prev != GradeGroupBC {
		t.Fatalf("reassign prev=%v err=%v", prev, err)
	}
	for _, bad := range []GradeGroup{"ZZ", " BC"} {
		if _, err := g.AssignGroup(bad, "c"); !errors.Is(err, ErrInvalidGradeGroup) {
			t.Errorf("AssignGroup(%q) err = %v", bad, err)
		}
	}
	if _, err := g.AssignGroup(GradeGroupNS, ""); !errors.Is(err, ErrUserRequired) {
		t.Errorf("no user err = %v", err)
	}
	if *g.Group() != GradeGroupAX {
		t.Fatal("failed assign must not change state")
	}
	if _, err := g.ClearGroup(""); !errors.Is(err, ErrUserRequired) {
		t.Errorf("clear no user err = %v", err)
	}
	prev, err = g.ClearGroup("lead")
	if err != nil || prev == nil || *prev != GradeGroupAX || g.HasGroup() || g.AssignedBy() != "lead" {
		t.Fatalf("clear prev=%v err=%v g=%+v", prev, err, g)
	}
}

func TestGradeGroupIsCopied(t *testing.T) {
	grp := GradeGroupNS
	g := ReconstructGrade("A", "", true, &grp)
	grp = GradeGroupBC
	if *g.Group() != GradeGroupNS {
		t.Fatal("reconstruct must copy group")
	}
	p := g.Group()
	*p = GradeGroupBB
	if *g.Group() != GradeGroupNS {
		t.Fatal("accessor must return a copy")
	}
}
