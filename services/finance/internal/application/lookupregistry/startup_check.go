// Package lookupregistry compares the live contents of mst_lookup_master_column
// against the lookup_source_column readers compiled into the fill handler, and
// reports any divergence at service startup.
//
// R30 background. The registered columns and the Go readers are two independent
// hand-maintained lists of the same thing, and they have already drifted in both
// directions in production:
//
//	T6  registered in the DB, no reader in Go → the column is offered in the
//	    lookup_source_column dropdown, a user picks it, and the fill silently
//	    yields nothing.
//	T7  reader in Go, not registered in the DB → the reader is live but the
//	    column is invisible to the UI and unvalidatable.
//
// Two guards already exist: warnNoReader logs an unknown column per request, and
// a CI test parses the migration files. Neither looks at the table as it
// actually exists in the running database — and the Lookup Masters admin page
// writes rows into mst_lookup_master_column directly, so production can hold
// columns no migration ever declared. This check closes that gap.
//
// The reader map deliberately stays static in Go. Deriving it from the database
// would make the fill behavior depend on editable rows; only the divergence
// detector is dynamic.
package lookupregistry

import (
	"context"
	"sort"
	"time"

	"github.com/rs/zerolog/log"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

// DefaultTimeout bounds the registry query so a slow or unreachable database
// cannot hold up service startup.
const DefaultTimeout = 5 * time.Second

const (
	logPrefix     = "lookup column registry: "
	fieldMaster   = "lookup_master_code"
	fieldColumns  = "columns"
	fieldRegCount = "registered_columns"
	fieldRdrCount = "reader_columns"
	fieldT6Count  = "registered_without_reader"
	fieldT7Count  = "reader_not_registered"
)

// ColumnLister is the narrow persistence port this check needs.
// Satisfied by lookupmaster.Repository.
type ColumnLister interface {
	ListAllColumns(ctx context.Context) ([]*lookupmaster.Column, error)
}

// Report is the outcome of one comparison between the registry table and the
// compiled reader inventory. Both divergence maps are keyed by lookup master
// code with sorted column names as values.
type Report struct {
	// RegisteredTotal counts distinct rows read from mst_lookup_master_column.
	RegisteredTotal int
	// ReaderTotal counts distinct columns the fill handler can resolve.
	ReaderTotal int
	// MissingReader is direction T6: registered in the DB, no reader in Go.
	MissingReader map[string][]string
	// Unregistered is direction T7: reader in Go, not registered in the DB.
	Unregistered map[string][]string
}

// MissingReaderCount returns the number of T6 divergences.
func (r Report) MissingReaderCount() int { return countColumns(r.MissingReader) }

// UnregisteredCount returns the number of T7 divergences.
func (r Report) UnregisteredCount() int { return countColumns(r.Unregistered) }

// IsClean reports whether neither direction found a divergence.
func (r Report) IsClean() bool { return r.MissingReaderCount() == 0 && r.UnregisteredCount() == 0 }

func countColumns(m map[string][]string) int {
	n := 0
	for _, cols := range m {
		n += len(cols)
	}
	return n
}

// Diff compares registry rows against the compiled reader inventory and returns
// both divergence directions. It is pure: no I/O, no logging.
func Diff(registered []*lookupmaster.Column, readers map[string]map[string]struct{}) Report {
	regSet, total := indexRegistered(registered)
	return Report{
		RegisteredTotal: total,
		ReaderTotal:     countSet(readers),
		MissingReader:   difference(regSet, readers),
		Unregistered:    difference(readers, regSet),
	}
}

// indexRegistered folds the registry rows into a master → column set, ignoring
// nil rows and duplicate (master, column) pairs.
func indexRegistered(registered []*lookupmaster.Column) (map[string]map[string]struct{}, int) {
	out := make(map[string]map[string]struct{}, len(registered))
	total := 0
	for _, c := range registered {
		if c == nil {
			continue
		}
		if out[c.MasterCode] == nil {
			out[c.MasterCode] = map[string]struct{}{}
		}
		if _, dup := out[c.MasterCode][c.ColumnName]; dup {
			continue
		}
		out[c.MasterCode][c.ColumnName] = struct{}{}
		total++
	}
	return out, total
}

func countSet(set map[string]map[string]struct{}) int {
	n := 0
	for _, cols := range set {
		n += len(cols)
	}
	return n
}

// difference returns the entries of from that have no counterpart in against,
// grouped by master code with column names sorted for stable log output.
func difference(from, against map[string]map[string]struct{}) map[string][]string {
	out := map[string][]string{}
	for master, cols := range from {
		for col := range cols {
			if _, ok := against[master][col]; ok {
				continue
			}
			out[master] = append(out[master], col)
		}
	}
	for _, cols := range out {
		sort.Strings(cols)
	}
	return out
}

// StartupChecker runs the registry comparison once, at service start.
type StartupChecker struct {
	columns ColumnLister
	readers map[string]map[string]struct{}
	timeout time.Duration
}

// NewStartupChecker builds a checker over the registry table and the compiled
// reader inventory. readers is injected rather than imported so this application
// package stays independent of the delivery layer that owns the reader maps.
func NewStartupChecker(columns ColumnLister, readers map[string]map[string]struct{}) *StartupChecker {
	return &StartupChecker{columns: columns, readers: readers, timeout: DefaultTimeout}
}

// Run performs the comparison and logs the result. It NEVER returns an error and
// must never be made fatal: the divergences it reports already exist in
// production today, and a query failure here (missing table, pending migration,
// slow database) must not stop the service from starting.
func (c *StartupChecker) Run(ctx context.Context) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	rows, err := c.columns.ListAllColumns(ctx)
	if err != nil {
		log.Warn().Err(err).Dur("timeout", c.timeout).
			Msg(logPrefix + "could not read mst_lookup_master_column — divergence check skipped (non-fatal)")
		return
	}

	LogReport(Diff(rows, c.readers))
}

// LogReport writes the report: WARN plus per-master column lists when anything
// diverged, INFO when clean. The clean case is logged on purpose — silence would
// be indistinguishable from the check never having run.
func LogReport(rep Report) {
	t6, t7 := rep.MissingReaderCount(), rep.UnregisteredCount()
	if t6 == 0 && t7 == 0 {
		log.Info().
			Int(fieldRegCount, rep.RegisteredTotal).
			Int(fieldRdrCount, rep.ReaderTotal).
			Msg(logPrefix + "no divergence between mst_lookup_master_column and the compiled readers")
		return
	}

	log.Warn().
		Int(fieldRegCount, rep.RegisteredTotal).
		Int(fieldRdrCount, rep.ReaderTotal).
		Int(fieldT6Count, t6).
		Int(fieldT7Count, t7).
		Msg(logPrefix + "DIVERGENCE between mst_lookup_master_column and the compiled readers")

	logDirection(rep.MissingReader,
		logPrefix+"T6 registered in DB but no reader in Go — selecting these fills nothing, silently")
	logDirection(rep.Unregistered,
		logPrefix+"T7 reader in Go but not registered in DB — not offered in the lookup_source_column dropdown")
}

func logDirection(byMaster map[string][]string, msg string) {
	masters := make([]string, 0, len(byMaster))
	for master := range byMaster {
		masters = append(masters, master)
	}
	sort.Strings(masters)

	for _, master := range masters {
		log.Warn().
			Str(fieldMaster, master).
			Strs(fieldColumns, byMaster[master]).
			Int("count", len(byMaster[master])).
			Msg(msg)
	}
}
