package grpc

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	financev1 "github.com/mutugading/goapps-backend/gen/finance/v1"
	cppdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	cptdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/lookupmaster"
	rmgroupdomain "github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// ─── ListMasterOptions RM_GROUP_OIL product scoping ─────────────────────────

// lookupRepoStub embeds lookupmaster.Repository (nil) and records the restriction
// ListMasterOptions passes down; it returns the oil groups filtered by it.
type lookupRepoStub struct {
	lookupmaster.Repository
	gotMaster   string
	gotRestrict []string
	all         []lookupmaster.MasterOption
}

func (s *lookupRepoStub) ListMasterOptionsInCodes(_ context.Context, masterCode, _ string, _ int, restrict []string) ([]lookupmaster.MasterOption, error) {
	s.gotMaster = masterCode
	s.gotRestrict = restrict
	if restrict == nil {
		return s.all, nil
	}
	allowed := map[string]bool{}
	for _, c := range restrict {
		allowed[c] = true
	}
	var out []lookupmaster.MasterOption
	for _, o := range s.all {
		if allowed[o.Value] {
			out = append(out, o)
		}
	}
	return out, nil
}

type oilPolicyStub struct {
	rules map[int64]*cppdomain.OilGroupRule
	err   error
}

func (s *oilPolicyStub) RuleForProduct(_ context.Context, id int64) (*cppdomain.OilGroupRule, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.rules[id], nil
}

func (s *oilPolicyStub) RulesForProducts(_ context.Context, _ []int64) (map[int64]*cppdomain.OilGroupRule, error) {
	return s.rules, s.err
}

func oilOptions() []lookupmaster.MasterOption {
	return []lookupmaster.MasterOption{
		{Value: "202006101", Label: "CONING OIL"},
		{Value: "202006077", Label: "SPIN FINISH OIL"},
	}
}

func int64Ptr(v int64) *int64 { return &v }

func TestListMasterOptions_RMGroupOil_ProductScoping(t *testing.T) {
	policy := &oilPolicyStub{rules: map[int64]*cppdomain.OilGroupRule{
		1: {TypeCode: "PTY", OilClass: "PTY", Allowed: []string{"202006101"}, Default: "202006101"},
		4: {TypeCode: "POY", OilClass: "POY"}, // oil class but nothing mapped
	}}
	cases := []struct {
		name       string
		master     string
		productID  *int64
		wantValues []string
		wantNilRes bool
	}{
		{"PTY product lists only CONING OIL", "RM_GROUP_OIL", int64Ptr(1), []string{"202006101"}, false},
		{"type without oil class lists all", "RM_GROUP_OIL", int64Ptr(3), []string{"202006101", "202006077"}, true},
		{"no product context lists all", "RM_GROUP_OIL", nil, []string{"202006101", "202006077"}, true},
		{"oil class with no mapping lists none", "RM_GROUP_OIL", int64Ptr(4), nil, false},
		{"other master ignores product", "MACHINE", int64Ptr(1), []string{"202006101", "202006077"}, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &lookupRepoStub{all: oilOptions()}
			h, err := NewLookupMasterHandler(repo, policy)
			require.NoError(t, err)
			resp, err := h.ListMasterOptions(context.Background(), &financev1.ListMasterOptionsRequest{
				MasterCode: tc.master, ProductSysId: tc.productID,
			})
			require.NoError(t, err)
			require.True(t, resp.GetBase().GetIsSuccess(), resp.GetBase().GetMessage())
			var got []string
			for _, o := range resp.GetData() {
				got = append(got, o.GetValue())
			}
			assert.Equal(t, tc.wantValues, got)
			assert.Equal(t, tc.wantNilRes, repo.gotRestrict == nil)
		})
	}
}

func TestListMasterOptions_RMGroupOil_PolicyError(t *testing.T) {
	repo := &lookupRepoStub{all: oilOptions()}
	h, err := NewLookupMasterHandler(repo, &oilPolicyStub{err: errors.New("db down")})
	require.NoError(t, err)
	resp, err := h.ListMasterOptions(context.Background(), &financev1.ListMasterOptionsRequest{
		MasterCode: "RM_GROUP_OIL", ProductSysId: int64Ptr(1),
	})
	require.NoError(t, err)
	assert.False(t, resp.GetBase().GetIsSuccess())
	assert.Empty(t, repo.gotMaster, "repo must not be queried when the rule cannot be resolved")
}

// ─── error mappings ─────────────────────────────────────────────────────────

func TestCppDomainError_OilGroupNotAllowed_Is400WithField(t *testing.T) {
	rule := &cppdomain.OilGroupRule{TypeCode: "PTY", Allowed: []string{"202006101"}}
	base := cppDomainError(rule.Validate("202006077"))
	assert.Equal(t, "400", base.GetStatusCode())
	require.Len(t, base.GetValidationErrors(), 1)
	assert.Equal(t, "OIL_NAME", base.GetValidationErrors()[0].GetField())
	assert.Contains(t, base.GetMessage(), `OIL_NAME "202006077" is not allowed for product type PTY; allowed: 202006101`)
}

