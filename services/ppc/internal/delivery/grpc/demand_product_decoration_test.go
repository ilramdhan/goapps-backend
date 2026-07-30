package grpc

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	customerdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/customer"
	demanddomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/demand"
)

func newTestDemand(id, productSysID int64) *demanddomain.Demand {
	return demanddomain.Reconstruct(demanddomain.ReconstructParams{
		ID:              id,
		Type:            "MTO",
		SubType:         "EXPORT",
		Source:          "ORION",
		CpmProductSysID: productSysID,
		QtyOriginal:     100,
		QtyRemaining:    100,
		Deadline:        time.Date(2026, 8, 11, 0, 0, 0, 0, time.UTC),
		Status:          "PENDING",
		Month:           "2026-08",
		CreatedAt:       time.Date(2026, 7, 27, 0, 0, 0, 0, time.UTC),
	})
}

// Regression: a demand carrying a resolved product sys id must come back with
// product_code/product_name populated, otherwise the UI renders "Not mapped"
// even though the demand is mapped.
func TestDemandToProtoPopulatesProductLabels(t *testing.T) {
	e := newTestDemand(11, 95826)
	products := map[int64]*financev1.CostMasterProduct{
		95826: {ProductCode: "CSTTPY2606000518", ProductName: "PTY 150/48/RND/DSD/NI/DH/N/1/Z"},
	}

	got := demandToProto(e, demandLabels{products: products})

	require.Equal(t, int64(95826), got.GetCpmProductSysId())
	assert.Equal(t, "CSTTPY2606000518", got.GetProductCode())
	assert.Equal(t, "PTY 150/48/RND/DSD/NI/DH/N/1/Z", got.GetProductName())
}

func TestDemandToProtoLeavesLabelsEmptyWhenUnmapped(t *testing.T) {
	got := demandToProto(newTestDemand(12, 0), demandLabels{products: map[int64]*financev1.CostMasterProduct{}})

	assert.Zero(t, got.GetCpmProductSysId())
	assert.Empty(t, got.GetProductCode())
	assert.Empty(t, got.GetProductName())
}

func TestDemandToProtoToleratesMissingLookupEntry(t *testing.T) {
	got := demandToProto(newTestDemand(13, 97073), demandLabels{})

	require.Equal(t, int64(97073), got.GetCpmProductSysId())
	assert.Empty(t, got.GetProductCode())
	assert.Empty(t, got.GetProductName())
}

// An unlinked demand pulled from Orion must still carry a human-readable
// identity: the Orion item code of the staging row it came from.
func TestDemandToProtoPopulatesOrionItemCode(t *testing.T) {
	got := demandToProto(newTestDemand(14, 0), demandLabels{orionCodes: map[int64]string{14: "PTY0002107"}})

	assert.Equal(t, "PTY0002107", got.GetOrionItemCode())
}

func TestDemandToProtoLeavesOrionItemCodeEmptyWhenAbsent(t *testing.T) {
	got := demandToProto(newTestDemand(15, 0), demandLabels{orionCodes: map[int64]string{99: "PTY0002107"}})

	assert.Empty(t, got.GetOrionItemCode())
}

// A demand stores only customer_id; the UI never shows a raw id, so the proto
// must carry the resolved code/name from the PPC customer master.
func TestDemandToProtoPopulatesCustomerLabels(t *testing.T) {
	got := demandToProto(newTestDemand(16, 0), demandLabels{
		customers: map[int64]customerdomain.Label{16: {Code: "DC00594", Name: "PT SINAR JAYA"}},
	})

	assert.Equal(t, "DC00594", got.GetCustomerCode())
	assert.Equal(t, "PT SINAR JAYA", got.GetCustomerName())
}

func TestDemandToProtoLeavesCustomerLabelsEmptyWhenUnresolved(t *testing.T) {
	got := demandToProto(newTestDemand(17, 0), demandLabels{})

	assert.Empty(t, got.GetCustomerCode())
	assert.Empty(t, got.GetCustomerName())
}

func TestDemandSysIDsDedupesAndSkipsUnmapped(t *testing.T) {
	ids := demandSysIDs([]*demanddomain.Demand{
		newTestDemand(1, 95826),
		newTestDemand(2, 0),
		newTestDemand(3, 95826),
		newTestDemand(4, 97073),
	})

	assert.Equal(t, []int64{95826, 97073}, ids)
}

func TestDemandSysIDsEmpty(t *testing.T) {
	assert.Empty(t, demandSysIDs(nil))
}
