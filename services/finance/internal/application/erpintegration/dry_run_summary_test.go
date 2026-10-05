package erpintegration

import (
	"testing"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func TestBuildDryRunSummary(t *testing.T) {
	rows := []domain.StdRow{
		vRow(vKeyAX, domain.SourceAX, "1.60000", 11),
		vRow(vKeyB, domain.SourceDerived, "1.50000", 11),
		vRow(vKeyMB, domain.SourceMB, "3.00000", 22),
	}
	bad := vRow(domain.ErpKey{ItemCode: "POY9", GradeCode: "A", ShadeCode: "X"}, domain.SourceDerived, "9", 1)
	bad.Status = domain.DeriveNoRule
	rows = append(rows, bad)

	t.Run("no baseline", func(t *testing.T) {
		s := BuildDryRunSummary(rows, nil, nil, 0)
		assert.Equal(t, int64(4), s.Rows)
		assert.Equal(t, int64(3), s.OKRows)
		assert.Equal(t, int64(3), s.FirstSeen)
		assert.Zero(t, s.Compared)
		assert.Nil(t, s.BaselineBatchID)
		assert.Equal(t, "6.10000", s.SumStd)
		assert.Equal(t, "1.50000", s.SumConv)
		assert.Equal(t, "0.30000", s.SumPvl)
		assert.Equal(t, int64(3), s.CountsByBasis["COST"])
		assert.Equal(t, int64(1), s.CountsBySource[string(domain.SourceAX)])
		assert.Empty(t, s.TopDelta)
		assert.Empty(t, s.RuleDiff)
	})

	t.Run("baseline top delta ordered and bounded", func(t *testing.T) {
		bl := &domain.ValidationBaseline{BatchID: 7, Period: "202606", Std: map[domain.ErpKey]decimal.Decimal{
			vKeyAX: dec("1.0"), vKeyB: dec("1.45"), vKeyMB: dec("0"),
		}}
		s := BuildDryRunSummary(rows, bl, nil, 2)
		require.NotNil(t, s.BaselineBatchID)
		assert.Equal(t, int64(7), *s.BaselineBatchID)
		assert.Equal(t, int64(3), s.Compared)
		assert.Zero(t, s.FirstSeen)
		require.Len(t, s.TopDelta, 2)
		assert.Equal(t, vKeyMB.String(), s.TopDelta[0].Key, "|3.0| is the largest")
		assert.Empty(t, s.TopDelta[0].DeltaPct, "zero baseline: unbounded")
		assert.Equal(t, vKeyAX.String(), s.TopDelta[1].Key)
		assert.Equal(t, "0.60000", s.TopDelta[1].Delta)
		assert.Equal(t, "60.00", s.TopDelta[1].DeltaPct)
	})
}

func TestBuildDryRun_RuleDiff(t *testing.T) {
	rs := deriveRuleSet(t, "0.05")
	b, err := domain.ReconstituteBatch(domain.BatchState{
		ID: 1, Period: tPeriod, Seq: 1, Mode: domain.ModeLive, Status: domain.StatusDerived,
		RuleSnapshot: rs.Canonical(), CreatedAt: testNow, CreatedBy: "t", UpdatedAt: testNow, UpdatedBy: "t",
	})
	require.NoError(t, err)

	same := buildDryRun(b, nil, &domain.ValidationBaseline{BatchID: 2, RuleSnapshot: rs.Canonical()})
	assert.Zero(t, same.RuleChanges)
	assert.Empty(t, same.RuleDiffError)

	fresh := buildDryRun(b, nil, nil)
	assert.Positive(t, fresh.RuleChanges, "no baseline: every rule is added")
	assert.Len(t, fresh.RuleDiff, fresh.RuleChanges)

	changed := buildDryRun(b, nil, &domain.ValidationBaseline{BatchID: 2, RuleSnapshot: deriveRuleSet(t, "0.10").Canonical()})
	assert.Equal(t, 1, changed.RuleChanges, "%v", changed.RuleDiff)

	broken := buildDryRun(b, nil, &domain.ValidationBaseline{BatchID: 2, RuleSnapshot: []byte("{not json")})
	assert.NotEmpty(t, broken.RuleDiffError)
	assert.Zero(t, broken.RuleChanges)
}
