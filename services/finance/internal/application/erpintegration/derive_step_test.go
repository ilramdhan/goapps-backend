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
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/job"
)

// --- fakes -----------------------------------------------------------------

// deriveFake wraps memBatches with a DeriveStore-capable runner.
type deriveFake struct {
	*memBatches
	ax       map[int64]domain.AxComponents
	shades   map[string]string
	locked   bool
	lockErr  error
	std      map[int64][]domain.StdRow
	replaced int
}

func newDeriveFake() *deriveFake {
	return &deriveFake{
		memBatches: newMemBatches(), ax: map[int64]domain.AxComponents{},
		shades: map[string]string{}, locked: true, std: map[int64][]domain.StdRow{},
	}
}

func (f *deriveFake) RunLocked(ctx context.Context, batchID int64, fn func(context.Context, domain.BatchStore) error) error {
	return fn(ctx, &deriveMemStore{memStore: memStore{m: f.memBatches, id: batchID}, f: f})
}

type deriveMemStore struct {
	memStore
	f *deriveFake
}

func (s *deriveMemStore) ListCoverage(context.Context) ([]domain.CoverageLine, error) {
	s.m.mu.Lock()
	defer s.m.mu.Unlock()
	return append([]domain.CoverageLine(nil), s.m.coverage[s.id]...), nil
}

func (s *deriveMemStore) IsPeriodLocked(context.Context, string, string) (bool, error) {
	return s.f.locked, s.f.lockErr
}

func (s *deriveMemStore) LoadAxComponents(_ context.Context, _ string, ids []int64) (map[int64]domain.AxComponents, error) {
	out := map[int64]domain.AxComponents{}
	for _, id := range ids {
		if a, ok := s.f.ax[id]; ok {
			out[id] = a
		}
	}
	return out, nil
}

func (s *deriveMemStore) ShadeNames(context.Context, []string) (map[string]string, error) {
	return s.f.shades, nil
}

func (s *deriveMemStore) ReplaceStdRows(_ context.Context, _ string, rows []domain.StdRow) (int64, error) {
	s.f.std[s.id] = append([]domain.StdRow(nil), rows...)
	s.f.replaced++
	return int64(len(rows)), nil
}

type fakeRules struct {
	rs  *erprule.RuleSet
	err error
}

func (f *fakeRules) LoadRuleSet(context.Context) (*erprule.RuleSet, error) { return f.rs, f.err }

// --- helpers ---------------------------------------------------------------

func dec(s string) decimal.Decimal { return decimal.RequireFromString(s) }

func ptrI64(v int64) *int64 { return &v }
func ptrI32(v int32) *int32 { return &v }

func deriveRuleSet(t *testing.T, loss string) *erprule.RuleSet {
	t.Helper()
	rules := []erprule.Rule{
		{Key: erprule.RuleKey{FgType: "Type 1", ProdType: erprule.ProdTypePOY, GradeGroup: erprule.GradeGroupNS}, Basis: erprule.BasisCost, ValLoss: dec(loss)},
		{Key: erprule.RuleKey{FgType: "Type 1", ProdType: erprule.ProdTypePOY, GradeGroup: erprule.GradeGroupBC}, Basis: erprule.BasisSPPTY, ValLoss: dec("0.5")},
	}
	prices := []erprule.Price{{Basis: erprule.BasisSPPTY, Price: dec("1.3")}}
	grades := []erprule.GradeAssignment{
		{GradeCode: "A", Group: erprule.GradeGroupNS},
		{GradeCode: "B", Group: erprule.GradeGroupBC},
		{GradeCode: "AX", Group: erprule.GradeGroupAX},
	}
	rs, err := erprule.NewRuleSet(rules, prices, grades)
	require.NoError(t, err)
	return rs
}

func axFor(costID, prodID int64) domain.AxComponents {
	return domain.AxComponents{
		CostID: costID, Version: 1, ProductSysID: prodID,
		CostPerUnit: dec("2.3456789"), TotalRMCost: dec("1.1234565"),
		FgType: "Type 1", ChpItemCode: "REG", ItemType: "POY",
	}
}

