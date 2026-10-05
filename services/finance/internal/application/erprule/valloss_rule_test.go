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

func newCreate(repo *fakeRules, audit AuditSink) *CreateVallossRuleHandler {
	h := NewCreateVallossRuleHandler(repo, audit)
	h.now = clock
	return h
}

func TestCreateVallossRule_EmitsAudit(t *testing.T) {
	repo, audit := newFakeRules(), &spyAudit{}
	rule, err := newCreate(repo, audit).Handle(context.Background(), CreateVallossRuleCommand{
		FgType: " Type 1 ", ProdType: "POY", GradeGroup: "BC", Basis: "SPPTY", ValLoss: "0.05", User: "alice",
	})
	require.NoError(t, err)
	assert.Equal(t, int64(1), rule.ID())
	assert.Equal(t, domain.FgType("Type 1"), rule.Key().FgType)
	assert.True(t, rule.ValLoss().Equal(dec("0.05")))

	require.Len(t, audit.events, 1)
	ev := audit.events[0]
	assert.Equal(t, auditdomain.OpRuleCreate, ev.Operation)
	assert.Equal(t, AuditEntityVallossRule, ev.EntityType)
	assert.Equal(t, int64(1), ev.EntityID)
	assert.Equal(t, "alice", ev.UserID)
	assert.Empty(t, ev.BeforeData)
	after := decodeJSON(t, ev.AfterData)
	assert.Equal(t, "Type 1", after["fg_type"])
	assert.Equal(t, "POY", after["prod_type"])
	assert.Equal(t, "BC", after["grade_group"])
	assert.Equal(t, "SPPTY", after["basis"])
	assert.Equal(t, "0.050000", after["val_loss"])
	assert.Equal(t, true, after["is_active"])
}

func TestCreateVallossRule_Invalid(t *testing.T) {
	base := CreateVallossRuleCommand{FgType: "Type 1", ProdType: "POY", GradeGroup: "BC", Basis: "COST", ValLoss: "1", User: "u"}
	cases := []struct {
		name   string
		mutate func(*CreateVallossRuleCommand)
		want   error
	}{
		{"empty fg", func(c *CreateVallossRuleCommand) { c.FgType = " " }, domain.ErrInvalidFgType},
		{"bad prod", func(c *CreateVallossRuleCommand) { c.ProdType = "XXX" }, domain.ErrInvalidProdType},
		{"ax group", func(c *CreateVallossRuleCommand) { c.GradeGroup = "AX" }, domain.ErrAxRule},
		{"bad group", func(c *CreateVallossRuleCommand) { c.GradeGroup = "ZZ" }, domain.ErrInvalidGradeGroup},
		{"bad basis", func(c *CreateVallossRuleCommand) { c.Basis = "SPXXX" }, domain.ErrInvalidBasis},
		{"not a number", func(c *CreateVallossRuleCommand) { c.ValLoss = "abc" }, domain.ErrInvalidPercent},
		{"negative", func(c *CreateVallossRuleCommand) { c.ValLoss = "-0.1" }, domain.ErrInvalidPercent},
		{"too large", func(c *CreateVallossRuleCommand) { c.ValLoss = "1000" }, domain.ErrInvalidPercent},
		{"7 dp", func(c *CreateVallossRuleCommand) { c.ValLoss = "0.0000001" }, domain.ErrInvalidPercent},
		{"no user", func(c *CreateVallossRuleCommand) { c.User = "" }, domain.ErrUserRequired},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo, audit := newFakeRules(), &spyAudit{}
			cmd := base
			tc.mutate(&cmd)
			_, err := newCreate(repo, audit).Handle(context.Background(), cmd)
			require.ErrorIs(t, err, tc.want)
			assert.Empty(t, repo.rows)
			assert.Empty(t, audit.events)
		})
	}
}

func TestCreateVallossRule_DuplicateAndRepoError(t *testing.T) {
	repo, audit := newFakeRules(), &spyAudit{}
	seedRule(t, repo, "Type 1", "POY", "BC", "COST", "0")
	_, err := newCreate(repo, audit).Handle(context.Background(), CreateVallossRuleCommand{
		FgType: "Type 1", ProdType: "POY", GradeGroup: "BC", Basis: "COST", ValLoss: "1", User: "u",
	})
	require.ErrorIs(t, err, domain.ErrDuplicateRule)

	repo.createErr = errors.New("boom")
	_, err = newCreate(repo, audit).Handle(context.Background(), CreateVallossRuleCommand{
		FgType: "Type 2", ProdType: "POY", GradeGroup: "BC", Basis: "COST", ValLoss: "1", User: "u",
	})
	require.EqualError(t, err, "boom")
	assert.Empty(t, audit.events)
}

