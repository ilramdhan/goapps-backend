// Command golden-export writes the legacy standard-cost golden fixture used by
// the P4-T3 golden test (plan P4-T2, design Part 2 §6.5, AC-02).
//
// RUNBOOK (USER ONLY; sub-agents and CI must NEVER run this against Oracle):
//
//  1. Use ONLY the existing READ-ONLY ALTHARADEV (DEV) user, the same one the
//     current ETLs use. Never a write-capable (MGTDAT, GOAPPS_IF), production
//     or staging account. The tool refuses any user not in
//     GOLDEN_RO_USERS and double-checks the session user with SELECT USER.
//
//  2. From goapps-backend/services/finance run:
//
//     ORACLE_RO_DSN='oracle://<ro_user>:<password>@<dev_host>:<port>/<service>' \
//     GOLDEN_RO_USERS='<ro_user>' \
//     go run ./tools/golden-export
//
//     Optional flags: -out <path> (default
//     internal/domain/erpintegration/testdata/golden_legacy_std_202608.csv.gz,
//     the path P4-T3 reads), -schema <owner> (default MGTDAT; "" for synonyms),
//     -timeout 10m, -expect 9602 (0 disables the count check).
//
//  3. The tool writes the fixture plus <out>.sha256 next to it and prints the
//     row count, the per-basis breakdown and the sha256. Paste that output into
//     the ledger under P4-T2. Expected rows: 9,602 (recon §3, DEV 2026-09-25).
//     A different count exits non-zero AFTER writing the file, so the user can
//     inspect it; do not commit it until the count is explained.
//
//  4. The fixture contains item/grade/shade codes and cost numbers only (no
//     personal data). Confirm with the user before it is committed.
//
// What it does: one guarded SELECT (recon §3 at row level: every non-AX row
// of OT_STD_COST_PRODUCTS_MGT joined to its AX row on item + shade, plus the
// OM_GRADE_CODE_1 grade group, the MGT_ITEM_COST_VAL_LOSS rule and the
// ITEMSELLPRIC selling price) through oracle.NewReadOnlyDB, so every statement
// passes oracle.CheckReadOnly and nothing can Exec. Numbers are read as
// TO_CHAR(..., 'TM9') text and parsed with shopspring/decimal, written with at
// least 5 dp (never float, never truncated). Legacy FLEX text is Oracle's own
// TO_CHAR(ROUND(NVL(x,0),5),'FM990D00000'). Rows are sorted in byte order and
// the gzip header carries no time or name, so re-runs on unchanged data give
// identical bytes. Nothing is written to Oracle or PostgreSQL.
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

const (
	envDSN   = "ORACLE_RO_DSN"
	envUsers = "GOLDEN_RO_USERS"
)

func main() {
	if err := run(os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, "golden-export:", err)
		os.Exit(1)
	}
}

// options are the parsed command-line settings.
type options struct {
	out     string
	schema  string
	timeout time.Duration
	expect  int
}

func parseFlags(args []string) (options, error) {
	var o options
	fs := flag.NewFlagSet("golden-export", flag.ContinueOnError)
	fs.StringVar(&o.out, "out", DefaultOutPath, "fixture output path (gzip CSV)")
	fs.StringVar(&o.schema, "schema", DefaultSchema, "owner schema of the legacy tables (\"\" = unqualified)")
	fs.DurationVar(&o.timeout, "timeout", 10*time.Minute, "overall query timeout")
	fs.IntVar(&o.expect, "expect", ExpectedRows, "expected row count (0 = do not check)")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	o.schema = strings.ToUpper(strings.TrimSpace(o.schema))
	return o, nil
}

func run(args []string, stdout io.Writer) error {
	o, err := parseFlags(args)
	if err != nil {
		return err
	}
	dsn := strings.TrimSpace(os.Getenv(envDSN))
	if dsn == "" {
		return fmt.Errorf("%s is empty; set the READ-ONLY DEV DSN", envDSN)
	}
	user, err := DSNUser(dsn)
	if err != nil {
		return err
	}
	if err := CheckUser(user, os.Getenv(envUsers)); err != nil {
		return fmt.Errorf("%w (set %s to the read-only user)", err, envUsers)
	}

	db, err := sql.Open("oracle", dsn)
	if err != nil {
		return fmt.Errorf("open oracle: %w", err)
	}
	defer func() {
		if cerr := db.Close(); cerr != nil {
			fmt.Fprintln(os.Stderr, "golden-export: close oracle:", cerr)
		}
	}()
	db.SetMaxOpenConns(1)

	ctx, cancel := context.WithTimeout(context.Background(), o.timeout)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("ping oracle: %w", err)
	}
	return export(ctx, oracle.NewReadOnlyDB(db), user, o, stdout)
}

// export is the connection-independent part of run (unit-tested with fakeoracle).
func export(ctx context.Context, q oracle.ReadOnlyQuerier, user string, o options, stdout io.Writer) (err error) {
	if err := VerifyIdentity(ctx, q, user); err != nil {
		return err
	}
	rows, sum, err := Fetch(ctx, q, o.schema)
	if err != nil {
		return err
	}
	if len(rows) == 0 {
		return fmt.Errorf("golden query returned 0 rows; nothing written")
	}
	if err := os.MkdirAll(filepath.Dir(o.out), 0o750); err != nil {
		return fmt.Errorf("mkdir output dir: %w", err)
	}
	f, err := os.Create(o.out) //nolint:gosec // output path chosen by the operator
	if err != nil {
		return fmt.Errorf("open output %s: %w", o.out, err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", o.out, cerr)
		}
	}()
	sha, err := WriteFixture(f, rows)
	if err != nil {
		return err
	}
	line := fmt.Sprintf("%s  %s\n", sha, filepath.Base(o.out))
	if err := os.WriteFile(o.out+".sha256", []byte(line), 0o600); err != nil {
		return fmt.Errorf("write checksum: %w", err)
	}
	if err := printSummary(stdout, o.out, sha, sum); err != nil {
		return err
	}
	if o.expect > 0 && sum.Rows != o.expect {
		return fmt.Errorf("row count %d != expected %d (file written for inspection; do not commit)", sum.Rows, o.expect)
	}
	return nil
}

func printSummary(w io.Writer, out, sha string, s Summary) error {
	var b strings.Builder
	fmt.Fprintf(&b, "fixture: %s\nrows: %d\nsha256: %s\n", out, s.Rows, sha)
	fmt.Fprintf(&b, "duplicate_keys: %d\nwide_numeric_cells(>5dp): %d\n", s.DuplicateKeys, s.WideCells)
	for _, k := range sortedKeys(s.PerBasis) {
		fmt.Fprintf(&b, "basis %s: %d\n", k, s.PerBasis[k])
	}
	for _, k := range sortedKeys(s.PerMatch) {
		fmt.Fprintf(&b, "recon3 %s: %d\n", k, s.PerMatch[k])
	}
	if _, err := io.WriteString(w, b.String()); err != nil {
		return fmt.Errorf("write summary: %w", err)
	}
	return nil
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
