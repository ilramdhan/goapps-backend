package oracle

// sql_safety_test.go implements the static SQL safety scans §S-T2 and §S-T3
// (design Part 1 §S, plan P0-T13, AC-03). It parses Go sources with go/ast and
// inspects every string literal, so no Oracle-destructive SQL can be added to
// shipped code without this test failing.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// allowlistFile is the only file permitted to hold Oracle write statements.
const allowlistFile = "internal/infrastructure/oracle/erp_writer_allowlist.go"

var (
	// §S-T2 verbs: any of these next to an Oracle object reference fails.
	safetyVerbRe = regexp.MustCompile(`(?i)\b(DELETE|TRUNCATE|DROP|ALTER|CREATE|GRANT|REVOKE|MERGE|RENAME)\b|(?i)\bUPDATE\s+\w`)
	// Oracle object references (MGTDAT schema and the ERP interface objects).
	safetyOracleRe = regexp.MustCompile(`(?i)MGTDAT\.|OT_ADJ_|CST_GOAPPS_|PKG_GOAPPS_ADJ|OM_ITEM|OT_STD_COST`)
	// Legacy ADJ / std-cost tables combined with any DML verb.
	safetyLegacyTableRe = regexp.MustCompile(`(?i)OT_ADJ_ITEM|OT_ADJ_HEAD|OT_STD_COST_PRODUCTS_MGT`)
	safetyDMLRe         = regexp.MustCompile(`(?i)\b(INSERT|UPDATE|DELETE|MERGE)\b`)
	// First keyword followed by more text marks a literal as a SQL statement
	// (bare keywords such as the guard's own token table are not statements).
	safetySQLStartRe = regexp.MustCompile(`(?i)^\s*(SELECT|WITH|INSERT|UPDATE|DELETE|MERGE|BEGIN|DECLARE|CALL|EXEC|EXECUTE|ALTER|CREATE|DROP|TRUNCATE|GRANT|REVOKE|RENAME|LOCK|COMMIT|ROLLBACK)\s+\S`)
	// §S-T3 forbidden references in ERP code.
	safetyLegacyCallRe = regexp.MustCompile(`ExecuteProcedure|STD_FG_|CHP_WAC_UPD|(?i:EXECUTE\s+IMMEDIATE)|\.DB\(\)\.Exec`)
)

// serviceRoot returns the finance service directory (this package is at
// internal/infrastructure/oracle).
func serviceRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		t.Fatalf("service root not found at %s: %v", root, err)
	}
	return root
}

