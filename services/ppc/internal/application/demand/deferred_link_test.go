package demand_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	demandapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/demand"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// linkRepo records created demands and serves one back by id, so a create can
// be followed by a link in the same test (exit criterion 1).
type linkRepo struct {
	resolveRepo
	created []*demanddomain.Demand
	stored  *demanddomain.Demand
}

func (r *linkRepo) Create(_ context.Context, entity *demanddomain.Demand) error {
	r.created = append(r.created, entity)
	r.stored = entity
	return nil
}

func (r *linkRepo) GetByID(_ context.Context, _ int64) (*demanddomain.Demand, error) {
	if r.stored == nil {
		return nil, demanddomain.ErrNotFound
	}
	return r.stored, nil
}

func (r *linkRepo) Update(_ context.Context, _ *demanddomain.Demand) error { return nil }

func manualCmd(demandType, subType string, sysID int64) demandapp.CreateCommand {
	return demandapp.CreateCommand{
		Type:            demandType,
		SubType:         subType,
		Source:          demanddomain.SourceManual,
		CpmProductSysID: sysID,
		QtyOriginal:     1000,
		Deadline:        time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC),
		GradeReq:        demanddomain.GradeReqNone,
		Month:           "2026-08",
		CreatedBy:       7,
	}
}

// Exit criterion 1: an MTS demand is saved with no product and linked later,
// entirely through the service API — no hand-edited rows.
func TestCreateThenMapProduct_MTSWithoutProduct_EndToEnd(t *testing.T) {
	repo := &linkRepo{}
	validator := &stubValidator{}
	svc := demandapp.NewService(repo, validator, nil)
	ctx := context.Background()

	created, err := svc.Create(ctx, manualCmd(demanddomain.TypeMTS, demanddomain.SubTypeInternal, 0))
	require.NoError(t, err)
	assert.Equal(t, demanddomain.StatusPendingProductLink, created.Status())
	assert.Equal(t, demanddomain.LinkReasonNoMasterYet, created.ProductLinkReason())
	assert.False(t, created.IsProductLinked())
	// Nothing to validate against finance yet — the master does not exist.
	assert.Empty(t, validator.seen)

	linked, err := svc.MapProduct(ctx, demandapp.MapProductCommand{
		ID:              created.ID(),
		CpmProductSysID: 97073,
	})
	require.NoError(t, err)
	assert.Equal(t, demanddomain.StatusPendingConfirmation, linked.Status())
	assert.Equal(t, int64(97073), linked.CpmProductSysID())
	assert.Empty(t, linked.ProductLinkReason())
	assert.True(t, linked.IsProductLinked())
	// The link IS validated against finance — a link is a write path.
	assert.Equal(t, []int64{97073}, validator.seen)
}

func TestCreate_SampleWithoutProduct_IsDeferred(t *testing.T) {
	repo := &linkRepo{}
	svc := demandapp.NewService(repo, &stubValidator{}, nil)

	created, err := svc.Create(context.Background(),
		manualCmd(demanddomain.TypeSample, "", 0))

	require.NoError(t, err)
	assert.Equal(t, demanddomain.StatusPendingProductLink, created.Status())
	assert.Equal(t, demanddomain.LinkReasonNoMasterYet, created.ProductLinkReason())
}

// The proto relaxation must not become a hole: a CONTRACT demand still needs a
// product up front.
func TestCreate_ContractWithoutProduct_Rejected(t *testing.T) {
	repo := &linkRepo{}
	svc := demandapp.NewService(repo, &stubValidator{}, nil)

	_, err := svc.Create(context.Background(),
		manualCmd(demanddomain.TypeContract, demanddomain.SubTypeNewExport, 0))

	assert.ErrorIs(t, err, demanddomain.ErrInvalidProduct)
	assert.Empty(t, repo.created, "a rejected demand must not be persisted")
}

