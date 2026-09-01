package costproductparameter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
	"github.com/mutugading/goapps-backend/services/finance/internal/domain/mbspin"
)

// =============================================================================
// fakeRepo — in-memory test double for cpp.Repository. Captures inputs and
// lets each test program targeted error returns via overrides.
// =============================================================================
type fakeRepo struct {
	productExists   bool
	isProductLocked bool
	getMetaErr      error
	getMeta         cpp.ParamMeta
	upsertErr       error
	deleteErr       error

	upsertedValues []*cpp.Value
	addedCapps     []*cpp.Applicability
	removedCapps   []removeKey

	listForProductOut []cpp.RequiredEntry
}

type removeKey struct {
	productSysID int64
	paramID      uuid.UUID
}

func (f *fakeRepo) ListForProduct(_ context.Context, _ int64, _ bool) ([]cpp.RequiredEntry, error) {
	return f.listForProductOut, nil
}

func (f *fakeRepo) GetMeta(_ context.Context, _ uuid.UUID) (*cpp.ParamMeta, error) {
	if f.getMetaErr != nil {
		return nil, f.getMetaErr
	}
	m := f.getMeta
	return &m, nil
}

func (f *fakeRepo) ProductExists(_ context.Context, _ int64) (bool, error) {
	return f.productExists, nil
}

func (f *fakeRepo) IsProductLocked(_ context.Context, _ int64) (bool, error) {
	return f.isProductLocked, nil
}

func (f *fakeRepo) Upsert(_ context.Context, v *cpp.Value) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upsertedValues = append(f.upsertedValues, v)
	return nil
}

func (f *fakeRepo) Delete(_ context.Context, productSysID int64, paramID uuid.UUID) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.removedCapps = append(f.removedCapps, removeKey{productSysID, paramID})
	return nil
}

func (f *fakeRepo) MissingRequired(_ context.Context, _ int64) ([]cpp.ParamMeta, error) {
	return nil, nil
}

func (f *fakeRepo) AddApplicable(_ context.Context, a *cpp.Applicability) error {
	f.addedCapps = append(f.addedCapps, a)
	return nil
}

func (f *fakeRepo) RemoveApplicable(_ context.Context, productSysID int64, paramID uuid.UUID) error {
	f.removedCapps = append(f.removedCapps, removeKey{productSysID, paramID})
	return nil
}

func (f *fakeRepo) UpdateApplicable(_ context.Context, _ int64, _ uuid.UUID, _ *bool, _ *int32, _ string) error {
	return nil
}

func (f *fakeRepo) ListAvailableParams(_ context.Context, _ int64) ([]cpp.ParamMeta, error) {
	return nil, nil
}

func (f *fakeRepo) CountApplicableForProducts(_ context.Context, _ []int64) (int32, error) {
	return 0, nil
}

func (f *fakeRepo) GetParamIDByCode(_ context.Context, _ string) (uuid.UUID, error) {
	return uuid.Nil, nil
}