func okLine(item, shade string, costID int64, grades ...string) domain.CoverageLine {
	return domain.CoverageLine{
		BatchID: 1, Kind: domain.ItemKindForCode(item), ItemCode: item, ShadeCode: shade,
		GradeCodes: grades, ProductSysID: ptrI64(costID + 100), CostID: ptrI64(costID), CostVersion: ptrI32(1),
		Status: domain.CoverageOK, QtyKg: decimal.NewFromInt(1),
	}
}

// coveredBatch stores a COVERED batch of mode with the given coverage.
func coveredBatch(f *deriveFake, mode domain.BatchMode, lines ...domain.CoverageLine) int64 {
	loaded := testNow
	id := f.put(domain.BatchState{
		Period: tPeriod, Mode: mode, Status: domain.StatusCovered, DemandLoadedAt: &loaded,
		CreatedAt: testNow, UpdatedAt: testNow,
	})
	for i := range lines {
		lines[i].BatchID = id
	}
	f.coverage[id] = lines
	f.demand[id] = []domain.DemandLine{{BatchID: id, ItemCode: "POY0000001", ItemName: " Yarn One "}}
	return id
}

func deriveKeys(rows []domain.StdRow) []string {
	out := make([]string, len(rows))
	for i, r := range rows {
		out[i] = r.Key.String() + "=" + string(r.Status)
	}
	return out
}

func stepAt(f *deriveFake, rs *erprule.RuleSet) *DeriveStep {
	return NewDeriveStep(f, &fakeRules{rs: rs}, 24*time.Hour).WithClock(func() time.Time { return testNow.Add(time.Hour) })
}

// --- expansion rule --------------------------------------------------------

func TestExpandCoverage_ExpansionRule(t *testing.T) {
	ax := map[int64]domain.AxComponents{1: axFor(1, 101), 2: axFor(2, 102)}
	t.Run("yarn AX plus distinct non-AX grades, sorted", func(t *testing.T) {
		in := ExpandCoverage([]domain.CoverageLine{okLine("POY0000001", "X", 1, "B", "AX", " A ", "B")}, ax, nil, nil)
		got := make([]string, len(in))
		for i, x := range in {
			got[i] = x.Key.String()
			require.NotNil(t, x.Ax)
		}
		assert.Equal(t, []string{"POY0000001/A/X", "POY0000001/AX/X", "POY0000001/B/X"}, got)
	})
	t.Run("yarn with only AX demand gives the AX row", func(t *testing.T) {
		in := ExpandCoverage([]domain.CoverageLine{okLine("POY0000001", "X", 1)}, ax, nil, nil)
		require.Len(t, in, 1)
		assert.Equal(t, domain.GradeAX, in[0].Key.GradeCode)
	})
	t.Run("MB grade A plus other grades (engine NO_RULE)", func(t *testing.T) {
		in := ExpandCoverage([]domain.CoverageLine{okLine("CMB0000001", "RED", 2, "B", "A")}, ax, nil, nil)
		require.Len(t, in, 2)
		assert.Equal(t, "CMB0000001/A/RED", in[0].Key.String())
		assert.Equal(t, "CMB0000001/B/RED", in[1].Key.String())
		rows, _ := domain.Derive(in, nil)
		assert.Equal(t, domain.DeriveOK, rows[0].Status)
		assert.Equal(t, domain.DeriveNoRule, rows[1].Status)
	})
	t.Run("non-OK coverage gives NO_AX rows", func(t *testing.T) {
		l := okLine("POY0000002", "Y", 9, "A")
		l.Status, l.CostID = domain.CoverageNoCost, nil
		in := ExpandCoverage([]domain.CoverageLine{l}, ax, nil, nil)
		require.Len(t, in, 2)
		for _, x := range in {
			assert.Nil(t, x.Ax)
		}
		rows, _ := domain.Derive(in, nil)
		assert.Equal(t, []string{"POY0000002/A/Y=NO_AX", "POY0000002/AX/Y=NO_AX"}, deriveKeys(rows))
	})
	t.Run("OK line whose cost is no longer usable is NO_AX", func(t *testing.T) {
		in := ExpandCoverage([]domain.CoverageLine{okLine("POY0000003", "Z", 77)}, ax, nil, nil)
		require.Len(t, in, 1)
		assert.Nil(t, in[0].Ax)
	})
	t.Run("order across items and names", func(t *testing.T) {
		lines := []domain.CoverageLine{okLine("PTY0000001", "B", 1), okLine("POY0000001", "Z", 1), okLine("POY0000001", "A", 1)}
		in := ExpandCoverage(lines, ax, map[string]string{"POY0000001": "Yarn"}, map[string]string{"Z": "Zed"})
		assert.Equal(t, "POY0000001/AX/A", in[0].Key.String())
		assert.Equal(t, "POY0000001/AX/Z", in[1].Key.String())
		assert.Equal(t, "Zed", in[1].ShadeName)
		assert.Equal(t, "Yarn", in[1].ItemName)
		assert.Equal(t, "PTY0000001/AX/B", in[2].Key.String())
	})
}

