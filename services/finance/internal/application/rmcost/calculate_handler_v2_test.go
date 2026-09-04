package rmcost

import (
	"testing"

	"github.com/google/uuid"

	rmcostdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmcost"
	rmgroupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// TestMdvFromExisting_NilOrUnset mirrors simRateFromExisting's zero-value
// contract for marketing_default_value: nil cost, nil V2Inputs, and unset
// MarketingDefaultValue must all resolve to 0 (no override).
func TestMdvFromExisting_NilOrUnset(t *testing.T) {
	if got := mdvFromExisting(nil); got != 0 {
		t.Errorf("nil existing: got %v, want 0", got)
	}

	comp := rmcostdomain.Computed{CostValuation: 1, CostMarketing: 1, CostSimulation: 1}
	cost, err := rmcostdomain.NewGroupCost("202604", "GRP-1", uuid.New(), "Group One", "KG", comp, "tester")
	if err != nil {
		t.Fatalf("NewGroupCost: %v", err)
	}
	if got := mdvFromExisting(cost); got != 0 {
		t.Errorf("nil V2Inputs: got %v, want 0", got)
	}

	cost.AttachV2(rmcostdomain.V2Inputs{ValuationFlag: flagAuto, MarketingFlag: flagAuto}, rmcostdomain.V2Rates{})
	if got := mdvFromExisting(cost); got != 0 {
		t.Errorf("unset MarketingDefaultValue: got %v, want 0", got)
	}
}

// TestMdvFromExisting_PreservesUserEditedValue locks the preservation
// contract: once a value has been snapshotted onto the existing cost row
// (via the "Edit Inputs" drawer), mdvFromExisting must surface it unchanged.
func TestMdvFromExisting_PreservesUserEditedValue(t *testing.T) {
	comp := rmcostdomain.Computed{CostValuation: 1, CostMarketing: 1, CostSimulation: 1}
	cost, err := rmcostdomain.NewGroupCost("202604", "GRP-1", uuid.New(), "Group One", "KG", comp, "tester")
	if err != nil {
		t.Fatalf("NewGroupCost: %v", err)
	}
	mdv := 42.5
	cost.AttachV2(rmcostdomain.V2Inputs{
		MarketingDefaultValue: &mdv,
		ValuationFlag:         flagAuto,
		MarketingFlag:         flagAuto,
	}, rmcostdomain.V2Rates{})

	if got := mdvFromExisting(cost); got != 42.5 {
		t.Errorf("got %v, want 42.5", got)
	}
}

// TestHeaderInputsV2FromHead_MDVOverride locks that headerInputsV2FromHead
// falls back to the master group head's marketing_default_value when no
// override is present (mdvOverride == 0), and otherwise takes the override
// (the user-edited value preserved by mdvFromExisting) over the head's value.
func TestHeaderInputsV2FromHead_MDVOverride(t *testing.T) {
	code, err := rmgroupdomain.NewCode("GRP-1")
	if err != nil {
		t.Fatalf("NewCode: %v", err)
	}
	head, err := rmgroupdomain.NewHead(code, "Group One", "", 5, 0.89, "tester")
	if err != nil {
		t.Fatalf("NewHead: %v", err)
	}
	headDefault := 10.0
	if err := head.AttachMarketingInputs(rmgroupdomain.MarketingInputs{DefaultValue: &headDefault}); err != nil {
		t.Fatalf("AttachMarketingInputs: %v", err)
	}

	// No override (0) — falls back to the head's default value.
	got := headerInputsV2FromHead(head, 0, 0)
	floatNear(t, "no override falls back to head default", got.MarketingDefaultValue, headDefault, 1e-9)

	// Override present — takes precedence over the head's default value.
	got = headerInputsV2FromHead(head, 0, 99.0)
	floatNear(t, "override wins over head default", got.MarketingDefaultValue, 99.0, 1e-9)
}
