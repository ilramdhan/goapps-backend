package mbhead_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func TestNew_Success(t *testing.T) {
	name := "MB Head A"
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MBH-2024-001", MgtName: &name, CreatedBy: "admin"})
	require.NoError(t, err)
	assert.Equal(t, "MBH-2024-001", e.MBCosting())
	assert.NotNil(t, e.MgtName())
	assert.Equal(t, "MBH-2024-001", *e.MgtName(), "name must equal costing, input ignored")
	assert.True(t, e.IsActive())
}

func TestNew_NilMgtNameDefaultsToCosting(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MBH-X", CreatedBy: "admin"})
	require.NoError(t, err)
	require.NotNil(t, e.MgtName())
	assert.Equal(t, "MBH-X", *e.MgtName())
}

func TestUpdate_NameFollowsCosting(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "OLD", CreatedBy: "admin"})
	require.NoError(t, err)
	other, next := "other", "NEW"
	require.NoError(t, e.Update(mbhead.UpdateInput{MgtName: &other}, "u"))
	assert.Equal(t, "OLD", *e.MgtName())
	require.NoError(t, e.Update(mbhead.UpdateInput{MBCosting: &next}, "u"))
	assert.Equal(t, "NEW", *e.MgtName())
}

func TestNew_EmptyMBCosting(t *testing.T) {
	_, err := mbhead.New(mbhead.NewParams{MBCosting: "", CreatedBy: "admin"})
	assert.ErrorIs(t, err, mbhead.ErrEmptyMBCosting)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	_, err := mbhead.New(mbhead.NewParams{MBCosting: "MBH-2024-001", CreatedBy: ""})
	assert.ErrorIs(t, err, mbhead.ErrEmptyCreatedBy)
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MBH-2024-001", CreatedBy: "admin"})
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), mbhead.ErrAlreadyDeleted)
}
