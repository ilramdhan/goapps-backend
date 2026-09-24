package costproducttype_test

import (
	"context"
	"errors"
	"testing"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproducttype"
	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproducttype"
)

type fakeOilRepo struct {
	configs  map[int32]*domain.OilConfig
	oil      map[string]domain.ResolvedOilGroup
	replaced []domain.ReplaceOilGroup
	class    string
	calls    int
}

func (f *fakeOilRepo) GetOilConfig(_ context.Context, id int32) (*domain.OilConfig, error) {
	c, ok := f.configs[id]
	if !ok {
		return nil, domain.ErrNotFound
	}
	return c, nil
}

func (f *fakeOilRepo) ResolveOilGroups(_ context.Context, codes []string) (map[string]domain.ResolvedOilGroup, error) {
	out := map[string]domain.ResolvedOilGroup{}
	for _, c := range codes {
		if g, ok := f.oil[c]; ok {
			out[c] = g
		}
	}
	return out, nil
}

func (f *fakeOilRepo) ReplaceOilConfig(_ context.Context, id int32, class string, groups []domain.ReplaceOilGroup, _ string) error {
	f.calls++
	f.class = class
	f.replaced = groups
	cfg := &domain.OilConfig{TypeID: id, OilClass: class}
	for _, g := range groups {
		for code, r := range f.oil {
			if r.GroupHeadID == g.GroupHeadID {
				cfg.Groups = append(cfg.Groups, domain.OilGroupEntry{GroupCode: code, GroupName: r.GroupName, IsDefault: g.IsDefault})
			}
		}
	}
	f.configs[id] = cfg
	return nil
}

func newFakeOilRepo() *fakeOilRepo {
	return &fakeOilRepo{
		configs: map[int32]*domain.OilConfig{1: {TypeID: 1}},
		oil: map[string]domain.ResolvedOilGroup{
			"202006101": {GroupHeadID: "7d935df4-0d80-4616-8698-797105e9e828", GroupCode: "202006101", GroupName: "CONING OIL"},
			"202006077": {GroupHeadID: "f4e3af82-1b9f-4cc3-9bbd-f91ce99e927b", GroupCode: "202006077", GroupName: "SPIN FINISH OIL"},
		},
	}
}

func TestSetOilConfigHandler(t *testing.T) {
	t.Parallel()
	g := func(code string, def bool) domain.OilGroupEntry {
		return domain.OilGroupEntry{GroupCode: code, IsDefault: def}
	}
	cases := []struct {
		name    string
		typeID  int32
		class   string
		groups  []domain.OilGroupEntry
		wantErr error
	}{
		{"valid PTY", 1, "pty", []domain.OilGroupEntry{g(" 202006101 ", true)}, nil},
		{"valid two groups one default", 1, "POY", []domain.OilGroupEntry{g("202006077", true), g("202006101", false)}, nil},
		{"clear config", 1, "", nil, nil},
		{"bad class", 1, "DTY", nil, domain.ErrInvalidOilClass},
		{"class without groups", 1, "PTY", nil, domain.ErrOilConfigNoGroups},
		{"no default", 1, "PTY", []domain.OilGroupEntry{g("202006101", false)}, domain.ErrOilConfigDefaultCount},
		{"two defaults", 1, "PTY", []domain.OilGroupEntry{g("202006101", true), g("202006077", true)}, domain.ErrOilConfigDefaultCount},
		{"groups without class", 1, "", []domain.OilGroupEntry{g("202006101", true)}, domain.ErrOilConfigGroupsWithoutClass},
		{"duplicate", 1, "PTY", []domain.OilGroupEntry{g("202006101", true), g("202006101", false)}, domain.ErrOilConfigDuplicateGroup},
		{"non-oil group", 1, "PTY", []domain.OilGroupEntry{g("CHM0000118", true)}, domain.ErrOilConfigGroupNotOil},
		{"unknown type", 99, "PTY", []domain.OilGroupEntry{g("202006101", true)}, domain.ErrNotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := newFakeOilRepo()
			h := app.NewSetOilConfigHandler(repo)
			out, err := h.Handle(context.Background(), app.SetOilConfigCommand{
				TypeID: tc.typeID, OilClass: tc.class, Groups: tc.groups, Actor: "u",
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				if repo.calls != 0 {
					t.Fatalf("invalid config must not be written")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if repo.calls != 1 || len(repo.replaced) != len(tc.groups) {
				t.Fatalf("want 1 replace with %d groups, got %d/%d", len(tc.groups), repo.calls, len(repo.replaced))
			}
			if out == nil || out.TypeID != tc.typeID {
				t.Fatalf("unexpected result %+v", out)
			}
		})
	}
}

func TestSetOilConfigHandler_NormalizesClass(t *testing.T) {
	t.Parallel()
	repo := newFakeOilRepo()
	h := app.NewSetOilConfigHandler(repo)
	if _, err := h.Handle(context.Background(), app.SetOilConfigCommand{
		TypeID: 1, OilClass: " superba ", Groups: []domain.OilGroupEntry{{GroupCode: "202006077", IsDefault: true}},
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if repo.class != domain.OilClassSuperba {
		t.Fatalf("class = %q, want SUPERBA", repo.class)
	}
}
