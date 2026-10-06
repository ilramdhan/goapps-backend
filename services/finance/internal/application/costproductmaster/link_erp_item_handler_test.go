package costproductmaster_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductmaster"
	auditdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costauditlog"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

const (
	linkTypeYarn int32 = 3
	linkTypeMB   int32 = 9
)

var linkNow = time.Date(2026, 9, 29, 8, 0, 0, 0, time.UTC)

// linkProductRepo serves one product from GetBySysID; everything else is fakeRepo.
type linkProductRepo struct {
	fakeRepo
	p      *domain.CostProductMaster
	getErr error
}

func (r *linkProductRepo) GetBySysID(_ context.Context, _ int64) (*domain.CostProductMaster, error) {
	if r.getErr != nil {
		return nil, r.getErr
	}
	return r.p, nil
}

type fakeErpLinkRepo struct {
	dups      []int64
	listErr   error
	saveErr   error
	gotKey    [2]string
	gotSelf   int64
	links     []domain.ErpLinkWrite
	attrs     []domain.ErpAttributesWrite
	listCalls int
}

func (f *fakeErpLinkRepo) ListActiveAxByErpKey(_ context.Context, item, shade string, exclude int64) ([]int64, error) {
	f.listCalls++
	f.gotKey = [2]string{item, shade}
	f.gotSelf = exclude
	return f.dups, f.listErr
}

func (f *fakeErpLinkRepo) SaveErpLink(_ context.Context, w domain.ErpLinkWrite) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.links = append(f.links, w)
	return nil
}

func (f *fakeErpLinkRepo) SaveErpAttributes(_ context.Context, w domain.ErpAttributesWrite) error {
	if f.saveErr != nil {
		return f.saveErr
	}
	f.attrs = append(f.attrs, w)
	return nil
}

var _ domain.ErpLinkRepository = (*fakeErpLinkRepo)(nil)

type fakeAuditSink struct {
	rows []auditdomain.NewInput
	err  error
}

func (f *fakeAuditSink) Emit(_ context.Context, in auditdomain.NewInput) error {
	f.rows = append(f.rows, in)
	return f.err
}

func linkProduct(active bool, typeID int32, grade, shade, erpItem string) *domain.CostProductMaster {
	return domain.Reconstruct(
		77, "CSTPOY2609000077", typeID, "Product", shade, grade, "",
		erpItem, "OLD-G1", "OLD-G2", nil, "",
		active, linkNow, "seed", linkNow, "seed",
		"", "", "", "", "", false,
	)
}

type linkFixture struct {
	repo  *linkProductRepo
	link  *fakeErpLinkRepo
	audit *fakeAuditSink
	h     *app.LinkErpItemHandler
}

func newLinkFixture(p *domain.CostProductMaster) *linkFixture {
	f := &linkFixture{repo: &linkProductRepo{p: p}, link: &fakeErpLinkRepo{}, audit: &fakeAuditSink{}}
	types := &fakeTypeRepo{byID: map[int32]string{linkTypeYarn: "POY", linkTypeMB: "MB"}}
	f.h = app.NewLinkErpItemHandler(f.repo, f.link, types, f.audit).WithClock(func() time.Time { return linkNow })
	return f
}

