package shift_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/shift"
)

func TestNewShift_Valid_Succeeds(t *testing.T) {
	s, err := shift.NewShift("1", "Shift 1", "06:00", "14:00", "seeder")
	require.NoError(t, err)
	assert.Equal(t, "1", s.Code())
	assert.Equal(t, "06:00", s.StartTime())
	assert.Equal(t, "14:00", s.EndTime())
	assert.True(t, s.IsActive())
}

func TestNewShift_CrossesMidnight_Succeeds(t *testing.T) {
	// end earlier than start is legal (shift 3: 22:00 -> 06:00 next day).
	s, err := shift.NewShift("3", "Shift 3", "22:00", "06:00", "seeder")
	require.NoError(t, err)
	assert.Equal(t, "22:00", s.StartTime())
	assert.Equal(t, "06:00", s.EndTime())
}

func TestNewShift_InvalidTimes_Fail(t *testing.T) {
	tests := []struct {
		name       string
		start, end string
		wantErr    error
	}{
		{"bad start hour", "24:00", "06:00", shift.ErrInvalidStartTime},
		{"bad start minute", "06:60", "14:00", shift.ErrInvalidStartTime},
		{"non-time start", "6am", "14:00", shift.ErrInvalidStartTime},
		{"empty start", "", "14:00", shift.ErrInvalidStartTime},
		{"bad end", "06:00", "25:00", shift.ErrInvalidEndTime},
		{"empty end", "06:00", "", shift.ErrInvalidEndTime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := shift.NewShift("1", "Shift 1", tt.start, tt.end, "seeder")
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestNewShift_InvalidCode_Fails(t *testing.T) {
	for _, code := range []string{"", "12", "A", "!"} {
		_, err := shift.NewShift(code, "Shift", "06:00", "14:00", "seeder")
		assert.ErrorIs(t, err, shift.ErrInvalidCode, "code %q", code)
	}
}

func TestNewShift_EmptyCreatedBy_Fails(t *testing.T) {
	_, err := shift.NewShift("1", "Shift 1", "06:00", "14:00", "")
	assert.ErrorIs(t, err, shift.ErrEmptyCreatedBy)
}

func TestShiftUpdate_ValidatesTimes(t *testing.T) {
	s, err := shift.NewShift("1", "Shift 1", "06:00", "14:00", "seeder")
	require.NoError(t, err)

	bad := "99:99"
	err = s.Update(nil, &bad, nil, nil, "editor")
	assert.ErrorIs(t, err, shift.ErrInvalidStartTime)

	good := "07:30"
	active := false
	err = s.Update(nil, &good, nil, &active, "editor")
	require.NoError(t, err)
	assert.Equal(t, "07:30", s.StartTime())
	assert.False(t, s.IsActive())
	require.NotNil(t, s.UpdatedBy())
	assert.Equal(t, "editor", *s.UpdatedBy())
}
