package lookupsync

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// dupNameHeadRepo reports N heads for any name; GetByMBCosting must never be reached.
type dupNameHeadRepo struct {
	mbhead.Repository
	n int
}

func (r dupNameHeadRepo) CountByMBCosting(context.Context, string) (int, error) { return r.n, nil }

func TestFillFromMBHead_AmbiguousName_ReturnsErrMBCostingAmbiguous(t *testing.T) {
	r := &Resolver{mbHeadRepo: dupNameHeadRepo{n: 2}}
	_, err := r.fillFromMBHead(context.Background(), "DUP", "MB_CODE")
	require.ErrorIs(t, err, mbhead.ErrMBCostingAmbiguous)
}