func TestLinkErpItemHandler_Validations_Rejected(t *testing.T) {
	tests := []struct {
		name    string
		p       *domain.CostProductMaster
		item    string
		shade   string
		dups    []int64
		wantErr error
	}{
		{"inactive product", linkProduct(false, linkTypeYarn, "AX", "X419T", ""), "POY0000275", "X419T", nil, domain.ErrInactive},
		{"non AX grade", linkProduct(true, linkTypeYarn, "AB", "X419T", ""), "POY0000275", "X419T", nil, domain.ErrLinkNotAxGrade},
		{"CMB item on non-MB product (V-12)", linkProduct(true, linkTypeYarn, "AX", "RED01", ""), "CMB0000001", "RED01", nil, domain.ErrLinkCmbRequiresMB},
		{"yarn item on MB product (V-12)", linkProduct(true, linkTypeMB, "AX", "RED01", ""), "POY0000275", "RED01", nil, domain.ErrLinkCmbRequiresMB},
		{"CMB item on unknown type", linkProduct(true, 999, "AX", "RED01", ""), "CMB0000001", "RED01", nil, domain.ErrLinkCmbRequiresMB},
		{"shade mismatch", linkProduct(true, linkTypeYarn, "AX", "X419T", ""), "POY0000275", "Z444T", nil, domain.ErrLinkShadeMismatch},
		{"empty-shade product vs shaded combo", linkProduct(true, linkTypeYarn, "AX", "", ""), "POY0000275", "X419T", nil, domain.ErrLinkProductHasNoShade},
		{"duplicate key (V-04)", linkProduct(true, linkTypeYarn, "AX", "X419T", ""), "POY0000275", "X419T", []int64{12, 13}, domain.ErrLinkDuplicate},
		{"invalid item code", linkProduct(true, linkTypeYarn, "AX", "X419T", ""), "POY\x01", "X419T", nil, domain.ErrLinkInvalidItemCode},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := newLinkFixture(tc.p)
			f.link.dups = tc.dups
			_, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{
				ProductSysID: 77, ErpItemCode: tc.item, ErpShadeCode: tc.shade, ActorUserID: "alice",
			})
			require.ErrorIs(t, err, tc.wantErr)
			assert.Empty(t, f.link.links, "nothing written")
			assert.Empty(t, f.audit.rows, "nothing audited")
			assert.Empty(t, tc.p.ErpItemCode())
		})
	}
}

func TestLinkErpItemHandler_DuplicateErrorNamesHolders(t *testing.T) {
	f := newLinkFixture(linkProduct(true, linkTypeYarn, "AX", "x419t", ""))
	f.link.dups = []int64{12, 13}
	_, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: " POY0000275 ", ErpShadeCode: "X419T"})
	require.ErrorIs(t, err, domain.ErrLinkDuplicate)
	assert.Contains(t, err.Error(), "[12 13]")
	assert.Equal(t, [2]string{"POY0000275", "X419T"}, f.link.gotKey, "key normalized")
	assert.Equal(t, int64(77), f.link.gotSelf, "self excluded")
}

func TestLinkErpItemHandler_Link_WritesItemOnlyAndAudits(t *testing.T) {
	p := linkProduct(true, linkTypeYarn, "AX", "X419T", "")
	f := newLinkFixture(p)
	res, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{
		ProductSysID: 77, ErpItemCode: "POY0000275", ErpShadeCode: "x419t", ActorUserID: "alice",
	})
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.Equal(t, "OLD-G1", p.ErpGradeCode1(), "grade_code_1 untouched")
	assert.Equal(t, "OLD-G2", p.ErpGradeCode2(), "grade_code_2 untouched")

	require.Len(t, f.link.links, 1)
	w := f.link.links[0]
	assert.Equal(t, int64(77), w.ProductSysID)
	assert.Empty(t, w.PrevItemCode)
	assert.Equal(t, "X419T", w.ShadeKey)
	assert.Equal(t, "POY0000275", w.NewItemCode)
	require.NotNil(t, w.LinkedAt)
	assert.Equal(t, linkNow, *w.LinkedAt)
	assert.Equal(t, "alice", w.LinkedBy)
	assert.Equal(t, "alice", w.UpdatedBy)

	require.Len(t, f.audit.rows, 1)
	row := f.audit.rows[0]
	assert.Equal(t, auditdomain.OpErpLink, row.Operation)
	assert.Equal(t, app.AuditEntityProductMaster, row.EntityType)
	assert.Equal(t, int64(77), row.EntityID)
	assert.Equal(t, "alice", row.UserID)
	require.NoError(t, row.Validate())
	var before, after map[string]any
	require.NoError(t, json.Unmarshal([]byte(row.BeforeData), &before))
	require.NoError(t, json.Unmarshal([]byte(row.AfterData), &after))
	assert.Empty(t, before["erp_item_code"])
	assert.Equal(t, "POY0000275", after["erp_item_code"])
}

