package costcalc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	calcdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costcalc"
)

// periodsFakeResultRepo exercises only ListDistinctPeriods; every other method
// panics via the embedded nil interface so an accidental new dependency fails
// loudly instead of silently returning zero values.
type periodsFakeResultRepo struct {
	calcdomain.ResultRepository
	periods []string
	err     error
}

func (f *periodsFakeResultRepo) ListDistinctPeriods(_ context.Context) ([]string, error) {
	return f.periods, f.err
}

func TestPeriodsHandler_Handle_ReturnsPeriods(t *testing.T) {
	t.Parallel()
	repo := &periodsFakeResultRepo{periods: []string{"202606", "202605", "202604"}}
	svc := &Service{resultRepo: repo}
	h := NewPeriodsHandler(svc)

	got, err := h.Handle(context.Background())
	require.NoError(t, err)
	require.Equal(t, []string{"202606", "202605", "202604"}, got)
}

func TestPeriodsHandler_Handle_PropagatesRepoError(t *testing.T) {
	t.Parallel()
	repo := &periodsFakeResultRepo{err: errors.New("db down")}
	svc := &Service{resultRepo: repo}
	h := NewPeriodsHandler(svc)

	_, err := h.Handle(context.Background())
	require.Error(t, err)
}
