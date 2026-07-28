package planitem_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

func fgParams() planitem.NewParams {
	demandID := int64(10)
	return planitem.NewParams{
		CpmProductSysID: 100,
		Type:            planitem.TypeFGDelivery,
		DemandID:        &demandID,
		QtyTarget:       500,
		Deadline:        time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC),
		RMSource:        planitem.RMSourceStore,
		MachineGroupID:  3,
		Month:           "2026-08",
		CreatedBy:       1,
	}
}

func TestNew_DemandDriven_Succeeds(t *testing.T) {
	pi, err := planitem.New(fgParams())
	require.NoError(t, err)
	assert.Equal(t, planitem.StatusDraft, pi.Status())
}

func TestNew_NeitherDemandNorParent_Fails(t *testing.T) {
	p := fgParams()
	p.DemandID = nil
	_, err := planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrDemandOrParentRequired)
}

func TestNew_BothDemandAndParent_Fails(t *testing.T) {
	p := fgParams()
	parent := int64(7)
	p.ParentItemID = &parent
	_, err := planitem.New(p)
	assert.ErrorIs(t, err, planitem.ErrDemandAndParentSet)
}

func TestNew_ParentDriven_Succeeds(t *testing.T) {
	p := fgParams()
	p.DemandID = nil
	parent := int64(7)
	p.ParentItemID = &parent
	p.Type = planitem.TypeIntermediate
	pi, err := planitem.New(p)
	require.NoError(t, err)
	require.NotNil(t, pi.ParentItemID())
	assert.Equal(t, int64(7), *pi.ParentItemID())
}

func TestUpdate_StatusLegalTransition(t *testing.T) {
	pi, _ := planitem.New(fgParams())
	confirmed := planitem.StatusConfirmed
	changes, err := pi.Update(planitem.UpdateParams{Status: &confirmed})
	require.NoError(t, err)
	assert.Equal(t, planitem.StatusConfirmed, pi.Status())
	require.Len(t, changes, 1)
	assert.Equal(t, "status", changes[0].Field)
}

func TestUpdate_StatusIllegalTransition(t *testing.T) {
	pi, _ := planitem.New(fgParams())
	completed := planitem.StatusCompleted // DRAFT → COMPLETED illegal
	_, err := pi.Update(planitem.UpdateParams{Status: &completed})
	assert.ErrorIs(t, err, planitem.ErrIllegalTransition)
}

func TestUpdate_RecordsFieldChanges(t *testing.T) {
	pi, _ := planitem.New(fgParams())
	newQty := 750.0
	changes, err := pi.Update(planitem.UpdateParams{QtyTarget: &newQty})
	require.NoError(t, err)
	require.Len(t, changes, 1)
	assert.Equal(t, "qty_target", changes[0].Field)
	assert.Equal(t, "500", changes[0].Before)
	assert.Equal(t, "750", changes[0].After)
}
