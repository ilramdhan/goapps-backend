package planitem_test

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	planitemapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/planitem"
	planitemdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/planitem"
)

// stubDemandLinks is a scripted DemandLinkChecker keyed by demand id.
type stubDemandLinks struct {
	linked map[int64]bool
	err    error
	seen   []int64
}

func (s *stubDemandLinks) DemandProductLinked(_ context.Context, demandID int64) (bool, error) {
	s.seen = append(s.seen, demandID)
	if s.err != nil {
		return false, s.err
	}
	return s.linked[demandID], nil
}

// demandCmd builds a plan item raised from a demand (rather than a parent).
func demandCmd(demandID int64) planitemapp.CreateCommand {
	cmd := createCmd()
	cmd.ParentItemID = nil
	cmd.DemandID = &demandID
	return cmd
}

// Exit criterion 3: no plan item may be created from a demand whose product is
// still unlinked.
func TestCreate_UnlinkedDemand_Rejected(t *testing.T) {
	repo := newMemRepo()
	links := &stubDemandLinks{linked: map[int64]bool{42: false}}
	svc := planitemapp.NewService(repo, nil, nil).
		WithCapacity(fixedCapacity{perDay: 100}).
		WithDemandLinks(links)

	_, err := svc.Create(context.Background(), demandCmd(42))

	require.Error(t, err)
	assert.ErrorIs(t, err, planitemdomain.ErrDemandProductNotLinked)
	assert.Equal(t,
		"invalid plan item: demand has no linked product yet; link the product to the demand first",
		err.Error())
	assert.Equal(t, []int64{42}, links.seen)
	assert.Empty(t, repo.items, "a rejected plan item must not be persisted")
}

func TestCreate_LinkedDemand_Allowed(t *testing.T) {
	repo := newMemRepo()
	links := &stubDemandLinks{linked: map[int64]bool{42: true}}
	svc := planitemapp.NewService(repo, nil, nil).
		WithCapacity(fixedCapacity{perDay: 100}).
		WithDemandLinks(links)

	res, err := svc.Create(context.Background(), demandCmd(42))

	require.NoError(t, err)
	require.NotNil(t, res.Item)
	assert.Equal(t, []int64{42}, links.seen)
	assert.Len(t, repo.items, 1)
}

// A cascade child carries a parent, not a demand — the guard must not fire and
// must not cost a lookup.
func TestCreate_NoDemandID_SkipsGuard(t *testing.T) {
	links := &stubDemandLinks{linked: map[int64]bool{}}
	svc := planitemapp.NewService(newMemRepo(), nil, nil).
		WithCapacity(fixedCapacity{perDay: 100}).
		WithDemandLinks(links)

	_, err := svc.Create(context.Background(), createCmd())

	require.NoError(t, err)
	assert.Empty(t, links.seen)
}

// The guard fails closed: a lookup error must not be swallowed into a create.
func TestCreate_DemandLookupFails_Propagates(t *testing.T) {
	boom := errors.New("demand read failed")
	repo := newMemRepo()
	svc := planitemapp.NewService(repo, nil, nil).
		WithCapacity(fixedCapacity{perDay: 100}).
		WithDemandLinks(&stubDemandLinks{err: boom})

	_, err := svc.Create(context.Background(), demandCmd(42))

	assert.ErrorIs(t, err, boom)
	assert.Empty(t, repo.items)
}

// Without a wired checker the service degrades gracefully rather than failing.
func TestCreate_NoDemandChecker_DegradesGracefully(t *testing.T) {
	svc, _ := newService()

	_, err := svc.Create(context.Background(), demandCmd(42))

	require.NoError(t, err)
}
