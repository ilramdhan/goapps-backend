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

// pullRepo scripts the staging rows a pull sees and records the demands created
// from them. It reuses resolveRepo for the rest of the Repository surface.
type pullRepo struct {
	resolveRepo
	staging []*demanddomain.SalesOrderStaging
	created []*demanddomain.Demand
}

func (r *pullRepo) GetStagingByIDs(_ context.Context, _ []int64) ([]*demanddomain.SalesOrderStaging, error) {
	return r.staging, nil
}

func (r *pullRepo) Create(_ context.Context, entity *demanddomain.Demand) error {
	r.created = append(r.created, entity)
	return nil
}

// Plan 03A gate: shade on the Orion staging row must survive the pull. It was
// silently dropped before, so nothing downstream could group by colour.
func TestPullFromOrion_CarriesShadeOntoDemand(t *testing.T) {
	deadline := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	sysID := int64(97073)
	repo := &pullRepo{staging: []*demanddomain.SalesOrderStaging{{
		SosID:           84706,
		ContractNo:      "CT-2026-0042",
		CpmProductSysID: &sysID,
		ShadeCode:       "5918-01",
		ShadeName:       "TURQUOISE",
		QtyOrdered:      1200,
		QtyRemaining:    1200,
		Deadline:        &deadline,
	}}}
	svc := demandapp.NewService(repo, &stubValidator{}, nil)

	created, err := svc.PullFromOrion(context.Background(), demandapp.PullFromOrionCommand{
		SosIDs:    []int64{84706},
		CreatedBy: 1,
	})

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Equal(t, "5918-01", created[0].ShadeCode())
	assert.Equal(t, "TURQUOISE", created[0].ShadeName())
	require.Len(t, repo.created, 1)
	assert.Equal(t, "5918-01", repo.created[0].ShadeCode())
}

// A staging row with no shade must pull cleanly rather than fail or invent one.
func TestPullFromOrion_EmptyShadeIsCarriedAsEmpty(t *testing.T) {
	deadline := time.Date(2026, 8, 15, 0, 0, 0, 0, time.UTC)
	repo := &pullRepo{staging: []*demanddomain.SalesOrderStaging{{
		SosID:        84707,
		QtyOrdered:   500,
		QtyRemaining: 500,
		Deadline:     &deadline,
	}}}
	svc := demandapp.NewService(repo, &stubValidator{}, nil)

	created, err := svc.PullFromOrion(context.Background(), demandapp.PullFromOrionCommand{
		SosIDs:    []int64{84707},
		CreatedBy: 1,
	})

	require.NoError(t, err)
	require.Len(t, created, 1)
	assert.Empty(t, created[0].ShadeCode())
	assert.Empty(t, created[0].ShadeName())
}
