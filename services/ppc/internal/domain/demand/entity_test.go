package demand_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

func validParams() demand.NewParams {
	return demand.NewParams{
		Type:            demand.TypeContract,
		SubType:         demand.SubTypeNewExport,
		Source:          demand.SourceManual,
		CpmProductSysID: 100,
		QtyOriginal:     1000,
		Deadline:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		GradeReq:        demand.GradeReqNone,
		Month:           "2026-08",
		CreatedBy:       7,
	}
}

func TestNew_ValidContract_Succeeds(t *testing.T) {
	d, err := demand.New(validParams())
	require.NoError(t, err)
	assert.Equal(t, demand.StatusPendingConfirmation, d.Status())
	assert.InDelta(t, 1000.0, d.QtyRemaining(), 1e-9)
}

func TestNew_InvalidSubTypeForType_Fails(t *testing.T) {
	p := validParams()
	p.SubType = demand.SubTypeInternal // INTERNAL only valid for MTS
	_, err := demand.New(p)
	assert.ErrorIs(t, err, demand.ErrInvalidSubType)
}

func TestNew_ClausePctRequired(t *testing.T) {
	p := validParams()
	p.GradeReq = demand.GradeReqAXAMClause
	_, err := demand.New(p)
	assert.ErrorIs(t, err, demand.ErrClausePctRequired)
}

func TestNew_NonPositiveQty_Fails(t *testing.T) {
	p := validParams()
	p.QtyOriginal = 0
	_, err := demand.New(p)
	assert.ErrorIs(t, err, demand.ErrInvalidQty)
}

func TestConfirm_LegalTransition(t *testing.T) {
	d, _ := demand.New(validParams())
	require.NoError(t, d.Confirm(9))
	assert.Equal(t, demand.StatusConfirmed, d.Status())
	require.NotNil(t, d.ConfirmedBy())
	assert.Equal(t, int64(9), *d.ConfirmedBy())
}

func TestConfirm_DoubleConfirm_Illegal(t *testing.T) {
	d, _ := demand.New(validParams())
	require.NoError(t, d.Confirm(9))
	// CONFIRMED → CONFIRMED is not a legal transition.
	err := d.Confirm(9)
	assert.ErrorIs(t, err, demand.ErrIllegalTransition)
}

func TestCancel_FromCancelled_Illegal(t *testing.T) {
	d, _ := demand.New(validParams())
	require.NoError(t, d.Cancel()) // PENDING → CANCELLED
	err := d.MarkCarriedOver()     // CANCELLED is terminal
	assert.ErrorIs(t, err, demand.ErrIllegalTransition)
}

func TestApproveMTS_NonMTS_Fails(t *testing.T) {
	d, _ := demand.New(validParams()) // CONTRACT
	err := d.ApproveMTS(true, 5)
	assert.ErrorIs(t, err, demand.ErrNotMTS)
}

func TestApproveMTS_Approves(t *testing.T) {
	p := validParams()
	p.Type = demand.TypeMTS
	p.SubType = demand.SubTypeInternal
	p.Source = demand.SourceManual
	d, err := demand.New(p)
	require.NoError(t, err)
	require.NoError(t, d.ApproveMTS(true, 5))
	assert.Equal(t, demand.StatusConfirmed, d.Status())
	assert.Equal(t, demand.SourceMTSApproved, d.Source())
}

func TestIsCarryCandidate(t *testing.T) {
	d, _ := demand.New(validParams())
	assert.False(t, d.IsCarryCandidate()) // PENDING_CONFIRMATION
	require.NoError(t, d.Confirm(1))
	assert.True(t, d.IsCarryCandidate()) // CONFIRMED with remaining
}
