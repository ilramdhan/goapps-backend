package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// --- fakes -----------------------------------------------------------------

// validateFake wraps memBatches with a ValidateStore-capable runner.
type validateFake struct {
	*memBatches
	std      map[int64][]domain.StdRow
	locked   bool
	baseline *domain.ValidationBaseline
	relabel  bool
	labels   map[int64]domain.CostLabel
	noShade  bool
	updates  []domain.StdValidationUpdate
	readErr  error
}

func newValidateFake() *validateFake {
	return &validateFake{memBatches: newMemBatches(), std: map[int64][]domain.StdRow{}, locked: true, labels: map[int64]domain.CostLabel{}}
}

func (f *validateFake) RunLocked(ctx context.Context, batchID int64, fn func(context.Context, domain.BatchStore) error) error {
	return fn(ctx, &validateMemStore{memStore: memStore{m: f.memBatches, id: batchID}, f: f})
}

type validateMemStore struct {
	memStore
	f *validateFake
}

func (s *validateMemStore) ListCoverage(context.Context) ([]domain.CoverageLine, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	return append([]domain.CoverageLine(nil), s.m.coverage[s.id]...), nil
}

func (s *validateMemStore) IsPeriodLocked(context.Context, string, string) (bool, error) {
	return s.f.locked, nil
}

func (s *validateMemStore) ListStdRowsForValidation(context.Context) ([]domain.StdRow, error) {
	if s.f.readErr != nil {
		return nil, s.f.readErr
	}
	return append([]domain.StdRow(nil), s.f.std[s.id]...), nil
}

func (s *validateMemStore) UpdateStdValidation(_ context.Context, us []domain.StdValidationUpdate) (int64, error) {
	s.f.updates = append([]domain.StdValidationUpdate(nil), us...)
	return int64(len(us)), nil
}

func (s *validateMemStore) ReplicaCodes(_ context.Context, items, shades, grades []string) (domain.ReplicaCodes, error) {
	rc := domain.ReplicaCodes{Items: set(items...), Shades: set(shades...), Grades: set(grades...)}
	if s.f.noShade {
		rc.Shades = map[string]struct{}{}
	}
	return rc, nil
}

func (s *validateMemStore) SourceCostLabels(_ context.Context, ids []int64) (map[int64]domain.CostLabel, error) {
	out := map[int64]domain.CostLabel{}
	for _, id := range ids {
		if l, ok := s.f.labels[id]; ok {
			out[id] = l
		}
	}
	return out, nil
}

func (s *validateMemStore) CurrencyRelabelApplied(context.Context, string) (bool, error) {
	return s.f.relabel, nil
}

func (s *validateMemStore) FindValidationBaseline(context.Context, string) (*domain.ValidationBaseline, error) {
	return s.f.baseline, nil
}

func set(vs ...string) map[string]struct{} {
	m := map[string]struct{}{}
	for _, v := range vs {
		m[v] = struct{}{}
	}
	return m
}

// --- fixtures --------------------------------------------------------------

var (
	vRuleHash1 = strings.Repeat("a", 64)
	vDeriveAt  = testNow.Add(-30 * time.Minute)
	vKeyAX     = domain.ErpKey{ItemCode: "POY100", GradeCode: "AX", ShadeCode: "NL"}
	vKeyB      = domain.ErpKey{ItemCode: "POY100", GradeCode: "B", ShadeCode: "NL"}
	vKeyMB     = domain.ErpKey{ItemCode: "CMB200", GradeCode: "A", ShadeCode: "RD"}
)

func nd(s string) decimal.NullDecimal { return decimal.NewNullDecimal(dec(s)) }

func vRow(k domain.ErpKey, src domain.StdSource, std string, costID int64) domain.StdRow {
	id, ver := costID, int32(1)
	return domain.StdRow{
		Key: k, Kind: domain.ItemKindForCode(k.ItemCode), Source: src, Status: domain.DeriveOK,
		Basis: erprule.BasisCost, AxCostSysID: &id, AxCostVersion: &ver,
		ChpConKg: nd("1"), ChpCost: nd("1.10000"), AxConvCost: nd("0.50000"),
		ConvCost: nd("0.50000"), StdCost: nd(std), AxCost: nd("1.60000"), ProdValLoss: nd("0.10000"),
	}
}

