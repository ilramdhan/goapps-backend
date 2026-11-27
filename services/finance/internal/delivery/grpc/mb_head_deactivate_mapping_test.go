package grpc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestMBHeadWriteErrToBase_DeactivateInUse(t *testing.T) {
	err := fmt.Errorf("wrap: %w", &mbhead.InUseError{UsedBy: []string{"MB-1", "MB-2"}})
	base := mbHeadWriteErrToBase(err)
	assert.Equal(t, "409", base.StatusCode)
	assert.Contains(t, base.Message, "MB-1, MB-2")
}

func TestMBHeadWriteErrToBase_ReactivateAndShadeFg(t *testing.T) {
	assert.Equal(t, "409", mbHeadWriteErrToBase(mbhead.ErrReactivateSuperseded).StatusCode)
	assert.Equal(t, "409", mbHeadWriteErrToBase(mbspin.ErrSpinShadeFgExists).StatusCode)
}

func TestInUseErrorCapsList(t *testing.T) {
	var u []string
	for i := 0; i < 13; i++ {
		u = append(u, fmt.Sprintf("M%d", i))
	}
	msg := (&mbhead.InUseError{UsedBy: u}).Error()
	assert.Contains(t, msg, "M9")
	assert.NotContains(t, msg, "M10")
	assert.Contains(t, msg, "+3 more")
}
