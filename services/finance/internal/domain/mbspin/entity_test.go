package mbspin_test

import (
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestNew_Success(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	assert.Equal(t, headID, e.HeadID())
	assert.Equal(t, "MB Spin A", e.MgtName())
	assert.True(t, e.IsActive())
	assert.Nil(t, e.CC())
	assert.Nil(t, e.CostRateMkt())
}

func TestNew_InvalidHeadID(t *testing.T) {
	_, err := mbspin.New(uuid.Nil, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, mbspin.ErrInvalidHeadID)
}

func TestNew_EmptyMgtName(t *testing.T) {
	headID := uuid.New()
	_, err := mbspin.New(headID, "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, mbspin.ErrEmptyMgtName)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	headID := uuid.New()
	_, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "")
	assert.ErrorIs(t, err, mbspin.ErrEmptyCreatedBy)
}

func TestUpdate_Success(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "Old Name", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	newName := "New Name"
	err = e.Update(mbspin.UpdateInput{MgtName: &newName}, "editor")
	require.NoError(t, err)
	assert.Equal(t, "New Name", e.MgtName())
}

func TestNew_WithCCAndCostRateMkt(t *testing.T) {
	headID := uuid.New()
	cc := "CC-001"
	rate := 12.5
	e, err := mbspin.New(headID, "MB Spin B", nil, nil, nil, nil, nil, nil, &cc, &rate, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NotNil(t, e.CC())
	assert.Equal(t, "CC-001", *e.CC())
	require.NotNil(t, e.CostRateMkt())
	assert.InDelta(t, 12.5, *e.CostRateMkt(), 0.001)
}

func TestUpdate_CCAndCostRateMkt(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "MB Spin C", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	cc := "CC-002"
	rate := 9.75
	err = e.Update(mbspin.UpdateInput{CC: &cc, CostRateMkt: &rate}, "editor")
	require.NoError(t, err)
	require.NotNil(t, e.CC())
	assert.Equal(t, "CC-002", *e.CC())
	require.NotNil(t, e.CostRateMkt())
	assert.InDelta(t, 9.75, *e.CostRateMkt(), 0.001)
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "Name", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), mbspin.ErrAlreadyDeleted)
}
