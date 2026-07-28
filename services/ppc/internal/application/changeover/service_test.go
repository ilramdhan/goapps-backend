package changeover_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	changeoverapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/changeover"
	domain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/changeover"
)

// fakeRepo is an in-memory changeover repository.
type fakeRepo struct {
	events map[int64]*domain.Event
	nextID int64
}

func newFakeRepo() *fakeRepo { return &fakeRepo{events: map[int64]*domain.Event{}} }

func (r *fakeRepo) Create(_ context.Context, e *domain.Event) error {
	r.nextID++
	e.SetID(r.nextID)
	r.events[r.nextID] = e
	return nil
}

func (r *fakeRepo) GetByID(_ context.Context, id int64) (*domain.Event, error) {
	e, ok := r.events[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return e, nil
}

func (r *fakeRepo) List(_ context.Context, _ domain.Filter) ([]*domain.Event, int64, error) {
	out := make([]*domain.Event, 0, len(r.events))
	for _, e := range r.events {
		out = append(out, e)
	}
	return out, int64(len(out)), nil
}

func (r *fakeRepo) UpdateActual(_ context.Context, e *domain.Event) error {
	r.events[e.ID()] = e
	return nil
}

// fakeSpecs resolves WO specs and machines from fixed maps.
type fakeSpecs struct {
	specs    map[int64]domain.Spec
	machines map[int64]int64
}

func (s fakeSpecs) SpecForWO(_ context.Context, woID int64) (domain.Spec, bool, error) {
	spec, ok := s.specs[woID]
	return spec, ok, nil
}

func (s fakeSpecs) MachineForWO(_ context.Context, woID int64) (int64, bool, error) {
	m, ok := s.machines[woID]
	return m, ok, nil
}

func fixtureSpecs() fakeSpecs {
	return fakeSpecs{
		specs: map[int64]domain.Spec{
			// From denier 150 → To denier 300 triggers C1 (denier change).
			10: {Denier: 150, ColorFamily: "WHITE", FilamentCount: 48, TwistDir: "S"},
			20: {Denier: 300, ColorFamily: "WHITE", FilamentCount: 48, TwistDir: "S"},
		},
		machines: map[int64]int64{20: 7},
	}
}

func TestService_Detect_DenierChange(t *testing.T) {
	svc := changeoverapp.NewService(newFakeRepo(), fixtureSpecs(), nil)

	res, err := svc.Detect(context.Background(), 10, 20, false)
	require.NoError(t, err)

	codes := componentCodes(res.Components)
	assert.Contains(t, codes, domain.CompBase)
	assert.Contains(t, codes, domain.CompC1) // denier change
	assert.Positive(t, res.DurationEstimated)
	assert.Positive(t, res.WasteEstimated)
	assert.NotEmpty(t, res.Group)
}

func TestService_Create_AutoDetectsAndResolvesMachine(t *testing.T) {
	repo := newFakeRepo()
	svc := changeoverapp.NewService(repo, fixtureSpecs(), nil)

	event, err := svc.Create(context.Background(), changeoverapp.CreateCommand{
		FromWOID: 10,
		ToWOID:   20,
		// MachineID omitted → resolved from ToWO (machine 7).
		Notes: "auto",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(7), event.MachineID())
	assert.Equal(t, domain.StatusPlanned, event.Status())
	assert.NotEmpty(t, event.Components())
	assert.Contains(t, componentCodes(event.Components()), domain.CompC1)
}

func TestService_Create_HonorsExplicitComponents(t *testing.T) {
	repo := newFakeRepo()
	svc := changeoverapp.NewService(repo, fixtureSpecs(), nil)

	comps := []domain.Component{
		domain.NewComponent(domain.CompBase, 30, 8),
		domain.NewComponent(domain.CompC7, 120, 40),
	}
	event, err := svc.Create(context.Background(), changeoverapp.CreateCommand{
		FromWOID:   10,
		ToWOID:     20,
		MachineID:  99,
		Components: comps,
	})
	require.NoError(t, err)
	assert.Equal(t, int64(99), event.MachineID())
	assert.ElementsMatch(t, []string{domain.CompBase, domain.CompC7}, componentCodes(event.Components()))
}

func TestService_StartThenUpdateActual(t *testing.T) {
	repo := newFakeRepo()
	svc := changeoverapp.NewService(repo, fixtureSpecs(), nil)

	created, err := svc.Create(context.Background(), changeoverapp.CreateCommand{FromWOID: 10, ToWOID: 20})
	require.NoError(t, err)

	started, err := svc.Start(context.Background(), created.ID())
	require.NoError(t, err)
	assert.Equal(t, domain.StatusInProgress, started.Status())

	done, err := svc.UpdateActual(context.Background(), changeoverapp.UpdateActualCommand{
		EventID:        created.ID(),
		DurationActual: 75,
		WasteActual:    22.5,
		Notes:          "done",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.StatusDone, done.Status())
	require.NotNil(t, done.DurationActual())
	assert.Equal(t, int32(75), *done.DurationActual())
}

func componentCodes(comps []domain.Component) []string {
	out := make([]string, len(comps))
	for i := range comps {
		out[i] = comps[i].Code()
	}
	return out
}
