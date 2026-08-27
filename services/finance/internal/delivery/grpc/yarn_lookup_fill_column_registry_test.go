package grpc

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ─────────────────────────────────────────────────────────────────────────────
// R30 — column-registry drift guard.
//
// The Go reader maps in yarn_lookup_fill_handler.go and the rows in
// mst_lookup_master_column are two hand-maintained lists of the same thing.
// They have already drifted in BOTH directions in production:
//
//   T6  registered in DB, no reader in Go → the column shows up in the
//       lookup_source_column dropdown, a user picks it, and the fill silently
//       yields nothing. (e.g. mbs_lesture, added by migration 000414.)
//   T7  reader in Go, not registered in DB → the reader is live and used, but
//       the column is invisible to the UI and unvalidatable.
//       (e.g. mbs_denier, mbs_dozing, mbs_mgt_name, mbs_filament.)
//
// There is no FOREIGN KEY and no backend validation of lookup_source_column
// (proto only bounds max_len=50; the create/update application handlers have no
// access to the lookup-master repo at all), so nothing else catches this. This
// test is that check. b35f6c4 made the failure *visible* at runtime via
// warnNoReader; this test makes it *impossible to introduce* at compile/CI time.
//
// Source of truth for "registered": the migration files themselves, parsed at
// test time (option (a) of spec §8). Parsing SQL is the more brittle of the two
// options, but the alternative — a hand-written expectation list inside this
// test — fails open: someone adds migration 000500 registering a column, forgets
// to update the list, and the test stays green while the drift it exists to
// catch walks right past it. Every sanity control below exists to make sure the
// parser can never fail open either: an empty or unrecognised parse is a FAIL,
// never a PASS.
// ─────────────────────────────────────────────────────────────────────────────

// masterColumn identifies one row of mst_lookup_master_column.
type masterColumn struct {
	Master string
	Column string
}

func (m masterColumn) String() string { return m.Master + "." + m.Column }

// ─── Documented exceptions ──────────────────────────────────────────────────
//
// An exception is a KNOWN, JUSTIFIED asymmetry. It is not a place to park work.
// Two rules keep it from rotting into a dumping ground:
//   1. Every exception carries a written reason (the map value) — no bare names.
//   2. A STALE exception FAILS the test (see TestLookupColumnExceptionsAreLive).
//      Once a reader is added or a column is registered, the exception must be
//      deleted, or CI goes red.

// t6Exceptions: registered in mst_lookup_master_column, deliberately no Go reader.
var t6Exceptions = map[masterColumn]string{
	// The four MB_SPIN Oracle text columns from migration 000414 (lines 23-31).
	// These are the original T6 finding. Adding readers is NOT a one-line fix:
	// none of them exists on the mbspin entity, none is selected by the
	// repository, and none is present in the proto message — wiring one up means
	// plumbing through 5 layers across two repos (proto → entity → repo → handler
	// → reader). Until that plumbing lands they stay registered-but-unfillable,
	// and warnNoReader logs each attempt at runtime.
	{"MB_SPIN", "mbs_lesture"}:      "no field on mbspin.Entity/repo/proto — needs 5-layer plumbing (registered by 000414)",
	{"MB_SPIN", "mbs_d_f"}:          "no field on mbspin.Entity/repo/proto — needs 5-layer plumbing (registered by 000414)",
	{"MB_SPIN", "mbs_mb_spg_orion"}: "no field on mbspin.Entity/repo/proto — needs 5-layer plumbing (registered by 000414)",
	{"MB_SPIN", "mbs_vs_number"}:    "no field on mbspin.Entity/repo/proto — needs 5-layer plumbing (registered by 000414)",

	// Same shape on the MB_HEAD side, registered by 000416 (lines 20-28).
	{"MB_HEAD", "mbh_lesture"}:      "no field on mbhead.Entity/repo/proto — needs 5-layer plumbing (registered by 000416)",
	{"MB_HEAD", "mbh_d_f"}:          "no field on mbhead.Entity/repo/proto — needs 5-layer plumbing (registered by 000416)",
	{"MB_HEAD", "mbh_mb_spg_orion"}: "no field on mbhead.Entity/repo/proto — needs 5-layer plumbing (registered by 000416)",
	{"MB_HEAD", "mbh_vs_number"}:    "no field on mbhead.Entity/repo/proto — needs 5-layer plumbing (registered by 000416)",

	// Registered by 000425 for the costing engine, ahead of the reader work.
	// 000475 already fixed one of this batch (mc_weightage) by adding both the
	// param and the reader; these five are the remainder of the same backlog.
	{"MACHINE", "mc_poy_bobbin_weight"}:  "registered by 000425 ahead of its reader — no machine.Entity getter yet",
	{"MACHINE", "mc_tot_fxd_cst"}:        "registered by 000425 ahead of its reader — no machine.Entity getter yet",
	{"MACHINE", "mc_bobbin_per_trolly"}:  "registered by 000425 ahead of its reader — no machine.Entity getter yet",
	{"MACHINE", "mc_box_cost"}:           "registered by 000425 ahead of its reader — no machine.Entity getter yet",
	{"MACHINE", "mc_captive_per_bobbin"}: "registered by 000425 ahead of its reader — no machine.Entity getter yet",

	// Registered by 000412 (which also removed the stale bbcr_* rows) and 000421.
	// fillFromBoxBobbinCost still reads rates off the *rate* rows via ListRates
	// and knows nothing about these per-master Oracle columns.
	{"BOX_BOBBIN_COST", "bbn_reuse"}:      "registered by 000412 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "box_reuse"}:      "registered by 000412 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "box_cost"}:       "registered by 000412 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "bobin_cost"}:     "registered by 000412 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "box_cost_val"}:   "registered by 000412 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "bobin_cost_val"}: "registered by 000412 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "bbn_reuse_val"}:  "registered by 000421/000425 — fillFromBoxBobbinCost has no case for it",
	{"BOX_BOBBIN_COST", "box_reuse_val"}:  "registered by 000421/000425 — fillFromBoxBobbinCost has no case for it",

	// Registered by 000412 for the product-grade master; no productgrade.Entity
	// getter exists for either, so no reader can be written yet.
	{"PRODUCT_GRADE", "loss_pct"}: "registered by 000412 — no productgrade.Entity getter yet",
	{"PRODUCT_GRADE", "seq_no"}:   "registered by 000412 — no productgrade.Entity getter yet",
}

