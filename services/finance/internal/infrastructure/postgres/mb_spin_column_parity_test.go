// Package postgres — column-parity enforcement between the two mst_mb_spin
// INSERT paths (P1-T3).
//
// createSpinOn (mb_spin_repository.go) is the auto-gen / plain-create path and
// is treated as the BASELINE of record: whatever column it writes, the
// duplicate/clone path in insertClone (mb_spin_duplicate.go) must also write
// (either COPIED, or deliberately NULLED/overridden — see the exception list
// below), or a future column added to one path will silently regress the
// other.
//
// This test never touches a database. It parses the literal SQL text of both
// INSERT statements straight out of the .go source files on disk (via
// runtime.Caller, so it works regardless of the working directory the test
// binary is run from) and diffs the two column lists. Deliberately Form 1 —
// no build tag, no INTEGRATION_TEST gate — so it runs on every `go test ./...`.
package postgres

import (
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

// mbSpinInsertColumnsExceptions lists mst_mb_spin columns that appear in
// createSpinOn's INSERT column list but are INTENTIONALLY allowed to be
// missing from insertClone's INSERT column list, because the duplicate path
// handles them in a genuinely different bucket rather than by omission.
//
// Verified by reading both INSERT statements directly (2026-08-31):
//   - mbs_oracle_sys_id and mbs_orion_item_code ARE present in insertClone's
//     column list too (mb_spin_duplicate.go INSERT, line ~295) — they are not
//     omitted, only ever bound to a literal NULL (D19: a clone is not yet
//     known to any ERP). So they do NOT need an exception entry: the parity
//     check below is "column name appears in both lists", which already holds
//     for them.
//
// As of this writing every column written by createSpinOn is also present in
// insertClone's column list (verified below), so this list is empty. It is
// kept as a named, documented slice — rather than removed — so a future,
// DELIBERATE divergence has one obvious place to be recorded with a reason,
// instead of someone silently loosening the assertion inline.
var mbSpinInsertColumnsExceptions = map[string]string{
	// (currently none — see comment above)
}

// TestMBSpinInsertColumnParity_CreateSpinOnVsInsertClone is the enforcement
// test: every mst_mb_spin column createSpinOn writes must also be present in
// insertClone's column list, unless explicitly excused above.
//
// This is the regression test for the bug fixed earlier today: insertClone
// was missing 10 columns (mbs_shade_code, mbs_shade_name, mbs_cross_section,
// mbs_vs_number, mbs_lusture_code, mbs_ldr_calculated_pct,
// mbs_ldr_adjustment_pct, mbs_ldr_type, mbs_ldr_is_actual, mbs_mb_costing)
// that createSpinOn already wrote. Deleting any one of those columns from
// insertClone's INSERT again must fail this test.
func TestMBSpinInsertColumnParity_CreateSpinOnVsInsertClone(t *testing.T) {
	baselineCols := extractInsertColumns(t, "mb_spin_repository.go", "createSpinOn")
	cloneCols := extractInsertColumns(t, "mb_spin_duplicate.go", "insertClone")

	cloneSet := make(map[string]bool, len(cloneCols))
	for _, c := range cloneCols {
		cloneSet[c] = true
	}

	var missing []string
	for _, c := range baselineCols {
		if cloneSet[c] {
			continue
		}
		if reason, excused := mbSpinInsertColumnsExceptions[c]; excused {
			t.Logf("column %q excused from parity: %s", c, reason)
			continue
		}
		missing = append(missing, c)
	}

	if len(missing) > 0 {
		t.Fatalf(
			"insertClone (mb_spin_duplicate.go) is missing %d column(s) that createSpinOn "+
				"(mb_spin_repository.go) writes, and none of them are in "+
				"mbSpinInsertColumnsExceptions: %v\n"+
				"baseline columns: %v\nclone columns: %v",
			len(missing), missing, baselineCols, cloneCols,
		)
	}
}

// insertColumnsRe captures the column list between `INSERT INTO mst_mb_spin (`
// and the matching `) VALUES` in either source file. Both files contain
// exactly one such statement, so a single non-greedy match per file is safe.
var insertColumnsRe = regexp.MustCompile(`(?s)INSERT INTO mst_mb_spin \(\s*(.*?)\s*\)\s*VALUES`)

// extractInsertColumns reads fileName from this test file's own directory
// (resolved via runtime.Caller so the test does not depend on the invoking
// working directory), locates the single `INSERT INTO mst_mb_spin (...)`
// statement, and returns its column names in source order.
//
// funcNameForError is only used to make a parse failure point at the right
// function in the assertion message.
func extractInsertColumns(t *testing.T, fileName, funcNameForError string) []string {
	t.Helper()

	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller(0) failed to resolve this test file's path")
	}
	srcPath := filepath.Join(filepath.Dir(thisFile), fileName)

	src, err := os.ReadFile(srcPath) //nolint:gosec // fixed, repo-local source file, not user input
	if err != nil {
		t.Fatalf("reading %s (for %s): %v", srcPath, funcNameForError, err)
	}

	match := insertColumnsRe.FindSubmatch(src)
	if match == nil {
		t.Fatalf("no `INSERT INTO mst_mb_spin (...)  VALUES` statement found in %s (expected inside %s)",
			srcPath, funcNameForError)
	}

	rawCols := strings.Split(string(match[1]), ",")
	cols := make([]string, 0, len(rawCols))
	for _, c := range rawCols {
		c = strings.TrimSpace(c)
		c = strings.TrimSpace(strings.ReplaceAll(c, "\t", " "))
		c = strings.Join(strings.Fields(c), " ")
		if c == "" {
			continue
		}
		cols = append(cols, c)
	}

	if len(cols) == 0 {
		t.Fatalf("parsed zero columns out of the INSERT statement in %s (expected inside %s) — regex likely broken",
			srcPath, funcNameForError)
	}

	return cols
}