func TestCreateVallossRule_AuditFailureDoesNotFail(t *testing.T) {
	repo := newFakeRules()
	audit := &spyAudit{err: errors.New("audit down")}
	_, err := newCreate(repo, audit).Handle(context.Background(), CreateVallossRuleCommand{
		FgType: "Type 1", ProdType: "POY", GradeGroup: "BC", Basis: "COST", ValLoss: "1", User: "u",
	})
	require.NoError(t, err)
	assert.Len(t, audit.events, 1)

	// A nil sink is allowed.
	_, err = newCreate(repo, nil).Handle(context.Background(), CreateVallossRuleCommand{
		FgType: "Type 2", ProdType: "POY", GradeGroup: "BC", Basis: "COST", ValLoss: "1", User: "u",
	})
	require.NoError(t, err)
}

func newUpdate(repo *fakeRules, audit AuditSink) *UpdateVallossRuleHandler {
	h := NewUpdateVallossRuleHandler(repo, audit)
	h.now = clock
	return h
}

func TestUpdateVallossRule_BeforeAfter(t *testing.T) {
	repo, audit := newFakeRules(), &spyAudit{}
	r := seedRule(t, repo, "Type 1", "PTY", "NS", "COST", "0")

	got, err := newUpdate(repo, audit).Handle(context.Background(), UpdateVallossRuleCommand{
		ID: r.ID(), Basis: "SPPTY", ValLoss: "12.5", User: "bob",
	})
	require.NoError(t, err)
	assert.Equal(t, domain.BasisSPPTY, got.Basis())
	assert.Equal(t, 1, repo.updates)
	stored, _ := repo.GetByID(context.Background(), r.ID())
	assert.True(t, stored.ValLoss().Equal(dec("12.5")))

	require.Len(t, audit.events, 1)
	ev := audit.events[0]
	assert.Equal(t, auditdomain.OpRuleUpdate, ev.Operation)
	assert.Equal(t, r.ID(), ev.EntityID)
	assert.Equal(t, "bob", ev.UserID)
	before, after := decodeJSON(t, ev.BeforeData), decodeJSON(t, ev.AfterData)
	assert.Equal(t, "COST", before["basis"])
	assert.Equal(t, "0.000000", before["val_loss"])
	assert.NotContains(t, before, "updated_at")
	assert.Equal(t, "SPPTY", after["basis"])
	assert.Equal(t, "12.500000", after["val_loss"])
	assert.Equal(t, "bob", after["updated_by"])
	assert.Equal(t, "2026-09-29T10:00:00Z", after["updated_at"])
}

func TestUpdateVallossRule_NoChangeIsNoop(t *testing.T) {
	repo, audit := newFakeRules(), &spyAudit{}
	r := seedRule(t, repo, "Type 1", "PTY", "NS", "COST", "0.5")
	_, err := newUpdate(repo, audit).Handle(context.Background(), UpdateVallossRuleCommand{
		ID: r.ID(), Basis: "COST", ValLoss: "0.500000", User: "bob",
	})
	require.NoError(t, err)
	assert.Zero(t, repo.updates)
	assert.Empty(t, audit.events)
}

func TestUpdateVallossRule_Errors(t *testing.T) {
	repo, audit := newFakeRules(), &spyAudit{}
	r := seedRule(t, repo, "Type 1", "PTY", "NS", "COST", "0")
	h := newUpdate(repo, audit)
	ctx := context.Background()

	_, err := h.Handle(ctx, UpdateVallossRuleCommand{ID: r.ID(), Basis: "BAD", ValLoss: "1", User: "u"})
	require.ErrorIs(t, err, domain.ErrInvalidBasis)
	_, err = h.Handle(ctx, UpdateVallossRuleCommand{ID: r.ID(), Basis: "COST", ValLoss: "x", User: "u"})
	require.ErrorIs(t, err, domain.ErrInvalidPercent)
	_, err = h.Handle(ctx, UpdateVallossRuleCommand{ID: 99, Basis: "COST", ValLoss: "1", User: "u"})
	require.ErrorIs(t, err, domain.ErrRuleNotFound)
	_, err = h.Handle(ctx, UpdateVallossRuleCommand{ID: r.ID(), Basis: "COST", ValLoss: "1", User: ""})
	require.ErrorIs(t, err, domain.ErrUserRequired)

	repo.updateErr = errors.New("db")
	_, err = h.Handle(ctx, UpdateVallossRuleCommand{ID: r.ID(), Basis: "COST", ValLoss: "1", User: "u"})
	require.EqualError(t, err, "db")
	repo.updateErr = nil

	// Inactive rule.
	stored := repo.rows[r.ID()]
	require.NoError(t, stored.Deactivate("u", fixedNow))
	_, err = h.Handle(ctx, UpdateVallossRuleCommand{ID: r.ID(), Basis: "COST", ValLoss: "1", User: "u"})
	require.ErrorIs(t, err, domain.ErrRuleInactive)
	assert.Empty(t, audit.events)
}

type deleteFixture struct {
	rules  *fakeRules
	prices *fakePrices
	loader *fakeLoader
	usage  *fakeUsage
	audit  *spyAudit
	h      *DeleteVallossRuleHandler
}

