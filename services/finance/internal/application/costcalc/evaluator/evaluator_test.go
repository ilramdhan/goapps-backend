package evaluator

import (
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestEvaluator_BasicArithmetic(t *testing.T) {
	ev, err := Compile("F1", "(a + b) * c")
	require.NoError(t, err)
	out, err := ev.Run(map[string]any{"a": 2.0, "b": 3.0, "c": 4.0})
	require.NoError(t, err)
	require.Equal(t, 20.0, out)
}

func TestEvaluator_IntegersCoerced(t *testing.T) {
	ev, err := Compile("F2", "a + b")
	require.NoError(t, err)
	out, err := ev.Run(map[string]any{"a": 5, "b": 7})
	require.NoError(t, err)
	require.Equal(t, 12.0, out)
}

func TestEvaluator_PercentExpression(t *testing.T) {
	// Realistic: cost + waste percentage
	ev, err := Compile("FCOST", "RM_COST * (1 + WASTE_PCT / 100)")
	require.NoError(t, err)
	out, err := ev.Run(map[string]any{"RM_COST": 1000.0, "WASTE_PCT": 5.0})
	require.NoError(t, err)
	require.InDelta(t, 1050.0, out, 0.0001)
}

func TestEvaluator_RejectsForbidden(t *testing.T) {
	for _, expr := range []string{
		"os.Args[0]",
		"exec.Command(\"x\")",
		"file.Open(\"/etc/passwd\")",
		"a + syscall.Foo",
	} {
		t.Run(expr, func(t *testing.T) {
			_, err := Compile("BAD", expr)
			require.ErrorIs(t, err, ErrUnsafeFunction)
		})
	}
}

func TestEvaluator_DivByZeroReturnsError(t *testing.T) {
	ev, err := Compile("F3", "a / b")
	require.NoError(t, err)
	// expr's float division of x/0 yields +Inf rather than a runtime error,
	// so Run() guards against non-finite results explicitly.
	_, err = ev.Run(map[string]any{"a": 1.0, "b": 0.0})
	require.ErrorIs(t, err, ErrNonFiniteResult)
}

func TestEvaluator_UndefinedVariableYieldsError(t *testing.T) {
	ev, err := Compile("F4", "a + missing")
	require.NoError(t, err)
	// AllowUndefinedVariables means compile passes, but at runtime the missing
	// var is nil → arithmetic with nil fails.
	_, err = ev.Run(map[string]any{"a": 1.0})
	require.Error(t, err)
}

func TestCache_CompilesOnce(t *testing.T) {
	c := NewCache()
	ev1, err := c.GetOrCompile("F1", "a + b")
	require.NoError(t, err)
	ev2, err := c.GetOrCompile("F1", "a + b")
	require.NoError(t, err)
	require.Same(t, ev1, ev2)
	require.Equal(t, 1, c.Size())
}

func TestCache_DifferentKeysDifferentEntries(t *testing.T) {
	c := NewCache()
	_, _ = c.GetOrCompile("F1", "a + b")
	_, _ = c.GetOrCompile("F2", "a + b") // different formulaCode → different entry
	_, _ = c.GetOrCompile("F1", "a * b") // same code, different expression → different entry
	require.Equal(t, 3, c.Size())
}

func TestCache_CompileErrorNotCached(t *testing.T) {
	c := NewCache()
	_, err := c.GetOrCompile("BAD", "os.Args")
	require.ErrorIs(t, err, ErrUnsafeFunction)
	require.Equal(t, 0, c.Size())
}

func TestCache_ConcurrentAccess(t *testing.T) {
	c := NewCache()
	var wg sync.WaitGroup
	for range 50 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.GetOrCompile("F1", "a + b")
			require.NoError(t, err)
		}()
	}
	wg.Wait()
	require.Equal(t, 1, c.Size())
}