func TestLinkErpItemHandler_CmbOnMB_OK(t *testing.T) {
	f := newLinkFixture(linkProduct(true, linkTypeMB, "", "RED01", ""))
	res, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "CMB0000001", ErpShadeCode: "RED01", ActorUserID: "a"})
	require.NoError(t, err)
	assert.True(t, res.Changed)
	require.Len(t, f.link.links, 1)
}

func TestLinkErpItemHandler_Relink_PassesPrevItem(t *testing.T) {
	f := newLinkFixture(linkProduct(true, linkTypeYarn, "AX", "X419T", "POY0000001"))
	_, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "POY0000275", ErpShadeCode: "X419T", ActorUserID: "a"})
	require.NoError(t, err)
	require.Len(t, f.link.links, 1)
	assert.Equal(t, "POY0000001", f.link.links[0].PrevItemCode)
}

func TestLinkErpItemHandler_SameItem_NoOp(t *testing.T) {
	f := newLinkFixture(linkProduct(true, linkTypeYarn, "AX", "X419T", "POY0000275"))
	res, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "POY0000275", ErpShadeCode: "X419T", ActorUserID: "a"})
	require.NoError(t, err)
	assert.False(t, res.Changed)
	assert.Empty(t, f.link.links)
	assert.Empty(t, f.audit.rows)
}

func TestLinkErpItemHandler_Unlink_SkipsValidationAndAudits(t *testing.T) {
	// Unlinking a now non-AX product must still work: only linking is validated.
	p := linkProduct(true, linkTypeYarn, "AB", "X419T", "POY0000275")
	f := newLinkFixture(p)
	res, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "  ", ActorUserID: "bob"})
	require.NoError(t, err)
	assert.True(t, res.Changed)
	assert.Equal(t, 0, f.link.listCalls, "no duplicate lookup on unlink")
	require.Len(t, f.link.links, 1)
	assert.Empty(t, f.link.links[0].NewItemCode)
	assert.Equal(t, "POY0000275", f.link.links[0].PrevItemCode)
	assert.Nil(t, f.link.links[0].LinkedAt)
	require.Len(t, f.audit.rows, 1)
	assert.Equal(t, auditdomain.OpErpLink, f.audit.rows[0].Operation)
}

func TestLinkErpItemHandler_RepoErrors(t *testing.T) {
	boom := errors.New("boom")

	f := newLinkFixture(nil)
	f.repo.getErr = domain.ErrNotFound
	_, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 1, ErpItemCode: "POY1"})
	require.ErrorIs(t, err, domain.ErrNotFound)

	f = newLinkFixture(linkProduct(true, linkTypeYarn, "AX", "", ""))
	f.link.listErr = boom
	_, err = f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "POY1"})
	require.ErrorIs(t, err, boom)

	f = newLinkFixture(linkProduct(true, linkTypeYarn, "AX", "", ""))
	f.link.saveErr = domain.ErrLinkStale
	_, err = f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "POY1"})
	require.ErrorIs(t, err, domain.ErrLinkStale)
	assert.Empty(t, f.audit.rows, "no audit when the write fails")
}

func TestLinkErpItemHandler_AuditFailureIsBestEffort(t *testing.T) {
	f := newLinkFixture(linkProduct(true, linkTypeYarn, "AX", "", ""))
	f.audit.err = errors.New("audit down")
	res, err := f.h.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "POY1"})
	require.NoError(t, err)
	assert.True(t, res.Changed)

	nilAudit := app.NewLinkErpItemHandler(&linkProductRepo{p: linkProduct(true, linkTypeYarn, "AX", "", "")},
		&fakeErpLinkRepo{}, &fakeTypeRepo{}, nil)
	_, err = nilAudit.Handle(context.Background(), app.LinkErpItemCommand{ProductSysID: 77, ErpItemCode: "POY1"})
	require.NoError(t, err)
}