// derivedBatch stores a clean DERIVED batch (1 yarn combo AX + B, 1 MB) and
// a baseline within 20%.
func derivedBatch(f *validateFake, mode domain.BatchMode) int64 {
	loaded := testNow
	sum, _ := json.Marshal(map[string]any{StepDerive: map[string]any{"run_at": vDeriveAt}})
	id := f.put(domain.BatchState{
		Period: tPeriod, Mode: mode, Status: domain.StatusDerived, DemandLoadedAt: &loaded,
		RuleHash: vRuleHash1, Summary: sum, CreatedAt: testNow, UpdatedAt: testNow,
	})
	pid1, pid2, c1, c2 := int64(1), int64(2), int64(11), int64(22)
	f.coverage[id] = []domain.CoverageLine{
		{BatchID: id, Kind: domain.ItemKindYarn, ItemCode: "POY100", ShadeCode: "NL", Status: domain.CoverageOK, ProductSysID: &pid1, CostID: &c1},
		{BatchID: id, Kind: domain.ItemKindMB, ItemCode: "CMB200", ShadeCode: "RD", Status: domain.CoverageOK, ProductSysID: &pid2, CostID: &c2},
	}
	f.demand[id] = []domain.DemandLine{
		{BatchID: id, TxnCode: "INVADJ", Kind: domain.ItemKindYarn, ItemCode: "POY100", GradeCode: "AX", ShadeCode: "NL"},
		{BatchID: id, TxnCode: "INVADJ", Kind: domain.ItemKindYarn, ItemCode: "POY100", GradeCode: "B", ShadeCode: "NL"},
		{BatchID: id, TxnCode: "MBINVADJ", Kind: domain.ItemKindMB, ItemCode: "CMB200", GradeCode: "A", ShadeCode: "RD"},
	}
	f.std[id] = []domain.StdRow{
		vRow(vKeyMB, domain.SourceMB, "3.00000", 22),
		vRow(vKeyAX, domain.SourceAX, "1.60000", 11),
		vRow(vKeyB, domain.SourceDerived, "1.50000", 11),
	}
	f.labels[11] = domain.CostLabel{Currency: "USD", CostPerUnit: dec("1.6")}
	f.labels[22] = domain.CostLabel{Currency: "USD", CostPerUnit: dec("1.6")}
	f.baseline = &domain.ValidationBaseline{BatchID: 900, Period: "202606", Std: map[domain.ErpKey]decimal.Decimal{
		vKeyAX: dec("1.55"), vKeyB: dec("1.45"), vKeyMB: dec("3.1"),
	}}
	return id
}

func validateAt(f *validateFake, prober domain.ErpAdjHeadProber) *ValidateStep {
	return NewValidateStep(f, prober, 24*time.Hour).WithClock(func() time.Time { return testNow.Add(time.Hour) })
}

func ackAt(f *validateFake, audit AuditSink) *AckWarningsHandler {
	return NewAckWarningsHandler(f, audit).WithClock(func() time.Time { return testNow.Add(2 * time.Hour) })
}

// --- validate step ---------------------------------------------------------

func TestValidateStep_CleanBatchIsValidated(t *testing.T) {
	ctx := context.Background()
	f := newValidateFake()
	id := derivedBatch(f, domain.ModeLive)

	sum, err := validateAt(f, &fakeProber{}).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.Zero(t, sum.Errors, "%v %v", sum.Findings, sum.Issues)
	assert.Zero(t, sum.Warnings, "%v", sum.Issues)
	assert.False(t, sum.NeedsAck)
	assert.True(t, sum.Validated)
	assert.Equal(t, string(domain.StatusValidated), sum.Status)
	assert.Equal(t, vRuleHash1, sum.RuleHash)
	require.NotNil(t, sum.DeriveRunAt)
	assert.True(t, sum.DeriveRunAt.Equal(vDeriveAt))
	assert.Equal(t, int64(3), sum.RowsUpdated)
	assert.Equal(t, domain.StatusValidated, f.state(id).Status)
	assert.Equal(t, domain.Invalidation{}, f.lastInv, "validate invalidates nothing")

	// PrevStd is the baseline std; dry run compares all 3 rows.
	require.Len(t, f.updates, 3)
	for _, u := range f.updates {
		assert.True(t, u.PrevStd.Valid, u.Key.String())
	}
	assert.Equal(t, int64(3), sum.DryRun.Compared)
	require.NotNil(t, sum.DryRun.BaselineBatchID)
	assert.Equal(t, int64(900), *sum.DryRun.BaselineBatchID)
	assert.Contains(t, f.summary(id), StepValidate)

	// Idempotent re-validation of a VALIDATED batch.
	sum2, err := validateAt(f, &fakeProber{}).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.True(t, sum2.Validated)
}

