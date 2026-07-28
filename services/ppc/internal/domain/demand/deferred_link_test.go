package demand_test

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

// unlinkedParams builds an MTS demand raised before its finance master exists —
// the canonical PENDING_PRODUCT_LINK case (locked decision D-1).
func unlinkedParams() demand.NewParams {
	p := validParams()
	p.Type = demand.TypeMTS
	p.SubType = demand.SubTypeInternal
	p.CpmProductSysID = 0
	p.ProductLinkReason = demand.LinkReasonNoMasterYet
	return p
}

func TestNew_NoProductWithReason_LandsPendingProductLink(t *testing.T) {
	d, err := demand.New(unlinkedParams())
	require.NoError(t, err)
	assert.Equal(t, demand.StatusPendingProductLink, d.Status())
	assert.Equal(t, demand.LinkReasonNoMasterYet, d.ProductLinkReason())
	assert.False(t, d.IsProductLinked())
}

func TestNew_NoProductWithoutReason_Fails(t *testing.T) {
	p := unlinkedParams()
	p.ProductLinkReason = ""
	_, err := demand.New(p)
	assert.ErrorIs(t, err, demand.ErrLinkReasonRequired)
}

func TestNew_NoProductWithUnknownReason_Fails(t *testing.T) {
	p := unlinkedParams()
	p.ProductLinkReason = "BECAUSE_I_SAID_SO"
	_, err := demand.New(p)
	assert.ErrorIs(t, err, demand.ErrLinkReasonRequired)
}

func TestNew_LinkedProductWithReason_Fails(t *testing.T) {
	p := validParams()
	p.ProductLinkReason = demand.LinkReasonNoMasterYet
	_, err := demand.New(p)
	assert.ErrorIs(t, err, demand.ErrLinkReasonNotAllowed)
}

// Exit criterion 2: the three reasons are distinct values, so NO_MASTER_YET
// (intentionally unresolved) can be told apart from AUTO_MATCH_FAILED.
func TestLinkReasons_AreDistinctAndValidated(t *testing.T) {
	reasons := []string{
		demand.LinkReasonAutoMatchFailed,
		demand.LinkReasonAmbiguous,
		demand.LinkReasonNoMasterYet,
	}
	seen := map[string]bool{}
	for _, r := range reasons {
		assert.True(t, demand.IsValidLinkReason(r), r)
		assert.False(t, seen[r], "duplicate reason %s", r)
		seen[r] = true
	}
	assert.False(t, demand.IsValidLinkReason(""))
	assert.False(t, demand.IsValidLinkReason("AUTO_MATCH_FAILE"))
}

// Exit criterion 1 (domain half): an unlinked demand can be linked later, and
// linking is what moves it into the normal lifecycle.
func TestSetProduct_OnPendingLink_LinksAndClearsReason(t *testing.T) {
	d, err := demand.New(unlinkedParams())
	require.NoError(t, err)

	require.NoError(t, d.SetProduct(97073))

	assert.Equal(t, demand.StatusPendingConfirmation, d.Status())
	assert.Equal(t, int64(97073), d.CpmProductSysID())
	assert.Empty(t, d.ProductLinkReason())
	assert.True(t, d.IsProductLinked())

	// And from there the demand behaves like any other.
	require.NoError(t, d.Confirm(9))
	assert.Equal(t, demand.StatusConfirmed, d.Status())
}

func TestSetProduct_OnPendingLink_RejectsNonPositive(t *testing.T) {
	d, err := demand.New(unlinkedParams())
	require.NoError(t, err)
	assert.ErrorIs(t, d.SetProduct(0), demand.ErrInvalidProduct)
	assert.Equal(t, demand.StatusPendingProductLink, d.Status())
}

// T4.3: PENDING_PRODUCT_LINK has exactly one outbound transition. Every other
// move must be refused — confirming, cancelling, carrying or splitting an
// unlinked demand would all commit production against an unknown product.
func TestPendingProductLink_OnlyOutboundTransitionIsLinking(t *testing.T) {
	tests := []struct {
		name    string
		attempt func(d *demand.Demand) error
		wantErr error
	}{
		{"confirm", func(d *demand.Demand) error { return d.Confirm(9) }, demand.ErrIllegalTransition},
		{"cancel", func(d *demand.Demand) error { return d.Cancel() }, demand.ErrIllegalTransition},
		{"carry over", func(d *demand.Demand) error { return d.MarkCarriedOver() }, demand.ErrIllegalTransition},
		{"defer", func(d *demand.Demand) error { return d.MarkDeferred() }, demand.ErrIllegalTransition},
		{"split", func(d *demand.Demand) error { return d.MarkSplit() }, demand.ErrIllegalTransition},
		{"approve MTS", func(d *demand.Demand) error { return d.ApproveMTS(true, 9) }, demand.ErrIllegalTransition},
		{"reject MTS", func(d *demand.Demand) error { return d.ApproveMTS(false, 9) }, demand.ErrIllegalTransition},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d, err := demand.New(unlinkedParams())
			require.NoError(t, err)

			assert.ErrorIs(t, tt.attempt(d), tt.wantErr)
			assert.Equal(t, demand.StatusPendingProductLink, d.Status(),
				"refused transition must leave the status untouched")
		})
	}

	// The one legal move.
	d, err := demand.New(unlinkedParams())
	require.NoError(t, err)
	require.NoError(t, d.SetProduct(97073))
	assert.Equal(t, demand.StatusPendingConfirmation, d.Status())
}

// A linked demand never regresses into PENDING_PRODUCT_LINK, and its product
// stays locked once mapped.
func TestSetProduct_AlreadyLinked_Rejected(t *testing.T) {
	d, err := demand.New(validParams())
	require.NoError(t, err)
	assert.ErrorIs(t, d.SetProduct(97073), demand.ErrProductAlreadyMapped)
	assert.Equal(t, int64(100), d.CpmProductSysID())
}
