package erprule

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func newUpsert(repo *fakePrices, audit AuditSink) *UpsertSellPriceHandler {
	h := NewUpsertSellPriceHandler(repo, audit)
	h.now = clock
	return h
}

func TestUpsertSellPrice_CreateThenUpdate(t *testing.T) {
	repo, audit := newFakePrices(), &spyAudit{}
	h := newUpsert(repo, audit)
	ctx := context.Background()

	sp, err := h.Handle(ctx, UpsertSellPriceCommand{Basis: "SPPTY", Price: "1.95", User: "dina"})
	require.NoError(t, err)
	assert.True(t, sp.Price().Equal(dec("1.95")))
	require.Len(t, audit.events, 1)
	ev := audit.events[0]
	assert.Equal(t, auditdomain.OpRuleCreate, ev.Operation)
	assert.Equal(t, AuditEntitySellPrice, ev.EntityType)
	assert.Equal(t, int64(1), ev.EntityID)
	assert.Empty(t, ev.BeforeData)
	assert.Equal(t, "1.950000", decodeJSON(t, ev.AfterData)["price"])

	_, err = h.Handle(ctx, UpsertSellPriceCommand{Basis: "SPPTY", Price: "2.1", User: "eko"})
	require.NoError(t, err)
	require.Len(t, audit.events, 2)
	ev = audit.events[1]
	assert.Equal(t, auditdomain.OpRuleUpdate, ev.Operation)
	assert.Equal(t, "eko", ev.UserID)
	assert.Equal(t, "1.950000", decodeJSON(t, ev.BeforeData)["price"])
	after := decodeJSON(t, ev.AfterData)
	assert.Equal(t, "2.100000", after["price"])
	assert.Equal(t, "dina", after["created_by"])
	assert.Equal(t, "eko", after["updated_by"])
}

func TestUpsertSellPrice_NoopAndReactivate(t *testing.T) {
	repo, audit := newFakePrices(), &spyAudit{}
	repo.rows[domain.BasisSPITY] = domain.ReconstructSellPrice(domain.BasisSPITY, dec("3"), true, fixedNow, "seed", nil, "")
	h := newUpsert(repo, audit)
	ctx := context.Background()

	_, err := h.Handle(ctx, UpsertSellPriceCommand{Basis: "SPITY", Price: "3.000000", User: "u"})
	require.NoError(t, err)
	assert.Zero(t, repo.upserts)
	assert.Empty(t, audit.events)

	// Inactive with the same price: the upsert re-activates it and is audited.
	repo.rows[domain.BasisSPITY] = domain.ReconstructSellPrice(domain.BasisSPITY, dec("3"), false, fixedNow, "seed", nil, "")
	sp, err := h.Handle(ctx, UpsertSellPriceCommand{Basis: "SPITY", Price: "3", User: "u"})
	require.NoError(t, err)
	assert.True(t, sp.IsActive())
	require.Len(t, audit.events, 1)
	assert.Equal(t, false, decodeJSON(t, audit.events[0].BeforeData)["is_active"])
	assert.Equal(t, true, decodeJSON(t, audit.events[0].AfterData)["is_active"])
	assert.Equal(t, int64(2), audit.events[0].EntityID)
}

func TestUpsertSellPrice_Errors(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name string
		cmd  UpsertSellPriceCommand
		want error
	}{
		{"cost basis", UpsertSellPriceCommand{Basis: "COST", Price: "1", User: "u"}, domain.ErrInvalidBasis},
		{"unknown basis", UpsertSellPriceCommand{Basis: "X", Price: "1", User: "u"}, domain.ErrInvalidBasis},
		{"nan", UpsertSellPriceCommand{Basis: "SPBSD", Price: "one", User: "u"}, domain.ErrInvalidPrice},
		{"zero", UpsertSellPriceCommand{Basis: "SPBSD", Price: "0", User: "u"}, domain.ErrInvalidPrice},
		{"7 dp", UpsertSellPriceCommand{Basis: "SPBSD", Price: "1.0000001", User: "u"}, domain.ErrInvalidPrice},
		{"no user", UpsertSellPriceCommand{Basis: "SPBSD", Price: "1", User: ""}, domain.ErrUserRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, audit := newFakePrices(), &spyAudit{}
			_, err := newUpsert(repo, audit).Handle(ctx, tc.cmd)
			require.ErrorIs(t, err, tc.want)
			assert.Zero(t, repo.upserts)
			assert.Empty(t, audit.events)
		})
	}

	t.Run("update invalid user", func(t *testing.T) {
		repo, audit := newFakePrices(), &spyAudit{}
		repo.rows[domain.BasisSPBSD] = domain.ReconstructSellPrice(domain.BasisSPBSD, dec("3"), true, fixedNow, "seed", nil, "")
		_, err := newUpsert(repo, audit).Handle(ctx, UpsertSellPriceCommand{Basis: "SPBSD", Price: "4", User: ""})
		require.ErrorIs(t, err, domain.ErrUserRequired)
	})
	t.Run("get error", func(t *testing.T) {
		repo := newFakePrices()
		repo.getErr = errors.New("db")
		_, err := newUpsert(repo, nil).Handle(ctx, UpsertSellPriceCommand{Basis: "SPBSD", Price: "1", User: "u"})
		require.EqualError(t, err, "db")
	})
	t.Run("upsert error on create and update", func(t *testing.T) {
		repo, audit := newFakePrices(), &spyAudit{}
		repo.upsertErr = errors.New("write")
		_, err := newUpsert(repo, audit).Handle(ctx, UpsertSellPriceCommand{Basis: "SPBSD", Price: "1", User: "u"})
		require.EqualError(t, err, "write")
		repo.rows[domain.BasisSPBSD] = domain.ReconstructSellPrice(domain.BasisSPBSD, dec("3"), true, fixedNow, "seed", nil, "")
		_, err = newUpsert(repo, audit).Handle(ctx, UpsertSellPriceCommand{Basis: "SPBSD", Price: "4", User: "u"})
		require.EqualError(t, err, "write")
		assert.Empty(t, audit.events)
	})
}

