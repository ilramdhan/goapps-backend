package lookupregistry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
)

const (
	masterMachine = "MACHINE"
	masterMBSpin  = "MB_SPIN"
	colMCSpeed    = "mc_speed"
	colMBSDenier  = "mbs_denier"
)

// stubLister is a ColumnLister that returns canned rows or a canned error and
// records whether it was called and with what deadline.
type stubLister struct {
	rows        []*lookupmaster.Column
	err         error
	calls       int
	hadDeadline bool
}

func (s *stubLister) ListAllColumns(ctx context.Context) ([]*lookupmaster.Column, error) {
	s.calls++
	_, s.hadDeadline = ctx.Deadline()
	return s.rows, s.err
}

func col(master, name string) *lookupmaster.Column {
	return &lookupmaster.Column{MasterCode: master, ColumnName: name, DataType: "NUMBER"}
}

func readers(pairs ...[2]string) map[string]map[string]struct{} {
	out := map[string]map[string]struct{}{}
	for _, p := range pairs {
		if out[p[0]] == nil {
			out[p[0]] = map[string]struct{}{}
		}
		out[p[0]][p[1]] = struct{}{}
	}
	return out
}

func TestDiff(t *testing.T) {
	tests := []struct {
		name            string
		registered      []*lookupmaster.Column
		readers         map[string]map[string]struct{}
		wantRegTotal    int
		wantReaderTotal int
		wantMissing     map[string][]string
		wantUnreg       map[string][]string
		wantClean       bool
	}{
		{
			name:            "clean — both sides identical",
			registered:      []*lookupmaster.Column{col(masterMachine, colMCSpeed), col(masterMBSpin, colMBSDenier)},
			readers:         readers([2]string{masterMachine, colMCSpeed}, [2]string{masterMBSpin, colMBSDenier}),
			wantRegTotal:    2,
			wantReaderTotal: 2,
			wantMissing:     map[string][]string{},
			wantUnreg:       map[string][]string{},
			wantClean:       true,
		},
		{
			name: "T6 — registered in DB with no reader in Go",
			registered: []*lookupmaster.Column{
				col(masterMachine, colMCSpeed),
				col(masterMachine, "mc_ghost"),
				col(masterMBSpin, "mbs_lesture"),
			},
			readers:         readers([2]string{masterMachine, colMCSpeed}),
			wantRegTotal:    3,
			wantReaderTotal: 1,
			wantMissing:     map[string][]string{masterMachine: {"mc_ghost"}, masterMBSpin: {"mbs_lesture"}},
			wantUnreg:       map[string][]string{},
		},
		{
			name:            "T7 — reader in Go not registered in DB",
			registered:      []*lookupmaster.Column{col(masterMachine, colMCSpeed)},
			readers:         readers([2]string{masterMachine, colMCSpeed}, [2]string{masterMBSpin, colMBSDenier}),
			wantRegTotal:    1,
			wantReaderTotal: 2,
			wantMissing:     map[string][]string{},
			wantUnreg:       map[string][]string{masterMBSpin: {colMBSDenier}},
		},
		{
			name: "both directions at once, columns sorted per master",
			registered: []*lookupmaster.Column{
				col(masterMachine, "mc_zeta"),
				col(masterMachine, "mc_alpha"),
			},
			readers:         readers([2]string{masterMBSpin, colMBSDenier}),
			wantRegTotal:    2,
			wantReaderTotal: 1,
			wantMissing:     map[string][]string{masterMachine: {"mc_alpha", "mc_zeta"}},
			wantUnreg:       map[string][]string{masterMBSpin: {colMBSDenier}},
		},
		{
			name: "same column name under a different master is still a divergence",
			// Guards against comparing bare column names instead of (master, column).
			registered:      []*lookupmaster.Column{col(masterMachine, colMCSpeed)},
			readers:         readers([2]string{masterMBSpin, colMCSpeed}),
			wantRegTotal:    1,
			wantReaderTotal: 1,
			wantMissing:     map[string][]string{masterMachine: {colMCSpeed}},
			wantUnreg:       map[string][]string{masterMBSpin: {colMCSpeed}},
		},
		{
			name:            "duplicate and nil rows do not inflate the registered count",
			registered:      []*lookupmaster.Column{col(masterMachine, colMCSpeed), col(masterMachine, colMCSpeed), nil},
			readers:         readers([2]string{masterMachine, colMCSpeed}),
			wantRegTotal:    1,
			wantReaderTotal: 1,
			wantMissing:     map[string][]string{},
			wantUnreg:       map[string][]string{},
			wantClean:       true,
		},
		{
			name:            "empty registry — every reader is unregistered",
			registered:      nil,
			readers:         readers([2]string{masterMachine, colMCSpeed}),
			wantRegTotal:    0,
			wantReaderTotal: 1,
			wantMissing:     map[string][]string{},
			wantUnreg:       map[string][]string{masterMachine: {colMCSpeed}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Diff(tt.registered, tt.readers)

			assert.Equal(t, tt.wantRegTotal, got.RegisteredTotal, "RegisteredTotal")
			assert.Equal(t, tt.wantReaderTotal, got.ReaderTotal, "ReaderTotal")
			assert.Equal(t, tt.wantMissing, got.MissingReader, "T6 MissingReader")
			assert.Equal(t, tt.wantUnreg, got.Unregistered, "T7 Unregistered")
			assert.Equal(t, tt.wantClean, got.IsClean(), "IsClean")
			assert.Equal(t, countColumns(tt.wantMissing), got.MissingReaderCount(), "MissingReaderCount")
			assert.Equal(t, countColumns(tt.wantUnreg), got.UnregisteredCount(), "UnregisteredCount")
		})
	}
}

