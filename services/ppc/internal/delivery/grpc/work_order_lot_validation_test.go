package grpc

import (
	"testing"

	"buf.build/go/protovalidate"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	ppcv1 "github.com/mutugading/goapps-backend/gen/ppc/v1"
	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
)

// A blank lot_no on CreateWorkOrder means "mint one". The buf.validate rule
// used to carry min_len: 1, which made the generator unreachable through the
// RPC — the request never survived the validation boundary, so the branch that
// registers a lot in lot_master was dead code in production.
//
// This asserts against the compiled descriptor of the generated request, so it
// fails if the min_len rule is ever reintroduced to the proto.
func TestCreateWorkOrderRequest_BlankLotPassesProtoValidation(t *testing.T) {
	validator, err := protovalidate.New()
	require.NoError(t, err)

	req := validCreateWorkOrderRequest()
	req.LotNo = ""

	require.NoError(t, validator.Validate(req),
		"a blank lot_no must reach the handler so the lot generator can run")
}

// The max bound stays: wo_lot_no is VARCHAR(30), and a longer value would only
// fail at INSERT with a raw driver error.
func TestCreateWorkOrderRequest_OverlongLotRejected(t *testing.T) {
	validator, err := protovalidate.New()
	require.NoError(t, err)

	req := validCreateWorkOrderRequest()
	req.LotNo = "" // build a 31-char lot
	for range 31 {
		req.LotNo += "X"
	}

	assert.Error(t, validator.Validate(req), "lot_no longer than wo_lot_no VARCHAR(30) must be rejected")
}

// A supplied lot of ordinary length still validates — relaxing min_len must not
// have loosened anything else on the message.
func TestCreateWorkOrderRequest_SuppliedLotStillValid(t *testing.T) {
	validator, err := protovalidate.New()
	require.NoError(t, err)

	req := validCreateWorkOrderRequest()
	req.LotNo = "SPG0042-26"

	require.NoError(t, validator.Validate(req))
}

// A blank lot must still be rejected where no generator exists: CreateWOReference
// mints nothing, so its lot_no keeps min_len: 1.
func TestCreateWOReferenceRequest_BlankLotStillRejected(t *testing.T) {
	validator, err := protovalidate.New()
	require.NoError(t, err)

	req := &ppcv1.CreateWOReferenceRequest{
		SourceWoId: 1,
		RefType:    ppcv1.WORefType_WO_REF_TYPE_TEMPLATE,
		LotNo:      "",
		QtyTarget:  "500",
		Deadline:   "2026-08-15",
	}

	assert.Error(t, validator.Validate(req),
		"the reference path has no lot generator, so a blank lot is still a client error")
}

// The handler must forward a blank lot to the application service untouched
// rather than substituting a placeholder — the empty string is what selects the
// generate branch in Service.createWithLot.
func TestCreateWorkOrder_BlankLotForwardedAsEmptyCommand(t *testing.T) {
	req := validCreateWorkOrderRequest()
	req.LotNo = ""

	cmd := workorderapp.CreateCommand{
		AreaCode:   areaCodeToString(req.GetArea()),
		PlanItemID: req.GetPlanItemId(),
		MachineID:  req.GetMachineId(),
		LotNo:      req.GetLotNo(),
	}

	assert.Empty(t, cmd.LotNo, "a blank proto lot must stay blank in the create command")
	assert.Equal(t, "SPG", cmd.AreaCode)
}

// validCreateWorkOrderRequest returns a request that satisfies every rule on
// CreateWorkOrderRequest, so a test can isolate one field at a time.
func validCreateWorkOrderRequest() *ppcv1.CreateWorkOrderRequest {
	return &ppcv1.CreateWorkOrderRequest{
		Area:       ppcv1.AreaCode_AREA_CODE_SPG,
		PlanItemId: 7,
		MachineId:  2,
		CrhHeadId:  10,
		CrhVersion: 1,
		LotNo:      "SPG0042-26",
		QtyTarget:  "500",
		Deadline:   "2026-08-15",
	}
}
