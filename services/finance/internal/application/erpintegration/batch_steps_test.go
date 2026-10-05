package erpintegration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

const tPeriod = "202607"

func demandRow(txn, item, grade, shade, qty string) domain.ErpDemandRow {
	return domain.ErpDemandRow{
		Period: tPeriod, TxnCode: txn, ItemCode: item, GradeCode: grade, ShadeCode: shade,
		HeadCount: 1, ItemCount: 1, RateVariants: 1, QtyKg: decimal.RequireFromString(qty),
	}
}

func sampleRows() []domain.ErpDemandRow {
	return []domain.ErpDemandRow{
		demandRow(domain.TxnInvAdj, "POY0000275", "AA", "X419T", "10.5"),
		demandRow(domain.TxnInvAdj, "POY0000275", "AB", "x419t ", "2.25"),
		demandRow(domain.TxnMbInvAdj, "CMB0000001", "AA", "RED", "1"),
	}
}

func draftBatch(m *memBatches) int64 {
	return m.put(domain.BatchState{Period: tPeriod, Status: domain.StatusDraft, CreatedAt: testNow, UpdatedAt: testNow})
}

// --- CreateBatch -----------------------------------------------------------

func TestCreateBatchGuards(t *testing.T) {
	ctx := context.Background()
	cmd := CreateBatchCommand{Period: tPeriod, Actor: "alice"}
	t.Run("nil lock checker fails closed", func(t *testing.T) {
		_, err := NewCreateBatchHandler(newMemBatches(), nil, &fakeProber{}).Handle(ctx, cmd)
		require.ErrorIs(t, err, ErrPeriodLockNotConfigured)
	})
	t.Run("period not locked", func(t *testing.T) {
		_, err := NewCreateBatchHandler(newMemBatches(), fakeLocks{}, &fakeProber{}).Handle(ctx, cmd)
		require.ErrorIs(t, err, domain.ErrPeriodNotLocked)
	})
	t.Run("nil prober fails closed", func(t *testing.T) {
		_, err := NewCreateBatchHandler(newMemBatches(), fakeLocks{locked: true}, nil).Handle(ctx, cmd)
		require.ErrorIs(t, err, ErrPostedProbeNotConfigured)
	})
	t.Run("posted heads refused (V-10)", func(t *testing.T) {
		_, err := NewCreateBatchHandler(newMemBatches(), fakeLocks{locked: true}, &fakeProber{posted: 2}).Handle(ctx, cmd)
		require.ErrorIs(t, err, domain.ErrPeriodPosted)
	})
	t.Run("probe error fails closed", func(t *testing.T) {
		_, err := NewCreateBatchHandler(newMemBatches(), fakeLocks{locked: true}, &fakeProber{err: errors.New("down")}).Handle(ctx, cmd)
		require.Error(t, err)
	})
	t.Run("in-flight batch refused", func(t *testing.T) {
		m := newMemBatches()
		draftBatch(m)
		_, err := NewCreateBatchHandler(m, fakeLocks{locked: true}, &fakeProber{}).Handle(ctx, cmd)
		require.ErrorIs(t, err, domain.ErrActiveBatchExists)
	})
	t.Run("ok creates DRAFT", func(t *testing.T) {
		m := newMemBatches()
		b, err := NewCreateBatchHandler(m, fakeLocks{locked: true}, &fakeProber{}).WithClock(func() time.Time { return testNow }).Handle(ctx, cmd)
		require.NoError(t, err)
		assert.Equal(t, domain.StatusDraft, b.Status())
		assert.Equal(t, domain.ModeLive, b.Mode())
		assert.Positive(t, b.ID())
	})
	t.Run("shadow skips live guards", func(t *testing.T) {
		b, err := NewCreateBatchHandler(newMemBatches(), nil, nil).Handle(ctx, CreateBatchCommand{Period: tPeriod, Mode: domain.ModeShadow, Actor: "alice"})
		require.NoError(t, err)
		assert.Equal(t, domain.ModeShadow, b.Mode())
	})
}

// --- LoadDemand ------------------------------------------------------------

