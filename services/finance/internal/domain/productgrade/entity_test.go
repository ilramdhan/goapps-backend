package productgrade_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/productgrade"
)

func TestNew_Success(t *testing.T) {
	e, err := productgrade.New("A", "Grade A", "Premium grade", 5.0, 2.0, 0.85, "DETAIL*", "GRADE_A", 100.0, 5.0, nil, nil, "", "admin")
	require.NoError(t, err)
	assert.Equal(t, "A", e.Code())
	assert.Equal(t, "Grade A", e.Name())
	assert.Equal(t, "Premium grade", e.Description())
	assert.Equal(t, 5.0, e.BCPerc())
	assert.Equal(t, 2.0, e.NonStdPerc())
	assert.Equal(t, 0.85, e.BCRecoveryRate())
	assert.Equal(t, "DETAIL*", e.PgDetailProduct())
	assert.Equal(t, "GRADE_A", e.PgGradeLabel())
	assert.Equal(t, 100.0, e.StdSellingPrice())
	assert.Equal(t, 5.0, e.SpValue())
	assert.True(t, e.IsActive())
}

func TestNew_EmptyCode(t *testing.T) {
	_, err := productgrade.New("", "Grade A", "", 0, 0, 0, "", "", 0, 0, nil, nil, "", "admin")
	assert.ErrorIs(t, err, productgrade.ErrEmptyCode)
}

func TestNew_EmptyName(t *testing.T) {
	_, err := productgrade.New("A", "", "", 0, 0, 0, "", "", 0, 0, nil, nil, "", "admin")
	assert.ErrorIs(t, err, productgrade.ErrEmptyName)
}

func TestNew_EmptyCreatedBy(t *testing.T) {
	_, err := productgrade.New("A", "Grade A", "", 0, 0, 0, "", "", 0, 0, nil, nil, "", "")
	assert.ErrorIs(t, err, productgrade.ErrEmptyCreatedBy)
}

func TestSoftDelete_Success(t *testing.T) {
	e, err := productgrade.New("A", "Grade A", "", 0, 0, 0, "", "", 0, 0, nil, nil, "", "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.True(t, e.IsDeleted())
}

func TestSoftDelete_AlreadyDeleted(t *testing.T) {
	e, err := productgrade.New("A", "Grade A", "", 0, 0, 0, "", "", 0, 0, nil, nil, "", "admin")
	require.NoError(t, err)
	require.NoError(t, e.SoftDelete("admin"))
	assert.ErrorIs(t, e.SoftDelete("admin"), productgrade.ErrAlreadyDeleted)
}
