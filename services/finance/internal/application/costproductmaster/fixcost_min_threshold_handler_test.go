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

// fakeFixThrRepo records SaveFixCostMinThreshold writes.
type fakeFixThrRepo struct {
	writes []domain.FixCostMinThresholdWrite
	err    error
}

func (f *fakeFixThrRepo) SaveFixCostMinThreshold(_ context.Context, w domain.FixCostMinThresholdWrite) error {
	if f.err != nil {
		return f.err
	}
	f.writes = append(f.writes, w)
	return nil
}

var _ domain.FixCostMinThresholdRepository = (*fakeFixThrRepo)(nil)

func fixThrFixture(cur *float64) (*linkProductRepo, *fakeFixThrRepo, *fakeAuditSink, *app.UpdateFixCostMinThresholdHandler) {
	p := linkProduct(true, linkTypeYarn, "AX", "", "")
	p.RestoreFixCostMinThreshold(cur)
	repo := &linkProductRepo{p: p}
	thr := &fakeFixThrRepo{}
	audit := &fakeAuditSink{}
	h := app.NewUpdateFixCostMinThresholdHandler(repo, thr, audit).WithClock(func() time.Time { return linkNow })
	return repo, thr, audit, h
}

func TestUpdateFixCostMinThresholdHandler_ChangedSavesAndAudits(t *testing.T) {
	old := 0.05
	_, thr, audit, h := fixThrFixture(&old)
	p, err := h.Handle(context.Background(), app.UpdateFixCostMinThresholdCommand{ProductSysID: 77, Threshold: 0.1, ActorUserID: "u1"})
	require.NoError(t, err)
	require.NotNil(t, p.FixCostMinThreshold())
	assert.InDelta(t, 0.1, *p.FixCostMinThreshold(), 1e-12)
	assert.Equal(t, []domain.FixCostMinThresholdWrite{{ProductSysID: 77, Threshold: 0.1, UpdatedAt: linkNow, UpdatedBy: "u1"}}, thr.writes)

	require.Len(t, audit.rows, 1)
	row := audit.rows[0]
	assert.Equal(t, auditdomain.OpUpdate, row.Operation)
	assert.Equal(t, app.AuditEntityProductMaster, row.EntityType)
	var before, after map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.BeforeData), &before))
	require.NoError(t, json.Unmarshal([]byte(row.AfterData), &after))
	assert.InDelta(t, 0.05, before["fixcost_min_threshold"], 1e-12)
	assert.InDelta(t, 0.1, after["fixcost_min_threshold"], 1e-12)
}

func TestUpdateFixCostMinThresholdHandler_FromAutoAndOff(t *testing.T) {
	_, thr, audit, h := fixThrFixture(nil)
	_, err := h.Handle(context.Background(), app.UpdateFixCostMinThresholdCommand{ProductSysID: 77, Threshold: 0, ActorUserID: "u1"})
	require.NoError(t, err)
	require.Len(t, thr.writes, 1)
	assert.Zero(t, thr.writes[0].Threshold)
	require.Len(t, audit.rows, 1)
	var before map[string]any
	require.NoError(t, json.Unmarshal([]byte(audit.rows[0].BeforeData), &before))
	assert.Nil(t, before["fixcost_min_threshold"])
}

func TestUpdateFixCostMinThresholdHandler_NoChangeWritesNothing(t *testing.T) {
	cur := 0.05
	_, thr, audit, h := fixThrFixture(&cur)
	_, err := h.Handle(context.Background(), app.UpdateFixCostMinThresholdCommand{ProductSysID: 77, Threshold: 0.05, ActorUserID: "u1"})
	require.NoError(t, err)
	assert.Empty(t, thr.writes)
	assert.Empty(t, audit.rows)
}

func TestUpdateFixCostMinThresholdHandler_Errors(t *testing.T) {
	_, thr, audit, h := fixThrFixture(nil)
	_, err := h.Handle(context.Background(), app.UpdateFixCostMinThresholdCommand{ProductSysID: 77, Threshold: 2})
	require.ErrorIs(t, err, domain.ErrInvalidFixCostMinThreshold)
	assert.Empty(t, thr.writes)
	assert.Empty(t, audit.rows)

	repo, thr, _, h := fixThrFixture(nil)
	repo.getErr = domain.ErrNotFound
	_, err = h.Handle(context.Background(), app.UpdateFixCostMinThresholdCommand{ProductSysID: 77, Threshold: 0.1})
	require.ErrorIs(t, err, domain.ErrNotFound)
	assert.Empty(t, thr.writes)

	_, thr, audit, h = fixThrFixture(nil)
	thr.err = errors.New("db down")
	_, err = h.Handle(context.Background(), app.UpdateFixCostMinThresholdCommand{ProductSysID: 77, Threshold: 0.1})
	require.Error(t, err)
	assert.Empty(t, audit.rows)
}