func TestLoadDemandIdempotent(t *testing.T) {
	ctx := context.Background()
	m := newMemBatches()
	id := draftBatch(m)
	reader := &fakeDemandReader{rows: sampleRows()}
	clock := testNow
	step := NewLoadDemandStep(m, reader, &fakeProber{}).WithClock(func() time.Time { return clock })

	first, err := step.Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDemandLoaded, m.state(id).Status)
	require.NotNil(t, m.state(id).DemandLoadedAt)
	assert.Equal(t, int64(3), first.Rows)
	assert.Equal(t, "13.75", first.QtyKg)
	firstLines := m.demand[id]

	clock = testNow.Add(time.Hour)
	second, err := step.Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.Equal(t, first.Digest, second.Digest, "AC-05 identical digest")
	assert.Equal(t, first.Rows, second.Rows, "AC-05 identical row count")
	assert.Equal(t, first.Items, second.Items)
	assert.Len(t, m.demand[id], len(firstLines))
	assert.Equal(t, clock, *m.state(id).DemandLoadedAt, "reload restamps loaded_at")
	assert.Contains(t, m.summary(id), StepLoadDemand)

	reader.rows = append(sampleRows(), demandRow(domain.TxnInvAdj, "POY0000999", "AA", "Z", "1"))
	third, err := step.Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.NotEqual(t, first.Digest, third.Digest)
}

func TestLoadDemandGuardsAndWarnings(t *testing.T) {
	ctx := context.Background()
	t.Run("nil reader", func(t *testing.T) {
		m := newMemBatches()
		_, err := NewLoadDemandStep(m, nil, nil).Run(ctx, draftBatch(m), "a", nil)
		require.ErrorIs(t, err, ErrDemandReaderNotConfigured)
	})
	t.Run("not allowed from terminal", func(t *testing.T) {
		m := newMemBatches()
		id := m.put(domain.BatchState{Period: tPeriod, Status: domain.StatusFailed, CreatedAt: testNow, UpdatedAt: testNow})
		_, err := NewLoadDemandStep(m, &fakeDemandReader{}, nil).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, ErrStepNotAllowed)
	})
	t.Run("reader error leaves batch untouched", func(t *testing.T) {
		m := newMemBatches()
		id := draftBatch(m)
		_, err := NewLoadDemandStep(m, &fakeDemandReader{err: errors.New("ora down")}, nil).Run(ctx, id, "a", nil)
		require.Error(t, err)
		assert.Equal(t, domain.StatusDraft, m.state(id).Status)
	})
	t.Run("posted heads are a warning only", func(t *testing.T) {
		m := newMemBatches()
		id := draftBatch(m)
		sum, err := NewLoadDemandStep(m, &fakeDemandReader{rows: sampleRows()}, &fakeProber{posted: 1}).Run(ctx, id, "a", nil)
		require.NoError(t, err)
		require.NotNil(t, sum.PostedHeads)
		assert.NotEmpty(t, sum.Warnings)
	})
}

// --- Coverage --------------------------------------------------------------

func ptr[T any](v T) *T { return &v }

func combo(item, shade string) domain.CoverageLine {
	return domain.CoverageLine{BatchID: 1, Kind: domain.ItemKindForCode(item), ItemCode: item, ShadeCode: shade, QtyKg: decimal.NewFromInt(1)}
}

