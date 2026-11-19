package mblegacyfill

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResultRowsToRowResultsTargets(t *testing.T) {
	items := []ResultRow{
		{Entity: EntitySpin, ID: "s1", Field: "den", Result: ResultApplied, BeforeJSON: `{}`, AfterJSON: `{}`},
		{Entity: EntitySpin, ID: "s2", Field: "den", Result: ResultFailed},
		{Entity: EntitySpin, ID: "s3", Field: "nope", Result: ResultApplied},
	}
	res := ResultRowsToRowResults(items)
	require.Len(t, res, 3)
	assert.Equal(t, ResultApplied, res[0].Result)
	assert.Equal(t, EntitySpin, res[0].Row.Entity)
	assert.NotEmpty(t, res[0].Row.Spec.Column)
	assert.Empty(t, res[2].Row.Spec.Column)

	ids := map[string]bool{}
	for _, tg := range SelectRecalcTargets(res) {
		ids[tg.SpinID] = true
	}
	assert.False(t, ids["s2"], "failed rows are never recalculated")
	assert.False(t, ids["s3"])
}
