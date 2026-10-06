package oracle

import (
	"context"
	"fmt"
	"sync"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

// FakeBatchStatus mirrors GSB_STATUS values in the fake.
type FakeBatchStatus string

// Fake batch statuses.
const (
	FakeStatusPushed   FakeBatchStatus = "PUSHED"
	FakeStatusValuated FakeBatchStatus = "VALUATED"
	FakeStatusApproved FakeBatchStatus = "APPROVED"
	FakeStatusLocked   FakeBatchStatus = "LOCKED"
	FakeStatusFailed   FakeBatchStatus = "FAILED"
)

// FakeCall records one call the fake received, keyed by allowlist key.
type FakeCall struct {
	Key     StatementKey
	BatchID int64
	ApprUID string
	Rows    int
}

// FakeWriter is an in-memory OracleWriter (writer_mode=fake). It records
// every call, keeps a per-batch status and can inject busy, timeout,
// unknown-outcome and ORA-2090x/2091x application errors. It never opens an
// Oracle connection.
type FakeWriter struct {
	mu       sync.Mutex
	calls    []FakeCall
	batches  map[int64]FakeBatchStatus
	rows     map[int64][]erpintegration.PushRow
	inject   map[StatementKey][]error
	frozen   map[string]bool
	periodOf map[int64]string
}

var _ erpintegration.OracleWriter = (*FakeWriter)(nil)

// NewFakeWriter returns an empty fake writer.
func NewFakeWriter() *FakeWriter {
	return &FakeWriter{
		batches:  map[int64]FakeBatchStatus{},
		rows:     map[int64][]erpintegration.PushRow{},
		inject:   map[StatementKey][]error{},
		frozen:   map[string]bool{},
		periodOf: map[int64]string{},
	}
}

// Inject queues errs for key; each call to that key consumes one error.
func (f *FakeWriter) Inject(key StatementKey, errs ...error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.inject[key] = append(f.inject[key], errs...)
}

// InjectBusy queues n ORA-00054 failures for key.
func (f *FakeWriter) InjectBusy(key StatementKey, n int) {
	for i := 0; i < n; i++ {
		f.Inject(key, fmt.Errorf("ORA-00054: resource busy: %w", erpintegration.ErrOracleBusy))
	}
}

// InjectOraCode queues one Oracle application error with code for key.
func (f *FakeWriter) InjectOraCode(key StatementKey, code int) {
	f.Inject(key, &erpintegration.OracleAppError{Code: code, Message: "injected by fake writer"})
}

// FreezePeriod makes VALUATE/APPROVE fail with ORA-20901 for period.
func (f *FakeWriter) FreezePeriod(period string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.frozen[period] = true
}

// Calls returns a copy of the recorded calls.
func (f *FakeWriter) Calls() []FakeCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]FakeCall, len(f.calls))
	copy(out, f.calls)
	return out
}

// Status returns the fake status of batchID.
func (f *FakeWriter) Status(batchID int64) (FakeBatchStatus, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.batches[batchID]
	return s, ok
}

// RowCount returns the number of cost rows stored for batchID.
func (f *FakeWriter) RowCount(batchID int64) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.rows[batchID])
}

// begin records the call and returns an injected error, if any. Caller holds mu.
func (f *FakeWriter) begin(ctx context.Context, c FakeCall) error {
	f.calls = append(f.calls, c)
	if err := ctx.Err(); err != nil {
		return fmt.Errorf("%w: %w", erpintegration.ErrOracleTimeout, err)
	}
	q := f.inject[c.Key]
	if len(q) == 0 {
		return nil
	}
	err := q[0]
	f.inject[c.Key] = q[1:]
	return err
}

// InsertBatch implements erpintegration.OracleWriter (W1, one tx).
func (f *FakeWriter) InsertBatch(ctx context.Context, b erpintegration.PushBatch, rows []erpintegration.PushRow) (erpintegration.WriteResult, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, FakeCall{Key: KeyW1InsertBatch, BatchID: b.BatchID, Rows: len(rows)}); err != nil {
		return erpintegration.WriteResult{}, err
	}
	if _, exists := f.batches[b.BatchID]; exists {
		return erpintegration.WriteResult{}, &erpintegration.OracleAppError{Code: 1, Message: "unique constraint (PK_GSB) violated"}
	}
	stored := make([]erpintegration.PushRow, len(rows))
	copy(stored, rows)
	f.batches[b.BatchID] = FakeStatusPushed
	f.rows[b.BatchID] = stored
	f.periodOf[b.BatchID] = b.Period
	return erpintegration.WriteResult{BatchRows: 1, CostRows: int64(len(rows))}, nil
}

// ValuateAdj implements erpintegration.OracleWriter.
func (f *FakeWriter) ValuateAdj(ctx context.Context, batchID int64) (erpintegration.Summary, error) {
	return f.transition(ctx, KeyW2ValuateAdj, batchID, "", FakeStatusPushed, FakeStatusValuated)
}

// ApproveAdj implements erpintegration.OracleWriter.
func (f *FakeWriter) ApproveAdj(ctx context.Context, batchID int64, apprUID string) (erpintegration.Summary, error) {
	return f.transition(ctx, KeyW2ApproveAdj, batchID, apprUID, FakeStatusValuated, FakeStatusApproved)
}

// RestoreAdj implements erpintegration.OracleWriter.
func (f *FakeWriter) RestoreAdj(ctx context.Context, batchID int64) (erpintegration.Summary, error) {
	return f.transition(ctx, KeyW2RestoreAdj, batchID, "", FakeStatusValuated, FakeStatusFailed)
}

// LockBatch implements erpintegration.OracleWriter.
func (f *FakeWriter) LockBatch(ctx context.Context, batchID int64) (erpintegration.Summary, error) {
	return f.transition(ctx, KeyW2LockBatch, batchID, "", FakeStatusApproved, FakeStatusLocked)
}

// transition applies a status change guarded by the expected source status.
func (f *FakeWriter) transition(ctx context.Context, key StatementKey, batchID int64, apprUID string, from, to FakeBatchStatus) (erpintegration.Summary, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err := f.begin(ctx, FakeCall{Key: key, BatchID: batchID, ApprUID: apprUID}); err != nil {
		return erpintegration.Summary{}, err
	}
	cur, ok := f.batches[batchID]
	if !ok {
		return erpintegration.Summary{}, &erpintegration.OracleAppError{Code: erpintegration.OraCode20902, Message: "batch not found"}
	}
	if f.frozen[f.periodOf[batchID]] && (key == KeyW2ValuateAdj || key == KeyW2ApproveAdj) {
		return erpintegration.Summary{}, &erpintegration.OracleAppError{Code: erpintegration.OraPeriodFrozen, Message: "period frozen"}
	}
	if cur == to {
		return erpintegration.Summary{Text: fmt.Sprintf("ALREADY_DONE batch=%d status=%s", batchID, cur)}, nil
	}
	if cur != from {
		return erpintegration.Summary{}, &erpintegration.OracleAppError{Code: erpintegration.OraCode20903, Message: fmt.Sprintf("batch status %s, expected %s", cur, from)}
	}
	f.batches[batchID] = to
	return erpintegration.Summary{Text: fmt.Sprintf("OK batch=%d rows=%d status=%s", batchID, len(f.rows[batchID]), to)}, nil
}
