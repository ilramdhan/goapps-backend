// Package spinfixedcost_test provides unit tests for the Spin Fixed Cost domain entity.
package spinfixedcost_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/spinfixedcost"
)

const testActor = "admin"

// validInput returns a NewInput that passes every validation rule.
func validInput() spinfixedcost.NewInput {
	return spinfixedcost.NewInput{
		Period:             "202606",
		CommonPoyDenier:    150,
		PoyProduction:      1_200_000,
		SpinPowerMonth:     500_000_000,
		SpinManpowerMonth:  200_000_000,
		SpinOverheadsMonth: 100_000_000,
		SpinConssprsMonth:  50_000_000,
		CreatedBy:          testActor,
	}
}

func f64(v float64) *float64 { return &v }
func boolPtr(v bool) *bool   { return &v }

func TestNew_ValidInput_Succeeds(t *testing.T) {
	in := validInput()

	entity, err := spinfixedcost.New(in)

	require.NoError(t, err)
	require.NotNil(t, entity)
	assert.NotEqual(t, uuid.Nil, entity.ID())
	assert.Equal(t, "202606", entity.Period())
	assert.InDelta(t, 150.0, entity.CommonPoyDenier(), 0.001)
	assert.InDelta(t, 1_200_000.0, entity.PoyProduction(), 0.001)
	assert.InDelta(t, 500_000_000.0, entity.SpinPowerMonth(), 0.001)
	assert.InDelta(t, 200_000_000.0, entity.SpinManpowerMonth(), 0.001)
	assert.InDelta(t, 100_000_000.0, entity.SpinOverheadsMonth(), 0.001)
	assert.InDelta(t, 50_000_000.0, entity.SpinConssprsMonth(), 0.001)
	assert.True(t, entity.IsActive())
	assert.False(t, entity.IsDeleted())
	assert.Equal(t, testActor, entity.CreatedBy())
	assert.False(t, entity.CreatedAt().IsZero())
	assert.Nil(t, entity.UpdatedAt())
	assert.Nil(t, entity.UpdatedBy())
	assert.Nil(t, entity.DeletedAt())
	assert.Nil(t, entity.DeletedBy())
}

func TestNew_InvalidPeriod_ReturnsErrInvalidPeriod(t *testing.T) {
	tests := []struct {
		name   string
		period string
	}{
		{"empty", ""},
		{"too short - four digits", "2026"},
		{"too long - seven digits", "2026041"},
		{"trailing letter", "20260a"},
		{"all letters", "abcdef"},
		{"leading space", " 20260"},
		{"embedded separator", "2026-6"},
		{"unicode digits", "２０２６０６"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			in.Period = tt.period

			entity, err := spinfixedcost.New(in)

			assert.Nil(t, entity)
			assert.ErrorIs(t, err, spinfixedcost.ErrInvalidPeriod)
		})
	}
}