// ---------------------------------------------------------------------------
// UpdateErpAttributesHandler
// ---------------------------------------------------------------------------

func sp(s string) *string { return &s }

func TestUpdateErpAttributesHandler_WritesAndAudits(t *testing.T) {
	p := linkProduct(true, linkTypeYarn, "AX", "X419T", "POY0000275")
	repo := &linkProductRepo{p: p}
	link := &fakeErpLinkRepo{}
	audit := &fakeAuditSink{}
	h := app.NewUpdateErpAttributesHandler(repo, link, audit).WithClock(func() time.Time { return linkNow })

	got, err := h.Handle(context.Background(), app.UpdateErpAttributesCommand{
		ProductSysID: 77, ActorUserID: "alice",
		Patch: domain.ErpAttributesPatch{FgType: sp("Type 1"), PrdPerDay: sp("12.5")},
	})
	require.NoError(t, err)
	assert.Equal(t, "Type 1", got.ErpAttributes().FgType)
	require.Len(t, link.attrs, 1)
	assert.Equal(t, "Type 1", link.attrs[0].Attributes.FgType)
	assert.Equal(t, "12.5", link.attrs[0].Attributes.PrdPerDay.Decimal.String())
	assert.Equal(t, linkNow, link.attrs[0].UpdatedAt)
	assert.Equal(t, "POY0000275", p.ErpItemCode(), "link untouched")

	require.Len(t, audit.rows, 1)
	assert.Equal(t, auditdomain.OpUpdate, audit.rows[0].Operation)
	assert.Equal(t, app.AuditEntityProductMaster, audit.rows[0].EntityType)
	assert.JSONEq(t, `{"erp_fg_type":null,"erp_chp_item_code":null,"erp_ms_batch_item":null,"erp_item_type":null,"erp_prd_per_day":null}`, audit.rows[0].BeforeData)
	assert.JSONEq(t, `{"erp_fg_type":"Type 1","erp_chp_item_code":null,"erp_ms_batch_item":null,"erp_item_type":null,"erp_prd_per_day":"12.5"}`, audit.rows[0].AfterData)
}

func TestUpdateErpAttributesHandler_NoOpAndErrors(t *testing.T) {
	p := linkProduct(true, linkTypeYarn, "AX", "", "")
	p.RestoreErpAttributes(domain.ErpAttributes{FgType: "Type 1"})
	link := &fakeErpLinkRepo{}
	audit := &fakeAuditSink{}
	h := app.NewUpdateErpAttributesHandler(&linkProductRepo{p: p}, link, audit)

	_, err := h.Handle(context.Background(), app.UpdateErpAttributesCommand{ProductSysID: 77})
	require.NoError(t, err, "empty patch")
	_, err = h.Handle(context.Background(), app.UpdateErpAttributesCommand{ProductSysID: 77, Patch: domain.ErpAttributesPatch{FgType: sp(" Type 1 ")}})
	require.NoError(t, err, "unchanged value")
	assert.Empty(t, link.attrs)
	assert.Empty(t, audit.rows)

	_, err = h.Handle(context.Background(), app.UpdateErpAttributesCommand{ProductSysID: 77, Patch: domain.ErpAttributesPatch{PrdPerDay: sp("-1")}})
	require.ErrorIs(t, err, domain.ErrInvalidErpAttributes)

	link.saveErr = errors.New("db down")
	_, err = h.Handle(context.Background(), app.UpdateErpAttributesCommand{ProductSysID: 77, Patch: domain.ErpAttributesPatch{ItemType: sp("FG")}})
	require.Error(t, err)
	assert.Empty(t, audit.rows)

	nf := app.NewUpdateErpAttributesHandler(&linkProductRepo{getErr: domain.ErrNotFound}, link, audit)
	_, err = nf.Handle(context.Background(), app.UpdateErpAttributesCommand{ProductSysID: 1, Patch: domain.ErpAttributesPatch{ItemType: sp("FG")}})
	require.ErrorIs(t, err, domain.ErrNotFound)
}
