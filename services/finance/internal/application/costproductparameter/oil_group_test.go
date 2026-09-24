package costproductparameter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/costimportjob"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// fakeOilPolicy is an in-memory cpp.OilGroupPolicy keyed by product sys id.
type fakeOilPolicy struct {
	rules map[int64]*cpp.OilGroupRule
	err   error
	calls int
}

func (f *fakeOilPolicy) RuleForProduct(_ context.Context, id int64) (*cpp.OilGroupRule, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.rules[id], nil
}

func (f *fakeOilPolicy) RulesForProducts(_ context.Context, ids []int64) (map[int64]*cpp.OilGroupRule, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	out := map[int64]*cpp.OilGroupRule{}
	for _, id := range ids {
		if r, ok := f.rules[id]; ok {
			out[id] = r
		}
	}
	return out, nil
}

const (
	oilPTYGroup = "202006101"
	oilPOYGroup = "202006077"
)

func ptyRule() *cpp.OilGroupRule {
	return &cpp.OilGroupRule{TypeCode: "PTY", OilClass: "PTY", Allowed: []string{oilPTYGroup}, Default: oilPTYGroup}
}

func strPtr(s string) *string { return &s }

func TestUpsert_OilName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		paramCode string
		rules     map[int64]*cpp.OilGroupRule
		value     string
		wantErr   error
	}{
		{"allowed PTY group", "OIL_NAME", map[int64]*cpp.OilGroupRule{1: ptyRule()}, oilPTYGroup, nil},
		{"disallowed group rejected", "OIL_NAME", map[int64]*cpp.OilGroupRule{1: ptyRule()}, oilPOYGroup, cpp.ErrOilGroupNotAllowed},
		{"non-oil type passes through", "OIL_NAME", map[int64]*cpp.OilGroupRule{}, oilPOYGroup, nil},
		{"other param not checked", "COLOR", map[int64]*cpp.OilGroupRule{1: ptyRule()}, oilPOYGroup, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{productExists: true, getMeta: cpp.ParamMeta{ParamCode: tc.paramCode, DataType: "TEXT"}}
			h := app.New(repo, nil).WithOilGroupPolicy(&fakeOilPolicy{rules: tc.rules})
			_, err := h.Upsert(context.Background(), app.UpsertCommand{
				ProductSysID: 1, ParamID: uuid.New(), ValueText: strPtr(tc.value), FilledBy: "u",
			})
			if tc.wantErr != nil {
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("want %v, got %v", tc.wantErr, err)
				}
				if len(repo.upsertedValues) != 0 {
					t.Fatalf("rejected value must not be written")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.upsertedValues) != 1 {
				t.Fatalf("want 1 upsert, got %d", len(repo.upsertedValues))
			}
		})
	}
}

func TestUpsert_OilName_PolicyError(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{productExists: true, getMeta: cpp.ParamMeta{ParamCode: "OIL_NAME", DataType: "TEXT"}}
	boom := errors.New("db down")
	h := app.New(repo, nil).WithOilGroupPolicy(&fakeOilPolicy{err: boom})
	_, err := h.Upsert(context.Background(), app.UpsertCommand{ProductSysID: 1, ParamID: uuid.New(), ValueText: strPtr(oilPTYGroup)})
	if !errors.Is(err, boom) {
		t.Fatalf("want policy error, got %v", err)
	}
	if len(repo.upsertedValues) != 0 {
		t.Fatalf("must not write on policy error")
	}
}

func TestAddApplicableWithChildren_OilNameDefault(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		paramCode  string
		current    string
		rules      map[int64]*cpp.OilGroupRule
		wantWrites int
	}{
		{"writes default when empty", "OIL_NAME", "", map[int64]*cpp.OilGroupRule{7: ptyRule()}, 1},
		{"keeps existing value", "OIL_NAME", oilPTYGroup, map[int64]*cpp.OilGroupRule{7: ptyRule()}, 0},
		{"non-oil type no default", "OIL_NAME", "", map[int64]*cpp.OilGroupRule{}, 0},
		{"other trigger param", "MACHINE_CODE", "", map[int64]*cpp.OilGroupRule{7: ptyRule()}, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{productExists: true, paramCodeByID: tc.paramCode, currentValueText: tc.current}
			h := app.New(repo, nil).WithOilGroupPolicy(&fakeOilPolicy{rules: tc.rules})
			if err := h.AddApplicableWithChildren(context.Background(), 7, uuid.New(), false, "u", nil); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(repo.upsertedValues) != tc.wantWrites {
				t.Fatalf("want %d writes, got %d", tc.wantWrites, len(repo.upsertedValues))
			}
			if tc.wantWrites == 1 {
				v := repo.upsertedValues[0]
				if v.ValueText == nil || *v.ValueText != oilPTYGroup || v.ProductSysID != 7 {
					t.Fatalf("unexpected default write: %+v", v)
				}
			}
		})
	}
}

func TestAddApplicable_OilNameDefault(t *testing.T) {
	t.Parallel()
	repo := &fakeRepo{productExists: true, paramCodeByID: "OIL_NAME"}
	h := app.New(repo, nil).WithOilGroupPolicy(&fakeOilPolicy{rules: map[int64]*cpp.OilGroupRule{3: ptyRule()}})
	if err := h.AddApplicable(context.Background(), 3, uuid.New(), false, nil, "u"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(repo.addedCapps) != 1 || len(repo.upsertedValues) != 1 {
		t.Fatalf("want 1 capp + 1 default value, got %d/%d", len(repo.addedCapps), len(repo.upsertedValues))
	}
}

// TestAsyncImportHandler_OilName: fakeRepo resolves every product code to sys id 0,
// so the policy rule is keyed at 0.
func TestAsyncImportHandler_OilName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		rules      map[int64]*cpp.OilGroupRule
		value      string
		wantWrites int
		wantText   string
	}{
		{"allowed", map[int64]*cpp.OilGroupRule{0: ptyRule()}, oilPTYGroup, 1, oilPTYGroup},
		{"disallowed row failed", map[int64]*cpp.OilGroupRule{0: ptyRule()}, oilPOYGroup, 0, ""},
		{"blank gets default", map[int64]*cpp.OilGroupRule{0: ptyRule()}, "", 1, oilPTYGroup},
		{"blank no default fails", map[int64]*cpp.OilGroupRule{0: {TypeCode: "PTY", OilClass: "PTY"}}, "", 0, ""},
		{"non-oil type as-is", map[int64]*cpp.OilGroupRule{}, oilPOYGroup, 1, oilPOYGroup},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			repo := &fakeRepo{productExists: true, getMeta: cpp.ParamMeta{ParamCode: "OIL_NAME", DataType: "TEXT"}}
			job := costimportjob.NewJob(costimportjob.EntityCPP, "t.xlsx", "u", "")
			h := app.NewAsyncImportHandler(repo, &fakeJobRepo{job: job}, nil).
				WithOilGroupPolicy(&fakeOilPolicy{rules: tc.rules})
			content := buildCPPImportXLSX(t, [][]string{{"P1", "OIL_NAME", "", tc.value, ""}})
			if err := h.Handle(context.Background(), 1, content, "t.xlsx"); err != nil {
				t.Fatalf("handle: %v", err)
			}
			if len(repo.upsertedValues) != tc.wantWrites {
				t.Fatalf("want %d writes, got %d", tc.wantWrites, len(repo.upsertedValues))
			}
			if tc.wantWrites == 1 && *repo.upsertedValues[0].ValueText != tc.wantText {
				t.Fatalf("want text %q, got %q", tc.wantText, *repo.upsertedValues[0].ValueText)
			}
		})
	}
}