// --- step ------------------------------------------------------------------

func TestDeriveStep_DerivesAndIsIdempotent(t *testing.T) {
	ctx := context.Background()
	f := newDeriveFake()
	f.ax[1] = axFor(1, 101)
	f.shades["X"] = "Shade X"
	id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1, "A", "B"))
	rs := deriveRuleSet(t, "0.05")

	sum, err := stepAt(f, rs).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.True(t, sum.Derived)
	assert.Equal(t, int64(3), sum.Rows)
	assert.Equal(t, int64(3), sum.RowCount)
	assert.Zero(t, sum.ErrorRows)

	st := f.state(id)
	assert.Equal(t, domain.StatusDerived, st.Status)
	assert.Equal(t, rs.Hash(), st.RuleHash)
	assert.Equal(t, rs.Canonical(), st.RuleSnapshot)
	require.NotNil(t, st.Totals)
	assert.Equal(t, int64(3), st.Totals.RowCount())
	assert.Equal(t, "Yarn One", f.std[id][0].ItemName)
	assert.Equal(t, "Shade X", f.std[id][0].ShadeName)

	digest, err := domain.ComputeStdDigest(f.std[id])
	require.NoError(t, err)
	assert.Equal(t, digest.RowsMD5, sum.RowsMD5)
	var persisted map[string]DeriveSummary
	require.NoError(t, json.Unmarshal(st.Summary, &persisted))
	assert.Equal(t, digest.RowsMD5, persisted[StepDerive].RowsMD5)
	assert.Equal(t, domain.StdDigestVersion, persisted[StepDerive].DigestVersion)
	assert.True(t, f.lastInv.WarningsAck)

	// AC-05: a re-derive with unchanged inputs is identical.
	again, err := stepAt(f, rs).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	st2 := f.state(id)
	assert.Equal(t, domain.StatusDerived, st2.Status)
	assert.Equal(t, st.RuleHash, st2.RuleHash)
	assert.Equal(t, st.RuleSnapshot, st2.RuleSnapshot)
	assert.True(t, st.Totals.Equal(*st2.Totals))
	assert.Equal(t, sum.RowsMD5, again.RowsMD5)
	assert.Equal(t, sum.SumStd, again.SumStd)
	assert.Equal(t, sum.SumConv, again.SumConv)
	assert.Equal(t, sum.SumPvl, again.SumPvl)
	assert.Equal(t, 2, f.replaced)
}