func TestNew_NegativeNumerics_AreRejected(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(*spinfixedcost.NewInput)
		wantErr error
	}{
		{
			name:    "negative common_poy_denier",
			mutate:  func(in *spinfixedcost.NewInput) { in.CommonPoyDenier = -1 },
			wantErr: spinfixedcost.ErrNonPositiveDenier,
		},
		{
			name:    "negative poy_production",
			mutate:  func(in *spinfixedcost.NewInput) { in.PoyProduction = -0.5 },
			wantErr: spinfixedcost.ErrNonPositiveProduction,
		},
		{
			name:    "negative spin_power_month",
			mutate:  func(in *spinfixedcost.NewInput) { in.SpinPowerMonth = -1 },
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
		{
			name:    "negative spin_manpower_month",
			mutate:  func(in *spinfixedcost.NewInput) { in.SpinManpowerMonth = -1 },
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
		{
			name:    "negative spin_overheads_month",
			mutate:  func(in *spinfixedcost.NewInput) { in.SpinOverheadsMonth = -1 },
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
		{
			name:    "negative spin_conssprs_month",
			mutate:  func(in *spinfixedcost.NewInput) { in.SpinConssprsMonth = -1 },
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			in := validInput()
			tt.mutate(&in)

			entity, err := spinfixedcost.New(in)

			assert.Nil(t, entity)
			assert.ErrorIs(t, err, tt.wantErr)
		})
	}
}

// TestNew_ZeroDivisors_AreRejected pins the asymmetry that matters most: the two
// divisors must be strictly positive, while the four monthly amounts may legitimately
// be zero (a month with no overheads is real; a zero divisor silently zeroes the pool).
func TestNew_ZeroDivisors_AreRejected(t *testing.T) {
	t.Run("zero common_poy_denier is rejected", func(t *testing.T) {
		in := validInput()
		in.CommonPoyDenier = 0

		entity, err := spinfixedcost.New(in)

		assert.Nil(t, entity)
		assert.ErrorIs(t, err, spinfixedcost.ErrNonPositiveDenier)
	})

	t.Run("zero poy_production is rejected", func(t *testing.T) {
		in := validInput()
		in.PoyProduction = 0

		entity, err := spinfixedcost.New(in)

		assert.Nil(t, entity)
		assert.ErrorIs(t, err, spinfixedcost.ErrNonPositiveProduction)
	})
}

func TestNew_ZeroMonthlyAmounts_AreAccepted(t *testing.T) {
	in := validInput()
	in.SpinPowerMonth = 0
	in.SpinManpowerMonth = 0
	in.SpinOverheadsMonth = 0
	in.SpinConssprsMonth = 0

	entity, err := spinfixedcost.New(in)

	require.NoError(t, err)
	require.NotNil(t, entity)
	assert.InDelta(t, 0.0, entity.SpinPowerMonth(), 0.001)
	assert.InDelta(t, 0.0, entity.SpinManpowerMonth(), 0.001)
	assert.InDelta(t, 0.0, entity.SpinOverheadsMonth(), 0.001)
	assert.InDelta(t, 0.0, entity.SpinConssprsMonth(), 0.001)
}

func TestNew_EmptyCreatedBy_ReturnsError(t *testing.T) {
	in := validInput()
	in.CreatedBy = ""

	entity, err := spinfixedcost.New(in)

	assert.Nil(t, entity)
	assert.ErrorIs(t, err, spinfixedcost.ErrEmptyCreatedBy)
}

func TestReconstruct_RoundTrip_GettersReturnPersistedValues(t *testing.T) {
	id := uuid.New()
	createdAt := time.Date(2026, 6, 1, 10, 0, 0, 0, time.UTC)
	updatedAt := time.Date(2026, 6, 15, 11, 30, 0, 0, time.UTC)
	deletedAt := time.Date(2026, 6, 20, 9, 0, 0, 0, time.UTC)
	updatedBy := "editor"
	deletedBy := "remover"

	entity := spinfixedcost.Reconstruct(spinfixedcost.ReconstructInput{
		ID:                 id,
		Period:             "202512",
		CommonPoyDenier:    167,
		PoyProduction:      987_654.25,
		SpinPowerMonth:     1,
		SpinManpowerMonth:  2,
		SpinOverheadsMonth: 3,
		SpinConssprsMonth:  4,
		IsActive:           false,
		CreatedAt:          createdAt,
		CreatedBy:          "creator",
		UpdatedAt:          &updatedAt,
		UpdatedBy:          &updatedBy,
		DeletedAt:          &deletedAt,
		DeletedBy:          &deletedBy,
	})

	require.NotNil(t, entity)
	assert.Equal(t, id, entity.ID())
	assert.Equal(t, "202512", entity.Period())
	assert.InDelta(t, 167.0, entity.CommonPoyDenier(), 0.001)
	assert.InDelta(t, 987_654.25, entity.PoyProduction(), 0.001)
	assert.InDelta(t, 1.0, entity.SpinPowerMonth(), 0.001)
	assert.InDelta(t, 2.0, entity.SpinManpowerMonth(), 0.001)
	assert.InDelta(t, 3.0, entity.SpinOverheadsMonth(), 0.001)
	assert.InDelta(t, 4.0, entity.SpinConssprsMonth(), 0.001)
	assert.False(t, entity.IsActive())
	assert.Equal(t, createdAt, entity.CreatedAt())
	assert.Equal(t, "creator", entity.CreatedBy())
	require.NotNil(t, entity.UpdatedAt())
	assert.Equal(t, updatedAt, *entity.UpdatedAt())
	require.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, updatedBy, *entity.UpdatedBy())
	require.NotNil(t, entity.DeletedAt())
	assert.Equal(t, deletedAt, *entity.DeletedAt())
	require.NotNil(t, entity.DeletedBy())
	assert.Equal(t, deletedBy, *entity.DeletedBy())
	assert.True(t, entity.IsDeleted())
}