func TestValidateStep_ErrorBlocks(t *testing.T) {
	ctx := context.Background()
	f := newValidateFake()
	id := derivedBatch(f, domain.ModeLive)
	f.noShade = true // V-06: shade missing from the replica

	sum, err := validateAt(f, &fakeProber{}).Run(ctx, id, "alice", nil)
	require.NoError(t, err, "a blocked outcome is not a step failure")
	assert.Positive(t, sum.Errors)
	assert.Positive(t, sum.ByCode[string(domain.IssueV06)])
	assert.False(t, sum.Validated)
	assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	assert.NotEmpty(t, sum.Issues)
	var withV06 int
	for _, u := range f.updates {
		for _, is := range u.Validation {
			if is.Code == domain.IssueV06 {
				withV06++
			}
		}
	}
	assert.Equal(t, 3, withV06, "row findings stored per row")
}

func TestValidateStep_DeriveIssuesNotDuplicated(t *testing.T) {
	f := newValidateFake()
	id := derivedBatch(f, domain.ModeLive)
	f.noShade = true
	_, err := validateAt(f, &fakeProber{}).Run(context.Background(), id, "alice", nil)
	require.NoError(t, err)
	// Re-run with the V-06 issues as derive-attached: no duplicate.
	for i := range f.std[id] {
		for _, u := range f.updates {
			if u.Key == f.std[id][i].Key {
				f.std[id][i].Issues = append([]domain.Issue(nil), u.Validation...)
			}
		}
	}
	_, err = validateAt(f, &fakeProber{}).Run(context.Background(), id, "alice", nil)
	require.NoError(t, err)
	for _, u := range f.updates {
		for _, is := range u.Validation {
			assert.NotEqual(t, domain.IssueV06, is.Code, "already derive-attached")
		}
	}
}

func TestValidateStep_UnackedWarningBlocks(t *testing.T) {
	f := newValidateFake()
	id := derivedBatch(f, domain.ModeLive)
	f.baseline.Std[vKeyAX] = dec("1.0") // +60%: V-05 warning

	sum, err := validateAt(f, &fakeProber{}).Run(context.Background(), id, "alice", nil)
	require.NoError(t, err)
	assert.Zero(t, sum.Errors)
	assert.Equal(t, 1, sum.Warnings)
	assert.True(t, sum.NeedsAck)
	assert.False(t, sum.Acked)
	assert.NotEmpty(t, sum.WarningSetHash)
	assert.False(t, sum.Validated)
	assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	require.NotEmpty(t, sum.DryRun.TopDelta)
	assert.Equal(t, vKeyAX.String(), sum.DryRun.TopDelta[0].Key)
}

func TestValidateStep_Guards(t *testing.T) {
	ctx := context.Background()
	t.Run("LIVE unlocked period", func(t *testing.T) {
		f := newValidateFake()
		id := derivedBatch(f, domain.ModeLive)
		f.locked = false
		_, err := validateAt(f, &fakeProber{}).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, domain.ErrPeriodNotLocked)
	})
	t.Run("SHADOW skips G10", func(t *testing.T) {
		f := newValidateFake()
		id := derivedBatch(f, domain.ModeShadow)
		f.locked = false
		sum, err := validateAt(f, &fakeProber{}).Run(ctx, id, "a", nil)
		require.NoError(t, err)
		assert.True(t, sum.Validated)
	})
	t.Run("wrong status", func(t *testing.T) {
		f := newValidateFake()
		id := f.put(domain.BatchState{Period: tPeriod, Status: domain.StatusCovered, CreatedAt: testNow, UpdatedAt: testNow})
		_, err := validateAt(f, &fakeProber{}).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, ErrStepNotAllowed)
	})
	t.Run("store unsupported", func(t *testing.T) {
		m := newMemBatches()
		id := m.put(domain.BatchState{Period: tPeriod, Status: domain.StatusDerived, CreatedAt: testNow, UpdatedAt: testNow})
		_, err := NewValidateStep(m, nil, 0).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, ErrValidateStoreUnsupported)
	})
	t.Run("read error propagates", func(t *testing.T) {
		f := newValidateFake()
		id := derivedBatch(f, domain.ModeLive)
		f.readErr = errors.New("boom")
		_, err := validateAt(f, &fakeProber{}).Run(ctx, id, "a", nil)
		require.Error(t, err)
		assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	})
}

