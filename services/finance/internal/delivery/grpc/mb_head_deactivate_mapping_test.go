package grpc

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func TestMBHeadWriteErrToBase_DeactivateInUse(t *testing.T) {
	err := fmt.Errorf("wrap: %w", &mbhead.InUseError{UsedBy: []string{"MB-1", "MB-2"}})
	base := mbHeadWriteErrToBase(err)
	assert.Equal(t, "409", base.StatusCode)
	assert.Contains(t, base.Message, "MB-1, MB-2")
}