// T4.4: the reason recorded on a pull distinguishes an ambiguous match from a
// failed one, and both from the manual NO_MASTER_YET case.
func TestPullFromOrion_RecordsLinkReasonPerMatchStatus(t *testing.T) {
	deadline := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	resolved := int64(97073)
	tests := []struct {
		name       string
		row        *demanddomain.SalesOrderStaging
		wantStatus string
		wantReason string
	}{
		{
			name:       "ambiguous match",
			row:        &demanddomain.SalesOrderStaging{SosID: 1, MatchStatus: demanddomain.MatchStatusAmbiguous, MatchCount: 3},
			wantStatus: demanddomain.StatusPendingProductLink,
			wantReason: demanddomain.LinkReasonAmbiguous,
		},
		{
			name:       "not found",
			row:        &demanddomain.SalesOrderStaging{SosID: 2, MatchStatus: demanddomain.MatchStatusNotFound},
			wantStatus: demanddomain.StatusPendingProductLink,
			wantReason: demanddomain.LinkReasonAutoMatchFailed,
		},
		{
			name:       "still unresolved",
			row:        &demanddomain.SalesOrderStaging{SosID: 3, MatchStatus: demanddomain.MatchStatusUnresolved},
			wantStatus: demanddomain.StatusPendingProductLink,
			wantReason: demanddomain.LinkReasonAutoMatchFailed,
		},
		{
			name:       "auto resolved",
			row:        &demanddomain.SalesOrderStaging{SosID: 4, MatchStatus: demanddomain.MatchStatusAuto, CpmProductSysID: &resolved},
			wantStatus: demanddomain.StatusPendingConfirmation,
			wantReason: "",
		},
		{
			name:       "manual pick",
			row:        &demanddomain.SalesOrderStaging{SosID: 5, MatchStatus: demanddomain.MatchStatusManual, CpmProductSysID: &resolved},
			wantStatus: demanddomain.StatusPendingConfirmation,
			wantReason: "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.row.QtyOrdered = 500
			tt.row.QtyRemaining = 500
			tt.row.Deadline = &deadline
			repo := &pullRepo{staging: []*demanddomain.SalesOrderStaging{tt.row}}
			svc := demandapp.NewService(repo, &stubValidator{}, nil)

			created, err := svc.PullFromOrion(context.Background(), demandapp.PullFromOrionCommand{
				SosIDs:    []int64{tt.row.SosID},
				CreatedBy: 1,
			})

			require.NoError(t, err)
			require.Len(t, created, 1)
			assert.Equal(t, tt.wantStatus, created[0].Status())
			assert.Equal(t, tt.wantReason, created[0].ProductLinkReason())
		})
	}
}

// Exit criterion 2, at the application boundary: an Orion pull that failed to
// match never records NO_MASTER_YET, and a manual MTS never records
// AUTO_MATCH_FAILED. The two paths stay tellable apart.
func TestLinkReason_PullAndManualPathsAreDistinguishable(t *testing.T) {
	deadline := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	pull := &pullRepo{staging: []*demanddomain.SalesOrderStaging{{
		SosID:        9,
		MatchStatus:  demanddomain.MatchStatusNotFound,
		QtyOrdered:   500,
		QtyRemaining: 500,
		Deadline:     &deadline,
	}}}
	pulled, err := demandapp.NewService(pull, &stubValidator{}, nil).
		PullFromOrion(context.Background(), demandapp.PullFromOrionCommand{SosIDs: []int64{9}, CreatedBy: 1})
	require.NoError(t, err)
	require.Len(t, pulled, 1)

	manual, err := demandapp.NewService(&linkRepo{}, &stubValidator{}, nil).
		Create(context.Background(), manualCmd(demanddomain.TypeMTS, demanddomain.SubTypeInternal, 0))
	require.NoError(t, err)

	assert.Equal(t, demanddomain.LinkReasonAutoMatchFailed, pulled[0].ProductLinkReason())
	assert.Equal(t, demanddomain.LinkReasonNoMasterYet, manual.ProductLinkReason())
	assert.NotEqual(t, pulled[0].ProductLinkReason(), manual.ProductLinkReason())
}

// T4.5 support: the port plan-item planning consults.
func TestDemandProductLinked_ReportsLinkState(t *testing.T) {
	repo := &linkRepo{}
	svc := demandapp.NewService(repo, &stubValidator{}, nil)
	ctx := context.Background()

	created, err := svc.Create(ctx, manualCmd(demanddomain.TypeMTS, demanddomain.SubTypeInternal, 0))
	require.NoError(t, err)

	linked, err := svc.DemandProductLinked(ctx, created.ID())
	require.NoError(t, err)
	assert.False(t, linked)

	_, err = svc.MapProduct(ctx, demandapp.MapProductCommand{ID: created.ID(), CpmProductSysID: 97073})
	require.NoError(t, err)

	linked, err = svc.DemandProductLinked(ctx, created.ID())
	require.NoError(t, err)
	assert.True(t, linked)
}