func TestValidateStep_V10FailsClosed(t *testing.T) {
	ctx := context.Background()
	for name, p := range map[string]domain.ErpAdjHeadProber{
		"nil prober":  nil,
		"probe error": &fakeProber{err: errors.New("oracle down")},
		"posted head": &fakeProber{posted: 2},
	} {
		t.Run(name, func(t *testing.T) {
			f := newValidateFake()
			id := derivedBatch(f, domain.ModeLive)
			sum, err := validateAt(f, p).Run(ctx, id, "a", nil)
			require.NoError(t, err)
			assert.Positive(t, sum.ByCode["V-10"], "%v", sum.Findings)
			assert.False(t, sum.Validated)
		})
	}
	t.Run("stale demand", func(t *testing.T) {
		f := newValidateFake()
		id := derivedBatch(f, domain.ModeLive)
		sum, err := NewValidateStep(f, &fakeProber{}, time.Hour).
			WithClock(func() time.Time { return testNow.Add(3 * time.Hour) }).Run(ctx, id, "a", nil)
		require.NoError(t, err)
		assert.Positive(t, sum.ByCode["V-10"])
	})
}

func TestValidateStep_RevokesValidatedOnNewError(t *testing.T) {
	ctx := context.Background()
	f := newValidateFake()
	id := derivedBatch(f, domain.ModeLive)
	_, err := validateAt(f, &fakeProber{}).Run(ctx, id, "a", nil)
	require.NoError(t, err)
	require.Equal(t, domain.StatusValidated, f.state(id).Status)

	sum, err := validateAt(f, &fakeProber{posted: 1}).Run(ctx, id, "a", nil)
	require.NoError(t, err)
	assert.False(t, sum.Validated)
	assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	assert.Equal(t, domain.Invalidation{}, f.lastInv)
}

// --- ack -------------------------------------------------------------------

func warnedBatch(t *testing.T, f *validateFake) (int64, ValidateSummary) {
	t.Helper()
	id := derivedBatch(f, domain.ModeLive)
	f.baseline.Std[vKeyAX] = dec("1.0")
	sum, err := validateAt(f, &fakeProber{}).Run(context.Background(), id, "alice", nil)
	require.NoError(t, err)
	require.True(t, sum.NeedsAck)
	return id, sum
}

func TestAckWarnings_PromotesAndAudits(t *testing.T) {
	ctx := context.Background()
	f := newValidateFake()
	id, sum := warnedBatch(t, f)
	audit := &recordingAudit{}

	res, err := ackAt(f, audit).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "boss", WarningSetHash: sum.WarningSetHash, HasPermission: true})
	require.NoError(t, err)
	assert.True(t, res.Validated)
	assert.Equal(t, sum.WarningSetHash, res.Ack.WarningSetHash)
	assert.Equal(t, "boss", res.Ack.By)
	assert.Equal(t, 1, res.Ack.Warnings)
	st := f.state(id)
	assert.Equal(t, domain.StatusValidated, st.Status)
	require.NotNil(t, st.WarningsAck)
	assert.Contains(t, f.summary(id), summaryKeyAck)
	require.Len(t, audit.inputs, 1)
	assert.Equal(t, auditdomain.OpErpWarnAck, audit.inputs[0].Operation)
	assert.Equal(t, id, audit.inputs[0].EntityID)

	// A re-validation with the same warning set stays VALIDATED.
	sum2, err := validateAt(f, &fakeProber{}).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.True(t, sum2.Acked)
	assert.True(t, sum2.Validated)

	// A changed warning set revokes (ack no longer matches).
	f.baseline.Std[vKeyB] = dec("1.0")
	sum3, err := validateAt(f, &fakeProber{}).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.False(t, sum3.Acked)
	assert.Equal(t, domain.StatusDerived, f.state(id).Status)
}

