package mbbatch

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc"
	costcalcdom "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// fakeAuditRepo records every aud_cost_history entry handed to it, and can be made to fail
// every write. Note its Write signature takes no *sql.Tx — the audit writer physically
// cannot enlist in the batch transaction.
type fakeAuditRepo struct {
	got  []*costcalcdom.AuditHistoryEntry
	fail bool
}

func (f *fakeAuditRepo) Write(_ context.Context, e *costcalcdom.AuditHistoryEntry) error {
	f.got = append(f.got, e)
	if f.fail {
		return errors.New("boom: audit insert failed")
	}
	return nil
}

var _ costcalcdom.AuditHistoryRepository = (*fakeAuditRepo)(nil)

// supersedeWriter is a ResultWriter returning a caller-chosen supersede outcome so the
// prevCostID gate can be exercised without a database.
type supersedeWriter struct {
	newCostID  int64
	prevTotal  float64
	prevCostID int64
	sawTx      bool
}

func (w *supersedeWriter) UpsertWithSupersedeTx(_ context.Context, tx *sql.Tx, _ *costcalcdom.Result) (int64, int, float64, int64, error) {
	if tx != nil {
		w.sawTx = true
	}
	return w.newCostID, 1, w.prevTotal, w.prevCostID, nil
}

func testOutput(costPerUnit float64) *costcalc.ComputeOutput {
	return &costcalc.ComputeOutput{
		CostPerUnit: costPerUnit, TotalRMCost: costPerUnit, TotalConversion: 0, TotalCost: costPerUnit,
	}
}

// 1 — a superseded cost row produces exactly one audit entry, with the MB_BATCH reason,
// the real job id, and the old/new totals taken from the supersede result.
func TestPersistResult_ReturnsAuditEntryWhenPreviousVersionExists(t *testing.T) {
	writer := &supersedeWriter{newCostID: 5001, prevTotal: 8.0, prevCostID: 4001}
	audit := &fakeAuditRepo{}
	s := &Service{resultWriter: writer, auditRepo: audit}

	var audits []*costcalcdom.AuditHistoryEntry
	err := s.persistResult(context.Background(), nil, 40895, "202608",
		costcalcdom.CalcTypeActual, 900, 4242, testOutput(10.0), &audits)
	require.NoError(t, err)
	require.Len(t, audits, 1, "a superseded row must produce a history entry")
	e := audits[0]

	require.Equal(t, int64(40895), e.ProductSysID)
	require.Equal(t, "202608", e.Period)
	require.Equal(t, costcalcdom.CalcTypeActual, e.CalcType)
	require.Equal(t, int64(4001), e.OldCostID)
	require.Equal(t, int64(5001), e.NewCostID)
	require.InDelta(t, 8.0, e.OldTotal, 1e-9)
	require.InDelta(t, 10.0, e.NewTotal, 1e-9)
	require.NotNil(t, e.VariancePct, "a non-zero previous total makes the variance computable")
	require.InDelta(t, 25.0, *e.VariancePct, 1e-9, "(10-8)/8*100")
	require.Equal(t, int64(4242), e.NewJobID, "the real cal_job id must reach the history row")
	require.Equal(t, "MB_BATCH", e.ChangeReason)
	require.Equal(t, "system:mb_batch", e.ChangedBy, "persistResult seeds the fallback actor; writeAudits replaces it")
}

// 2 — the first calculation for a period supersedes nothing, so it is not a "change" and
// must not produce a history row (mirrors costcalc's `if prevID != 0` gate).
func TestPersistResult_NoAuditEntryOnFirstCalculation(t *testing.T) {
	writer := &supersedeWriter{newCostID: 5001, prevTotal: 0, prevCostID: 0}
	s := &Service{resultWriter: writer, auditRepo: &fakeAuditRepo{}}

	var audits []*costcalcdom.AuditHistoryEntry
	err := s.persistResult(context.Background(), nil, 40895, "202608",
		costcalcdom.CalcTypeActual, 900, 4242, testOutput(10.0), &audits)
	require.NoError(t, err)
	require.Empty(t, audits, "prevCostID == 0 means first calculation, not a change")
}

