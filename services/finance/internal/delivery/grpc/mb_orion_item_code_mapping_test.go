package grpc

import (
	"fmt"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

func TestMBHeadEntityToProto_OrionItemCode(t *testing.T) {
	code := "ORI-1"
	e, err := mbhead.New(mbhead.NewParams{MBCosting: "MB001", CreatedBy: "admin", OrionItemCode: &code})
	require.NoError(t, err)
	assert.Equal(t, "ORI-1", mbHeadEntityToProto(e).MbhOrionItemCode)

	e, err = mbhead.New(mbhead.NewParams{MBCosting: "MB002", CreatedBy: "admin"})
	require.NoError(t, err)
	assert.Empty(t, mbHeadEntityToProto(e).MbhOrionItemCode)
}

func TestMBHeadWriteErrToBase_DuplicateOrionItemCodeIsConflict(t *testing.T) {
	base := mbHeadWriteErrToBase(fmt.Errorf("%w: %q", mbhead.ErrDuplicateOrionItemCode, "ORI-1"))
	assert.Equal(t, "409", base.StatusCode)
	assert.False(t, base.IsSuccess)
	assert.Equal(t, "Orion Item Code already exists", base.Message)
}

func TestMBSpinEntityToProto_OrionItemCode(t *testing.T) {
	code := "ORI-SPIN"
	e, err := mbspin.New(uuid.New(), "SPIN", nil, &code, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, "admin")
	require.NoError(t, err)
	assert.Equal(t, "ORI-SPIN", mbSpinEntityToProto(e).MbsOrionItemCode)
}
