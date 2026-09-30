package costcalc

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/costcalc/evaluator"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/yarntxweight"
)

var txWeightGrades = []string{"AE", "A9", "A", "B", "C"}

func TestTxWeight_Modes(t *testing.T) {
	rules := map[string]TxWeightRule{
		"AE": {Mode: yarntxweight.ModeLessBy, Value: 0.5},
		"A9": {Mode: yarntxweight.ModeMultiply, Value: 0.25},
		"A":  {Mode: yarntxweight.ModeFixed, Value: 1.75},
	}
	const axWt, fallback = 2.0, 9.0

	assert.InDelta(t, 1.5, txWeight(rules, "AE", axWt, fallback), 1e-12, "LESS_BY = axWt - value")
	assert.InDelta(t, 0.5, txWeight(rules, "A9", axWt, fallback), 1e-12, "MULTIPLY = axWt * value")
	assert.InDelta(t, 1.75, txWeight(rules, "A", axWt, fallback), 1e-12, "FIXED = value")
	assert.InDelta(t, 1.5, txWeight(rules, " ae ", axWt, fallback), 1e-12, "grade is trimmed/case-insensitive")
	assert.InDelta(t, fallback, txWeight(rules, "B", axWt, fallback), 1e-12, "no rule -> fallback")
}

func TestTxWeight_FallbackEdgeCases(t *testing.T) {
	assert.InDelta(t, 3.0, txWeight(nil, "AE", 2.0, 3.0), 1e-12, "nil rules -> fallback")
	rules := map[string]TxWeightRule{"AE": {Mode: yarntxweight.ModeFixed, Value: 1}}
	assert.InDelta(t, 3.0, txWeight(rules, 42, 2.0, 3.0), 1e-12, "non-string grade -> fallback")
	assert.InDelta(t, 3.0, txWeight(rules, "AE", "x", 3.0), 1e-12, "non-numeric axWt -> fallback")
	assert.InDelta(t, 3.0, txWeight(rules, "AE", nil, 3), 1e-12, "int fallback accepted")
	assert.InDelta(t, 0.0, txWeight(rules, "AE"), 1e-12, "too few args -> 0")
	assert.InDelta(t, 1.0, txWeight(rules, "AE", 5), 1e-12, "missing fallback still applies rule (FIXED=1)")
}

// TestTxWeight_ThroughEvaluator proves the built-in is callable from a real
// expr-lang program in the exact shape migration 000530 writes, with and
// without a rule, via buildInitialScope (the production injection point).
func TestTxWeight_ThroughEvaluator(t *testing.T) {
	cache := evaluator.NewCache()
	capp := map[string]float64{"AX_WT": 2.0, "AX_PERC": 80, "AE_PERC": 4, "A9_PERC": 2, "A_PERC": 8, "B_PERC": 4, "C_PERC": 2}

	for _, tc := range []struct {
		name  string
		rules map[string]TxWeightRule
		want  map[string]float64
	}{
		{
			name:  "no rules keeps ratio formula",
			rules: nil,
			want:  map[string]float64{"AE": 0.1, "A9": 0.05, "A": 0.2, "B": 0.1, "C": 0.05},
		},
		{
			name: "rules override per grade",
			rules: map[string]TxWeightRule{
				"AE": {Mode: yarntxweight.ModeLessBy, Value: 1.9},
				"B":  {Mode: yarntxweight.ModeMultiply, Value: 0.1},
				"C":  {Mode: yarntxweight.ModeFixed, Value: 0.3},
			},
			want: map[string]float64{"AE": 0.1, "A9": 0.05, "A": 0.2, "B": 0.2, "C": 0.3},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			scope, _ := buildInitialScope(ComputeInput{CAPP: capp, TxWeight: tc.rules})
			for _, g := range txWeightGrades {
				ev, err := cache.GetOrCompile("F_YARN_"+g+"_WT", txWeightExpression(g))
				require.NoError(t, err)
				got, err := ev.Run(scope)
				require.NoError(t, err)
				assert.InDelta(t, tc.want[g], got, 1e-9, "grade %s", g)
			}
		})
	}
}

// txWeightExpression is the post-000530 expression (SQL quoting removed).
func txWeightExpression(g string) string {
	return "tx_weight('" + g + "', AX_WT, AX_WT * " + g + "_PERC / AX_PERC)"
}

// TestMigration000530_RewiresGradeWeightFormulas asserts the migration text
// matches the expressions the tests above evaluate, and that both directions
// are guarded on the exact prior expression plus the marker.
func TestMigration000530_RewiresGradeWeightFormulas(t *testing.T) {
	const upPath = "../../../migrations/postgres/000530_rewire_grade_weight_formulas_tx_weight.up.sql"
	const downPath = "../../../migrations/postgres/000530_rewire_grade_weight_formulas_tx_weight.down.sql"
	upRaw, err := os.ReadFile(upPath)
	require.NoError(t, err)
	downRaw, err := os.ReadFile(downPath)
	require.NoError(t, err)
	up, down := stripSQLComments(string(upRaw)), stripSQLComments(string(downRaw))

	for _, g := range txWeightGrades {
		oldExpr := "'AX_WT * " + g + "_PERC / AX_PERC'"
		newExpr := "'tx_weight(''" + g + "'', AX_WT, AX_WT * " + g + "_PERC / AX_PERC)'"
		code := "'F_YARN_" + g + "_WT'"

		assert.Contains(t, up, "SET expression = "+newExpr, "up sets %s", code)
		assert.Contains(t, up, "AND expression = "+oldExpr, "up guarded on 000408 text for %s", code)
		assert.Contains(t, up, "formula_code = "+code)
		assert.Contains(t, down, "SET expression = "+oldExpr, "down restores 000408 text for %s", code)
		assert.Contains(t, down, "AND expression = "+newExpr, "down guarded on rewired text for %s", code)
	}
	assert.Contains(t, up, "updated_by = 'tx_weight_000530'")
	assert.Contains(t, down, "updated_by = 'tx_weight_000530'")
	assert.Contains(t, up, "RAISE NOTICE")
	assert.Contains(t, up, "COMMIT;")
	assert.Contains(t, down, "COMMIT;")
}
