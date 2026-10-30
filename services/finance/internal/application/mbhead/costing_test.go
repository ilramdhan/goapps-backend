package mbhead_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/application/mbhead"
	mbheaddomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/mbhead"
)

func newHeadWithCosting(t *testing.T, costing string) *mbheaddomain.Entity {
	t.Helper()
	e, err := mbheaddomain.New(mbheaddomain.NewParams{MBCosting: costing, CreatedBy: "admin"})
	require.NoError(t, err)
	return e
}

func TestUpdateHandler_MBCostingRename(t *testing.T) {
	ctx := context.Background()

	tests := []struct {
		name       string
		incoming   *string
		taken      bool
		wantErr    error
		wantRename bool
		wantCost   string
	}{
		{name: "absent costing - plain update", incoming: nil, wantCost: "MB001"},
		{name: "unchanged costing - no duplicate check, plain update", incoming: strptr("MB001"), wantCost: "MB001"},
		{name: "trim-equal costing - treated as unchanged", incoming: strptr("  MB001 "), wantCost: "MB001"},
		{name: "renamed to taken code - conflict", incoming: strptr("MB002"), taken: true, wantErr: mbheaddomain.ErrAlreadyExists},
		{name: "renamed to free code - rename tx", incoming: strptr("MB003"), wantRename: true, wantCost: "MB003"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepository)
			id := uuid.New()
			repo.On("GetByID", ctx, id).Return(newHeadWithCosting(t, "MB001"), nil)
			changed := tt.incoming != nil && tt.wantCost != "MB001" || tt.wantErr != nil
			if changed {
				repo.On("ExistsByMBCosting", ctx, *tt.incoming).Return(tt.taken, nil)
			}
			switch {
			case tt.wantErr != nil:
			case tt.wantRename:
				repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", (*mbheaddomain.ParamSnapshot)(nil)).Return(nil, nil)
			default:
				repo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
			}

			got, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{
				ID: id, MBCosting: tt.incoming, UpdatedBy: "editor",
			})

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				repo.AssertNotCalled(t, "Update", mock.Anything, mock.Anything)
				repo.AssertNotCalled(t, "UpdateWithCascade", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantCost, got.MBCosting())
			}
			if !changed {
				repo.AssertNotCalled(t, "ExistsByMBCosting", mock.Anything, mock.Anything)
			}
			repo.AssertExpectations(t)
		})
	}
}
