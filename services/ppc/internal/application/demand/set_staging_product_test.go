package demand_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// stubValidator is a scripted ProductValidator.
type stubValidator struct {
	err  error
	seen []int64
}

func (v *stubValidator) ValidateProduct(_ context.Context, sysID int64) error {
	v.seen = append(v.seen, sysID)
	return v.err
}

func TestSetStagingProduct_ValidatesThenPersistsAsManual(t *testing.T) {
	repo := &resolveRepo{}
	validator := &stubValidator{}
	svc := demandapp.NewService(repo, validator, nil)

	row, err := svc.SetStagingProduct(context.Background(), demandapp.SetStagingProductCommand{
		SosID:           84706,
		CpmProductSysID: 97073,
	})

	require.NoError(t, err)
	assert.Equal(t, []int64{97073}, validator.seen)
	assert.Equal(t, [][2]int64{{84706, 97073}}, repo.setProductCalls)
	require.NotNil(t, row.CpmProductSysID)
	assert.Equal(t, int64(97073), *row.CpmProductSysID)
	assert.Equal(t, demanddomain.MatchStatusManual, row.MatchStatus)
}

// The write path must fail closed: an unknown or inactive product never reaches
// the database.
func TestSetStagingProduct_RejectsInvalidProduct(t *testing.T) {
	repo := &resolveRepo{}
	validator := &stubValidator{err: errors.New("invalid product: not found")}
	svc := demandapp.NewService(repo, validator, nil)

	_, err := svc.SetStagingProduct(context.Background(), demandapp.SetStagingProductCommand{
		SosID:           84706,
		CpmProductSysID: 12345,
	})

	require.Error(t, err)
	assert.Empty(t, repo.setProductCalls)
}

func TestSetStagingProduct_RejectsNonPositiveProduct(t *testing.T) {
	repo := &resolveRepo{}
	validator := &stubValidator{}
	svc := demandapp.NewService(repo, validator, nil)

	_, err := svc.SetStagingProduct(context.Background(), demandapp.SetStagingProductCommand{SosID: 1})

	require.ErrorIs(t, err, demanddomain.ErrInvalidProduct)
	assert.Empty(t, validator.seen)
	assert.Empty(t, repo.setProductCalls)
}

func TestSetStagingProduct_PropagatesRepoError(t *testing.T) {
	repo := &resolveRepo{setProductErr: demanddomain.ErrStagingNotUpdatable}
	svc := demandapp.NewService(repo, &stubValidator{}, nil)

	_, err := svc.SetStagingProduct(context.Background(), demandapp.SetStagingProductCommand{
		SosID:           1,
		CpmProductSysID: 97073,
	})

	require.ErrorIs(t, err, demanddomain.ErrStagingNotUpdatable)
}
