package evaluator

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestRunWithDiag_NaN covers the math.IsNaN branch directly. Before K-35 no
// test exercised a pure 0/0 NaN — the existing div-by-zero test produces +Inf,
// which took the IsInf branch instead.
func TestRunWithDiag_NaN(t *testing.T) {
	ev, err := Compile("F_NAN", "a / b")
	require.NoError(t, err)

	got, kind, runErr := ev.RunWithDiag(map[string]any{"a": 0.0, "b": 0.0})
	require.NoError(t, runErr, "NaN is swallowed, not surfaced as an error")
	require.Equal(t, NonFiniteNaN, kind)
	require.Equal(t, float64(0), got)
}

// TestRunWithDiag_NegInf pins the sign-losing case: -1/0 is -Inf, and the
// conversion turns it into +0. The kind is the only surviving evidence of the
// sign.
func TestRunWithDiag_NegInf(t *testing.T) {
	ev, err := Compile("F_NEG_INF", "a / b")
	require.NoError(t, err)

	got, kind, runErr := ev.RunWithDiag(map[string]any{"a": -1.0, "b": 0.0})
	require.NoError(t, runErr)
	require.Equal(t, NonFiniteNegInf, kind)
	require.Equal(t, float64(0), got)
	require.False(t, math.Signbit(got), "the fabricated zero is +0, so the negative sign is gone")
}

// TestRunWithDiag_PosInf covers x/0 with a positive numerator.
func TestRunWithDiag_PosInf(t *testing.T) {
	ev, err := Compile("F_POS_INF", "a / b")
	require.NoError(t, err)

	got, kind, runErr := ev.RunWithDiag(map[string]any{"a": 1.0, "b": 0.0})
	require.NoError(t, runErr)
	require.Equal(t, NonFinitePosInf, kind)
	require.Equal(t, float64(0), got)
}

// TestRunWithDiag_Overflow covers overflow to +Inf via multiplication rather
// than division — the same conversion applies.
func TestRunWithDiag_Overflow(t *testing.T) {
	ev, err := Compile("F_OVERFLOW", "a * 10.0")
	require.NoError(t, err)

	got, kind, runErr := ev.RunWithDiag(map[string]any{"a": 1e308})
	require.NoError(t, runErr)
	require.Equal(t, NonFinitePosInf, kind)
	require.Equal(t, float64(0), got)
}

// TestRunWithDiag_FiniteResultsUnchanged is the numeric-regression guard for
// K-35: for every finite case, RunWithDiag must return the exact same float64
// as Run, with no non-finite kind. Bit-for-bit equality, not a delta — the
// premise of this change is that no costing number moves.
func TestRunWithDiag_FiniteResultsUnchanged(t *testing.T) {
	cases := []struct {
		name  string
		expr  string
		scope map[string]any
		want  float64
	}{
		{"add", "a + b", map[string]any{"a": 1.5, "b": 2.25}, 3.75},
		{"div", "a / b", map[string]any{"a": 10.0, "b": 4.0}, 2.5},
		{"chain", "a * b - c / d", map[string]any{"a": 3.0, "b": 7.0, "c": 9.0, "d": 2.0}, 16.5},
		{"int_input", "a + 1", map[string]any{"a": 41}, 42},
		{"zero_result", "a - b", map[string]any{"a": 5.0, "b": 5.0}, 0},
		{"negative", "0.0 - a", map[string]any{"a": 3.0}, -3},
		{"conditional_zero_branch", "b > 0 ? a / b : 0", map[string]any{"a": 1.0, "b": 0.0}, 0},
		{"tiny_exact_binary", "a * b", map[string]any{"a": 0x1p-300, "b": 0.5}, 0x1p-301},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ev, err := Compile("F_REG", tc.expr)
			require.NoError(t, err)

			viaRun, runErr := ev.Run(tc.scope)
			require.NoError(t, runErr)

			viaDiag, kind, diagErr := ev.RunWithDiag(tc.scope)
			require.NoError(t, diagErr)

			require.Equal(t, NonFiniteNone, kind, "finite input must not be classified non-finite")
			require.Equal(t, tc.want, viaDiag)
			require.Equal(t, viaRun, viaDiag, "Run and RunWithDiag must agree bit-for-bit")
		})
	}
}

// TestClassifyNonFinite covers the classifier in isolation, including that
// +0 and -0 are both finite.
func TestClassifyNonFinite(t *testing.T) {
	require.Equal(t, NonFiniteNaN, classifyNonFinite(math.NaN()))
	require.Equal(t, NonFinitePosInf, classifyNonFinite(math.Inf(1)))
	require.Equal(t, NonFiniteNegInf, classifyNonFinite(math.Inf(-1)))
	require.Equal(t, NonFiniteNone, classifyNonFinite(0))
	require.Equal(t, NonFiniteNone, classifyNonFinite(math.Copysign(0, -1)))
	require.Equal(t, NonFiniteNone, classifyNonFinite(math.MaxFloat64))
}