func TestAckWarnings_Refusals(t *testing.T) {
	ctx := context.Background()
	t.Run("no permission", func(t *testing.T) {
		f := newValidateFake()
		id, sum := warnedBatch(t, f)
		audit := &recordingAudit{}
		_, err := ackAt(f, audit).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "x", WarningSetHash: sum.WarningSetHash})
		require.ErrorIs(t, err, ErrAckPermissionDenied)
		assert.Nil(t, f.state(id).WarningsAck)
		assert.Empty(t, audit.inputs)
	})
	t.Run("hash mismatch", func(t *testing.T) {
		f := newValidateFake()
		id, _ := warnedBatch(t, f)
		_, err := ackAt(f, nil).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "x", WarningSetHash: "deadbeef", HasPermission: true})
		require.ErrorIs(t, err, ErrAckHashMismatch)
		_, err = ackAt(f, nil).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "x", WarningSetHash: " ", HasPermission: true})
		require.ErrorIs(t, err, ErrAckHashMismatch)
	})
	t.Run("nothing to ack", func(t *testing.T) {
		f := newValidateFake()
		id := derivedBatch(f, domain.ModeLive)
		_, err := ackAt(f, nil).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "x", WarningSetHash: "h", HasPermission: true})
		require.ErrorIs(t, err, ErrNothingToAck)
	})
	t.Run("re-derived since validate", func(t *testing.T) {
		f := newValidateFake()
		id, sum := warnedBatch(t, f)
		st := f.state(id)
		st.RuleHash = strings.Repeat("b", 64)
		f.batches[id] = st
		_, err := ackAt(f, nil).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "x", WarningSetHash: sum.WarningSetHash, HasPermission: true})
		require.ErrorIs(t, err, ErrAckHashMismatch)
	})
	t.Run("ack with errors does not promote", func(t *testing.T) {
		f := newValidateFake()
		id := derivedBatch(f, domain.ModeLive)
		f.baseline.Std[vKeyAX] = dec("1.0")
		f.noShade = true
		sum, err := validateAt(f, &fakeProber{}).Run(ctx, id, "a", nil)
		require.NoError(t, err)
		require.Positive(t, sum.Errors)
		res, err := ackAt(f, nil).Handle(ctx, AckWarningsCommand{BatchID: id, Actor: "x", WarningSetHash: sum.WarningSetHash, HasPermission: true})
		require.NoError(t, err)
		assert.False(t, res.Validated)
		assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	})
}

// --- trigger + executor ----------------------------------------------------

func TestValidateTriggerAndExecutor(t *testing.T) {
	ctx := context.Background()
	f := newValidateFake()
	id := derivedBatch(f, domain.ModeLive)
	jobs := newMemJobs()
	trig := NewStepTriggerHandler(jobs, f, &fakePublisher{}, time.Hour).WithClock(func() time.Time { return testNow })

	exec, err := trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepValidate, Actor: "alice"})
	require.NoError(t, err)
	ex := NewJobExecutor(jobs, f, nil, nil)
	require.ErrorIs(t, ex.Execute(ctx, exec.ID()), ErrUnknownStep, "validate not configured")

	exec, err = trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepValidate, Actor: "alice"})
	require.NoError(t, err)
	require.NoError(t, ex.WithValidate(validateAt(f, &fakeProber{})).Execute(ctx, exec.ID()))
	assert.Equal(t, job.StatusSuccess, exec.Status())
	var res ValidateSummary
	require.NoError(t, json.Unmarshal(exec.ResultSummary(), &res))
	assert.True(t, res.Validated)

	cov := f.put(domain.BatchState{Period: "202608", Status: domain.StatusCovered, CreatedAt: testNow, UpdatedAt: testNow})
	_, err = trig.Handle(ctx, StepTriggerCommand{BatchID: cov, Step: StepValidate, Actor: "a"})
	require.ErrorIs(t, err, ErrStepNotAllowed)
}
