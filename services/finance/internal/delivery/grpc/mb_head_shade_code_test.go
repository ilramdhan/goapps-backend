package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
)

// TestCreateMBHead_ShadeCodeSourcedFromMbhCode reproduces putaran 83 bug #2: the
// ShadeCombobox in the create/edit form binds to mbhCode (the live shade column),
// while mbhShadeCode is a legacy field the UI never populates. The handler must
// read the domain ShadeCode from MbhCode, not from the always-empty MbhShadeCode
// — otherwise the shade shows as "-" in the detail view despite being selected.
func TestCreateMBHead_ShadeCodeSourcedFromMbhCode(t *testing.T) {
	h, repo := newFrozenSealHandler(t)

	resp, err := h.CreateMBHead(context.Background(), &financev1.CreateMBHeadRequest{
		MbhMbCosting: "MB-SHADE-1",
		MbhCode:      strPtr("SH-001"),
		// MbhShadeCode intentionally absent, matching what the real frontend sends.
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	require.NotNil(t, repo.stored)

	assert.Equal(t, "SH-001", repo.stored.ShadeCode(),
		"ShadeCode must be populated from MbhCode, the field the UI actually fills")
	require.NotNil(t, resp.Data)
	assert.Equal(t, "SH-001", resp.Data.ShadeCode)
}

// TestUpdateMBHead_ShadeCodeSourcedFromMbhCode is the update-side counterpart.
func TestUpdateMBHead_ShadeCodeSourcedFromMbhCode(t *testing.T) {
	h, repo := newFrozenSealHandler(t)
	seedFrozenHead(t, repo, "Current")

	resp, err := h.UpdateMBHead(context.Background(), &financev1.UpdateMBHeadRequest{
		MbhId:   repo.stored.ID().String(),
		MbhCode: strPtr("SH-002"),
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Base)
	assert.True(t, resp.Base.IsSuccess)
	require.NotNil(t, repo.stored)

	assert.Equal(t, "SH-002", repo.stored.ShadeCode(),
		"ShadeCode must be populated from MbhCode, the field the UI actually fills")
	require.NotNil(t, resp.Data)
	assert.Equal(t, "SH-002", resp.Data.ShadeCode)
}
