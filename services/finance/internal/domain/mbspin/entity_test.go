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
	e, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	assert.Equal(t, headID, e.HeadID())
	assert.Equal(t, "MB Spin A", e.MgtName())
	assert.True(t, e.IsActive())
}

func TestNew_InvalidHeadID(t *testing.T) {
	_, err := mbspin.New(uuid.Nil, "MB Spin A", nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, mbspin.ErrInvalidHeadID)
}

func TestNew_EmptyMgtName(t *testing.T) {
	headID := uuid.New()
	_, err := mbspin.New(headID, "", nil, nil, nil, nil, nil, "admin")
	assert.ErrorIs(t, err, mbspin.ErrEmptyMgtName)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	headID := uuid.New()
	_, err := mbspin.New(headID, "MB Spin A", nil, nil, nil, nil, nil, "")
	assert.ErrorIs(t, err, mbspin.ErrEmptyCreatedBy)
}

func TestUpdate_Success(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "Old Name", nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)

	newName := "New Name"
	err = e.Update(mbspin.UpdateInput{MgtName: &newName}, "editor")
	require.NoError(t, err)
	assert.Equal(t, "New Name", e.MgtName())
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	headID := uuid.New()
	e, err := mbspin.New(headID, "Name", nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), mbspin.ErrAlreadyDeleted)
}
