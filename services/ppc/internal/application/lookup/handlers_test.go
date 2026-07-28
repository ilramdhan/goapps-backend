package lookup_test

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	lookupapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/lookup"
	lookupdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/lookup"
)

// memRepo is an in-memory lookup.Repository for use-case tests. List honors the
// category filter and active filter, mirroring the SQL WHERE clauses.
type memRepo struct {
	rows   []*lookupdomain.Lookup
	nextID int64
}

func (m *memRepo) Create(_ context.Context, e *lookupdomain.Lookup) error {
	m.nextID++
	stored := lookupdomain.Reconstruct(
		m.nextID, e.Category(), e.Code(), e.Label(), e.SortOrder(),
		e.IsActive(), e.CreatedAt(), e.CreatedBy(), nil, nil,
	)
	m.rows = append(m.rows, stored)
	*e = *stored
	return nil
}

func (m *memRepo) GetByID(_ context.Context, id int64) (*lookupdomain.Lookup, error) {
	for _, r := range m.rows {
		if r.ID() == id {
			return r, nil
		}
	}
	return nil, lookupdomain.ErrNotFound
}

func (m *memRepo) List(_ context.Context, f lookupdomain.ListFilter) ([]*lookupdomain.Lookup, int64, error) {
	var out []*lookupdomain.Lookup
	for _, r := range m.rows {
		if f.Category != "" && r.Category() != f.Category {
			continue
		}
		if f.IsActive != nil && r.IsActive() != *f.IsActive {
			continue
		}
		out = append(out, r)
	}
	return out, int64(len(out)), nil
}

func (m *memRepo) Update(_ context.Context, _ *lookupdomain.Lookup) error { return nil }
func (m *memRepo) Delete(_ context.Context, _ int64) error                { return nil }

func seedRepo(t *testing.T) *memRepo {
	t.Helper()
	repo := &memRepo{}
	ctx := context.Background()
	svc := lookupapp.NewService(repo)
	seed := []struct{ cat, code, label string }{
		{"PPC_AREA", "TXT", "Texturizing (TXT)"},
		{"PPC_AREA", "SPG", "Spinning (SPG)"},
		{"PPC_GRADE_REQ", "AX_ONLY", "AX Only"},
	}
	for i, s := range seed {
		_, err := svc.Create(ctx, lookupapp.CreateCommand{
			Category: s.cat, Code: s.code, Label: s.label, SortOrder: int32(i), CreatedBy: "seeder",
		})
		require.NoError(t, err)
	}
	return repo
}

func TestList_FilterByCategory_ReturnsOnlyThatCategory(t *testing.T) {
	repo := seedRepo(t)
	svc := lookupapp.NewService(repo)

	res, err := svc.List(context.Background(), lookupapp.ListQuery{Category: "PPC_AREA"})
	require.NoError(t, err)
	assert.Equal(t, int64(2), res.TotalItems)
	for _, item := range res.Items {
		assert.Equal(t, "PPC_AREA", item.Category())
	}
}

func TestList_NoCategory_ReturnsAll(t *testing.T) {
	repo := seedRepo(t)
	svc := lookupapp.NewService(repo)

	res, err := svc.List(context.Background(), lookupapp.ListQuery{})
	require.NoError(t, err)
	assert.Equal(t, int64(3), res.TotalItems)
}

func TestCreate_CodeEqualsEnumValue_LabelHumanFriendly(t *testing.T) {
	repo := &memRepo{}
	svc := lookupapp.NewService(repo)

	got, err := svc.Create(context.Background(), lookupapp.CreateCommand{
		Category: "PPC_GRADE_REQ", Code: "AX_AM_CLAUSE", Label: "AX + AM Clause", SortOrder: 2, CreatedBy: "seeder",
	})
	require.NoError(t, err)
	assert.Equal(t, "AX_AM_CLAUSE", got.Code())
	assert.Equal(t, "AX + AM Clause", got.Label())
}

func TestCreate_EmptyCategory_Fails(t *testing.T) {
	svc := lookupapp.NewService(&memRepo{})
	_, err := svc.Create(context.Background(), lookupapp.CreateCommand{
		Category: "", Code: "TXT", Label: "x", CreatedBy: "seeder",
	})
	assert.ErrorIs(t, err, lookupdomain.ErrEmptyCategory)
}
