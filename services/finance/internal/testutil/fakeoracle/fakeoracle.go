// Package fakeoracle is an in-memory Oracle row source for ERP reader tests
// (P0-T17). It implements oracle.ReadOnlyQuerier, applies the same read-only
// guard as production, and serves canned rows loaded from synthetic CSV
// fixtures. It never opens a network connection.
package fakeoracle

import (
	"context"
	"database/sql"
	"encoding/csv"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// ErrNoDataset is returned when no registered dataset matches a query.
var ErrNoDataset = errors.New("fakeoracle: no dataset registered for query")

// Dataset is a canned result set. An empty cell is returned as NULL.
type Dataset struct {
	Columns []string
	Rows    [][]string
}

// Querier is an in-memory oracle.ReadOnlyQuerier.
type Querier struct {
	mu       sync.Mutex
	datasets map[string]Dataset
	queries  []string
	failWith error
}

var _ oracle.ReadOnlyQuerier = (*Querier)(nil)

// New returns an empty fake querier.
func New() *Querier {
	return &Querier{datasets: map[string]Dataset{}}
}

// Register serves ds for every query that references table (for example
// "MGTDAT.OM_ITEM"). Matching is case-insensitive on a word boundary.
func (q *Querier) Register(table string, ds Dataset) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.datasets[strings.ToUpper(table)] = ds
}

// LoadCSV registers the CSV fixture at path (header row = columns) for table.
func (q *Querier) LoadCSV(table, path string) (err error) {
	f, err := os.Open(path) //nolint:gosec // test fixture path chosen by the test
	if err != nil {
		return fmt.Errorf("open fixture: %w", err)
	}
	defer func() {
		if cerr := f.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close fixture: %w", cerr)
		}
	}()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	records, rerr := r.ReadAll()
	if rerr != nil {
		return fmt.Errorf("read fixture: %w", rerr)
	}
	if len(records) == 0 {
		return fmt.Errorf("fixture %s has no header", path)
	}
	q.Register(table, Dataset{Columns: records[0], Rows: records[1:]})
	return nil
}

// FailWith makes every subsequent query fail with err (nil clears it).
func (q *Querier) FailWith(err error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.failWith = err
}

// Queries returns the statements received so far.
func (q *Querier) Queries() []string {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]string, len(q.queries))
	copy(out, q.queries)
	return out
}

// QueryRO implements oracle.ReadOnlyQuerier.
func (q *Querier) QueryRO(_ context.Context, query string, _ ...any) (oracle.Rows, error) {
	if err := oracle.CheckReadOnly(query); err != nil {
		return nil, err
	}
	q.mu.Lock()
	defer q.mu.Unlock()
	q.queries = append(q.queries, query)
	if q.failWith != nil {
		return nil, q.failWith
	}
	upper := strings.ToUpper(query)
	for table, ds := range q.datasets {
		if referencesTable(upper, table) {
			return &rows{ds: ds, pos: -1}, nil
		}
	}
	return nil, ErrNoDataset
}

// referencesTable reports whether query mentions table as a whole word.
func referencesTable(query, table string) bool {
	for i := 0; ; {
		j := strings.Index(query[i:], table)
		if j < 0 {
			return false
		}
		end := i + j + len(table)
		if end >= len(query) || !isIdentRune(query[end]) {
			return true
		}
		i = end
	}
}

func isIdentRune(b byte) bool {
	return b == '_' || b == '$' || b == '#' || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// rows iterates a Dataset.
type rows struct {
	ds     Dataset
	pos    int
	closed bool
}

func (r *rows) Next() bool {
	if r.closed {
		return false
	}
	r.pos++
	return r.pos < len(r.ds.Rows)
}

func (r *rows) Scan(dest ...any) error {
	if r.pos < 0 || r.pos >= len(r.ds.Rows) {
		return errors.New("fakeoracle: Scan called without a current row")
	}
	if len(dest) != len(r.ds.Columns) {
		return fmt.Errorf("fakeoracle: expected %d scan targets, got %d", len(r.ds.Columns), len(dest))
	}
	row := r.ds.Rows[r.pos]
	for i, d := range dest {
		var cell any
		if i < len(row) && row[i] != "" {
			cell = row[i]
		}
		if err := assign(d, cell); err != nil {
			return fmt.Errorf("fakeoracle: column %s: %w", r.ds.Columns[i], err)
		}
	}
	return nil
}

// assign stores cell (nil or string) into dest.
func assign(dest, cell any) error {
	switch d := dest.(type) {
	case sql.Scanner:
		return d.Scan(cell)
	case *string:
		*d = ""
		if s, ok := cell.(string); ok {
			*d = s
		}
		return nil
	case *any:
		*d = cell
		return nil
	}
	return fmt.Errorf("unsupported scan target %T", dest)
}

func (r *rows) Err() error { return nil }

func (r *rows) Close() error {
	r.closed = true
	return nil
}
