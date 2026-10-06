package costcalc

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResultCurrencyFor(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "USD", ResultCurrencyFor(CalcTypeActual))
	assert.Equal(t, "IDR", ResultCurrencyFor(CalcTypeForecast))
	assert.Equal(t, "IDR", ResultCurrencyFor(CalcTypeSelling))
	assert.Equal(t, "IDR", ResultCurrencyFor(""))
}
