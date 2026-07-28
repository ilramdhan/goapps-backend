package workorder_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	"github.com/mutugading/goapps-backend/services/ppc/internal/domain/shared"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

const (
	testItemCode   = "ITM-001"
	testShadeCode  = "NL"
	stdWeightParam = "cccccccc-cccc-cccc-cccc-cccccccccccc"
)

// ---------------------------------------------------------------------------
// Stubs for the lot-provisioning collaborators.
// ---------------------------------------------------------------------------

// stubLots answers lot_master existence checks from a fixed set.
type stubLots struct{ known map[string]bool }

func (s *stubLots) LotExists(_ context.Context, lotNo string) (bool, error) {
	return s.known[lotNo], nil
}

// stubPlanItems maps every plan item to one finance product.
type stubPlanItems struct{ productSysID int64 }

func (s *stubPlanItems) ProductSysID(context.Context, int64) (int64, error) {
	return s.productSysID, nil
}

// stubLotSpecs returns the item/shade codes a generated lot is keyed by. Empty
// codes model finance-degraded, which must block generation rather than invent
// a lot.
type stubLotSpecs struct{ item, shade string }

func (s *stubLotSpecs) LotSpec(context.Context, int64) (string, string, error) {
	return s.item, s.shade, nil
}

// stubParamDefs feeds the resolver a single STD_WEIGHT definition whose default
// supplies the standard full bobbin weight.
type stubParamDefs struct{ stdWeight string }

func (s *stubParamDefs) ListParamDefs(context.Context, string) ([]workorderdomain.ParamDef, error) {
	if s.stdWeight == "" {
		return nil, nil
	}
	return []workorderdomain.ParamDef{{
		ParamID:      stdWeightParam,
		ParamCode:    workorderdomain.WellKnownStdWeight,
		ParamName:    "Standard bobbin weight",
		DataType:     "NUMBER",
		DisplayGroup: "Machine",
		DefaultValue: s.stdWeight,
	}}, nil
}

// stubLotProv records the provision request and registers the minted lot in an
// in-memory lot_master, mirroring the single-transaction Postgres provisioner.
type stubLotProv struct {
	repo    *memRepo
	lots    *stubLots
	weights map[string][2]float64 // lot -> {full, unfull}
	seq     int
	last    workorderdomain.LotProvisionRequest
	calls   int
}

func newStubLotProv(repo *memRepo, lots *stubLots) *stubLotProv {
	return &stubLotProv{repo: repo, lots: lots, weights: map[string][2]float64{}}
}

func (p *stubLotProv) CreateWithGeneratedLot(
	ctx context.Context,
	req workorderdomain.LotProvisionRequest,
	build func(lotNo string) (*workorderdomain.WorkOrder, error),
) (*workorderdomain.WorkOrder, error) {
	p.calls++
	p.last = req
	p.seq++
	lotNo := workorderdomain.FormatLotNo(req.AreaCode, req.Year, p.seq)
	// Registering the lot is what makes the ETL able to price its bobbins.
	p.lots.known[lotNo] = true
	p.weights[lotNo] = [2]float64{req.StdWeightFull, req.StdWeightUnfull}
	entity, err := build(lotNo)
	if err != nil {
		return nil, err
	}
	if err := p.repo.Create(ctx, entity); err != nil {
		return nil, err
	}
	return entity, nil
}

// lotSvcDeps assembles a service with the full lot-provisioning chain wired.
func lotSvcDeps(repo *memRepo, lots *stubLots, specs *stubLotSpecs, stdWeight string, prov workorderapp.LotProvisioner) *workorderapp.Service {
	return workorderapp.NewService(repo, workorderapp.Deps{
		Lots:      lots,
		PlanItems: &stubPlanItems{productSysID: 900},
		Resolver:  workorderdomain.NewResolver(&stubParamDefs{stdWeight: stdWeight}, nil, nil, nil),
		LotSpecs:  specs,
		LotProv:   prov,
	})
}

func createCmd(lotNo string) workorderapp.CreateCommand {
	return workorderapp.CreateCommand{
		AreaCode:   "TXT",
		PlanItemID: 7,
		MachineID:  2,
		CrhHeadID:  10,
		CrhVersion: 1,
		LotNo:      lotNo,
		QtyTarget:  500,
		Deadline:   time.Now().Add(48 * time.Hour),
		CreatedBy:  3,
	}
}

// ---------------------------------------------------------------------------
// Lot number format
// ---------------------------------------------------------------------------

func TestFormatLotNo_TranscribableForm(t *testing.T) {
	tests := []struct {
		name      string
		area      string
		year, seq int
		want      string
	}{
		{"first of year", "TXT", 2026, 1, "TXT0001-26"},
		{"mid sequence", "SPG", 2026, 42, "SPG0042-26"},
		{"four digits", "TWT", 2027, 1234, "TWT1234-27"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := workorderdomain.FormatLotNo(tt.area, tt.year, tt.seq)
			assert.Equal(t, tt.want, got)
			// wo_lot_no is VARCHAR(30); an operator also has to copy this onto
			// a paper doff tag, so it must stay short.
			assert.LessOrEqual(t, len(got), 30)
		})
	}
}

// ---------------------------------------------------------------------------
// Manual lot: must exist in lot_master
// ---------------------------------------------------------------------------

func TestCreate_ManualLotNotInMaster_Rejected(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{item: testItemCode, shade: testShadeCode}, "5", newStubLotProv(repo, lots))

	_, err := svc.Create(context.Background(), createCmd("TYPO-1"))
	require.ErrorIs(t, err, workorderdomain.ErrLotNotFound)
	assert.Empty(t, repo.orders, "a WO must not be persisted against an unknown lot")
}