func newDeleteFixture() *deleteFixture {
	f := &deleteFixture{rules: newFakeRules(), prices: newFakePrices(), usage: &fakeUsage{inUse: map[string]bool{}}, audit: &spyAudit{}}
	f.loader = &fakeLoader{rules: f.rules, prices: f.prices}
	f.h = NewDeleteVallossRuleHandler(f.rules, f.loader, f.usage, f.audit)
	f.h.now = clock
	return f
}

func TestDeleteVallossRule_SoftDeleteWithAudit(t *testing.T) {
	f := newDeleteFixture()
	r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
	seedRule(t, f.rules, "Type 2", "POY", "BB", "COST", "3")

	require.NoError(t, f.h.Handle(context.Background(), DeleteVallossRuleCommand{ID: r.ID(), User: "carol"}))
	assert.False(t, f.rules.rows[r.ID()].IsActive())
	require.Len(t, f.usage.queried, 1)
	assert.Len(t, f.usage.queried[0], 64)

	require.Len(t, f.audit.events, 1)
	ev := f.audit.events[0]
	assert.Equal(t, auditdomain.OpRuleDelete, ev.Operation)
	assert.Equal(t, r.ID(), ev.EntityID)
	assert.Equal(t, "carol", ev.UserID)
	assert.Equal(t, true, decodeJSON(t, ev.BeforeData)["is_active"])
	after := decodeJSON(t, ev.AfterData)
	assert.Equal(t, false, after["is_active"])
	assert.Equal(t, "carol", after["updated_by"])
}

func TestDeleteVallossRule_RefusedWhileHashInUse(t *testing.T) {
	f := newDeleteFixture()
	r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
	rs, err := f.loader.LoadRuleSet(context.Background())
	require.NoError(t, err)
	f.usage.inUse[rs.Hash()] = true

	err = f.h.Handle(context.Background(), DeleteVallossRuleCommand{ID: r.ID(), User: "carol"})
	require.ErrorIs(t, err, domain.ErrRuleInUse)
	assert.True(t, f.rules.rows[r.ID()].IsActive())
	assert.Zero(t, f.rules.updates)
	assert.Empty(t, f.audit.events)
}

func TestDeleteVallossRule_Errors(t *testing.T) {
	ctx := context.Background()

	t.Run("not found", func(t *testing.T) {
		f := newDeleteFixture()
		require.ErrorIs(t, f.h.Handle(ctx, DeleteVallossRuleCommand{ID: 7, User: "u"}), domain.ErrRuleNotFound)
	})
	t.Run("already inactive", func(t *testing.T) {
		f := newDeleteFixture()
		r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
		require.NoError(t, f.rules.rows[r.ID()].Deactivate("u", fixedNow))
		require.ErrorIs(t, f.h.Handle(ctx, DeleteVallossRuleCommand{ID: r.ID(), User: "u"}), domain.ErrRuleInactive)
		assert.Zero(t, f.loader.loads)
	})
	t.Run("load fails closed", func(t *testing.T) {
		f := newDeleteFixture()
		r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
		f.loader.loadErr = errors.New("snapshot")
		err := f.h.Handle(ctx, DeleteVallossRuleCommand{ID: r.ID(), User: "u"})
		require.ErrorContains(t, err, "snapshot")
		assert.True(t, f.rules.rows[r.ID()].IsActive())
	})
	t.Run("usage fails closed", func(t *testing.T) {
		f := newDeleteFixture()
		r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
		f.usage.err = errors.New("batch table")
		err := f.h.Handle(ctx, DeleteVallossRuleCommand{ID: r.ID(), User: "u"})
		require.ErrorContains(t, err, "batch table")
		assert.True(t, f.rules.rows[r.ID()].IsActive())
	})
	t.Run("missing ports fail closed", func(t *testing.T) {
		f := newDeleteFixture()
		r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
		h := NewDeleteVallossRuleHandler(f.rules, nil, nil, f.audit)
		require.ErrorIs(t, h.Handle(ctx, DeleteVallossRuleCommand{ID: r.ID(), User: "u"}), domain.ErrRuleInUse)
		assert.True(t, f.rules.rows[r.ID()].IsActive())
	})
	t.Run("no user", func(t *testing.T) {
		f := newDeleteFixture()
		r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
		require.ErrorIs(t, f.h.Handle(ctx, DeleteVallossRuleCommand{ID: r.ID(), User: " "}), domain.ErrUserRequired)
	})
	t.Run("update fails", func(t *testing.T) {
		f := newDeleteFixture()
		r := seedRule(t, f.rules, "Type 1", "POY", "BB", "COST", "2")
		f.rules.updateErr = errors.New("db")
		require.EqualError(t, f.h.Handle(ctx, DeleteVallossRuleCommand{ID: r.ID(), User: "u"}), "db")
		assert.Empty(t, f.audit.events)
	})
}