// TestReconstruct_LiveRow_HasNilAuditPointers guards against Reconstruct fabricating
// timestamps for rows that were never updated or deleted.
func TestReconstruct_LiveRow_HasNilAuditPointers(t *testing.T) {
	entity := spinfixedcost.Reconstruct(spinfixedcost.ReconstructInput{
		ID:              uuid.New(),
		Period:          "202601",
		CommonPoyDenier: 150,
		PoyProduction:   1000,
		IsActive:        true,
		CreatedAt:       time.Now(),
		CreatedBy:       testActor,
	})

	assert.Nil(t, entity.UpdatedAt())
	assert.Nil(t, entity.UpdatedBy())
	assert.Nil(t, entity.DeletedAt())
	assert.Nil(t, entity.DeletedBy())
	assert.False(t, entity.IsDeleted())
	assert.True(t, entity.IsActive())
}

func TestUpdate_MutatesIntendedFields(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)

	err = entity.Update(spinfixedcost.UpdateInput{
		CommonPoyDenier:    f64(167),
		PoyProduction:      f64(2_000_000),
		SpinPowerMonth:     f64(600_000_000),
		SpinManpowerMonth:  f64(0),
		SpinOverheadsMonth: f64(1),
		SpinConssprsMonth:  f64(2),
		IsActive:           boolPtr(false),
	}, "editor")

	require.NoError(t, err)
	assert.InDelta(t, 167.0, entity.CommonPoyDenier(), 0.001)
	assert.InDelta(t, 2_000_000.0, entity.PoyProduction(), 0.001)
	assert.InDelta(t, 600_000_000.0, entity.SpinPowerMonth(), 0.001)
	assert.InDelta(t, 0.0, entity.SpinManpowerMonth(), 0.001)
	assert.InDelta(t, 1.0, entity.SpinOverheadsMonth(), 0.001)
	assert.InDelta(t, 2.0, entity.SpinConssprsMonth(), 0.001)
	assert.False(t, entity.IsActive())
	require.NotNil(t, entity.UpdatedAt())
	require.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, "editor", *entity.UpdatedBy())
}

func TestUpdate_NilFields_LeaveValuesUntouched(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)

	err = entity.Update(spinfixedcost.UpdateInput{}, "editor")

	require.NoError(t, err)
	assert.InDelta(t, 150.0, entity.CommonPoyDenier(), 0.001)
	assert.InDelta(t, 1_200_000.0, entity.PoyProduction(), 0.001)
	assert.InDelta(t, 500_000_000.0, entity.SpinPowerMonth(), 0.001)
	assert.True(t, entity.IsActive())
	require.NotNil(t, entity.UpdatedBy())
	assert.Equal(t, "editor", *entity.UpdatedBy())
}

// TestUpdate_CannotChangePeriod asserts the immutability of period. UpdateInput has no
// Period field, so this is a structural guarantee — the assertion exists so that adding
// one later breaks a test rather than silently letting the calc engine's anchor move.
func TestUpdate_CannotChangePeriod(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)
	original := entity.Period()

	err = entity.Update(spinfixedcost.UpdateInput{
		CommonPoyDenier: f64(200),
		IsActive:        boolPtr(false),
	}, "editor")

	require.NoError(t, err)
	assert.Equal(t, original, entity.Period())
	assert.Equal(t, "202606", entity.Period())
}