func (f *fakeRepo) GetProductSysIDByCode(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (f *fakeRepo) ListApplicable(_ context.Context, _ int64) ([]cpp.CAPPRow, error) {
	return nil, nil
}

func (f *fakeRepo) ListAllApplicable(_ context.Context) ([]cpp.CAPPRow, error) {
	return nil, nil
}

func (f *fakeRepo) ListAllValues(_ context.Context) ([]cpp.CPPRow, error) {
	return nil, nil
}

func (f *fakeRepo) GetParamCodeByID(_ context.Context, _ uuid.UUID) (string, error) {
	return "TEST_PARAM", nil
}

func (f *fakeRepo) GetCurrentValueAsText(_ context.Context, _ int64, _ uuid.UUID) (string, error) {
	return "", nil
}

func (f *fakeRepo) AddApplicableWithChildren(_ context.Context, _ int64, _ uuid.UUID, _ bool, _ string, _ []uuid.UUID) error {
	return nil
}

func (f *fakeRepo) GetRemovePreview(_ context.Context, _ int64, _ uuid.UUID) (cpp.RemovePreview, error) {
	return cpp.RemovePreview{}, nil
}

func (f *fakeRepo) RemoveApplicableWithChildren(_ context.Context, _ int64, _ uuid.UUID, _ string) error {
	return nil
}

func (f *fakeRepo) BulkUpsertValues(_ context.Context, _ []cpp.CPPUpsertInput, _ string) (int, int, error) {
	return 0, 0, nil
}

func (f *fakeRepo) BulkUpsertApplicable(_ context.Context, _ []cpp.CAPPUpsertInput, _ string) (int, int, error) {
	return 0, 0, nil
}

func (f *fakeRepo) ListAllParams(_ context.Context) ([]cpp.ParamMeta, error) {
	return nil, nil
}

// =============================================================================
// fakeMBSpinRepo — minimal test double for mbspin.Repository. Only ExistsByID
// and ResolveUniqueByOrionItemCode are exercised by resolveMBSpinID; every
// other method is an inert stub present solely so this type satisfies the
// (wider) mbspin.Repository interface.
// =============================================================================
type fakeMBSpinRepo struct {
	// existsByID maps a UUID (string form) to whether ExistsByID should report it exists.
	existsByID map[string]bool
	// uniqueByOrionCode maps an ORION code to the single spin ID that should
	// resolve. A code absent from this map means "zero or many matches" (ok=false).
	uniqueByOrionCode map[string]uuid.UUID
}

func (f *fakeMBSpinRepo) Create(_ context.Context, _ *mbspin.Entity) error { return nil }
func (f *fakeMBSpinRepo) GetByID(_ context.Context, _ uuid.UUID) (*mbspin.Entity, error) {
	return nil, mbspin.ErrNotFound
}
func (f *fakeMBSpinRepo) List(_ context.Context, _ mbspin.ListFilter) ([]*mbspin.Entity, int64, error) {
	return nil, 0, nil
}
func (f *fakeMBSpinRepo) Update(_ context.Context, _ *mbspin.Entity) error { return nil }
func (f *fakeMBSpinRepo) SoftDelete(_ context.Context, _ uuid.UUID, _ string) error {
	return nil
}

func (f *fakeMBSpinRepo) ExistsByID(_ context.Context, id uuid.UUID) (bool, error) {
	return f.existsByID[id.String()], nil
}

func (f *fakeMBSpinRepo) GetByMBCosting(_ context.Context, _ string) (*mbspin.Entity, error) {
	return nil, mbspin.ErrNotFound
}

func (f *fakeMBSpinRepo) GetByOrionItemCode(_ context.Context, _ string) (*mbspin.Entity, error) {
	return nil, mbspin.ErrNotFound
}

func (f *fakeMBSpinRepo) DuplicateSpin(_ context.Context, _ mbspin.DuplicateInput) (mbspin.DuplicateOutput, error) {
	return mbspin.DuplicateOutput{}, nil
}

func (f *fakeMBSpinRepo) ListChildren(_ context.Context, _ uuid.UUID) ([]*mbspin.Entity, error) {
	return nil, nil
}

func (f *fakeMBSpinRepo) ExistsByOrionItemCode(_ context.Context, _ string) (bool, error) {
	return false, nil
}

func (f *fakeMBSpinRepo) ResolveUniqueByOrionItemCode(_ context.Context, code string) (uuid.UUID, bool, error) {
	id, ok := f.uniqueByOrionCode[code]
	return id, ok, nil
}

func (f *fakeMBSpinRepo) ListByOrionItemCode(_ context.Context, _ string) ([]*mbspin.Entity, error) {
	return nil, nil
}

func (f *fakeMBSpinRepo) HasChildren(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (f *fakeMBSpinRepo) IsUsedByCostProduct(_ context.Context, _ uuid.UUID) (bool, error) {
	return false, nil
}

// =============================================================================
// Upsert
// =============================================================================
func TestUpsert(t *testing.T) {
	t.Parallel()

	paramID := uuid.New()
	strPtr := func(s string) *string { return &s }

	t.Run("product missing returns ErrProductNotFound", func(t *testing.T) {
		t.Parallel()
		h := app.New(&fakeRepo{productExists: false}, nil)
		_, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 99,
			ParamID:      paramID,
			ValueNumeric: strPtr("1"),
			FilledBy:     "actor",
		})
		if !errors.Is(err, cpp.ErrProductNotFound) {
			t.Fatalf("want ErrProductNotFound, got %v", err)
		}
	})

	t.Run("period-dependent param rejects", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{ParamID: paramID, DataType: "NUMBER", IsPeriodDependent: true},
		}
		h := app.New(repo, nil)
		_, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueNumeric: strPtr("12"), FilledBy: "actor",
		})
		if !errors.Is(err, cpp.ErrPeriodDependent) {
			t.Fatalf("want ErrPeriodDependent, got %v", err)
		}
	})

	t.Run("data_type mismatch rejects", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{ParamID: paramID, DataType: "NUMBER"},
		}
		h := app.New(repo, nil)
		_, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr("not numeric"), FilledBy: "actor",
		})
		if !errors.Is(err, cpp.ErrInvalidDataType) {
			t.Fatalf("want ErrInvalidDataType, got %v", err)
		}
	})

	t.Run("happy path writes via repo", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{ParamID: paramID, DataType: "NUMBER"},
		}
		h := app.New(repo, nil)
		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 42, ParamID: paramID, ValueNumeric: strPtr("9.99"), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repo.upsertedValues) != 1 {
			t.Fatalf("want 1 upsert, got %d", len(repo.upsertedValues))
		}
		if v.ProductSysID != 42 || v.ParamID != paramID {
			t.Fatalf("value identifiers wrong: got %+v", v)
		}
	})
}

