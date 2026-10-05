package validation

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erpintegration"
)

func TestDefaultRegistry_Codes(t *testing.T) {
	assert.Equal(t, []erpintegration.IssueCode{
		CodeV01, CodeV02, CodeV03, CodeV04, CodeV05, CodeV06, CodeV07,
		CodeV08, CodeV09, CodeV10, CodeV11, CodeV12,
	}, DefaultRegistry().Codes())
}

func brokenInput(t *testing.T) Input {
	in := cleanInput(t)
	in.Rows[1].Key.ItemCode = "ACY100" // V-08w warning + V-06 (not in replica)
	in.PrevStd[in.Rows[1].Key] = dec("1.45")
	in.Rows[1].StdCost = nd("25")      // V-07 (ACY guarded) + V-05 warning
	in.Rows[0].StdCost = nd("0")       // V-01 + V-07
	delete(in.PrevStd, in.Rows[2].Key) // V-05 info
	in.PostedHeads = nil               // V-10
	return in
}

func TestRegistry_RunDeterministicOrder(t *testing.T) {
	a := Validate(brokenInput(t))
	b := Validate(brokenInput(t))
	require.Equal(t, a, b)

	// Reverse registration order yields the same sorted result.
	rev := NewRegistry(V12(), V11(), V10(), V09(), V08(), V07(), V06(), V05(), V04(), V03(), V02(), V01())
	c := rev.Run(brokenInput(t))
	assert.Equal(t, a.Findings, c.Findings)

	cs := codes(a.Findings)
	for i := 1; i < len(cs); i++ {
		assert.LessOrEqual(t, codeRank(cs[i-1]), codeRank(cs[i]), "order %v", cs)
	}
	assert.Contains(t, cs, CodeV01)
	assert.Contains(t, cs, CodeV05)
	assert.Contains(t, cs, CodeV06)
	assert.Contains(t, cs, CodeV07)
	assert.Contains(t, cs, CodeV08w)
	assert.Contains(t, cs, CodeV10)
}

func TestResult_Accessors(t *testing.T) {
	res := Validate(brokenInput(t))
	assert.True(t, res.HasErrors())
	assert.True(t, res.NeedsAck())
	cnt := res.Counts()
	assert.Equal(t, len(res.Errors()), cnt[SeverityError])
	assert.Equal(t, len(res.Warnings()), cnt[SeverityWarning])
	assert.Equal(t, 1, cnt[SeverityInfo])
	assert.Len(t, res.Infos(), 1)
	assert.Equal(t, 3, res.CountsByCode()[CodeV05]) // row0 -100%, row1 +72%, row2 first-seen
	for _, w := range res.Warnings() {
		assert.True(t, w.RequiresAck())
	}

	ri := res.RowIssues()
	_, hasBatch := ri[erpintegration.ErpKey{}]
	assert.False(t, hasBatch, "batch findings are not row issues")
	assert.NotEmpty(t, ri[res.Warnings()[0].Key])

	h1 := res.WarningSetHash()
	assert.Len(t, h1, 64)
	assert.Equal(t, h1, Validate(brokenInput(t)).WarningSetHash())
	in := brokenInput(t)
	in.PrevStd[in.Rows[1].Key] = dec("1.0") // changes the V-05 message
	assert.NotEqual(t, h1, Validate(in).WarningSetHash())
}

func TestSortFindings_TieBreakers(t *testing.T) {
	k := key("POY1", "AX", "N")
	fs := []Finding{
		{Code: "V-99", Scope: ScopeRow},
		{Code: CodeV07, Scope: ScopePeriodSet, Key: k, HeadSysID: 2},
		{Code: CodeV07, Scope: ScopePeriodSet, Key: k, HeadSysID: 1},
		{Code: CodeV07, Scope: ScopeRow, Key: k, Severity: SeverityWarning, Message: "b"},
		{Code: CodeV07, Scope: ScopeRow, Key: k, Severity: SeverityError, Message: "b"},
		{Code: CodeV07, Scope: ScopeRow, Key: k, Severity: SeverityError, Message: "a"},
		{Code: CodeV01, Scope: ScopeRow, Key: k},
	}
	SortFindings(fs)
	assert.Equal(t, CodeV01, fs[0].Code)
	assert.Equal(t, "a", fs[1].Message)
	assert.Equal(t, SeverityError, fs[2].Severity)
	assert.Equal(t, SeverityWarning, fs[3].Severity)
	assert.Equal(t, int64(1), fs[4].HeadSysID)
	assert.Equal(t, int64(2), fs[5].HeadSysID)
	assert.Equal(t, erpintegration.IssueCode("V-99"), fs[6].Code)
}