// TestStartupCheckerRunIsNonFatal is the most important guarantee in this file:
// whatever the registry query does, Run must return normally. Making this check
// fatal would take production down on deploy, because the divergences it reports
// already exist there today.
func TestStartupCheckerRunIsNonFatal(t *testing.T) {
	tests := []struct {
		name string
		stub *stubLister
	}{
		{
			name: "query error — table missing or migration not yet applied",
			stub: &stubLister{err: errors.New(`pq: relation "mst_lookup_master_column" does not exist`)},
		},
		{
			name: "context deadline exceeded — slow database",
			stub: &stubLister{err: context.DeadlineExceeded},
		},
		{
			name: "clean result",
			stub: &stubLister{rows: []*lookupmaster.Column{col(masterMachine, colMCSpeed)}},
		},
		{
			name: "divergent result in both directions",
			stub: &stubLister{rows: []*lookupmaster.Column{col(masterMachine, "mc_ghost")}},
		},
		{
			name: "empty registry",
			stub: &stubLister{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewStartupChecker(tt.stub, readers([2]string{masterMachine, colMCSpeed}))

			assert.NotPanics(t, func() { c.Run(context.Background()) },
				"Run must never panic — startup depends on it")

			assert.Equal(t, 1, tt.stub.calls, "registry should be queried exactly once")
			assert.True(t, tt.stub.hadDeadline,
				"Run must impose its own deadline so a slow database cannot stall startup")
		})
	}
}

// TestStartupCheckerRunHonoursCancelledParent proves Run still returns promptly
// when the parent context is already dead.
func TestStartupCheckerRunHonoursCancelledParent(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	stub := &stubLister{err: context.Canceled}
	c := NewStartupChecker(stub, readers([2]string{masterMachine, colMCSpeed}))

	done := make(chan struct{})
	go func() {
		c.Run(ctx)
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		require.Fail(t, "Run did not return with an already-cancelled parent context")
	}
}

func TestDefaultTimeoutIsShort(t *testing.T) {
	assert.Positive(t, DefaultTimeout, "a zero timeout would abort the check immediately")
	assert.LessOrEqual(t, DefaultTimeout, 10*time.Second,
		"the startup check must not be able to hold up service start for long")
}

func TestLogReportDoesNotPanic(t *testing.T) {
	assert.NotPanics(t, func() {
		LogReport(Diff(nil, nil))
		LogReport(Diff([]*lookupmaster.Column{col(masterMachine, "mc_ghost")},
			readers([2]string{masterMBSpin, colMBSDenier})))
	})
}