// =============================================================================
// UpsertBatch — non-atomic: failures captured by param code.
// =============================================================================
func TestUpsertBatch_PartialFailure(t *testing.T) {
	t.Parallel()

	paramOK := uuid.New()
	paramBad := uuid.New()
	strPtr := func(s string) *string { return &s }

	// First Upsert (paramOK) goes through with DataType=NUMBER.
	// Second Upsert (paramBad) is rejected because we'll mark the param as
	// period-dependent via getMeta. We can't toggle per-call easily with this
	// simple fake, so simulate by making both NUMBER and triggering a value-
	// shape error on the second via mismatched value type.
	repo := &fakeRepo{
		productExists: true,
		getMeta:       cpp.ParamMeta{DataType: "NUMBER"},
	}
	h := app.New(repo, nil)

	res, err := h.UpsertBatch(context.Background(), 7, []app.UpsertCommand{
		{ParamID: paramOK, ValueNumeric: strPtr("1"), FilledBy: "actor"},
		{ParamID: paramBad, ValueText: strPtr("oops"), FilledBy: "actor"},
	})
	if err != nil {
		t.Fatalf("UpsertBatch should not fail at the orchestration level, got %v", err)
	}
	if res.UpsertedCount != 1 {
		t.Fatalf("want UpsertedCount=1, got %d", res.UpsertedCount)
	}
	if res.FailedCount != 1 {
		t.Fatalf("want FailedCount=1, got %d", res.FailedCount)
	}
	if len(res.FailedParamCodes) != 1 || res.FailedParamCodes[0] != paramBad.String() {
		t.Fatalf("expected failed list to contain %s, got %+v", paramBad, res.FailedParamCodes)
	}
}

// =============================================================================
// AddApplicable
// =============================================================================
func TestAddApplicable_GuardsProductAndParam(t *testing.T) {
	t.Parallel()

	paramID := uuid.New()

	t.Run("missing product", func(t *testing.T) {
		t.Parallel()
		h := app.New(&fakeRepo{productExists: false}, nil)
		err := h.AddApplicable(context.Background(), 1, paramID, true, nil, "actor")
		if !errors.Is(err, cpp.ErrProductNotFound) {
			t.Fatalf("want ErrProductNotFound, got %v", err)
		}
	})

	t.Run("period-dependent param rejected", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{IsPeriodDependent: true},
		}
		h := app.New(repo, nil)
		err := h.AddApplicable(context.Background(), 1, paramID, true, nil, "actor")
		if !errors.Is(err, cpp.ErrPeriodDependent) {
			t.Fatalf("want ErrPeriodDependent, got %v", err)
		}
	})

	t.Run("happy path persists CAPP row", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{ParamID: paramID, DataType: "NUMBER"},
		}
		h := app.New(repo, nil)
		err := h.AddApplicable(context.Background(), 42, paramID, true, nil, "actor")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(repo.addedCapps) != 1 {
			t.Fatalf("want 1 capp row, got %d", len(repo.addedCapps))
		}
		got := repo.addedCapps[0]
		if got.ProductSysID != 42 || got.ParamID != paramID || !got.IsRequired {
			t.Fatalf("unexpected capp row: %+v", got)
		}
	})
}

