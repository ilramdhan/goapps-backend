package costproductparameter_test

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	app "github.com/mutugading/goapps-backend/services/finance/internal/application/costproductparameter"
	cpp "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductparameter"
)

// =============================================================================
// fakeRepo — in-memory test double for cpp.Repository. Captures inputs and
// lets each test program targeted error returns via overrides.
// =============================================================================
type fakeRepo struct {
	productExists bool
	getMetaErr    error
	getMeta       cpp.ParamMeta
	upsertErr     error
	deleteErr     error

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

// =============================================================================
// Upsert
// =============================================================================
func TestUpsert(t *testing.T) {
	t.Parallel()

	paramID := uuid.New()
	strPtr := func(s string) *string { return &s }

	t.Run("product missing returns ErrProductNotFound", func(t *testing.T) {
		t.Parallel()
		h := app.New(&fakeRepo{productExists: false})
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
		h := app.New(repo)
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
		h := app.New(repo)
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
		h := app.New(repo)
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
	h := app.New(repo)

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
		h := app.New(&fakeRepo{productExists: false})
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
		h := app.New(repo)
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
		h := app.New(repo)
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
