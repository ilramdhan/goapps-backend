package costproductrequest_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductrequest"
	cpr "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductrequest"
	costroute "github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// fakeUnlinkRequestRepo is a configurable fake for cpr.Repository, used only by
// UnlinkRouteHandler tests.
type fakeUnlinkRequestRepo struct {
	req        *cpr.Request
	getErr     error
	saveErr    error
	saveCalled bool
	savedReq   *cpr.Request
}

func (r *fakeUnlinkRequestRepo) Create(_ context.Context, _ *cpr.Request) error { return nil }

func (r *fakeUnlinkRequestRepo) GetByID(_ context.Context, _ int64) (*cpr.Request, error) {
	return r.req, r.getErr
}

func (r *fakeUnlinkRequestRepo) GetByNo(_ context.Context, _ string) (*cpr.Request, error) {
	return nil, nil
}

func (r *fakeUnlinkRequestRepo) Save(_ context.Context, req *cpr.Request) error {
	r.saveCalled = true
	r.savedReq = req
	return r.saveErr
}

func (r *fakeUnlinkRequestRepo) List(_ context.Context, _ cpr.Filter) ([]*cpr.Request, int64, error) {
	return nil, 0, nil
}

func (r *fakeUnlinkRequestRepo) ListAll(_ context.Context, _ cpr.Filter) ([]*cpr.Request, error) {
	return nil, nil
}

// fakeUnlinkRouteRepo is a configurable no-op fake for costroute.Repository,
// used only by UnlinkRouteHandler tests. Only GetHead is exercised.
type fakeUnlinkRouteRepo struct {
	head       *costroute.Head
	getHeadErr error
}

func (r *fakeUnlinkRouteRepo) GetHead(_ context.Context, _ int64) (*costroute.Head, error) {
	return r.head, r.getHeadErr
}

func (r *fakeUnlinkRouteRepo) PromoteFromDraft(_ context.Context, _ costroute.PromoteInput) (int64, error) {
	return 0, nil
}

func (r *fakeUnlinkRouteRepo) GetActiveByProduct(_ context.Context, _ int64) (*costroute.Head, error) {
	return nil, costroute.ErrNotFound
}

func (r *fakeUnlinkRouteRepo) GetGraph(_ context.Context, _ int64) (*costroute.Graph, error) {
	return nil, costroute.ErrNotFound
}

func (r *fakeUnlinkRouteRepo) SaveGraph(_ context.Context, _ int64, _ *costroute.Graph, _ string) (*costroute.Graph, error) {
	return nil, nil
}

func (r *fakeUnlinkRouteRepo) SaveHead(_ context.Context, _ *costroute.Head, _ string) error {
	return nil
}

func (r *fakeUnlinkRouteRepo) DeleteHead(_ context.Context, _ int64, _ string) error { return nil }

func (r *fakeUnlinkRouteRepo) ListHeads(_ context.Context, _ costroute.Filter) ([]*costroute.Head, int64, error) {
	return nil, 0, nil
}

func (r *fakeUnlinkRouteRepo) DuplicateRoute(_ context.Context, _ costroute.DuplicateInput) (costroute.DuplicateOutput, error) {
	return costroute.DuplicateOutput{}, nil
}

func (r *fakeUnlinkRouteRepo) AttachRoute(_ context.Context, _ costroute.AttachInput) (costroute.AttachOutput, error) {
	return costroute.AttachOutput{}, nil
}

func (r *fakeUnlinkRouteRepo) ListLinkedRequests(_ context.Context, _ int64) ([]costroute.LinkedRequest, error) {
	return nil, nil
}

func (r *fakeUnlinkRouteRepo) BulkUpsertHeads(_ context.Context, _ []costroute.HeadUpsertInput, _ string) ([]costroute.HeadUpsertResult, error) {
	return nil, nil
}

func (r *fakeUnlinkRouteRepo) BulkUpsertSeqs(_ context.Context, _ []costroute.SeqUpsertInput, _ string) ([]costroute.SeqUpsertResult, error) {
	return nil, nil
}

func (r *fakeUnlinkRouteRepo) BulkReplaceRMs(_ context.Context, _ int64, _ []costroute.RMInput, _ string) error {
	return nil
}

func (r *fakeUnlinkRouteRepo) ListAllHeadsForExport(_ context.Context, _ []int64) ([]costroute.ExportRouteHead, error) {
	return nil, nil
}

func (r *fakeUnlinkRouteRepo) ListAllSeqsForExport(_ context.Context, _ []int64) ([]costroute.ExportRouteSeq, error) {
	return nil, nil
}