// =============================================================================
// Upsert — MB_SPIN companion column resolution (cpp_value_mb_spin_id).
//
// ValueText is ALWAYS written unchanged — these tests assert only whether
// ValueMBSpinID (the additive companion) got resolved, per the ambiguity-safe
// rule: exactly one match wins, anything else (0 or >1, or no resolver
// configured, or a non-MB_SPIN param) leaves it nil.
// =============================================================================
func TestUpsert_MBSpinResolution(t *testing.T) {
	t.Parallel()

	paramID := uuid.New()
	strPtr := func(s string) *string { return &s }
	mbSpinMeta := cpp.ParamMeta{ParamID: paramID, DataType: "TEXT", LookupMasterCode: "MB_SPIN"}

	t.Run("unique ORION code resolves to its mbs_id", func(t *testing.T) {
		t.Parallel()
		spinID := uuid.New()
		repo := &fakeRepo{productExists: true, getMeta: mbSpinMeta}
		mbRepo := &fakeMBSpinRepo{uniqueByOrionCode: map[string]uuid.UUID{"ORION-1": spinID}}
		h := app.New(repo, mbRepo)

		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr("ORION-1"), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.ValueMBSpinID == nil || *v.ValueMBSpinID != spinID {
			t.Fatalf("want resolved ValueMBSpinID=%s, got %+v", spinID, v.ValueMBSpinID)
		}
		if v.ValueText == nil || *v.ValueText != "ORION-1" {
			t.Fatalf("ValueText must stay unchanged, got %+v", v.ValueText)
		}
	})

	t.Run("ambiguous/unmatched ORION code leaves ValueMBSpinID nil, save still succeeds", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{productExists: true, getMeta: mbSpinMeta}
		// "ORION-DUP" deliberately absent from uniqueByOrionCode: simulates 0-or-many matches.
		mbRepo := &fakeMBSpinRepo{uniqueByOrionCode: map[string]uuid.UUID{}}
		h := app.New(repo, mbRepo)

		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr("ORION-DUP"), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.ValueMBSpinID != nil {
			t.Fatalf("want nil ValueMBSpinID for ambiguous code, got %s", *v.ValueMBSpinID)
		}
		if v.ValueText == nil || *v.ValueText != "ORION-DUP" {
			t.Fatalf("ValueText must stay unchanged, got %+v", v.ValueText)
		}
		if len(repo.upsertedValues) != 1 {
			t.Fatalf("save must still proceed despite ambiguity, got %d upserts", len(repo.upsertedValues))
		}
	})

	t.Run("valid UUID matching an existing spin wins outright, no ORION lookup needed", func(t *testing.T) {
		t.Parallel()
		spinID := uuid.New()
		repo := &fakeRepo{productExists: true, getMeta: mbSpinMeta}
		mbRepo := &fakeMBSpinRepo{existsByID: map[string]bool{spinID.String(): true}}
		h := app.New(repo, mbRepo)

		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr(spinID.String()), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.ValueMBSpinID == nil || *v.ValueMBSpinID != spinID {
			t.Fatalf("want resolved ValueMBSpinID=%s, got %+v", spinID, v.ValueMBSpinID)
		}
	})

	t.Run("UUID-shaped value with no matching spin resolves to nil, never guesses", func(t *testing.T) {
		t.Parallel()
		unknownID := uuid.New()
		repo := &fakeRepo{productExists: true, getMeta: mbSpinMeta}
		mbRepo := &fakeMBSpinRepo{existsByID: map[string]bool{}}
		h := app.New(repo, mbRepo)

		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr(unknownID.String()), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.ValueMBSpinID != nil {
			t.Fatalf("want nil ValueMBSpinID for unmatched UUID, got %s", *v.ValueMBSpinID)
		}
	})

	t.Run("non-MB_SPIN param never triggers resolution, even with a resolver configured", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{
			productExists: true,
			getMeta:       cpp.ParamMeta{ParamID: paramID, DataType: "TEXT"}, // LookupMasterCode unset
		}
		mbRepo := &fakeMBSpinRepo{uniqueByOrionCode: map[string]uuid.UUID{"ORION-1": uuid.New()}}
		h := app.New(repo, mbRepo)

		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr("ORION-1"), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.ValueMBSpinID != nil {
			t.Fatalf("non-MB_SPIN param must never resolve ValueMBSpinID, got %s", *v.ValueMBSpinID)
		}
	})

	t.Run("nil mbSpinRepo (resolver not configured) behaves exactly as before this feature", func(t *testing.T) {
		t.Parallel()
		repo := &fakeRepo{productExists: true, getMeta: mbSpinMeta}
		h := app.New(repo, nil)

		v, err := h.Upsert(context.Background(), app.UpsertCommand{
			ProductSysID: 1, ParamID: paramID, ValueText: strPtr("ORION-1"), FilledBy: "actor",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if v.ValueMBSpinID != nil {
			t.Fatalf("want nil ValueMBSpinID with no resolver configured, got %s", *v.ValueMBSpinID)
		}
		if len(repo.upsertedValues) != 1 {
			t.Fatalf("save must still succeed unchanged, got %d upserts", len(repo.upsertedValues))
		}
	})
}