func TestClassifyCoverageStatuses(t *testing.T) {
	yarnP := []ProductCandidate{{SysID: 10, TypeCode: "ITY"}}
	mbP := []ProductCandidate{{SysID: 20, TypeCode: "MB"}}
	usd := func(id int64, status string) map[int64]ActualCost {
		return map[int64]ActualCost{id: {CostID: 900, Version: 2, Status: status, Currency: "USD", CostPerUnit: decimal.RequireFromString("2.5")}}
	}
	cases := []struct {
		name   string
		period string
		combo  domain.CoverageLine
		cands  []ProductCandidate
		costs  map[int64]ActualCost
		want   domain.CoverageStatus
	}{
		{"ok yarn", tPeriod, combo("POY1", "X"), yarnP, usd(10, "APPROVED"), domain.CoverageOK},
		{"ok mb", tPeriod, combo("CMB1", "RED"), mbP, usd(20, "APPROVED"), domain.CoverageOK},
		{"no mapping", tPeriod, combo("POY1", "X"), nil, nil, domain.CoverageNoMapping},
		{"dup mapping", tPeriod, combo("POY1", "X"), []ProductCandidate{{SysID: 12}, {SysID: 11}}, usd(11, "APPROVED"), domain.CoverageDupMapping},
		{"cmb on yarn type", tPeriod, combo("CMB1", "RED"), yarnP, usd(10, "APPROVED"), domain.CoverageInvalid},
		{"yarn on mb type", tPeriod, combo("POY1", "X"), mbP, usd(20, "APPROVED"), domain.CoverageInvalid},
		{"no cost", tPeriod, combo("POY1", "X"), yarnP, nil, domain.CoverageNoCost},
		{"verified only", tPeriod, combo("POY1", "X"), yarnP, usd(10, "VERIFIED"), domain.CoverageNotApproved},
		{"calculated", tPeriod, combo("POY1", "X"), yarnP, usd(10, "CALCULATED"), domain.CoverageNotApproved},
		{"idr outlier", tPeriod, combo("POY1", "X"), yarnP, map[int64]ActualCost{10: {CostID: 1, Status: "APPROVED", Currency: "IDR", CostPerUnit: decimal.NewFromInt(35000)}}, domain.CoverageNotUSD},
		{"idr non-outlier interim ok", tPeriod, combo("POY1", "X"), yarnP, map[int64]ActualCost{10: {CostID: 1, Status: "APPROVED", Currency: "IDR", CostPerUnit: decimal.RequireFromString("2.5")}}, domain.CoverageOK},
		{"eur rejected", tPeriod, combo("POY1", "X"), yarnP, map[int64]ActualCost{10: {CostID: 1, Status: "APPROVED", Currency: "EUR", CostPerUnit: decimal.NewFromInt(2)}}, domain.CoverageNotUSD},
		{"202605 idr stays error", "202605", combo("POY1", "X"), yarnP, map[int64]ActualCost{10: {CostID: 1, Status: "APPROVED", Currency: "IDR", CostPerUnit: decimal.RequireFromString("2.5")}}, domain.CoverageNotUSD},
		{"202605 usd outlier error", "202605", combo("POY1", "X"), yarnP, map[int64]ActualCost{10: {CostID: 1, Status: "APPROVED", Currency: "USD", CostPerUnit: decimal.NewFromInt(35000)}}, domain.CoverageNotUSD},
		{"202605 usd ok", "202605", combo("POY1", "X"), yarnP, usd(10, "APPROVED"), domain.CoverageOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ClassifyCoverage(tc.period, tc.combo, tc.cands, tc.costs)
			assert.Equal(t, tc.want, got.Status, got.Reason)
			require.NoError(t, got.Validate())
			switch tc.want {
			case domain.CoverageOK:
				assert.NotNil(t, got.ProductSysID)
				assert.NotNil(t, got.CostID)
				assert.Empty(t, got.Reason)
			case domain.CoverageDupMapping:
				assert.Nil(t, got.ProductSysID, "never guessed")
				var c struct {
					Candidates []candidateJSON `json:"candidates"`
				}
				require.NoError(t, json.Unmarshal(got.Candidates, &c))
				require.Len(t, c.Candidates, 2)
				assert.Equal(t, int64(11), c.Candidates[0].ProductSysID)
				assert.Equal(t, int64(12), c.Candidates[1].ProductSysID)
				assert.Contains(t, got.Reason, "11,12")
			default:
				assert.NotEmpty(t, got.Reason)
			}
		})
	}
}

func TestGroupDemandCombos(t *testing.T) {
	var lines []domain.DemandLine
	for _, r := range sampleRows() {
		l, err := domain.NewDemandLineFromErpRow(1, r, testNow)
		require.NoError(t, err)
		lines = append(lines, l)
	}
	got := GroupDemandCombos(1, lines)
	require.Len(t, got, 2)
	assert.Equal(t, "CMB0000001", got[0].ItemCode)
	assert.Equal(t, domain.ItemKindMB, got[0].Kind)
	assert.Equal(t, "POY0000275", got[1].ItemCode)
	assert.Equal(t, []string{"AA", "AB"}, got[1].GradeCodes)
	assert.Equal(t, "12.75", got[1].QtyKg.String())
}

func coveredSetup(t *testing.T) (*memBatches, int64, *fakeSource) {
	t.Helper()
	m := newMemBatches()
	id := draftBatch(m)
	_, err := NewLoadDemandStep(m, &fakeDemandReader{rows: sampleRows()}, nil).
		WithClock(func() time.Time { return testNow }).Run(context.Background(), id, "a", nil)
	require.NoError(t, err)
	src := &fakeSource{
		products: map[ErpProductKey][]ProductCandidate{
			NewErpProductKey("POY0000275", "X419T"): {{SysID: 10, TypeCode: "ITY"}},
			NewErpProductKey("CMB0000001", "RED"):   {{SysID: 20, TypeCode: "MB"}},
		},
		costs: map[int64]ActualCost{
			10: {CostID: 100, Version: 1, Status: "APPROVED", Currency: "USD", CostPerUnit: decimal.NewFromInt(2)},
			20: {CostID: 200, Version: 1, Status: "APPROVED", Currency: "USD", CostPerUnit: decimal.NewFromInt(3)},
		},
	}
	return m, id, src
}