// 3 — the core of the decision: an audit writer that always fails must not fail anything.
// writeAudits returns no error at all, and the cost rows are already committed by then.
func TestWriteAudits_AuditFailureIsNonBlocking(t *testing.T) {
	audit := &fakeAuditRepo{fail: true}
	s := &Service{auditRepo: audit}

	entries := []*costcalcdom.AuditHistoryEntry{
		newAuditEntry(40895, "202608", costcalcdom.CalcTypeActual, 4242, 5001, 4001, 8.0, 10.0),
		newAuditEntry(40895, "202608", costcalcdom.CalcTypeSelling, 4242, 5002, 4002, 8.0, 10.0),
	}
	require.NotPanics(t, func() {
		s.writeAudits(context.Background(), 4242, "202608", entries)
	})
	require.Len(t, audit.got, 2, "a failing entry must not stop the remaining entries")
}

// A nil auditRepo (the test/no-audit wiring) must be a no-op, not a nil dereference.
func TestWriteAudits_NilRepoIsNoOp(t *testing.T) {
	s := &Service{}
	require.NotPanics(t, func() {
		s.writeAudits(context.Background(), 4242, "202608",
			[]*costcalcdom.AuditHistoryEntry{newAuditEntry(1, "202608", costcalcdom.CalcTypeActual, 1, 2, 3, 1, 2)})
	})
}

// 4 — structural proof that the audit does NOT travel through the batch transaction:
// persistResult is the only code holding the *sql.Tx, and it writes no audit at all — it
// returns the entry instead. Writing happens later, in writeAudits, which has no tx
// parameter (and AuditHistoryRepository.Write has none either).
//
// This is a structural argument, not an end-to-end ordering proof: asserting that the
// INSERT physically lands after COMMIT would need a real database, which these unit tests
// deliberately do not touch.
func TestPersistResult_DoesNotWriteAuditThroughTransaction(t *testing.T) {
	writer := &supersedeWriter{newCostID: 5001, prevTotal: 8.0, prevCostID: 4001}
	audit := &fakeAuditRepo{}
	s := &Service{resultWriter: writer, auditRepo: audit}

	var audits []*costcalcdom.AuditHistoryEntry
	err := s.persistResult(context.Background(), nil, 40895, "202608",
		costcalcdom.CalcTypeActual, 900, 4242, testOutput(10.0), &audits)
	require.NoError(t, err)
	require.Len(t, audits, 1)
	require.Empty(t, audit.got, "persistResult, the tx holder, must never call the audit writer")

	s.writeAudits(context.Background(), 4242, "202608", audits)
	require.Len(t, audit.got, 1, "the write happens only in the post-commit stage")
}

// Variance is nil — stored as NULL — when there is no previous total to divide by, so
// "could not be computed" is not confused with a genuine "unchanged" 0.0.
func TestNewAuditEntry_ZeroPrevTotalYieldsNilVariance(t *testing.T) {
	e := newAuditEntry(1, "202608", costcalcdom.CalcTypeForecast, 7, 2, 3, 0.0, 12.5)
	require.Nil(t, e.VariancePct, "prevTotal == 0 is not computable, it is not a 0% change")
	require.InDelta(t, 12.5, e.NewTotal, 1e-9)
}

// The counterpart: a cost that genuinely did not move records a real 0.0, distinguishable
// from the NULL above.
func TestNewAuditEntry_UnchangedCostYieldsZeroVariance(t *testing.T) {
	e := newAuditEntry(1, "202608", costcalcdom.CalcTypeForecast, 7, 2, 3, 12.5, 12.5)
	require.NotNil(t, e.VariancePct, "an unchanged cost IS computable: it is 0%")
	require.InDelta(t, 0.0, *e.VariancePct, 1e-9)
}

// fakeJobReader serves one cal_job lookup outcome to resolveActor.
type fakeJobReader struct {
	job    *costcalcdom.Job
	err    error
	nCalls int
}