// t7Exceptions: a Go reader exists, deliberately not registered by any migration.
var t7Exceptions = map[masterColumn]string{
	// The original T7 finding — mbs_denier, mbs_mgt_name, mbs_filament, plus the
	// D30 planned-LDR reader mbs_ldr_prsn — was RESOLVED by migration 000477,
	// which registers those four in mst_lookup_master_column. Its MB_HEAD twin
	// mbh_ldr_prsn was RESOLVED the same way by 000478. None of those is an
	// exception any more and they must not be listed here;
	// TestLookupColumnExceptionsAreLive fails on a stale entry.
	//
	// G5 (2026-08-22): mbs_dozing was pulled back OUT of 000477 and is once again a
	// live-reader-but-unregistered column, so it is an exception again. Its units are
	// mixed across the 121 heads (65 oil-rate scale, 21 run_ldr scale) and the
	// "(legacy)" label did not convey that, so offering it in the "Source Column"
	// dropdown would mislead. The reader stays (000407 params still point at it and
	// removing it would empty live fills); only the dropdown offer is withheld,
	// until a NEW migration re-registers it once the units are reconciled.
	{"MB_SPIN", "mbs_dozing"}: "G5: reader kept live for 000407-wired params, but deliberately unregistered — mixed units (oil-rate vs run_ldr scale) would mislead in the dropdown; re-register via a new migration once units are reconciled",

	// fillFromBoxBobbinCost still switches on these two, but migration 000412
	// DELETEd them ("columns never existed") in favour of bobin_cost/box_cost.
	// The cases are retained because existing mst_parameter rows may still point
	// at them; they resolve from the rate rows, not from a master column.
	{"BOX_BOBBIN_COST", "bbcr_bob_rate_mkt"}: "de-registered by 000412; switch case kept for legacy params — resolved from rate rows, not a master column",
	{"BOX_BOBBIN_COST", "bbcr_box_rate_mkt"}: "de-registered by 000412; switch case kept for legacy params — resolved from rate rows, not a master column",
}

// ─── Go-side inventory ──────────────────────────────────────────────────────

// goReaderColumns returns every lookup_source_column the Go side can resolve,
// keyed by lookup master code. It delegates to LookupReaderColumns() in
// yarn_lookup_fill_handler.go — the same inventory the startup divergence check
// consumes — so this test can never validate against a second, stale copy.
func goReaderColumns() map[string]map[string]bool {
	out := map[string]map[string]bool{}
	for master, cols := range LookupReaderColumns() {
		out[master] = map[string]bool{}
		for col := range cols {
			out[master][col] = true
		}
	}
	return out
}