func TestCoverageStepCoveredAndReopen(t *testing.T) {
	ctx := context.Background()
	m, id, src := coveredSetup(t)
	step := NewCoverageStep(m, src, 24*time.Hour).WithClock(func() time.Time { return testNow.Add(time.Hour) })

	sum, err := step.Run(ctx, id, "a", nil)
	require.NoError(t, err)
	assert.True(t, sum.AllOK)
	assert.Equal(t, int64(2), sum.Combos)
	assert.Equal(t, domain.StatusCovered, m.state(id).Status)
	assert.Contains(t, m.summary(id), StepCoverage)
	assert.Contains(t, m.summary(id), StepLoadDemand, "load summary kept")

	again, err := step.Run(ctx, id, "a", nil)
	require.NoError(t, err)
	assert.Equal(t, sum.Digest, again.Digest, "re-coverage is idempotent")
	assert.Equal(t, domain.StatusCovered, m.state(id).Status)

	delete(src.costs, 20)
	gap, err := step.Run(ctx, id, "a", nil)
	require.NoError(t, err)
	assert.False(t, gap.AllOK)
	assert.Equal(t, int64(1), gap.Counts[string(domain.CoverageNoCost)])
	assert.Equal(t, int64(1), gap.Blocking)
	assert.Equal(t, domain.StatusDemandLoaded, m.state(id).Status, "COVERED reopens on a gap")
	assert.True(t, m.lastInv.Coverage)
	assert.False(t, m.lastInv.Demand, "demand kept on reopen")
	assert.Len(t, m.demand[id], 3)
	assert.Equal(t, []string{"CMB0000001/RED=NO_COST", "POY0000275/X419T=OK"}, sortedStatuses(m.coverage[id]))
}

func TestCoverageStepGapStaysDemandLoaded(t *testing.T) {
	m, id, src := coveredSetup(t)
	src.products[NewErpProductKey("POY0000275", "X419T")] = []ProductCandidate{{SysID: 10}, {SysID: 11}}
	sum, err := NewCoverageStep(m, src, 0).Run(context.Background(), id, "a", nil)
	require.NoError(t, err)
	assert.False(t, sum.AllOK)
	assert.Equal(t, int64(1), sum.Ambiguous)
	assert.Equal(t, domain.StatusDemandLoaded, m.state(id).Status)
}

func TestCoverageStepStalenessAndGuards(t *testing.T) {
	ctx := context.Background()
	t.Run("stale demand refused", func(t *testing.T) {
		m, id, src := coveredSetup(t)
		_, err := NewCoverageStep(m, src, time.Hour).WithClock(func() time.Time { return testNow.Add(2 * time.Hour) }).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, domain.ErrDemandStale)
		assert.Equal(t, domain.StatusDemandLoaded, m.state(id).Status)
		assert.Empty(t, m.coverage[id])
	})
	t.Run("draft not allowed", func(t *testing.T) {
		m := newMemBatches()
		_, err := NewCoverageStep(m, &fakeSource{}, 0).Run(ctx, draftBatch(m), "a", nil)
		require.ErrorIs(t, err, ErrStepNotAllowed)
	})
	t.Run("source error propagates", func(t *testing.T) {
		m, id, src := coveredSetup(t)
		src.err = errors.New("pg down")
		_, err := NewCoverageStep(m, src, 0).Run(ctx, id, "a", nil)
		require.Error(t, err)
	})
	t.Run("CheckDemandFresh", func(t *testing.T) {
		m, id, _ := coveredSetup(t)
		b, err := m.GetByID(ctx, id)
		require.NoError(t, err)
		require.NoError(t, domain.CheckDemandFresh(b, testNow.Add(time.Hour), 2*time.Hour))
		require.NoError(t, domain.CheckDemandFresh(b, testNow.Add(100*time.Hour), 0))
		require.ErrorIs(t, domain.CheckDemandFresh(b, testNow.Add(3*time.Hour), 2*time.Hour), domain.ErrDemandStale)
		d, _ := domain.NewBatch(tPeriod, domain.ModeLive, "a", testNow)
		require.ErrorIs(t, domain.CheckDemandFresh(d, testNow, time.Hour), domain.ErrDemandNotLoaded)
	})
}

// --- Trigger + executor ----------------------------------------------------