func (f *fakeJobReader) GetByID(_ context.Context, _ int64) (*costcalcdom.Job, error) {
	f.nCalls++
	return f.job, f.err
}

var _ JobActorReader = (*fakeJobReader)(nil)

// jobWithCreator builds a Job whose CreatedBy is the given actor, through the domain
// constructor so the test cannot drift from the real cal_job shape.
func jobWithCreator(t *testing.T, actor string) *costcalcdom.Job {
	t.Helper()
	j, err := costcalcdom.NewJob("202608", costcalcdom.CalcTypeActual, costcalcdom.ScopeMBBatch, nil, triggeredByMBBatch, actor)
	require.NoError(t, err)
	return j
}

// The real triggering user reaches every audit row, and the cal_job is read only ONCE for
// the whole batch rather than per row.
func TestWriteAudits_StampsRealActorFromCalJob(t *testing.T) {
	reader := &fakeJobReader{job: jobWithCreator(t, "ilham.ramadhan")}
	audit := &fakeAuditRepo{}
	s := &Service{auditRepo: audit, jobReader: reader}

	s.writeAudits(context.Background(), 4242, "202608", []*costcalcdom.AuditHistoryEntry{
		newAuditEntry(1, "202608", costcalcdom.CalcTypeActual, 4242, 5001, 4001, 8.0, 10.0),
		newAuditEntry(1, "202608", costcalcdom.CalcTypeSelling, 4242, 5002, 4002, 8.0, 10.0),
	})

	require.Len(t, audit.got, 2)
	for _, e := range audit.got {
		require.Equal(t, "ilham.ramadhan", e.ChangedBy)
	}
	require.Equal(t, 1, reader.nCalls, "the cal_job must be resolved once per batch, not once per row")
}

// A failing lookup must not lose the audit rows: ach_changed_by is NOT NULL, so the rows
// fall back to the system actor and are still written.
func TestWriteAudits_FallsBackToSystemActorOnLookupError(t *testing.T) {
	reader := &fakeJobReader{err: errors.New("boom: cal_job unreadable")}
	audit := &fakeAuditRepo{}
	s := &Service{auditRepo: audit, jobReader: reader}

	s.writeAudits(context.Background(), 4242, "202608", []*costcalcdom.AuditHistoryEntry{
		newAuditEntry(1, "202608", costcalcdom.CalcTypeActual, 4242, 5001, 4001, 8.0, 10.0),
	})

	require.Len(t, audit.got, 1, "a failed actor lookup must never drop the audit row")
	require.Equal(t, triggeredByMBBatch, audit.got[0].ChangedBy)
}

// An empty cj_created_by is the same story as a failed lookup: fall back, never write "".
func TestWriteAudits_FallsBackWhenCalJobHasNoCreator(t *testing.T) {
	reader := &fakeJobReader{job: jobWithCreator(t, "")}
	audit := &fakeAuditRepo{}
	s := &Service{auditRepo: audit, jobReader: reader}

	s.writeAudits(context.Background(), 4242, "202608", []*costcalcdom.AuditHistoryEntry{
		newAuditEntry(1, "202608", costcalcdom.CalcTypeActual, 4242, 5001, 4001, 8.0, 10.0),
	})

	require.Len(t, audit.got, 1)
	require.Equal(t, triggeredByMBBatch, audit.got[0].ChangedBy)
}

// A nil jobReader (the no-lookup wiring) keeps the pre-existing constant behavior.
func TestWriteAudits_NilJobReaderUsesSystemActor(t *testing.T) {
	audit := &fakeAuditRepo{}
	s := &Service{auditRepo: audit}

	s.writeAudits(context.Background(), 4242, "202608", []*costcalcdom.AuditHistoryEntry{
		newAuditEntry(1, "202608", costcalcdom.CalcTypeActual, 4242, 5001, 4001, 8.0, 10.0),
	})

	require.Len(t, audit.got, 1)
	require.Equal(t, triggeredByMBBatch, audit.got[0].ChangedBy)
}