func TestDeriveStep_V08StaysCovered(t *testing.T) {
	ctx := context.Background()
	f := newDeriveFake()
	f.ax[1] = axFor(1, 101)
	// Grade "Q" has no grade group -> NO_GRADE_GROUP (V-08).
	id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1, "Q"))
	rs := deriveRuleSet(t, "0.05")

	sum, err := stepAt(f, rs).Run(ctx, id, "alice", nil)
	require.NoError(t, err)
	assert.False(t, sum.Derived)
	assert.Equal(t, int64(1), sum.ErrorRows)
	require.NotEmpty(t, sum.Issues)
	assert.Contains(t, sum.Issues[0], string(domain.IssueV08))
	st := f.state(id)
	assert.Equal(t, domain.StatusCovered, st.Status)
	assert.Empty(t, st.RuleHash)
	assert.Nil(t, st.Totals)
	assert.Len(t, f.std[id], 2, "failing rows stay visible")
	assert.Equal(t, domain.Invalidation{}, f.lastInv)

	t.Run("re-derive from DERIVED with a new gap reopens to COVERED", func(t *testing.T) {
		f2 := newDeriveFake()
		f2.ax[1] = axFor(1, 101)
		id2 := coveredBatch(f2, domain.ModeLive, okLine("POY0000001", "X", 1, "A"))
		_, err := stepAt(f2, rs).Run(ctx, id2, "a", nil)
		require.NoError(t, err)
		require.Equal(t, domain.StatusDerived, f2.state(id2).Status)
		f2.coverage[id2][0].GradeCodes = []string{"A", "Q"}
		sum, err := stepAt(f2, rs).Run(ctx, id2, "a", nil)
		require.NoError(t, err)
		assert.False(t, sum.Derived)
		st := f2.state(id2)
		assert.Equal(t, domain.StatusCovered, st.Status)
		assert.Empty(t, st.RuleHash)
		assert.Nil(t, st.Totals)
	})
	t.Run("NO_AX coverage line blocks DERIVED", func(t *testing.T) {
		f3 := newDeriveFake()
		l := okLine("POY0000001", "X", 5)
		id3 := coveredBatch(f3, domain.ModeLive, l)
		sum, err := stepAt(f3, rs).Run(ctx, id3, "a", nil)
		require.NoError(t, err)
		assert.Equal(t, int64(1), sum.Counts[string(domain.DeriveNoAX)])
		assert.Equal(t, domain.StatusCovered, f3.state(id3).Status)
	})
}

func TestDeriveStep_Guards(t *testing.T) {
	ctx := context.Background()
	rs := deriveRuleSet(t, "0.05")
	t.Run("SHADOW derives without a period lock", func(t *testing.T) {
		f := newDeriveFake()
		f.locked = false
		f.ax[1] = axFor(1, 101)
		id := coveredBatch(f, domain.ModeShadow, okLine("POY0000001", "X", 1))
		sum, err := stepAt(f, rs).Run(ctx, id, "a", nil)
		require.NoError(t, err)
		assert.True(t, sum.Derived)
		assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	})
	t.Run("LIVE without period lock refused (G10)", func(t *testing.T) {
		f := newDeriveFake()
		f.locked = false
		id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1))
		_, err := stepAt(f, rs).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, domain.ErrPeriodNotLocked)
		assert.Zero(t, f.replaced)
	})
	t.Run("lock check error", func(t *testing.T) {
		f := newDeriveFake()
		f.lockErr = errors.New("pg")
		id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1))
		_, err := stepAt(f, rs).Run(ctx, id, "a", nil)
		require.Error(t, err)
	})
	t.Run("stale demand refused", func(t *testing.T) {
		f := newDeriveFake()
		id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1))
		_, err := NewDeriveStep(f, &fakeRules{rs: rs}, time.Hour).
			WithClock(func() time.Time { return testNow.Add(2 * time.Hour) }).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, domain.ErrDemandStale)
	})
	t.Run("DEMAND_LOADED not allowed", func(t *testing.T) {
		f := newDeriveFake()
		loaded := testNow
		id := f.put(domain.BatchState{Period: tPeriod, Status: domain.StatusDemandLoaded, DemandLoadedAt: &loaded, CreatedAt: testNow, UpdatedAt: testNow})
		_, err := stepAt(f, rs).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, ErrStepNotAllowed)
	})
	t.Run("nil loader fails closed", func(t *testing.T) {
		_, err := NewDeriveStep(newDeriveFake(), nil, 0).Run(ctx, 1, "a", nil)
		require.ErrorIs(t, err, ErrRuleSetLoaderNotConfigured)
	})
	t.Run("loader error and nil set", func(t *testing.T) {
		f := newDeriveFake()
		id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1))
		_, err := NewDeriveStep(f, &fakeRules{err: errors.New("pg")}, 0).Run(ctx, id, "a", nil)
		require.Error(t, err)
		_, err = NewDeriveStep(f, &fakeRules{}, 0).Run(ctx, id, "a", nil)
		require.ErrorIs(t, err, ErrRuleSetLoaderNotConfigured)
	})
	t.Run("store without derive support", func(t *testing.T) {
		m := newMemBatches()
		_, err := NewDeriveStep(m, &fakeRules{rs: rs}, 0).Run(ctx, draftBatch(m), "a", nil)
		require.ErrorIs(t, err, ErrDeriveStoreUnsupported)
	})
}