func TestRMGroupWriteErrToBase_OilGroupInUse_Is400(t *testing.T) {
	base := rmGroupWriteErrToBase(rmgroupdomain.ErrOilGroupInUse)
	assert.Equal(t, "400", base.GetStatusCode())
	require.Len(t, base.GetValidationErrors(), 1)
	assert.Equal(t, "is_oil_group", base.GetValidationErrors()[0].GetField())
}

func TestProductTypeErrToBase_OilConfigErrors_Are400(t *testing.T) {
	for _, err := range []error{
		cptdomain.ErrInvalidOilClass, cptdomain.ErrOilConfigNoGroups, cptdomain.ErrOilConfigDefaultCount,
		cptdomain.ErrOilConfigGroupsWithoutClass, cptdomain.ErrOilConfigDuplicateGroup, cptdomain.ErrOilConfigGroupNotOil,
	} {
		base := productTypeErrToBase(err)
		assert.Equal(t, "400", base.GetStatusCode(), err.Error())
		assert.Len(t, base.GetValidationErrors(), 1, err.Error())
	}
}

func TestRMGroupHeadToProto_IsOilGroup(t *testing.T) {
	code, err := rmgroupdomain.NewCode("202006101")
	require.NoError(t, err)
	head, err := rmgroupdomain.NewHead(code, "CONING OIL", "", 0, 0, "u")
	require.NoError(t, err)
	assert.False(t, rmGroupHeadToProto(head).GetIsOilGroup())
	head.SetOilGroup(true)
	assert.True(t, rmGroupHeadToProto(head).GetIsOilGroup())
}

// ─── product type oil config RPCs ───────────────────────────────────────────

type oilConfigRepoStub struct {
	cfg *cptdomain.OilConfig
}

func (s *oilConfigRepoStub) GetOilConfig(_ context.Context, id int32) (*cptdomain.OilConfig, error) {
	if s.cfg == nil || s.cfg.TypeID != id {
		return nil, cptdomain.ErrNotFound
	}
	return s.cfg, nil
}

func (s *oilConfigRepoStub) ResolveOilGroups(_ context.Context, codes []string) (map[string]cptdomain.ResolvedOilGroup, error) {
	out := map[string]cptdomain.ResolvedOilGroup{}
	for _, c := range codes {
		if c == "202006101" {
			out[c] = cptdomain.ResolvedOilGroup{GroupHeadID: "7d935df4-0d80-4616-8698-797105e9e828", GroupCode: c, GroupName: "CONING OIL"}
		}
	}
	return out, nil
}

func (s *oilConfigRepoStub) ReplaceOilConfig(_ context.Context, id int32, class string, groups []cptdomain.ReplaceOilGroup, _ string) error {
	s.cfg = &cptdomain.OilConfig{TypeID: id, OilClass: class}
	for _, g := range groups {
		s.cfg.Groups = append(s.cfg.Groups, cptdomain.OilGroupEntry{GroupCode: "202006101", GroupName: "CONING OIL", IsDefault: g.IsDefault})
	}
	return nil
}

func TestCostProductTypeOilConfigRPCs(t *testing.T) {
	v, err := NewValidationHelper()
	require.NoError(t, err)
	repo := &oilConfigRepoStub{cfg: &cptdomain.OilConfig{TypeID: 1}}
	h := (&CostProductTypeHandler{validation: v}).WithOilConfig(repo)
	ctx := context.Background()

	setResp, err := h.SetCostProductTypeOilConfig(ctx, &financev1.SetCostProductTypeOilConfigRequest{
		TypeId: 1, OilClass: "PTY",
		Groups: []*financev1.CostProductTypeOilGroup{{GroupCode: "202006101", IsDefault: true}},
	})
	require.NoError(t, err)
	require.True(t, setResp.GetBase().GetIsSuccess(), setResp.GetBase().GetMessage())
	assert.Equal(t, "PTY", setResp.GetOilClass())
	require.Len(t, setResp.GetGroups(), 1)
	assert.True(t, setResp.GetGroups()[0].GetIsDefault())

	getResp, err := h.GetCostProductTypeOilConfig(ctx, &financev1.GetCostProductTypeOilConfigRequest{TypeId: 1})
	require.NoError(t, err)
	require.True(t, getResp.GetBase().GetIsSuccess())
	assert.Equal(t, "CONING OIL", getResp.GetGroups()[0].GetGroupName())

	badResp, err := h.SetCostProductTypeOilConfig(ctx, &financev1.SetCostProductTypeOilConfigRequest{
		TypeId: 1, OilClass: "PTY",
		Groups: []*financev1.CostProductTypeOilGroup{{GroupCode: "202006101", IsDefault: false}},
	})
	require.NoError(t, err)
	assert.Equal(t, "400", badResp.GetBase().GetStatusCode())

	nfResp, err := h.GetCostProductTypeOilConfig(ctx, &financev1.GetCostProductTypeOilConfigRequest{TypeId: 9})
	require.NoError(t, err)
	assert.Equal(t, "404", nfResp.GetBase().GetStatusCode())

	unwired := &CostProductTypeHandler{validation: v}
	unResp, err := unwired.GetCostProductTypeOilConfig(ctx, &financev1.GetCostProductTypeOilConfigRequest{TypeId: 1})
	require.NoError(t, err)
	assert.False(t, unResp.GetBase().GetIsSuccess())
}
