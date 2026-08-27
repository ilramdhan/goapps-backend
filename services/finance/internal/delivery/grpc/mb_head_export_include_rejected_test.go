package grpc

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	appmbhead "github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

// captureExportMBHeadRepo embeds fakeFrozenMBHeadRepo (defined in
// mb_head_frozen_check_status_test.go, same package) to get a full mbhead.Repository for
// free, and shadows only ListAll to capture the filter ExportMBHeads hands down — proving
// the new proto field `include_rejected` reaches mbhead.ExportFilter.IncludeRejected.
type captureExportMBHeadRepo struct {
	fakeFrozenMBHeadRepo
	lastFilter mbhead.ExportFilter
}

func (c *captureExportMBHeadRepo) ListAll(_ context.Context, filter mbhead.ExportFilter) ([]*mbhead.Entity, error) {
	c.lastFilter = filter
	return nil, nil
}

// captureRecipeFullReader captures the filter ExportMBRecipeFull hands down, proving the
// new proto field `include_rejected` reaches appmbhead.RecipeFullFilter.IncludeRejected.
type captureRecipeFullReader struct {
	lastFilter appmbhead.RecipeFullFilter
}

func (c *captureRecipeFullReader) ListRecipeFullRows(
	_ context.Context, filter appmbhead.RecipeFullFilter,
) ([]appmbhead.RecipeFullRow, error) {
	c.lastFilter = filter
	return nil, nil
}

// TestMBHeadHandler_ExportMBHeads_IncludeRejectedFilter covers both regression cases for
// the new include_rejected proto field: a request that omits it must forward false (the
// safe default that excludes REJECTED documents — proto3 zero value), and a request that
// sets it true must forward true verbatim.
func TestMBHeadHandler_ExportMBHeads_IncludeRejectedFilter(t *testing.T) {
	t.Run("absent field means false (rejected excluded)", func(t *testing.T) {
		repo := &captureExportMBHeadRepo{}
		h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
		require.NoError(t, err)

		_, err = h.ExportMBHeads(context.Background(), &financev1.ExportMBHeadsRequest{})
		require.NoError(t, err)
		require.False(t, repo.lastFilter.IncludeRejected)
	})

	t.Run("include_rejected=true is forwarded verbatim", func(t *testing.T) {
		repo := &captureExportMBHeadRepo{}
		h, err := NewMBHeadHandler(repo, nil, fakeMBMachineRepo{})
		require.NoError(t, err)

		_, err = h.ExportMBHeads(context.Background(), &financev1.ExportMBHeadsRequest{
			IncludeRejected: true,
		})
		require.NoError(t, err)
		require.True(t, repo.lastFilter.IncludeRejected)
	})
}

// TestMBHeadHandler_ExportMBRecipeFull_IncludeRejectedFilter is the same pair of cases for
// the denormalized full-recipe export (P12), which forwards through
// appmbhead.RecipeFullFilter.IncludeRejected instead.
func TestMBHeadHandler_ExportMBRecipeFull_IncludeRejectedFilter(t *testing.T) {
	t.Run("absent field means false (rejected excluded)", func(t *testing.T) {
		repo := &fakeFrozenMBHeadRepo{}
		reader := &captureRecipeFullReader{}
		h, err := NewMBHeadHandlerWithRecipeFull(repo, nil, fakeMBMachineRepo{}, nil, reader)
		require.NoError(t, err)

		_, err = h.ExportMBRecipeFull(context.Background(), &financev1.ExportMBRecipeFullRequest{})
		require.NoError(t, err)
		require.False(t, reader.lastFilter.IncludeRejected)
	})

	t.Run("include_rejected=true is forwarded verbatim", func(t *testing.T) {
		repo := &fakeFrozenMBHeadRepo{}
		reader := &captureRecipeFullReader{}
		h, err := NewMBHeadHandlerWithRecipeFull(repo, nil, fakeMBMachineRepo{}, nil, reader)
		require.NoError(t, err)

		_, err = h.ExportMBRecipeFull(context.Background(), &financev1.ExportMBRecipeFullRequest{
			IncludeRejected: true,
		})
		require.NoError(t, err)
		require.True(t, reader.lastFilter.IncludeRejected)
	})
}