func (r *fakeUnlinkRouteRepo) ListAllRMsForExport(_ context.Context, _ []int64) ([]costroute.ExportRouteRM, error) {
	return nil, nil
}

// unlinkReqAt builds a *cpr.Request in the given status, linked to headID
// (0 means not linked).
func unlinkReqAt(status string, headID int64) *cpr.Request {
	return cpr.Reconstruct(cpr.ReconstructInput{
		RequestID:             1,
		RequestNo:             "CR-2026-0001",
		RequestTypeID:         1,
		Title:                 "t",
		CustomerName:          "Acme",
		ProductClassification: cpr.ClassExisting,
		UrgencyLevel:          cpr.UrgencyMedium,
		Status:                status,
		RequesterUserID:       "user-1",
		LinkedRouteHeadID:     headID,
		CreatedAt:             time.Now().UTC(),
		UpdatedAt:             time.Now().UTC(),
	})
}

func TestUnlinkRouteHandler_Handle_RejectsWhenLocked(t *testing.T) {
	t.Parallel()
	reqRepo := &fakeUnlinkRequestRepo{req: unlinkReqAt(cpr.StatusUnderReview, 42)}
	routeRepo := &fakeUnlinkRouteRepo{head: &costroute.Head{HeadID: 42, RoutingStatus: costroute.StatusLocked}}
	h := app.NewUnlinkRouteHandler(reqRepo, routeRepo)

	got, err := h.Handle(context.Background(), app.UnlinkRouteCommand{RequestID: 1, ActorUserID: "user-1"})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, costroute.ErrLocked), "want ErrLocked, got %v", err)
	assert.False(t, reqRepo.saveCalled, "Save must NOT be called when route is locked")
}

func TestUnlinkRouteHandler_Handle_SucceedsWhenNotLocked(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name   string
		status string
	}{
		{"route DRAFT", costroute.StatusDraft},
		{"route COMPLETE", costroute.StatusComplete},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			reqRepo := &fakeUnlinkRequestRepo{req: unlinkReqAt(cpr.StatusUnderReview, 42)}
			routeRepo := &fakeUnlinkRouteRepo{head: &costroute.Head{HeadID: 42, RoutingStatus: tc.status}}
			h := app.NewUnlinkRouteHandler(reqRepo, routeRepo)

			got, err := h.Handle(context.Background(), app.UnlinkRouteCommand{RequestID: 1, ActorUserID: "user-1"})

			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, int64(0), got.LinkedRouteHeadID())
			assert.True(t, reqRepo.saveCalled, "Save must be called on successful unlink")
		})
	}
}

func TestUnlinkRouteHandler_Handle_SucceedsWhenNoRouteLinked(t *testing.T) {
	t.Parallel()
	reqRepo := &fakeUnlinkRequestRepo{req: unlinkReqAt(cpr.StatusUnderReview, 0)}
	routeRepo := &fakeUnlinkRouteRepo{} // GetHead must not be needed since headID == 0
	h := app.NewUnlinkRouteHandler(reqRepo, routeRepo)

	got, err := h.Handle(context.Background(), app.UnlinkRouteCommand{RequestID: 1, ActorUserID: "user-1"})

	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, int64(0), got.LinkedRouteHeadID())
	assert.True(t, reqRepo.saveCalled)
}

func TestUnlinkRouteHandler_Handle_PropagatesGetHeadError(t *testing.T) {
	t.Parallel()
	repoErr := errors.New("db unavailable")
	reqRepo := &fakeUnlinkRequestRepo{req: unlinkReqAt(cpr.StatusUnderReview, 42)}
	routeRepo := &fakeUnlinkRouteRepo{getHeadErr: repoErr}
	h := app.NewUnlinkRouteHandler(reqRepo, routeRepo)

	got, err := h.Handle(context.Background(), app.UnlinkRouteCommand{RequestID: 1, ActorUserID: "user-1"})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, repoErr))
	assert.False(t, reqRepo.saveCalled)
}

func TestUnlinkRouteHandler_Handle_PropagatesGetByIDError(t *testing.T) {
	t.Parallel()
	repoErr := errors.New("request not found")
	reqRepo := &fakeUnlinkRequestRepo{getErr: repoErr}
	routeRepo := &fakeUnlinkRouteRepo{}
	h := app.NewUnlinkRouteHandler(reqRepo, routeRepo)

	got, err := h.Handle(context.Background(), app.UnlinkRouteCommand{RequestID: 1, ActorUserID: "user-1"})

	require.Error(t, err)
	assert.Nil(t, got)
	assert.True(t, errors.Is(err, repoErr))
	assert.False(t, reqRepo.saveCalled)
}
