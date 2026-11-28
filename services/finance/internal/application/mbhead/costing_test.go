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
		wantRename bool
		wantCost   string
	}{
		{name: "absent costing - plain update", incoming: nil, wantCost: "MB001"},
		{name: "unchanged costing - no duplicate check, plain update", incoming: strptr("MB001"), wantCost: "MB001"},
		{name: "trim-equal costing - treated as unchanged", incoming: strptr("  MB001 "), wantCost: "MB001"},
		{name: "renamed to a name another head uses - allowed (names are not unique)", incoming: strptr("MB002"), wantRename: true, wantCost: "MB002"},
		{name: "renamed to free code - rename tx", incoming: strptr("MB003"), wantRename: true, wantCost: "MB003"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := new(MockRepository)
			id := uuid.New()
			repo.On("GetByID", ctx, id).Return(newHeadWithCosting(t, "MB001"), nil)
			switch {
			case tt.wantRename:
				repo.On("UpdateWithCascade", ctx, mock.AnythingOfType("*mbhead.Entity"), "MB001", (*mbheaddomain.ParamSnapshot)(nil)).Return(nil, nil)
			default:
				repo.On("Update", ctx, mock.AnythingOfType("*mbhead.Entity")).Return(nil)
			}

			got, err := mbhead.NewUpdateHandler(repo, nil).Handle(ctx, mbhead.UpdateCommand{
				ID: id, MBCosting: tt.incoming, UpdatedBy: "editor",
			})

			require.NoError(t, err)
			assert.Equal(t, tt.wantCost, got.MBCosting())
			repo.AssertExpectations(t)
		})
	}
}