func TestStepTriggerAndExecutor(t *testing.T) {
	ctx := context.Background()
	m := newMemBatches()
	id := draftBatch(m)
	jobs := newMemJobs()
	pub := &fakePublisher{}
	trig := NewStepTriggerHandler(jobs, m, pub, time.Hour).WithClock(func() time.Time { return testNow })

	_, err := trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepCoverage, Actor: "a"})
	require.ErrorIs(t, err, ErrStepNotAllowed, "coverage before demand")
	_, err = trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: "bogus", Actor: "a"})
	require.ErrorIs(t, err, ErrUnknownStep)

	exec, err := trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepLoadDemand, Actor: "alice"})
	require.NoError(t, err)
	assert.Equal(t, job.TypeErpIntegration, exec.JobType())
	assert.Equal(t, exec.ID().String(), m.state(id).JobID)
	require.Len(t, pub.calls, 1)

	_, src := func() (int, *fakeSource) { _, _, s := coveredSetup(t); return 0, s }()
	load := NewLoadDemandStep(m, &fakeDemandReader{rows: sampleRows()}, nil).WithClock(func() time.Time { return testNow })
	cov := NewCoverageStep(m, src, time.Hour).WithClock(func() time.Time { return testNow })
	ex := NewJobExecutor(jobs, m, load, cov)

	require.NoError(t, ex.Execute(ctx, exec.ID()))
	assert.Equal(t, job.StatusSuccess, exec.Status())
	assert.Equal(t, []int{progressStart, progressRead, progressCompute, progressPersist}, jobs.progress[exec.ID()])
	assert.Equal(t, domain.StatusDemandLoaded, m.state(id).Status)
	require.NoError(t, ex.Execute(ctx, exec.ID()), "terminal job acked")

	covExec, err := trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepCoverage, Actor: "alice"})
	require.NoError(t, err)
	require.NoError(t, ex.Execute(ctx, covExec.ID()))
	assert.Equal(t, domain.StatusCovered, m.state(id).Status)
	var res CoverageSummary
	require.NoError(t, json.Unmarshal(covExec.ResultSummary(), &res))
	assert.True(t, res.AllOK)

	t.Run("stale trigger refused", func(t *testing.T) {
		late := NewStepTriggerHandler(jobs, m, pub, time.Hour).WithClock(func() time.Time { return testNow.Add(2 * time.Hour) })
		_, err := late.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepCoverage, Actor: "a"})
		require.ErrorIs(t, err, domain.ErrDemandStale)
	})
	t.Run("active job refused", func(t *testing.T) {
		jobs.active = true
		defer func() { jobs.active = false }()
		_, err := trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepLoadDemand, Actor: "a"})
		require.ErrorIs(t, err, job.ErrDuplicateActiveJob)
	})
	t.Run("nil publisher", func(t *testing.T) {
		_, err := NewStepTriggerHandler(jobs, m, nil, 0).Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepLoadDemand, Actor: "a"})
		require.ErrorIs(t, err, ErrPublisherUnavailable)
	})
	t.Run("publish failure fails job", func(t *testing.T) {
		_, err := NewStepTriggerHandler(jobs, m, &fakePublisher{err: errors.New("amqp")}, 0).Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepLoadDemand, Actor: "a"})
		require.Error(t, err)
	})
}

func TestExecutorFailureRecordsError(t *testing.T) {
	ctx := context.Background()
	m := newMemBatches()
	id := draftBatch(m)
	jobs := newMemJobs()
	params, _ := json.Marshal(JobParams{BatchID: id})
	exec, err := job.NewExecution(job.TypeErpIntegration, StepLoadDemand, tPeriod, "alice", 5, params)
	require.NoError(t, err)
	require.NoError(t, jobs.Create(ctx, exec))

	ex := NewJobExecutor(jobs, m, NewLoadDemandStep(m, &fakeDemandReader{err: errors.New("ora down")}, nil), nil)
	require.Error(t, ex.Execute(ctx, exec.ID()))
	assert.Equal(t, job.StatusFailed, exec.Status())
	assert.Equal(t, domain.StatusDraft, m.state(id).Status, "step failure keeps batch status")
	assert.Contains(t, m.state(id).Error, "ora down")

	bad, _ := job.NewExecution(job.TypeErpIntegration, "nope", tPeriod, "alice", 5, params)
	require.NoError(t, jobs.Create(ctx, bad))
	require.ErrorIs(t, ex.Execute(ctx, bad.ID()), ErrUnknownStep)

	noParams, _ := job.NewExecution(job.TypeErpIntegration, StepLoadDemand, tPeriod, "alice", 5, nil)
	require.NoError(t, jobs.Create(ctx, noParams))
	require.ErrorIs(t, ex.Execute(ctx, noParams.ID()), ErrInvalidJobParams)
}