// ─── Migration-side inventory ───────────────────────────────────────────────

const migrationsDir = "../../../migrations/postgres"

var (
	reInsertStmt = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(?:public\.)?mst_lookup_master_column\b(.*?);`)
	reDeleteStmt = regexp.MustCompile(`(?is)DELETE\s+FROM\s+(?:public\.)?mst_lookup_master_column\b(.*?);`)
	reValueTuple = regexp.MustCompile(`\(\s*'([A-Z_]+)'\s*,\s*'([a-z0-9_]+)'`)
	reDelMaster  = regexp.MustCompile(`(?is)lmc_master_code\s*=\s*'([A-Z_]+)'`)
	reDelInList  = regexp.MustCompile(`(?is)lmc_column_name\s+IN\s*\(([^)]*)\)`)
	reQuoted     = regexp.MustCompile(`'([a-z0-9_]+)'`)
)

// migrationStats carries the parse counters the sanity controls assert on.
type migrationStats struct {
	FilesScanned  int
	FilesMatched  int
	InsertStmts   int
	DeleteStmts   int
	RowsInserted  int
	RowsDeleted   int
	UnparsedStmts []string
}

// parseRegisteredColumns replays every .up.sql migration in filename order and
// returns the resulting mst_lookup_master_column contents. INSERTs use
// ON CONFLICT DO NOTHING (idempotent set-add); the one DELETE removes rows.
func parseRegisteredColumns(t *testing.T) (map[masterColumn]bool, migrationStats) {
	t.Helper()

	files, err := filepath.Glob(filepath.Join(migrationsDir, "*.up.sql"))
	require.NoErrorf(t, err, "globbing %s failed", migrationsDir)
	require.NotEmptyf(t, files, "no *.up.sql migrations found under %s — the parser cannot "+
		"prove anything from an empty input; check migrationsDir is still correct relative to this package",
		migrationsDir)
	sort.Strings(files) // numeric prefixes ⇒ lexical order == apply order

	registered := map[masterColumn]bool{}
	st := migrationStats{FilesScanned: len(files)}

	for _, f := range files {
		raw, readErr := os.ReadFile(f) //nolint:gosec // test-only read of repo-local migrations
		require.NoErrorf(t, readErr, "reading migration %s", f)
		sql := string(raw)
		if !strings.Contains(strings.ToLower(sql), "mst_lookup_master_column") {
			continue
		}
		st.FilesMatched++

		for _, m := range reInsertStmt.FindAllStringSubmatch(sql, -1) {
			body := m[1]
			idx := strings.Index(strings.ToUpper(body), "VALUES")
			require.Greaterf(t, idx, -1, "INSERT into mst_lookup_master_column in %s has no VALUES clause — "+
				"the parser does not understand this statement shape and would silently register nothing", filepath.Base(f))
			tuples := reValueTuple.FindAllStringSubmatch(body[idx:], -1)
			require.NotEmptyf(t, tuples, "INSERT into mst_lookup_master_column in %s parsed to zero rows — "+
				"the statement shape changed; fix reValueTuple rather than letting the check pass empty", filepath.Base(f))
			st.InsertStmts++
			for _, tp := range tuples {
				registered[masterColumn{Master: tp[1], Column: tp[2]}] = true
				st.RowsInserted++
			}
		}

		for _, m := range reDeleteStmt.FindAllStringSubmatch(sql, -1) {
			body := m[1]
			master := reDelMaster.FindStringSubmatch(body)
			inList := reDelInList.FindStringSubmatch(body)
			if master == nil || inList == nil {
				st.UnparsedStmts = append(st.UnparsedStmts, filepath.Base(f))
				continue
			}
			st.DeleteStmts++
			for _, q := range reQuoted.FindAllStringSubmatch(inList[1], -1) {
				delete(registered, masterColumn{Master: master[1], Column: q[1]})
				st.RowsDeleted++
			}
		}
	}
	return registered, st
}

// ─── Sanity controls ────────────────────────────────────────────────────────

// TestLookupColumnParserSanity is the guard on the guard. A green two-way check
// means nothing if either side parsed to an empty or truncated set, so every
// assumption the parser makes is asserted here with a concrete floor. These
// floors are deliberately well below the current counts: they catch "the parser
// broke" without breaking every time a migration adds a column.
func TestLookupColumnParserSanity(t *testing.T) {
	registered, st := parseRegisteredColumns(t)

	assert.Emptyf(t, st.UnparsedStmts,
		"DELETE statement(s) against mst_lookup_master_column in %v use a shape this parser does not "+
			"understand, so their rows were NOT removed from the expected set. Extend reDelMaster/reDelInList.",
		st.UnparsedStmts)

	require.GreaterOrEqualf(t, st.FilesMatched, 8,
		"only %d migration file(s) mention mst_lookup_master_column (expected >= 8) — migrationsDir %q is "+
			"probably wrong or the migrations moved; refusing to validate against a near-empty set",
		st.FilesMatched, migrationsDir)
	require.GreaterOrEqualf(t, st.InsertStmts, 8,
		"parsed only %d INSERT statement(s) into mst_lookup_master_column (expected >= 8) — reInsertStmt no "+
			"longer matches the statements in the migrations", st.InsertStmts)
	require.GreaterOrEqual(t, st.DeleteStmts, 1,
		"expected at least the 000412 DELETE of the stale bbcr_* rows to be parsed; got none")
	require.GreaterOrEqualf(t, len(registered), 40,
		"only %d registered column(s) parsed (expected >= 40) — an under-populated expectation set makes the "+
			"T6 direction pass vacuously", len(registered))

	// Every lookup master routed by GetLookupFillValues must contribute columns.
	// A master that parses to zero columns would make its whole T6 direction
	// vacuous while the test still reports green.
	byMaster := map[string]int{}
	for mc := range registered {
		byMaster[mc.Master]++
	}
	for _, master := range []string{"MACHINE", "INTERMINGLING", "PRODUCT_GRADE", "MB_HEAD", "MB_SPIN", "BOX_BOBBIN_COST"} {
		assert.Positivef(t, byMaster[master],
			"lookup master %q parsed to zero registered columns — GetLookupFillValues routes it, so this is a "+
				"parse failure, not a legitimately empty master", master)
	}

	// And the Go side must not be empty either.
	readers := goReaderColumns()
	for _, master := range []string{"MACHINE", "INTERMINGLING", "PRODUCT_GRADE", "MB_HEAD", "MB_SPIN", "BOX_BOBBIN_COST"} {
		assert.NotEmptyf(t, readers[master],
			"no Go readers inventoried for lookup master %q — goReaderColumns() is out of sync with the reader "+
				"maps in yarn_lookup_fill_handler.go", master)
	}
}

// TestBoxBobbinSwitchColumnsMatchSource keeps boxBobbinCostColumns honest.
// fillFromBoxBobbinCost resolves columns with a literal switch, so unlike the
// reader maps it cannot be enumerated at runtime — the list above is a copy, and
// a copy is exactly the kind of hand-maintained duplicate R30 is about. This
// reads the handler source and asserts the copy still matches the switch.
func TestBoxBobbinSwitchColumnsMatchSource(t *testing.T) {
	raw, err := os.ReadFile("yarn_lookup_fill_handler.go")
	require.NoError(t, err, "reading yarn_lookup_fill_handler.go")

	src := string(raw)
	start := strings.Index(src, "func (h *YarnLookupFillHandler) fillFromBoxBobbinCost(")
	require.NotEqual(t, -1, start, "fillFromBoxBobbinCost not found — was it renamed? Update this test.")
	end := strings.Index(src[start:], "\nfunc ")
	require.NotEqual(t, -1, end, "could not delimit fillFromBoxBobbinCost body")
	body := src[start : start+end]

	caseRe := regexp.MustCompile(`(?m)^\s*case\s+"([a-z0-9_]+)":`)
	found := caseRe.FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, found, "no `case \"...\":` labels found in fillFromBoxBobbinCost — the switch shape "+
		"changed and this check would otherwise pass vacuously")

	inSource := make([]string, 0, len(found))
	for _, m := range found {
		inSource = append(inSource, m[1])
	}
	sort.Strings(inSource)
	copied := append([]string(nil), boxBobbinCostColumns...)
	sort.Strings(copied)

	assert.Equal(t, inSource, copied,
		"boxBobbinCostColumns is out of sync with the switch in fillFromBoxBobbinCost. Update the slice in "+
			"yarn_lookup_fill_handler.go to match the case labels, otherwise the BOX_BOBBIN_COST side of the R30 "+
			"drift check — and the startup divergence check that shares this inventory — is wrong.")
}

