package costproductmaster_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

// fakeRMModeRepo records SaveRMRatioMode writes.
type fakeRMModeRepo struct {
	writes []domain.RMRatioModeWrite
	err    error
}

func (f *fakeRMModeRepo) SaveRMRatioMode(_ context.Context, w domain.RMRatioModeWrite) error {
	if f.err != nil {
		return f.err
	}
	f.writes = append(f.writes, w)
	return nil
}

var _ domain.RMRatioModeRepository = (*fakeRMModeRepo)(nil)

func rmModeFixture(override, effective string) (*linkProductRepo, *fakeRMModeRepo, *fakeAuditSink, *app.UpdateRMRatioModeHandler) {
	p := linkProduct(true, linkTypeYarn, "AX", "", "")
	p.RestoreRMRatioMode(override, effective)
	repo := &linkProductRepo{p: p}
	modes := &fakeRMModeRepo{}
	audit := &fakeAuditSink{}
	h := app.NewUpdateRMRatioModeHandler(repo, modes, audit).WithClock(func() time.Time { return linkNow })
	return repo, modes, audit, h
}

func TestUpdateRMRatioModeHandler_SetDenier(t *testing.T) {
	_, modes, audit, h := rmModeFixture("", "INPUT")
	p, err := h.Handle(context.Background(), app.UpdateRMRatioModeCommand{ProductSysID: 77, Mode: "DENIER", ActorUserID: "u1"})
	require.NoError(t, err)
	assert.Equal(t, "DENIER", p.RMRatioModeOverride())
	require.Len(t, modes.writes, 1)
	assert.Equal(t, domain.RMRatioModeWrite{ProductSysID: 77, Override: "DENIER", UpdatedAt: linkNow, UpdatedBy: "u1"}, modes.writes[0])

	require.Len(t, audit.rows, 1)
	row := audit.rows[0]
	assert.Equal(t, auditdomain.OpUpdate, row.Operation)
	assert.Equal(t, app.AuditEntityProductMaster, row.EntityType)
	var before, after map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.BeforeData), &before))
	require.NoError(t, json.Unmarshal([]byte(row.AfterData), &after))
	assert.Nil(t, before["rm_ratio_mode_override"])
	assert.Equal(t, "INPUT", before["effective_rm_ratio_mode"])
	assert.Equal(t, "DENIER", after["rm_ratio_mode_override"])
}

func TestUpdateRMRatioModeHandler_ClearWritesEmpty(t *testing.T) {
	_, modes, audit, h := rmModeFixture("DENIER", "DENIER")
	p, err := h.Handle(context.Background(), app.UpdateRMRatioModeCommand{ProductSysID: 77, Mode: "", ActorUserID: "u1"})
	require.NoError(t, err)
	assert.Empty(t, p.RMRatioModeOverride())
	require.Len(t, modes.writes, 1)
	assert.Empty(t, modes.writes[0].Override, "\"\" is persisted as NULL")
	assert.Len(t, audit.rows, 1)
}

func TestUpdateRMRatioModeHandler_NoChangeWritesNothing(t *testing.T) {
	_, modes, audit, h := rmModeFixture("INPUT", "INPUT")
	_, err := h.Handle(context.Background(), app.UpdateRMRatioModeCommand{ProductSysID: 77, Mode: "input", ActorUserID: "u1"})
	require.NoError(t, err)
	assert.Empty(t, modes.writes)
	assert.Empty(t, audit.rows)
}

func TestUpdateRMRatioModeHandler_Errors(t *testing.T) {
	_, modes, audit, h := rmModeFixture("", "INPUT")
	_, err := h.Handle(context.Background(), app.UpdateRMRatioModeCommand{ProductSysID: 77, Mode: "X"})
	require.ErrorIs(t, err, domain.ErrInvalidRMRatioModeOverride)
	assert.Empty(t, modes.writes)
	assert.Empty(t, audit.rows)

	repo, modes, _, h := rmModeFixture("", "INPUT")
	repo.getErr = domain.ErrNotFound
	_, err = h.Handle(context.Background(), app.UpdateRMRatioModeCommand{ProductSysID: 77, Mode: "DENIER"})
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, modes.writes)

	_, modes, audit, h = rmModeFixture("", "INPUT")
	modes.err = errors.New("db down")
	_, err = h.Handle(context.Background(), app.UpdateRMRatioModeCommand{ProductSysID: 77, Mode: "DENIER"})
	require.Error(t, err)
	assert.Empty(t, audit.rows)
}
