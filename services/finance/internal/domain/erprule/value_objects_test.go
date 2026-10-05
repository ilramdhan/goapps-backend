package erprule

import (
	"errors"
	"strings"
	"testing"

	"github.com/shopspring/decimal"
)

func d(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func TestParseBasis(t *testing.T) {
	for _, s := range []string{"COST", "SPPTY", "SPITY", " SPBSD "} {
		if _, err := ParseBasis(s); err != nil {
			t.Errorf("ParseBasis(%q) = %v", s, err)
		}
	}
	for _, s := range []string{"", "cost", "SP", "AX"} {
		if _, err := ParseBasis(s); !errors.Is(err, ErrInvalidBasis) {
			t.Errorf("ParseBasis(%q) err = %v, want ErrInvalidBasis", s, err)
		}
	}
	if BasisCost.String() != "COST" || BasisCost.IsSellPrice() || !BasisSPITY.IsSellPrice() {
		t.Error("basis helpers wrong")
	}
}

func TestParseSellPriceBasis(t *testing.T) {
	if b, err := ParseSellPriceBasis("SPPTY"); err != nil || b != BasisSPPTY {
		t.Fatalf("got %v %v", b, err)
	}
	for _, s := range []string{"COST", "X"} {
		if _, err := ParseSellPriceBasis(s); !errors.Is(err, ErrInvalidBasis) {
			t.Errorf("ParseSellPriceBasis(%q) err = %v", s, err)
		}
	}
}

func TestParseProdType(t *testing.T) {
	for _, s := range []string{"POY", "PTY", "ITY"} {
		p, err := ParseProdType(s)
		if err != nil || p.String() != s {
			t.Errorf("ParseProdType(%q) = %v %v", s, p, err)
		}
	}
	for _, s := range []string{"", "poy", "ACY", "MMK"} {
		if _, err := ParseProdType(s); !errors.Is(err, ErrInvalidProdType) {
			t.Errorf("ParseProdType(%q) err = %v", s, err)
		}
	}
}

func TestParseGradeGroup(t *testing.T) {
	for _, s := range []string{"NS", "AE", "BC", "BB", "JLT", "POYA", "AX"} {
		g, err := ParseGradeGroup(s)
		if err != nil || g.String() != s {
			t.Errorf("ParseGradeGroup(%q) = %v %v", s, g, err)
		}
	}
	for _, s := range []string{"", "ns", "NO_GRADE_GROUP", "CI"} {
		if _, err := ParseGradeGroup(s); !errors.Is(err, ErrInvalidGradeGroup) {
			t.Errorf("ParseGradeGroup(%q) err = %v", s, err)
		}
	}
	if _, err := ParseRuleGradeGroup("AX"); !errors.Is(err, ErrAxRule) {
		t.Errorf("AX rule group err = %v", err)
	}
	if _, err := ParseRuleGradeGroup("zz"); !errors.Is(err, ErrInvalidGradeGroup) {
		t.Errorf("bad rule group err = %v", err)
	}
	if g, err := ParseRuleGradeGroup("BC"); err != nil || g != GradeGroupBC {
		t.Errorf("BC = %v %v", g, err)
	}
}

func TestParseFgType(t *testing.T) {
	f, err := ParseFgType("  Type 13 ")
	if err != nil || f.String() != "Type 13" {
		t.Fatalf("got %q %v", f, err)
	}
	for _, s := range []string{"", "   ", strings.Repeat("x", 16)} {
		if _, err := ParseFgType(s); !errors.Is(err, ErrInvalidFgType) {
			t.Errorf("ParseFgType(%q) err = %v", s, err)
		}
	}
}

func TestNewRuleKey(t *testing.T) {
	k, err := NewRuleKey("Type 1", "POY", "BC")
	if err != nil || k.String() != "Type 1/POY/BC" {
		t.Fatalf("got %v %v", k, err)
	}
	cases := []struct {
		f, p, g string
		want    error
	}{
		{"", "POY", "BC", ErrInvalidFgType},
		{"Type 1", "ACY", "BC", ErrInvalidProdType},
		{"Type 1", "POY", "XX", ErrInvalidGradeGroup},
		{"Type 1", "POY", "AX", ErrAxRule},
	}
	for _, c := range cases {
		if _, err := NewRuleKey(c.f, c.p, c.g); !errors.Is(err, c.want) {
			t.Errorf("NewRuleKey(%q,%q,%q) err = %v, want %v", c.f, c.p, c.g, err, c.want)
		}
	}
}

func TestRuleKeyLess(t *testing.T) {
	a := RuleKey{"Type 1", ProdTypePOY, GradeGroupBC}
	cases := []struct {
		b    RuleKey
		want bool
	}{
		{RuleKey{"Type 2", ProdTypePOY, GradeGroupBC}, true},
		{RuleKey{"Type 1", ProdTypePTY, GradeGroupBC}, true},
		{RuleKey{"Type 1", ProdTypePOY, GradeGroupJLT}, true},
		{RuleKey{"Type 1", ProdTypePOY, GradeGroupBB}, false},
		{a, false},
		{RuleKey{"Type 1", ProdTypeITY, GradeGroupBC}, false},
		{RuleKey{"Type 0", ProdTypePOY, GradeGroupBC}, false},
	}
	for _, c := range cases {
		if got := a.Less(c.b); got != c.want {
			t.Errorf("%s < %s = %v, want %v", a, c.b, got, c.want)
		}
	}
	// "Type 10" sorts before "Type 2": plain string order, matching the
	// canonical snapshot ordering (design §6.6).
	if !(RuleKey{FgType: "Type 10"}).Less(RuleKey{FgType: "Type 2"}) {
		t.Error("string ordering expected")
	}
}

func TestValidateValLoss(t *testing.T) {
	for _, s := range []string{"0", "0.05", "0.51", "1", "999.999999", "0.000001"} {
		if err := ValidateValLoss(d(s)); err != nil {
			t.Errorf("ValidateValLoss(%s) = %v", s, err)
		}
	}
	for _, s := range []string{"-0.000001", "-1", "1000", "999.9999991", "0.0000005"} {
		if err := ValidateValLoss(d(s)); !errors.Is(err, ErrInvalidPercent) {
			t.Errorf("ValidateValLoss(%s) err = %v", s, err)
		}
	}
}

func TestValidatePrice(t *testing.T) {
	for _, s := range []string{"1.3", "0.000001", "99999999999999.999999"} {
		if err := ValidatePrice(d(s)); err != nil {
			t.Errorf("ValidatePrice(%s) = %v", s, err)
		}
	}
	for _, s := range []string{"0", "-1.3", "100000000000000", "1.0000001"} {
		if err := ValidatePrice(d(s)); !errors.Is(err, ErrInvalidPrice) {
			t.Errorf("ValidatePrice(%s) err = %v", s, err)
		}
	}
}

func TestValidateGradeCode(t *testing.T) {
	if c, err := ValidateGradeCode(" A9/A "); err != nil || c != "A9/A" {
		t.Fatalf("got %q %v", c, err)
	}
	for _, s := range []string{"", " ", strings.Repeat("g", 21)} {
		if _, err := ValidateGradeCode(s); !errors.Is(err, ErrInvalidGradeCode) {
			t.Errorf("ValidateGradeCode(%q) err = %v", s, err)
		}
	}
}

func TestValidateUser(t *testing.T) {
	if u, err := validateUser(" alice "); err != nil || u != "alice" {
		t.Fatalf("got %q %v", u, err)
	}
	for _, s := range []string{"", "  ", strings.Repeat("u", 65)} {
		if _, err := validateUser(s); !errors.Is(err, ErrUserRequired) {
			t.Errorf("validateUser(%q) err = %v", s, err)
		}
	}
}
