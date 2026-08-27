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
	assert.Equal(t, "MB Head A", *e.MgtName())
	assert.True(t, e.IsActive())
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