func TestCreate_ManualLotInMaster_Accepted(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{"TXT0009-26": true}}
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{item: testItemCode, shade: testShadeCode}, "5", newStubLotProv(repo, lots))

	wo, err := svc.Create(context.Background(), createCmd("TXT0009-26"))
	require.NoError(t, err)
	assert.Equal(t, "TXT0009-26", wo.LotNo())
}

func TestCreateWOReference_LotNotInMaster_Rejected(t *testing.T) {
	repo := newMemRepo()
	src := seedSourceWO(t, repo)
	lots := &stubLots{known: map[string]bool{}}
	svc := workorderapp.NewService(repo, workorderapp.Deps{Lots: lots})

	_, err := svc.CreateWOReference(context.Background(), workorderapp.CreateWOReferenceCommand{
		SourceWOID: src.ID(),
		RefType:    workorderdomain.RefTypeTemplate,
		LotNo:      "UNKNOWN",
		QtyTarget:  600,
		Deadline:   time.Now().Add(72 * time.Hour),
		CreatedBy:  3,
	})
	require.ErrorIs(t, err, workorderdomain.ErrLotNotFound)
}

// ---------------------------------------------------------------------------
// Auto-generated lot: registered in lot_master with usable standard weights
// ---------------------------------------------------------------------------

func TestCreate_BlankLot_RegistersLotWithStandardWeights(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := newStubLotProv(repo, lots)
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{item: testItemCode, shade: testShadeCode}, "5", prov)

	wo, err := svc.Create(context.Background(), createCmd(""))
	require.NoError(t, err)

	assert.Equal(t, 1, prov.calls)
	assert.Equal(t, workorderdomain.FormatLotNo("TXT", time.Now().Year(), 1), wo.LotNo())
	assert.True(t, lots.known[wo.LotNo()], "generated lot must be registered in lot_master")

	// The provision request must carry real inputs, not placeholders.
	assert.Equal(t, testItemCode, prov.last.ItemCode)
	assert.Equal(t, testShadeCode, prov.last.ShadeCode)
	assert.InDelta(t, 5.0, prov.last.StdWeightFull, 1e-9)
	assert.InDelta(t, 5.0*workorderdomain.LotUnfullWeightRatio, prov.last.StdWeightUnfull, 1e-9)
	assert.NotEmpty(t, prov.last.Notes, "estimated unfull weight must be flagged in the lot notes")

	// The concrete bug this closes: the ETL can price bobbins booked against
	// the generated lot. Before, the lot was never registered, so the standard
	// weights lookup found nothing and qty_bobbin came out zero.
	w, ok := prov.weights[wo.LotNo()]
	require.True(t, ok, "lot standard weights must resolve for the generated lot")
	qtyBobbin := shared.QtyTXT(10, 2, w[0], w[1])
	assert.Greater(t, qtyBobbin, 0.0)
	assert.InDelta(t, 10*5.0+2*2.5, qtyBobbin, 1e-9)
}

func TestCreate_BlankLot_NoStdWeight_Rejected(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := newStubLotProv(repo, lots)
	// No STD_WEIGHT definition → resolution yields nothing.
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{item: testItemCode, shade: testShadeCode}, "", prov)

	_, err := svc.Create(context.Background(), createCmd(""))
	require.ErrorIs(t, err, workorderdomain.ErrLotSpecUnavailable)
	assert.Zero(t, prov.calls, "no lot may be registered against invented weights")
	assert.Empty(t, repo.orders)
}

func TestCreate_BlankLot_NoItemShade_Rejected(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	prov := newStubLotProv(repo, lots)
	// Finance degraded → empty item/shade codes.
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{}, "5", prov)

	_, err := svc.Create(context.Background(), createCmd(""))
	require.ErrorIs(t, err, workorderdomain.ErrLotSpecUnavailable)
	assert.Zero(t, prov.calls)
}

func TestCreate_BlankLot_NoProvisioner_Rejected(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{item: testItemCode, shade: testShadeCode}, "5", nil)

	_, err := svc.Create(context.Background(), createCmd(""))
	require.ErrorIs(t, err, workorderdomain.ErrLotGenerationUnavailable)
	assert.Empty(t, repo.orders)
}

func TestLotProvisionRequest_Validate(t *testing.T) {
	base := workorderdomain.LotProvisionRequest{
		AreaCode: "TXT", Year: 2026, ItemCode: testItemCode, ShadeCode: testShadeCode,
		StdWeightFull: 5, StdWeightUnfull: 2.5,
	}
	require.NoError(t, base.Validate())

	tests := []struct {
		name   string
		mutate func(r *workorderdomain.LotProvisionRequest)
	}{
		{"no area", func(r *workorderdomain.LotProvisionRequest) { r.AreaCode = "" }},
		{"no item code", func(r *workorderdomain.LotProvisionRequest) { r.ItemCode = "" }},
		{"no shade code", func(r *workorderdomain.LotProvisionRequest) { r.ShadeCode = "" }},
		{"zero full weight", func(r *workorderdomain.LotProvisionRequest) { r.StdWeightFull = 0 }},
		{"zero unfull weight", func(r *workorderdomain.LotProvisionRequest) { r.StdWeightUnfull = 0 }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := base
			tt.mutate(&r)
			assert.Error(t, r.Validate())
		})
	}
}