func TestUpdate_ReAppliesValidationRules(t *testing.T) {
	tests := []struct {
		name    string
		in      spinfixedcost.UpdateInput
		wantErr error
	}{
		{
			name:    "negative common_poy_denier",
			in:      spinfixedcost.UpdateInput{CommonPoyDenier: f64(-1)},
			wantErr: spinfixedcost.ErrNonPositiveDenier,
		},
		{
			name:    "zero common_poy_denier",
			in:      spinfixedcost.UpdateInput{CommonPoyDenier: f64(0)},
			wantErr: spinfixedcost.ErrNonPositiveDenier,
		},
		{
			name:    "negative poy_production",
			in:      spinfixedcost.UpdateInput{PoyProduction: f64(-1)},
			wantErr: spinfixedcost.ErrNonPositiveProduction,
		},
		{
			name:    "zero poy_production",
			in:      spinfixedcost.UpdateInput{PoyProduction: f64(0)},
			wantErr: spinfixedcost.ErrNonPositiveProduction,
		},
		{
			name:    "negative spin_power_month",
			in:      spinfixedcost.UpdateInput{SpinPowerMonth: f64(-1)},
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
		{
			name:    "negative spin_manpower_month",
			in:      spinfixedcost.UpdateInput{SpinManpowerMonth: f64(-1)},
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
		{
			name:    "negative spin_overheads_month",
			in:      spinfixedcost.UpdateInput{SpinOverheadsMonth: f64(-1)},
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
		{
			name:    "negative spin_conssprs_month",
			in:      spinfixedcost.UpdateInput{SpinConssprsMonth: f64(-1)},
			wantErr: spinfixedcost.ErrNegativeAmount,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity, err := spinfixedcost.New(validInput())
			require.NoError(t, err)

			err = entity.Update(tt.in, "editor")

			assert.ErrorIs(t, err, tt.wantErr)
			// A rejected update must not have stamped the audit columns.
			assert.Nil(t, entity.UpdatedAt())
			assert.Nil(t, entity.UpdatedBy())
		})
	}
}

// TestUpdate_ZeroMonthlyAmounts_AreAccepted mirrors the New-path asymmetry.
func TestUpdate_ZeroMonthlyAmounts_AreAccepted(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)

	err = entity.Update(spinfixedcost.UpdateInput{
		SpinPowerMonth:     f64(0),
		SpinManpowerMonth:  f64(0),
		SpinOverheadsMonth: f64(0),
		SpinConssprsMonth:  f64(0),
	}, "editor")

	require.NoError(t, err)
	assert.InDelta(t, 0.0, entity.SpinPowerMonth(), 0.001)
	assert.InDelta(t, 0.0, entity.SpinConssprsMonth(), 0.001)
}

func TestUpdate_AfterSoftDelete_ReturnsErrAlreadyDeleted(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)
	require.NoError(t, entity.SoftDelete("remover"))

	err = entity.Update(spinfixedcost.UpdateInput{CommonPoyDenier: f64(200)}, "editor")

	assert.ErrorIs(t, err, spinfixedcost.ErrAlreadyDeleted)
	assert.InDelta(t, 150.0, entity.CommonPoyDenier(), 0.001)
}

func TestSoftDelete_SetsAuditColumnsAndDeactivates(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)
	before := time.Now().Add(-time.Second)

	err = entity.SoftDelete("remover")

	require.NoError(t, err)
	assert.True(t, entity.IsDeleted())
	require.NotNil(t, entity.DeletedAt())
	assert.True(t, entity.DeletedAt().After(before))
	require.NotNil(t, entity.DeletedBy())
	assert.Equal(t, "remover", *entity.DeletedBy())
	// A deleted row must also stop being active, otherwise the anchor guard would
	// still treat it as a live anchor.
	assert.False(t, entity.IsActive())
}

// TestSoftDelete_SecondCall_ReturnsErrAlreadyDeleted documents the actual behavior:
// SoftDelete is not idempotent, it errors, and the first deleter is preserved.
func TestSoftDelete_SecondCall_ReturnsErrAlreadyDeleted(t *testing.T) {
	entity, err := spinfixedcost.New(validInput())
	require.NoError(t, err)
	require.NoError(t, entity.SoftDelete("first"))
	firstDeletedAt := *entity.DeletedAt()

	err = entity.SoftDelete("second")

	assert.ErrorIs(t, err, spinfixedcost.ErrAlreadyDeleted)
	require.NotNil(t, entity.DeletedBy())
	assert.Equal(t, "first", *entity.DeletedBy())
	assert.Equal(t, firstDeletedAt, *entity.DeletedAt())
}

