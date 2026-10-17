package postgres

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/postgres/rmratioquery"
)

func TestToDomainDenierOverrides(t *testing.T) {
	assert.Nil(t, toDomainDenierOverrides(nil))
	assert.Nil(t, toDomainDenierOverrides(map[int64]rmratioquery.DenierOverrides{}))

	got := toDomainDenierOverrides(map[int64]rmratioquery.DenierOverrides{
		5: {Product: map[int64]float64{7: 30}, Group: map[string]float64{"SPD1": 12}},
	})
	assert.Equal(t, map[int64]costroute.DenierOverrides{
		5: {Product: map[int64]float64{7: 30}, Group: map[string]float64{"SPD1": 12}},
	}, got)
}