// ─── The two-way check ──────────────────────────────────────────────────────

// TestRegisteredColumnsHaveReaders is direction T6: every column a migration
// registers must be resolvable by the Go handler, or be a documented exception.
// Without this, a migration can add a dropdown option that silently fills
// nothing — which is exactly what 000414 did.
func TestRegisteredColumnsHaveReaders(t *testing.T) {
	registered, _ := parseRegisteredColumns(t)
	readers := goReaderColumns()

	var offenders []string
	for mc := range registered {
		if readers[mc.Master][mc.Column] {
			continue
		}
		if _, excused := t6Exceptions[mc]; excused {
			continue
		}
		offenders = append(offenders, mc.String())
	}
	sort.Strings(offenders)

	assert.Emptyf(t, offenders,
		"R30 drift, direction T6 (registered in DB, no reader in Go): %v\n"+
			"These columns are inserted into mst_lookup_master_column by a migration, so the UI offers them as a "+
			"lookup_source_column — but yarn_lookup_fill_handler.go cannot resolve them, so picking one silently "+
			"fills nothing (only warnNoReader logs it, at runtime).\n"+
			"Fix by EITHER adding a reader to the matching map in yarn_lookup_fill_handler.go, OR — if the reader "+
			"genuinely cannot be written yet — adding the column to t6Exceptions in this file WITH a written reason.",
		offenders)
}