// goSources lists non-test .go files under the given roots (relative to the
// service root), as slash-separated relative paths. Test files are excluded:
// they hold deliberately rejected SQL as guard fixtures and never ship.
func goSources(t *testing.T, root string, dirs ...string) []string {
	t.Helper()
	var out []string
	for _, d := range dirs {
		base := filepath.Join(root, d)
		if _, err := os.Stat(base); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(base, func(path string, e fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if e.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return out
}

// stringLiterals returns every string literal in file (unquoted) with its position.
func stringLiterals(t *testing.T, root, rel string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, filepath.Join(root, rel), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", rel, err)
	}
	out := map[string]string{}
	ast.Inspect(f, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		s, err := strconv.Unquote(lit.Value)
		if err != nil {
			s = lit.Value
		}
		out[fset.Position(lit.Pos()).String()] = s
		return true
	})
	return out
}

// isErpFile reports whether rel belongs to the ERP integration code paths.
func isErpFile(rel string) bool {
	return strings.Contains(rel, "/erpintegration/") ||
		strings.HasPrefix(rel, "internal/infrastructure/oracle/erp_") ||
		strings.HasPrefix(rel, "internal/testutil/fakeoracle/") ||
		strings.HasPrefix(rel, "tools/golden-export/") ||
		rel == "internal/infrastructure/oracle/readonly_guard.go" ||
		rel == "internal/infrastructure/oracle/rowsource.go"
}

// TestSQLSafety_NoDestructiveOracleSQL is §S-T2 over all shipped code.
func TestSQLSafety_NoDestructiveOracleSQL(t *testing.T) {
	root := serviceRoot(t)
	files := goSources(t, root, "internal", "cmd", "tools")
	if len(files) == 0 {
		t.Fatal("no Go sources found to scan")
	}
	for _, rel := range files {
		for pos, s := range stringLiterals(t, root, rel) {
			if safetyVerbRe.MatchString(s) && safetyOracleRe.MatchString(s) {
				t.Errorf("§S-T2 %s: destructive verb against an Oracle object: %q", pos, s)
			}
			if safetyLegacyTableRe.MatchString(s) && safetyDMLRe.MatchString(s) && rel != allowlistFile {
				t.Errorf("§S-T2 %s: DML against a legacy ADJ/std-cost table: %q", pos, s)
			}
		}
	}
}

// TestSQLSafety_ErpLiteralsAreReadOnly is §S-T2 for the ERP packages: every
// SQL literal outside the allowlist file must pass the read-only guard.
func TestSQLSafety_ErpLiteralsAreReadOnly(t *testing.T) {
	root := serviceRoot(t)
	for _, rel := range goSources(t, root, "internal", "cmd", "tools") {
		if !isErpFile(rel) || rel == allowlistFile || strings.HasPrefix(rel, "internal/infrastructure/postgres/") {
			continue
		}
		for pos, s := range stringLiterals(t, root, rel) {
			if !safetySQLStartRe.MatchString(s) {
				continue
			}
			if err := CheckReadOnly(s); err != nil {
				t.Errorf("§S-T2 %s: ERP SQL literal is not read-only (%v): %q", pos, err, s)
			}
		}
	}
}

// TestSQLSafety_NoLegacyProcsInErpCode is §S-T3.
func TestSQLSafety_NoLegacyProcsInErpCode(t *testing.T) {
	root := serviceRoot(t)
	scanned := 0
	for _, rel := range goSources(t, root, "internal", "cmd", "tools") {
		if !isErpFile(rel) {
			continue
		}
		scanned++
		src, err := os.ReadFile(filepath.Join(root, rel)) //nolint:gosec // paths come from WalkDir under the service root
		if err != nil {
			t.Fatal(err)
		}
		if m := safetyLegacyCallRe.Find(src); m != nil {
			t.Errorf("§S-T3 %s references forbidden legacy call %q", rel, m)
		}
	}
	if scanned == 0 {
		t.Fatal("§S-T3 scanned no ERP files")
	}
}

// TestSQLSafety_AllowlistIsOnlyWriteSource asserts the allowlist file itself
// holds no destructive verb (only INSERT and PKG_GOAPPS_ADJ calls).
func TestSQLSafety_AllowlistIsOnlyWriteSource(t *testing.T) {
	root := serviceRoot(t)
	lits := stringLiterals(t, root, allowlistFile)
	if len(lits) == 0 {
		t.Fatal("allowlist file has no literals")
	}
	for pos, s := range lits {
		if safetyVerbRe.MatchString(s) {
			t.Errorf("%s: allowlist holds a destructive verb: %q", pos, s)
		}
	}
}

// TestSQLSafety_ReconQueriesAreReadOnly pins the P5-T6 recon read-back
// statements: each must pass the read-only guard, reference only the
// GoApps interface tables or the ADJ tables, and never lock rows.
func TestSQLSafety_ReconQueriesAreReadOnly(t *testing.T) {
	for name, q := range map[string]string{
		"batch": erpReconBatchQuery, "cost": erpReconCostQuery, "adj": erpReconAdjQuery,
	} {
		if err := CheckReadOnly(q); err != nil {
			t.Errorf("recon %s query rejected by the guard: %v", name, err)
		}
		if safetyVerbRe.MatchString(q) || safetyDMLRe.MatchString(q) {
			t.Errorf("recon %s query holds a write verb: %q", name, q)
		}
		if strings.Contains(strings.ToUpper(q), "FOR UPDATE") {
			t.Errorf("recon %s query locks rows", name)
		}
	}
}
