package grpc

import (
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	cpm "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costroute"
)

// The guard "an MB-typed product may never be created manually" is raised deep in the
// application/infrastructure layers as cpm.ErrMBProductNotManuallyCreatable. These tests
// pin the *delivery* contract: the client must see an actionable 400 carrying the
// sentinel's reason, and the error must NOT fall through to the generic Internal (500)
// arm of either mapper.
//
// Mapping sites under test:
//   - productMasterErrToBase — cost_product_master_handler.go
//   - routeErrToBase         — cost_route_handler.go

func TestProductMasterErrToBase_MBProductNotManuallyCreatable(t *testing.T) {
	base := productMasterErrToBase(cpm.ErrMBProductNotManuallyCreatable)

	require.NotNil(t, base)
	assert.False(t, base.GetIsSuccess(), "guard rejection must not be reported as success")
	assert.Equal(t, "400", base.GetStatusCode(),
		"MB-not-manually-creatable is a caller mistake (400), not a server fault")
	assert.NotEqual(t, "500", base.GetStatusCode(),
		"must not fall through to the generic Internal arm")
	// A meaningful message, not an empty or opaque one: the sentinel's own text explains
	// that MB products are born from the MB Recipe workflow.
	assert.Equal(t, cpm.ErrMBProductNotManuallyCreatable.Error(), base.GetMessage())
	assert.Contains(t, base.GetMessage(), "MB Recipe workflow")
}

func TestRouteErrToBase_MBProductNotManuallyCreatable(t *testing.T) {
	base := routeErrToBase(cpm.ErrMBProductNotManuallyCreatable)

	require.NotNil(t, base)
	assert.False(t, base.GetIsSuccess())
	assert.Equal(t, "400", base.GetStatusCode(),
		"DuplicateRoute refusing to clone an MB product is a 400, not a 500")
	assert.NotEqual(t, "500", base.GetStatusCode())
	assert.Equal(t, cpm.ErrMBProductNotManuallyCreatable.Error(), base.GetMessage())
	assert.Contains(t, base.GetMessage(), "MB Recipe workflow")
}

// The sentinel travels up through several layers, so it reaches the mappers wrapped.
// errors.Is must still find it — a mapper keyed on equality would silently regress to 500.
func TestErrToBase_MBProductNotManuallyCreatable_SurvivesWrapping(t *testing.T) {
	wrapped := fmt.Errorf("create product master: %w",
		fmt.Errorf("repository: %w", cpm.ErrMBProductNotManuallyCreatable))

	for name, mapper := range map[string]func(error) interface{ GetStatusCode() string }{
		"productMasterErrToBase": func(e error) interface{ GetStatusCode() string } {
			return productMasterErrToBase(e)
		},
		"routeErrToBase": func(e error) interface{ GetStatusCode() string } {
			return routeErrToBase(e)
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, "400", mapper(wrapped).GetStatusCode(),
				"wrapped sentinel must still map to 400, not Internal")
		})
	}
}

// Negative control: an unrelated error must still reach the Internal arm. Without this,
// a mapper that returned 400 for everything would pass the assertions above.
func TestErrToBase_UnrelatedErrorStillInternal(t *testing.T) {
	other := errors.New("boom")

	assert.Equal(t, "500", productMasterErrToBase(other).GetStatusCode())
	assert.Equal(t, "500", routeErrToBase(other).GetStatusCode())

	// And a neighbouring route sentinel keeps its own distinct code, proving the new arm
	// did not swallow sibling cases.
	assert.Equal(t, "404", routeErrToBase(costroute.ErrNotFound).GetStatusCode())
}