func TestDeriveStep_OldBatchKeepsSnapshotAfterRuleChange(t *testing.T) {
	ctx := context.Background()
	f := newDeriveFake()
	f.ax[1] = axFor(1, 101)
	oldRS := deriveRuleSet(t, "0.05")
	newRS := deriveRuleSet(t, "0.07")
	require.NotEqual(t, oldRS.Hash(), newRS.Hash())

	oldID := coveredBatch(f, domain.ModeShadow, okLine("POY0000001", "X", 1, "A"))
	_, err := stepAt(f, oldRS).Run(ctx, oldID, "a", nil)
	require.NoError(t, err)
	newID := coveredBatch(f, domain.ModeShadow, okLine("POY0000001", "X", 1, "A"))
	_, err = stepAt(f, newRS).Run(ctx, newID, "a", nil)
	require.NoError(t, err)

	oldSt, newSt := f.state(oldID), f.state(newID)
	assert.Equal(t, oldRS.Hash(), oldSt.RuleHash)
	assert.Equal(t, newRS.Hash(), newSt.RuleHash)
	parsed, err := erprule.ParseCanonical(oldSt.RuleSnapshot)
	require.NoError(t, err)
	assert.Equal(t, oldRS.Hash(), parsed.Hash(), "old snapshot still reads as the old rule set")
	assert.False(t, oldSt.Totals.Equal(*newSt.Totals), "value loss change moves the totals")
}

func TestDeriveTriggerAndExecutor(t *testing.T) {
	ctx := context.Background()
	f := newDeriveFake()
	f.ax[1] = axFor(1, 101)
	id := coveredBatch(f, domain.ModeLive, okLine("POY0000001", "X", 1, "A"))
	jobs := newMemJobs()
	trig := NewStepTriggerHandler(jobs, f, &fakePublisher{}, time.Hour).WithClock(func() time.Time { return testNow })
	exec, err := trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepDerive, Actor: "alice"})
	require.NoError(t, err)

	ex := NewJobExecutor(jobs, f, nil, nil)
	require.ErrorIs(t, ex.Execute(ctx, exec.ID()), ErrUnknownStep, "derive not configured")

	exec, err = trig.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepDerive, Actor: "alice"})
	require.NoError(t, err)
	ex = ex.WithDerive(NewDeriveStep(f, &fakeRules{rs: deriveRuleSet(t, "0.05")}, time.Hour).WithClock(func() time.Time { return testNow }))
	require.NoError(t, ex.Execute(ctx, exec.ID()))
	assert.Equal(t, job.StatusSuccess, exec.Status())
	assert.Equal(t, domain.StatusDerived, f.state(id).Status)
	var res DeriveSummary
	require.NoError(t, json.Unmarshal(exec.ResultSummary(), &res))
	assert.True(t, res.Derived)

	t.Run("trigger refuses DEMAND_LOADED and stale demand", func(t *testing.T) {
		loaded := testNow
		dl := f.put(domain.BatchState{Period: "202608", Status: domain.StatusDemandLoaded, DemandLoadedAt: &loaded, CreatedAt: testNow, UpdatedAt: testNow})
		_, err := trig.Handle(ctx, StepTriggerCommand{BatchID: dl, Step: StepDerive, Actor: "a"})
		require.ErrorIs(t, err, ErrStepNotAllowed)
		late := NewStepTriggerHandler(jobs, f, &fakePublisher{}, time.Hour).WithClock(func() time.Time { return testNow.Add(3 * time.Hour) })
		_, err = late.Handle(ctx, StepTriggerCommand{BatchID: id, Step: StepDerive, Actor: "a"})
		require.ErrorIs(t, err, domain.ErrDemandStale)
	})
}
