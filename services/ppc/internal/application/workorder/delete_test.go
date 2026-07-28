package workorder_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

func TestDelete_DraftWO_Succeeds(t *testing.T) {
	r := newMemRepo()
	wo := seedSourceWO(t, r)
	svc := newSvc(r)

	err := svc.Delete(context.Background(), wo.ID())

	require.NoError(t, err)
}

func TestDelete_NonDraftWO_ReturnsErrNotDeletable(t *testing.T) {
	r := newMemRepo()
	wo := seedSourceWO(t, r)
	require.NoError(t, wo.Submit())
	r.orders[wo.ID()] = wo
	svc := newSvc(r)

	err := svc.Delete(context.Background(), wo.ID())

	require.Error(t, err)
	assert.True(t, errors.Is(err, workorderdomain.ErrNotDeletable))
}

func TestDelete_NotFound_PropagatesError(t *testing.T) {
	r := newMemRepo()
	svc := newSvc(r)

	err := svc.Delete(context.Background(), 999)

	require.Error(t, err)
	assert.True(t, errors.Is(err, workorderdomain.ErrNotFound))
}
