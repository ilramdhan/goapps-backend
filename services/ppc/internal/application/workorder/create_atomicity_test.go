package workorder_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	workorderapp "github.com/mutugading/goapps-backend/services/ppc/internal/application/workorder"
	workorderdomain "github.com/mutugading/goapps-backend/services/ppc/internal/domain/workorder"
)

// A WO and its planned parameters are one unit of work. A header that commits
// without its parameters gives the PC operator an empty sheet with nothing in
// the UI to say the rows are simply missing — it reads as a WO with no spec
// rather than as a failure. These tests pin both halves of that: the parameters
// are written by the same call that writes the header (so they share its
// transaction), and a failed header write leaves nothing behind.

// errCreate stands in for a constraint violation or a dropped connection on the
// WO insert.
var errCreate = errors.New("wo insert failed")

const knownLot = "TXT0009-26"

func atomicitySvc(repo *memRepo, lots *stubLots) (*workorderapp.Service, *stubLotProv) {
	prov := newStubLotProv(repo, lots)
	svc := lotSvcDeps(repo, lots, &stubLotSpecs{item: testItemCode, shade: testShadeCode}, "5", prov)
	return svc, prov
}

// ── Create: parameters ride the header's write ────────────────────────────────

func TestCreate_ManualLot_PersistsParametersWithHeader(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{knownLot: true}}
	svc, _ := atomicitySvc(repo, lots)

	wo, err := svc.Create(context.Background(), createCmd(knownLot))
	require.NoError(t, err)

	// Written by Create itself, not by a follow-up call: memRepo only records
	// parameters that were attached to the entity handed to Create.
	stored := repo.params[wo.ID()]
	require.NotEmpty(t, stored, "resolved parameters must land with the WO header")
	assert.Equal(t, wo.ID(), stored[0].WOID, "the repository stamps the WO id it just generated")
	require.NotEmpty(t, wo.Parameters())
}

func TestCreate_BlankLot_PersistsParametersWithHeader(t *testing.T) {
	repo := newMemRepo()
	lots := &stubLots{known: map[string]bool{}}
	svc, prov := atomicitySvc(repo, lots)

	wo, err := svc.Create(context.Background(), createCmd(""))
	require.NoError(t, err)
	require.Equal(t, 1, prov.calls, "generated-lot path must go through the provisioner")

	stored := repo.params[wo.ID()]
	require.NotEmpty(t, stored, "generated-lot creates must carry their parameters too")
	assert.Equal(t, wo.ID(), stored[0].WOID)
}

// ── Create: a failed write leaves nothing behind ──────────────────────────────

func TestCreate_ManualLot_WriteFails_NothingPersisted(t *testing.T) {
	repo := newMemRepo()
	repo.createErr = errCreate
	lots := &stubLots{known: map[string]bool{knownLot: true}}
	svc, _ := atomicitySvc(repo, lots)

	_, err := svc.Create(context.Background(), createCmd(knownLot))
	require.ErrorIs(t, err, errCreate)
	assert.Empty(t, repo.orders, "no header may survive a failed create")
	assert.Empty(t, repo.params, "no orphan parameter rows may survive either")
}

func TestCreate_BlankLot_WriteFails_NothingPersisted(t *testing.T) {
	repo := newMemRepo()
	repo.createErr = errCreate
	lots := &stubLots{known: map[string]bool{}}
	svc, _ := atomicitySvc(repo, lots)

	_, err := svc.Create(context.Background(), createCmd(""))
	require.ErrorIs(t, err, errCreate)
	assert.Empty(t, repo.orders)
	assert.Empty(t, repo.params)
}

// ── CreateWOReference: the carry-forward path ─────────────────────────────────

func referenceCmd(srcID int64, lotNo string) workorderapp.CreateWOReferenceCommand {
	return workorderapp.CreateWOReferenceCommand{
		SourceWOID: srcID,
		RefType:    workorderdomain.RefTypeContinuation,
		LotNo:      lotNo,
		QtyTarget:  200,
		Deadline:   time.Now().Add(72 * time.Hour),
		CreatedBy:  3,
	}
}

func TestCreateWOReference_BlankLot_PersistsParametersWithHeader(t *testing.T) {
	repo := newMemRepo()
	src := seedSourceWO(t, repo)
	lots := &stubLots{known: map[string]bool{}}
	svc, prov := atomicitySvc(repo, lots)

	newWO, err := svc.CreateWOReference(context.Background(), referenceCmd(src.ID(), ""))
	require.NoError(t, err)
	require.Equal(t, 1, prov.calls)
	require.NotEqual(t, src.ID(), newWO.ID())

	stored := repo.params[newWO.ID()]
	require.NotEmpty(t, stored, "a carry-forward must not commit a header with no parameters")
	assert.Equal(t, newWO.ID(), stored[0].WOID)
}

func TestCreateWOReference_WriteFails_NothingPersisted(t *testing.T) {
	repo := newMemRepo()
	src := seedSourceWO(t, repo)
	// Seeding is done, so only the reference WO's own write fails.
	repo.createErr = errCreate
	lots := &stubLots{known: map[string]bool{knownLot: true}}
	svc, _ := atomicitySvc(repo, lots)

	_, err := svc.CreateWOReference(context.Background(), referenceCmd(src.ID(), knownLot))
	require.ErrorIs(t, err, errCreate)
	assert.Len(t, repo.orders, 1, "only the source WO may remain")
	assert.Len(t, repo.params, 1, "and only the source WO's parameters")
	assert.NotContains(t, repo.params, src.ID()+1, "the rolled-back reference must leave no parameters")
}
