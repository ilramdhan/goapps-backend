package erprule

import (
	"bufio"
	"errors"
	"flag"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "rewrite testdata golden files")

const (
	seedMigration   = "../../../migrations/postgres/000548_seed_erp_rule_master.up.sql"
	seedHashGolden  = "testdata/ruleset_seed_hash.golden"
	seedCanonGolden = "testdata/ruleset_seed_canonical.golden.json"
)

var (
	reSeedRule  = regexp.MustCompile(`^\('([^']+)', '([^']+)', '([^']+)', '([^']+)', ([0-9.]+), 'migration:000548'\)`)
	reSeedPrice = regexp.MustCompile(`^\('(SP[A-Z]+)', ([0-9.]+), 'migration:000548'\)`)
	reSeedGrade = regexp.MustCompile(`^\('([^']+)', '([A-Z]+)', 'migration:000548'\)`)
)

// loadSeed parses the value rows of migration 000548 so that the golden hash
// is tied to the migration on disk, not to a hand-copied list.
func loadSeed(t *testing.T) ([]Rule, []Price, []GradeAssignment) {
	t.Helper()
	f, err := os.Open(filepath.FromSlash(seedMigration))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	var (
		rules  []Rule
		prices []Price
		grades []GradeAssignment
	)
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if m := reSeedRule.FindStringSubmatch(line); m != nil {
			key, err := NewRuleKey(m[1], m[2], m[3])
			if err != nil {
				t.Fatalf("seed rule %q: %v", line, err)
			}
			b, err := ParseBasis(m[4])
			if err != nil {
				t.Fatal(err)
			}
			rules = append(rules, Rule{Key: key, Basis: b, ValLoss: d(m[5])})
			continue
		}
		if m := reSeedPrice.FindStringSubmatch(line); m != nil {
			b, err := ParseSellPriceBasis(m[1])
			if err != nil {
				t.Fatal(err)
			}
			prices = append(prices, Price{Basis: b, Price: d(m[2])})
			continue
		}
		if m := reSeedGrade.FindStringSubmatch(line); m != nil {
			g, err := ParseGradeGroup(m[2])
			if err != nil {
				t.Fatal(err)
			}
			grades = append(grades, GradeAssignment{GradeCode: m[1], Group: g})
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	if len(rules) != 115 || len(prices) != 3 || len(grades) != 22 {
		t.Fatalf("seed parse = %d rules / %d prices / %d grades, want 115/3/22",
			len(rules), len(prices), len(grades))
	}
	return rules, prices, grades
}

func readGolden(t *testing.T, path string, got []byte) []byte {
	t.Helper()
	if *updateGolden {
		if err := os.WriteFile(path, got, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create)", err)
	}
	return want
}

func TestRuleSetSeedGolden(t *testing.T) {
	r, p, g := loadSeed(t)
	rs := mustRuleSet(t, r, p, g)

	canon := rs.Canonical()
	if want := readGolden(t, seedCanonGolden, append(append([]byte{}, canon...), '\n')); strings.TrimSuffix(string(want), "\n") != string(canon) {
		t.Errorf("canonical JSON differs from %s", seedCanonGolden)
	}
	hash := rs.Hash()
	want := strings.TrimSpace(string(readGolden(t, seedHashGolden, []byte(hash+"\n"))))
	if hash != want {
		t.Errorf("seed hash = %s, golden %s", hash, want)
	}
	if len(hash) != 64 || strings.ToLower(hash) != hash {
		t.Errorf("hash %q is not 64-char lower hex", hash)
	}
}

func TestRuleSetHashIsOrderIndependent(t *testing.T) {
	r, p, g := loadSeed(t)
	base := mustRuleSet(t, r, p, g).Hash()
	rnd := rand.New(rand.NewSource(20260929)) //nolint:gosec // deterministic shuffle for a test
	for i := 0; i < 25; i++ {
		rr := append([]Rule(nil), r...)
		pp := append([]Price(nil), p...)
		gg := append([]GradeAssignment(nil), g...)
		rnd.Shuffle(len(rr), func(a, b int) { rr[a], rr[b] = rr[b], rr[a] })
		rnd.Shuffle(len(pp), func(a, b int) { pp[a], pp[b] = pp[b], pp[a] })
		rnd.Shuffle(len(gg), func(a, b int) { gg[a], gg[b] = gg[b], gg[a] })
		if h := mustRuleSet(t, rr, pp, gg).Hash(); h != base {
			t.Fatalf("shuffle %d: hash %s != %s", i, h, base)
		}
	}
}

func TestRuleSetHashIgnoresDecimalRepresentation(t *testing.T) {
	a := mustRuleSet(t,
		[]Rule{{RuleKey{"Type 1", ProdTypePTY, GradeGroupNS}, BasisCost, d("0.05")}},
		[]Price{{BasisSPPTY, d("1.3")}}, nil)
	b := mustRuleSet(t,
		[]Rule{{RuleKey{"Type 1", ProdTypePTY, GradeGroupNS}, BasisCost, d("0.050000")}},
		[]Price{{BasisSPPTY, d("1.300")}}, nil)
	if a.Hash() != b.Hash() {
		t.Error("equal decimals with different scales hash differently")
	}
}

func TestRuleSetHashChangesOnSingleValue(t *testing.T) {
	r, p, g := loadSeed(t)
	base := mustRuleSet(t, r, p, g).Hash()

	mutate := map[string]func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment){
		"val_loss +1e-6": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			rr[7].ValLoss = rr[7].ValLoss.Add(d("0.000001"))
			return rr, pp, gg
		},
		"basis": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			if rr[0].Basis == BasisCost {
				rr[0].Basis = BasisSPPTY
			} else {
				rr[0].Basis = BasisCost
			}
			return rr, pp, gg
		},
		"fg_type": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			rr[3].Key.FgType = "Type 99"
			return rr, pp, gg
		},
		"price +1e-6": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			pp[1].Price = pp[1].Price.Add(d("0.000001"))
			return rr, pp, gg
		},
		"grade group": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			if gg[0].Group == GradeGroupBC {
				gg[0].Group = GradeGroupNS
			} else {
				gg[0].Group = GradeGroupBC
			}
			return rr, pp, gg
		},
		"drop rule": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return rr[1:], pp, gg
		},
		"drop grade": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return rr, pp, gg[1:]
		},
		"duplicate rule": func(rr []Rule, pp []Price, gg []GradeAssignment) ([]Rule, []Price, []GradeAssignment) {
			return append(rr, rr[0]), pp, gg
		},
	}
	seen := map[string]string{base: "base"}
	for name, fn := range mutate {
		t.Run(name, func(t *testing.T) {
			rr, pp, gg := fn(append([]Rule(nil), r...), append([]Price(nil), p...), append([]GradeAssignment(nil), g...))
			h := mustRuleSet(t, rr, pp, gg).Hash()
			if prev, dup := seen[h]; dup {
				t.Fatalf("hash equals %s", prev)
			}
			seen[h] = name
		})
	}
}