func TestDeactivatesRow(t *testing.T) {
	tests := []struct {
		name            string
		currentIsActive bool
		isActive        *bool
		want            bool
	}{
		{"nil IsActive on active row", true, nil, false},
		{"nil IsActive on inactive row", false, nil, false},
		{"true on active row", true, boolPtr(true), false},
		{"true on inactive row (reactivation)", false, boolPtr(true), false},
		{"false on active row", true, boolPtr(false), true},
		{"false on already inactive row", false, boolPtr(false), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entity := spinfixedcost.Reconstruct(spinfixedcost.ReconstructInput{
				ID:              uuid.New(),
				Period:          "202606",
				CommonPoyDenier: 150,
				PoyProduction:   1000,
				IsActive:        tt.currentIsActive,
				CreatedAt:       time.Now(),
				CreatedBy:       testActor,
			})

			got := spinfixedcost.UpdateInput{IsActive: tt.isActive}.DeactivatesRow(entity)

			assert.Equal(t, tt.want, got)
		})
	}
}

// TestPeriodLexicographicOrdering pins the assumption the anchor guard and the calc
// engine's ORDER BY both rely on: because msfc_period is zero-padded YYYYMM, plain
// string comparison is chronological comparison.
func TestPeriodLexicographicOrdering(t *testing.T) {
	tests := []struct {
		name     string
		earlier  string
		later    string
		wantLess bool
	}{
		{"same year consecutive months", "202609", "202610", true},
		{"year rollover", "202512", "202601", true},
		{"january before december same year", "202601", "202612", true},
		{"decade rollover", "201912", "202001", true},
		{"identical periods are not less", "202606", "202606", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantLess, tt.earlier < tt.later)
		})
	}
}

func TestListFilter_Validate_AppliesDefaultsAndClamps(t *testing.T) {
	tests := []struct {
		name          string
		filter        spinfixedcost.ListFilter
		wantPage      int
		wantPageSize  int
		wantSortBy    string
		wantSortOrder string
	}{
		{
			name:          "empty filter gets defaults",
			filter:        spinfixedcost.ListFilter{},
			wantPage:      1,
			wantPageSize:  10,
			wantSortBy:    "period",
			wantSortOrder: "desc",
		},
		{
			name:          "zero and negative page clamp to 1",
			filter:        spinfixedcost.ListFilter{Page: -5, PageSize: 25, SortBy: "created_at", SortOrder: "asc"},
			wantPage:      1,
			wantPageSize:  25,
			wantSortBy:    "created_at",
			wantSortOrder: "asc",
		},
		{
			name:          "page size above 100 clamps to 100",
			filter:        spinfixedcost.ListFilter{Page: 3, PageSize: 5000},
			wantPage:      3,
			wantPageSize:  100,
			wantSortBy:    "period",
			wantSortOrder: "desc",
		},
		{
			name:          "explicit values are preserved",
			filter:        spinfixedcost.ListFilter{Page: 2, PageSize: 50, SortBy: "updated_at", SortOrder: "asc"},
			wantPage:      2,
			wantPageSize:  50,
			wantSortBy:    "updated_at",
			wantSortOrder: "asc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.filter
			f.Validate()

			assert.Equal(t, tt.wantPage, f.Page)
			assert.Equal(t, tt.wantPageSize, f.PageSize)
			assert.Equal(t, tt.wantSortBy, f.SortBy)
			assert.Equal(t, tt.wantSortOrder, f.SortOrder)
		})
	}
}

func TestListFilter_Offset(t *testing.T) {
	tests := []struct {
		name     string
		page     int
		pageSize int
		want     int
	}{
		{"first page", 1, 10, 0},
		{"second page", 2, 10, 10},
		{"third page of 25", 3, 25, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := spinfixedcost.ListFilter{Page: tt.page, PageSize: tt.pageSize}

			assert.Equal(t, tt.want, f.Offset())
		})
	}
}
