package grpc

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// TestMBSpinEntityToProto_VSNumber proves mbs_vs_number round-trips from the
// domain entity onto the response proto (migration 000414, wired putaran 83).
func TestMBSpinEntityToProto_VSNumber(t *testing.T) {
	e := newSpinWithFlags(t, nil, nil)
	vs := "NA"
	e.HydrateVSNumber(&vs)

	p := mbSpinEntityToProto(e)

	require.NotNil(t, p.MbsVsNumber)
	assert.Equal(t, "NA", *p.MbsVsNumber)
}

// TestMBSpinEntityToProto_VSNumber_AbsentStaysAbsent guards against nil being
// coerced into an empty string on the wire.
func TestMBSpinEntityToProto_VSNumber_AbsentStaysAbsent(t *testing.T) {
	e := newSpinWithFlags(t, nil, nil)

	p := mbSpinEntityToProto(e)

	assert.Nil(t, p.MbsVsNumber)
}

// TestCreateMBSpin_VSNumberWiring proves the create request path carries
// mbs_vs_number through to the domain entity.
func TestCreateMBSpin_VSNumberWiring(t *testing.T) {
	vs := "NA"
	repo := &captureRepo{}
	h, err := NewMBSpinHandler(repo)
	require.NoError(t, err)

	resp, err := h.CreateMBSpin(context.Background(), &financev1.CreateMBSpinRequest{
		MbhId:       uuid.New().String(),
		MbsMgtName:  "SPIN-VS",
		MbsVsNumber: &vs,
	})
	require.NoError(t, err)
	require.True(t, resp.Base.IsSuccess, resp.Base.Message)
	require.NotNil(t, repo.created)
	require.NotNil(t, repo.created.VSNumber())
	assert.Equal(t, "NA", *repo.created.VSNumber())
	require.NotNil(t, resp.Data.MbsVsNumber)
	assert.Equal(t, "NA", *resp.Data.MbsVsNumber)
}

// TestUpdateMBSpin_VSNumberWiring proves the update path forwards mbs_vs_number,
// and that omitting it leaves the stored value untouched.
func TestUpdateMBSpin_VSNumberWiring(t *testing.T) {
	t.Run("explicit value updates the stored VS number", func(t *testing.T) {
		repo := &captureRepo{stored: newSpinWithFlags(t, nil, nil)}
		h, err := NewMBSpinHandler(repo)
		require.NoError(t, err)

		vs := "VS-123"
		resp, err := h.UpdateMBSpin(context.Background(), &financev1.UpdateMBSpinRequest{
			MbhId:       uuid.New().String(),
			MbsId:       uuid.New().String(),
			MbsVsNumber: &vs,
		})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.NotNil(t, repo.updated)
		require.NotNil(t, repo.updated.VSNumber())
		assert.Equal(t, "VS-123", *repo.updated.VSNumber())
	})

	t.Run("omitted VS number leaves the stored value untouched", func(t *testing.T) {
		existing := "NA"
		stored := newSpinWithFlags(t, nil, nil)
		stored.HydrateVSNumber(&existing)
		repo := &captureRepo{stored: stored}
		h, err := NewMBSpinHandler(repo)
		require.NoError(t, err)

		resp, err := h.UpdateMBSpin(context.Background(), &financev1.UpdateMBSpinRequest{
			MbhId: uuid.New().String(),
			MbsId: uuid.New().String(),
		})
		require.NoError(t, err)
		require.True(t, resp.Base.IsSuccess, resp.Base.Message)
		require.NotNil(t, repo.updated)
		require.NotNil(t, repo.updated.VSNumber(), "omitting mbs_vs_number must not clear it")
		assert.Equal(t, "NA", *repo.updated.VSNumber())
	})
}