func TestRuleSetCanonicalShape(t *testing.T) {
	rs := mustRuleSet(t,
		[]Rule{
			{RuleKey{"Type 2", ProdTypePTY, GradeGroupNS}, BasisCost, d("0.05")},
			{RuleKey{"Type 10", ProdTypeITY, GradeGroupBB}, BasisSPITY, d("0.6")},
		},
		[]Price{{BasisSPPTY, d("1.3")}, {BasisSPITY, d("1.5")}},
		[]GradeAssignment{{"B1", GradeGroupBC}, {"A&<>", GradeGroupNS}})
	want := `{"grade_groups":[{"grade_code":"A&<>","grade_group":"NS"},{"grade_code":"B1","grade_group":"BC"}],` +
		`"prices":[{"basis":"SPITY","price":"1.500000"},{"basis":"SPPTY","price":"1.300000"}],` +
		`"rules":[{"basis":"SPITY","fg_type":"Type 10","grade_group":"BB","prod_type":"ITY","val_loss":"0.600000"},` +
		`{"basis":"COST","fg_type":"Type 2","grade_group":"NS","prod_type":"PTY","val_loss":"0.050000"}],` +
		`"version":1}`
	if got := string(rs.Canonical()); got != want {
		t.Errorf("Canonical()\n got %s\nwant %s", got, want)
	}

	empty := mustRuleSet(t, nil, nil, nil)
	if got := string(empty.Canonical()); got != `{"grade_groups":[],"prices":[],"rules":[],"version":1}` {
		t.Errorf("empty Canonical() = %s", got)
	}
}

func TestParseCanonicalRoundTrip(t *testing.T) {
	r, p, g := loadSeed(t)
	r = append(r, r[0]) // keep an ambiguous key through the round trip
	rs := mustRuleSet(t, r, p, g)
	back, err := ParseCanonical(rs.Canonical())
	if err != nil {
		t.Fatal(err)
	}
	if back.Hash() != rs.Hash() || string(back.Canonical()) != string(rs.Canonical()) {
		t.Error("round trip changed the snapshot")
	}
	if len(back.AmbiguousKeys()) != 1 {
		t.Errorf("ambiguous keys after round trip = %v", back.AmbiguousKeys())
	}
}

func TestParseCanonicalRejects(t *testing.T) {
	for name, in := range map[string]string{
		"not json":       `{`,
		"trailing":       `{"grade_groups":[],"prices":[],"rules":[],"version":1}{}`,
		"unknown field":  `{"grade_groups":[],"prices":[],"rules":[],"version":1,"x":1}`,
		"version":        `{"grade_groups":[],"prices":[],"rules":[],"version":2}`,
		"bad val_loss":   `{"grade_groups":[],"prices":[],"rules":[{"basis":"COST","fg_type":"Type 1","grade_group":"NS","prod_type":"PTY","val_loss":"x"}],"version":1}`,
		"bad price":      `{"grade_groups":[],"prices":[{"basis":"SPPTY","price":"x"}],"rules":[],"version":1}`,
		"ax rule":        `{"grade_groups":[],"prices":[],"rules":[{"basis":"COST","fg_type":"Type 1","grade_group":"AX","prod_type":"PTY","val_loss":"0.000000"}],"version":1}`,
		"7 dp val_loss":  `{"grade_groups":[],"prices":[],"rules":[{"basis":"COST","fg_type":"Type 1","grade_group":"NS","prod_type":"PTY","val_loss":"0.0000001"}],"version":1}`,
		"dup grade code": `{"grade_groups":[{"grade_code":"A","grade_group":"NS"},{"grade_code":"A","grade_group":"BC"}],"prices":[],"rules":[],"version":1}`,
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := ParseCanonical([]byte(in)); !errors.Is(err, ErrInvalidSnapshot) {
				t.Errorf("err = %v, want ErrInvalidSnapshot", err)
			}
		})
	}
}