// TestReadersAreRegisteredColumns is direction T7: every column the Go handler
// can resolve must be registered by a migration, or be a documented exception.
// Skipping this half would have missed mbs_denier/mbs_dozing/mbs_mgt_name/
// mbs_filament entirely — live readers invisible to the dropdown.
func TestReadersAreRegisteredColumns(t *testing.T) {
	registered, _ := parseRegisteredColumns(t)
	readers := goReaderColumns()

	var offenders []string
	for master, cols := range readers {
		for col := range cols {
			mc := masterColumn{Master: master, Column: col}
			if registered[mc] {
				continue
			}
			if _, excused := t7Exceptions[mc]; excused {
				continue
			}
			offenders = append(offenders, mc.String())
		}
	}
	sort.Strings(offenders)

	assert.Emptyf(t, offenders,
		"R30 drift, direction T7 (reader in Go, not registered in DB): %v\n"+
			"yarn_lookup_fill_handler.go can resolve these columns, but no migration inserts them into "+
			"mst_lookup_master_column — so they never appear in the lookup_source_column dropdown and nothing "+
			"validates a param that points at them.\n"+
			"Fix by EITHER adding an INSERT for the column in a new migration, OR — if it is deliberately "+
			"unlisted — adding it to t7Exceptions in this file WITH a written reason.",
		offenders)
}

// TestLookupColumnExceptionsAreLive stops the exception lists becoming a
// permanent dump. An exception that no longer describes reality — the reader
// landed, or the migration registered the column — must be deleted, not left
// behind quietly suppressing a future real divergence.
func TestLookupColumnExceptionsAreLive(t *testing.T) {
	registered, _ := parseRegisteredColumns(t)
	readers := goReaderColumns()

	require.NotEmpty(t, t6Exceptions, "t6Exceptions is empty — if the backlog really is cleared, delete this "+
		"guard too; an empty list here more likely means the file was gutted")
	require.NotEmpty(t, t7Exceptions, "t7Exceptions is empty — see the note on t6Exceptions")

	for mc, reason := range t6Exceptions {
		assert.NotEmptyf(t, strings.TrimSpace(reason), "t6 exception %s has no written reason", mc)
		assert.Truef(t, registered[mc],
			"stale t6 exception %s: no migration registers this column any more, so the exception excuses "+
				"nothing. Remove it from t6Exceptions.", mc)
		assert.Falsef(t, readers[mc.Master][mc.Column],
			"stale t6 exception %s: a Go reader now exists for this column, so it is no longer an exception. "+
				"Remove it from t6Exceptions.", mc)
	}

	for mc, reason := range t7Exceptions {
		assert.NotEmptyf(t, strings.TrimSpace(reason), "t7 exception %s has no written reason", mc)
		assert.Truef(t, readers[mc.Master][mc.Column],
			"stale t7 exception %s: no Go reader exists for this column, so the exception excuses nothing. "+
				"Remove it from t7Exceptions.", mc)
		assert.Falsef(t, registered[mc],
			"stale t7 exception %s: a migration now registers this column, so it is no longer an exception. "+
				"Remove it from t7Exceptions.", mc)
	}
}