func TestSellPriceAuditID(t *testing.T) {
	assert.Equal(t, int64(1), SellPriceAuditID(domain.BasisSPPTY))
	assert.Equal(t, int64(2), SellPriceAuditID(domain.BasisSPITY))
	assert.Equal(t, int64(3), SellPriceAuditID(domain.BasisSPBSD))
	assert.Equal(t, int64(0), SellPriceAuditID(domain.BasisCost))
}

func TestAssignGradeGroup_AssignChangeClear(t *testing.T) {
	repo, audit := newFakeGrades(), &spyAudit{}
	repo.add("A1", "Grade A1", nil)
	h := NewAssignGradeGroupHandler(repo, audit)
	ctx := context.Background()

	g, err := h.Handle(ctx, AssignGradeGroupCommand{GradeCode: " A1 ", GradeGroup: "BC", User: "fani"})
	require.NoError(t, err)
	require.NotNil(t, g.Group())
	assert.Equal(t, domain.GradeGroupBC, *repo.rows["A1"].Group())
	require.Len(t, audit.events, 1)
	ev := audit.events[0]
	assert.Equal(t, auditdomain.OpRuleUpdate, ev.Operation)
	assert.Equal(t, AuditEntityGradeGroup, ev.EntityType)
	assert.Equal(t, "fani", ev.UserID)
	before, after := decodeJSON(t, ev.BeforeData), decodeJSON(t, ev.AfterData)
	assert.Equal(t, "A1", before["grade_code"])
	assert.Nil(t, before["grade_group"])
	assert.Contains(t, before, "grade_group")
	assert.Equal(t, "BC", after["grade_group"])
	assert.Equal(t, "Grade A1", after["grade_name"])

	// AX may be assigned to a grade (it is only refused on rules).
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "AX", User: "fani"})
	require.NoError(t, err)
	require.Len(t, audit.events, 2)
	assert.Equal(t, "BC", decodeJSON(t, audit.events[1].BeforeData)["grade_group"])
	assert.Equal(t, "AX", decodeJSON(t, audit.events[1].AfterData)["grade_group"])

	// Clear.
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "", User: "fani"})
	require.NoError(t, err)
	assert.Nil(t, repo.rows["A1"].Group())
	require.Len(t, audit.events, 3)
	assert.Equal(t, "AX", decodeJSON(t, audit.events[2].BeforeData)["grade_group"])
	assert.Nil(t, decodeJSON(t, audit.events[2].AfterData)["grade_group"])
	assert.Equal(t, 3, repo.sets)
}

func TestAssignGradeGroup_NoopAndErrors(t *testing.T) {
	ctx := context.Background()
	repo, audit := newFakeGrades(), &spyAudit{}
	repo.add("A1", "Grade A1", gg(domain.GradeGroupNS))
	repo.add("A2", "Grade A2", nil)
	h := NewAssignGradeGroupHandler(repo, audit)

	_, err := h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "NS", User: "u"})
	require.NoError(t, err)
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A2", GradeGroup: " ", User: "u"})
	require.NoError(t, err)
	assert.Zero(t, repo.sets)
	assert.Empty(t, audit.events)

	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "", GradeGroup: "NS", User: "u"})
	require.ErrorIs(t, err, domain.ErrInvalidGradeCode)
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "QQ", User: "u"})
	require.ErrorIs(t, err, domain.ErrInvalidGradeGroup)
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "ZZ", GradeGroup: "NS", User: "u"})
	require.ErrorIs(t, err, domain.ErrGradeNotFound)
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "BB", User: ""})
	require.ErrorIs(t, err, domain.ErrUserRequired)
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "", User: ""})
	require.ErrorIs(t, err, domain.ErrUserRequired)

	repo.setErr = errors.New("db")
	_, err = h.Handle(ctx, AssignGradeGroupCommand{GradeCode: "A1", GradeGroup: "BB", User: "u"})
	require.EqualError(t, err, "db")
	assert.Equal(t, domain.GradeGroupNS, *repo.rows["A1"].Group())
	assert.Empty(t, audit.events)
}

func TestSameGroup(t *testing.T) {
	assert.True(t, sameGroup(nil, nil))
	assert.False(t, sameGroup(gg(domain.GradeGroupNS), nil))
	assert.False(t, sameGroup(nil, gg(domain.GradeGroupNS)))
	assert.True(t, sameGroup(gg(domain.GradeGroupNS), gg(domain.GradeGroupNS)))
	assert.False(t, sameGroup(gg(domain.GradeGroupNS), gg(domain.GradeGroupBB)))
}
