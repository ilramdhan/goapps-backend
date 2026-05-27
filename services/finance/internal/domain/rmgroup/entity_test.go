package rmgroup_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/rmgroup"
)

// ----------------------------------------------------------------------------
// Value object tests
// ----------------------------------------------------------------------------

func TestNewCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"valid alpha", "CHM0000118", "CHM0000118", nil},
		{"valid with space", "BLUE MGTS-5109", "BLUE MGTS-5109", nil},
		{"valid with hyphen", "PIG0000005-COM", "PIG0000005-COM", nil},
		{"lowercase normalized", "pig0000005-com", "PIG0000005-COM", nil},
		{"trimmed", "  CHM0000118  ", "CHM0000118", nil},
		{"empty", "", "", rmgroup.ErrEmptyCode},
		{"whitespace only", "   ", "", rmgroup.ErrEmptyCode},
		{"too long", strings.Repeat("A", 31), "", rmgroup.ErrCodeTooLong},
		{"starts with hyphen", "-ABC", "", rmgroup.ErrInvalidCodeFormat},
		{"invalid char underscore", "ABC_123", "", rmgroup.ErrInvalidCodeFormat},
		{"invalid char dot", "ABC.123", "", rmgroup.ErrInvalidCodeFormat},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := rmgroup.NewCode(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestCodeEqualAndIsEmpty(t *testing.T) {
	t.Parallel()
	a, _ := rmgroup.NewCode("CHM0000118")
	b, _ := rmgroup.NewCode("chm0000118")
	c, _ := rmgroup.NewCode("OTHER")

	assert.True(t, a.Equal(b))
	assert.False(t, a.Equal(c))
	assert.False(t, a.IsEmpty())
	assert.True(t, (rmgroup.Code{}).IsEmpty())
}

func TestNewItemCode(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr error
	}{
		{"valid", "ITEM001", "ITEM001", nil},
		{"trimmed", "  ITEM001  ", "ITEM001", nil},
		{"empty", "", "", rmgroup.ErrEmptyItemCode},
		{"too long", strings.Repeat("A", 21), "", rmgroup.ErrItemCodeTooLong},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := rmgroup.NewItemCode(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}

func TestParseFlag(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    rmgroup.Flag
		wantErr error
	}{
		{"CONS", "CONS", rmgroup.FlagCons, nil},
		{"stores lowercase", "stores", rmgroup.FlagStores, nil},
		{"DEPT", "DEPT", rmgroup.FlagDept, nil},
		{"PO_1", "PO_1", rmgroup.FlagPO1, nil},
		{"PO_2", "PO_2", rmgroup.FlagPO2, nil},
		{"PO_3", "PO_3", rmgroup.FlagPO3, nil},
		{"INIT trimmed", "  INIT  ", rmgroup.FlagInit, nil},
		{"unknown", "XYZ", "", rmgroup.ErrInvalidFlag},
		{"empty", "", "", rmgroup.ErrInvalidFlag},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := rmgroup.ParseFlag(tt.input)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestFlagPredicates(t *testing.T) {
	t.Parallel()
	assert.True(t, rmgroup.FlagCons.IsValid())
	assert.True(t, rmgroup.FlagInit.IsValid())
	assert.False(t, rmgroup.Flag("BOGUS").IsValid())
	assert.True(t, rmgroup.FlagInit.IsInit())
	assert.False(t, rmgroup.FlagCons.IsInit())
	assert.Equal(t, "CONS", rmgroup.FlagCons.String())
}

// ----------------------------------------------------------------------------
// Head — constructor tests
// ----------------------------------------------------------------------------

func mustCode(t *testing.T, raw string) rmgroup.Code {
	t.Helper()
	c, err := rmgroup.NewCode(raw)
	require.NoError(t, err)
	return c
}

func TestNewHead(t *testing.T) {
	t.Parallel()

	validCode := mustCode(t, "CHM0000118")

	tests := []struct {
		name           string
		code           rmgroup.Code
		groupName      string
		costPercentage float64
		costPerKg      float64
		createdBy      string
		wantErr        error
	}{
		{"valid", validCode, "Pigment Group A", 10, 100, "admin", nil},
		{"empty code", rmgroup.Code{}, "Name", 0, 0, "admin", rmgroup.ErrEmptyCode},
		{"empty name", validCode, "", 0, 0, "admin", rmgroup.ErrEmptyName},
		{"name too long", validCode, strings.Repeat("n", 201), 0, 0, "admin", rmgroup.ErrNameTooLong},
		{"empty createdBy", validCode, "Name", 0, 0, "", rmgroup.ErrEmptyCreatedBy},
		{"negative cost percentage", validCode, "Name", -1, 0, "admin", rmgroup.ErrNegativeCostPercentage},
		{"negative cost per kg", validCode, "Name", 0, -1, "admin", rmgroup.ErrNegativeCostPerKg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := rmgroup.NewHead(tt.code, tt.groupName, "", tt.costPercentage, tt.costPerKg, tt.createdBy)
			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, got)
			assert.Equal(t, tt.code.String(), got.Code().String())
			assert.Equal(t, tt.groupName, got.Name())
			assert.Equal(t, tt.costPercentage, got.CostPercentage())
			assert.Equal(t, tt.costPerKg, got.CostPerKg())
			assert.Equal(t, rmgroup.FlagCons, got.FlagValuation())
			assert.Equal(t, rmgroup.FlagCons, got.FlagMarketing())
			assert.Equal(t, rmgroup.FlagCons, got.FlagSimulation())
			assert.True(t, got.IsActive())
			assert.False(t, got.IsDeleted())
			assert.NotEqual(t, uuid.Nil, got.ID())
		})
	}
}

// ----------------------------------------------------------------------------
// Head — Update tests
// ----------------------------------------------------------------------------

func newTestHead(t *testing.T) *rmgroup.Head {
	t.Helper()
	h, err := rmgroup.NewHead(mustCode(t, "CHM0000118"), "Name", "", 10, 100, "admin")
	require.NoError(t, err)
	return h
}

func TestHead_Update_FieldChanges(t *testing.T) {
	t.Parallel()

	newName := "Updated"
	newDescription := "desc"
	newColorant := "red"
	newCI := "ci"
	newCostPct := 20.0
	newCostPerKg := 200.0
	inactive := false

	h := newTestHead(t)
	err := h.Update(rmgroup.UpdateInput{
		Name:           &newName,
		Description:    &newDescription,
		Colorant:       &newColorant,
		CIName:         &newCI,
		CostPercentage: &newCostPct,
		CostPerKg:      &newCostPerKg,
		IsActive:       &inactive,
	}, "editor")
	require.NoError(t, err)

	assert.Equal(t, newName, h.Name())
	assert.Equal(t, newDescription, h.Description())
	assert.Equal(t, newColorant, h.Colorant())
	assert.Equal(t, newCI, h.CIName())
	assert.Equal(t, newCostPct, h.CostPercentage())
	assert.Equal(t, newCostPerKg, h.CostPerKg())
	assert.False(t, h.IsActive())
	require.NotNil(t, h.UpdatedBy())
	assert.Equal(t, "editor", *h.UpdatedBy())
	require.NotNil(t, h.UpdatedAt())
}

func TestHead_Update_Validations(t *testing.T) {
	t.Parallel()

	empty := ""
	tooLong := strings.Repeat("x", 201)
	negativePct := -1.0
	negativePerKg := -1.0

	tests := []struct {
		name    string
		input   rmgroup.UpdateInput
		wantErr error
	}{
		{"empty name", rmgroup.UpdateInput{Name: &empty}, rmgroup.ErrEmptyName},
		{"name too long", rmgroup.UpdateInput{Name: &tooLong}, rmgroup.ErrNameTooLong},
		{"negative cost percentage", rmgroup.UpdateInput{CostPercentage: &negativePct}, rmgroup.ErrNegativeCostPercentage},
		{"negative cost per kg", rmgroup.UpdateInput{CostPerKg: &negativePerKg}, rmgroup.ErrNegativeCostPerKg},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newTestHead(t)
			err := h.Update(tt.input, "editor")
			require.ErrorIs(t, err, tt.wantErr)
		})
	}
}

func TestHead_Update_EmptyUpdatedBy(t *testing.T) {
	t.Parallel()
	h := newTestHead(t)
	err := h.Update(rmgroup.UpdateInput{}, "")
	require.ErrorIs(t, err, rmgroup.ErrEmptyUpdatedBy)
}

func TestHead_Update_AfterDelete(t *testing.T) {
	t.Parallel()
	h := newTestHead(t)
	require.NoError(t, h.SoftDelete("admin"))
	err := h.Update(rmgroup.UpdateInput{}, "admin")
	require.ErrorIs(t, err, rmgroup.ErrAlreadyDeleted)
}

func TestHead_Update_FlagInitRequiresInitVal(t *testing.T) {
	t.Parallel()

	initFlag := rmgroup.FlagInit
	h := newTestHead(t)

	// INIT without a init_val → error.
	err := h.Update(rmgroup.UpdateInput{FlagValuation: &initFlag}, "editor")
	require.ErrorIs(t, err, rmgroup.ErrInitValueRequired)

	// INIT with init_val → ok.
	val := 42.5
	err = h.Update(rmgroup.UpdateInput{
		FlagValuation:    &initFlag,
		InitValValuation: &val,
	}, "editor")
	require.NoError(t, err)
	require.NotNil(t, h.InitValValuation())
	assert.Equal(t, val, *h.InitValValuation())

	// Clearing init_val while flag still INIT → error.
	err = h.Update(rmgroup.UpdateInput{ClearInitValValuation: true}, "editor")
	require.ErrorIs(t, err, rmgroup.ErrInitValueRequired)
}

func TestHead_Update_InvalidFlag(t *testing.T) {
	t.Parallel()
	h := newTestHead(t)
	bogus := rmgroup.Flag("BOGUS")
	err := h.Update(rmgroup.UpdateInput{FlagValuation: &bogus}, "editor")
	require.ErrorIs(t, err, rmgroup.ErrInvalidFlag)
}

func TestHead_Update_NegativeInitValue(t *testing.T) {
	t.Parallel()
	h := newTestHead(t)
	neg := -0.01
	err := h.Update(rmgroup.UpdateInput{InitValMarketing: &neg}, "editor")
	require.ErrorIs(t, err, rmgroup.ErrNegativeInitValue)
}

func TestHead_Update_ClearInitVal(t *testing.T) {
	t.Parallel()
	h := newTestHead(t)
	val := 10.0
	require.NoError(t, h.Update(rmgroup.UpdateInput{InitValMarketing: &val}, "editor"))
	require.NotNil(t, h.InitValMarketing())

	require.NoError(t, h.Update(rmgroup.UpdateInput{ClearInitValMarketing: true}, "editor"))
	assert.Nil(t, h.InitValMarketing())
}

func TestHead_SoftDelete(t *testing.T) {
	t.Parallel()

	h := newTestHead(t)
	require.NoError(t, h.SoftDelete("admin"))
	assert.True(t, h.IsDeleted())
	assert.False(t, h.IsActive())
	require.NotNil(t, h.DeletedBy())
	assert.Equal(t, "admin", *h.DeletedBy())

	// Idempotent — second call errors.
	err := h.SoftDelete("admin")
	require.ErrorIs(t, err, rmgroup.ErrAlreadyDeleted)
}

func TestHead_SoftDelete_EmptyBy(t *testing.T) {
	t.Parallel()
	h := newTestHead(t)
	err := h.SoftDelete("")
	require.ErrorIs(t, err, rmgroup.ErrEmptyUpdatedBy)
}

// ----------------------------------------------------------------------------
// Detail tests
// ----------------------------------------------------------------------------

func mustItemCode(t *testing.T, raw string) rmgroup.ItemCode {
	t.Helper()
	ic, err := rmgroup.NewItemCode(raw)
	require.NoError(t, err)
	return ic
}

func TestNewDetail(t *testing.T) {
	t.Parallel()

	headID := uuid.New()
	ic := mustItemCode(t, "ITEM001")

	d, err := rmgroup.NewDetail(headID, ic, "admin")
	require.NoError(t, err)
	assert.Equal(t, headID, d.HeadID())
	assert.Equal(t, "ITEM001", d.ItemCode().String())
	assert.True(t, d.IsActive())
	assert.False(t, d.IsDummy())
	assert.False(t, d.IsDeleted())
	assert.NotEqual(t, uuid.Nil, d.ID())
}

func TestNewDetail_Validations(t *testing.T) {
	t.Parallel()

	headID := uuid.New()
	ic := mustItemCode(t, "ITEM001")

	_, err := rmgroup.NewDetail(headID, rmgroup.ItemCode{}, "admin")
	require.ErrorIs(t, err, rmgroup.ErrEmptyItemCode)

	_, err = rmgroup.NewDetail(headID, ic, "")
	require.ErrorIs(t, err, rmgroup.ErrEmptyCreatedBy)
}

func newTestDetail(t *testing.T) *rmgroup.Detail {
	t.Helper()
	d, err := rmgroup.NewDetail(uuid.New(), mustItemCode(t, "ITEM001"), "admin")
	require.NoError(t, err)
	return d
}

func TestDetail_Update_Fields(t *testing.T) {
	t.Parallel()

	itemName := "Item Name"
	itemType := "TYPE"
	grade := "G1"
	itemGrade := "Grade 1"
	uom := "KG"
	mktPct := 12.5
	mktVal := 100.0
	sortOrder := int32(5)
	active := false
	dummy := true

	d := newTestDetail(t)
	err := d.Update(rmgroup.DetailUpdateInput{
		ItemName:         &itemName,
		ItemTypeCode:     &itemType,
		GradeCode:        &grade,
		ItemGrade:        &itemGrade,
		UOMCode:          &uom,
		MarketPercentage: &mktPct,
		MarketValueRp:    &mktVal,
		SortOrder:        &sortOrder,
		IsActive:         &active,
		IsDummy:          &dummy,
	}, "editor")
	require.NoError(t, err)

	assert.Equal(t, itemName, d.ItemName())
	assert.Equal(t, itemType, d.ItemTypeCode())
	assert.Equal(t, grade, d.GradeCode())
	assert.Equal(t, itemGrade, d.ItemGrade())
	assert.Equal(t, uom, d.UOMCode())
	require.NotNil(t, d.MarketPercentage())
	assert.Equal(t, mktPct, *d.MarketPercentage())
	require.NotNil(t, d.MarketValueRp())
	assert.Equal(t, mktVal, *d.MarketValueRp())
	assert.Equal(t, sortOrder, d.SortOrder())
	assert.False(t, d.IsActive())
	assert.True(t, d.IsDummy())
}

func TestDetail_Update_NegativeMarketValues(t *testing.T) {
	t.Parallel()

	neg := -1.0

	d := newTestDetail(t)
	err := d.Update(rmgroup.DetailUpdateInput{MarketPercentage: &neg}, "editor")
	require.ErrorIs(t, err, rmgroup.ErrNegativeMarketPercentage)

	err = d.Update(rmgroup.DetailUpdateInput{MarketValueRp: &neg}, "editor")
	require.ErrorIs(t, err, rmgroup.ErrNegativeMarketValue)
}

func TestDetail_Update_ClearMarketFields(t *testing.T) {
	t.Parallel()

	pct := 10.0
	val := 100.0

	d := newTestDetail(t)
	require.NoError(t, d.Update(rmgroup.DetailUpdateInput{
		MarketPercentage: &pct,
		MarketValueRp:    &val,
	}, "editor"))
	require.NotNil(t, d.MarketPercentage())
	require.NotNil(t, d.MarketValueRp())

	require.NoError(t, d.Update(rmgroup.DetailUpdateInput{
		ClearMarketPercentage: true,
		ClearMarketValueRp:    true,
	}, "editor"))
	assert.Nil(t, d.MarketPercentage())
	assert.Nil(t, d.MarketValueRp())
}

func TestDetail_Update_AfterDelete(t *testing.T) {
	t.Parallel()
	d := newTestDetail(t)
	require.NoError(t, d.SoftDelete("admin"))
	err := d.Update(rmgroup.DetailUpdateInput{}, "admin")
	require.ErrorIs(t, err, rmgroup.ErrAlreadyDeleted)
}

func TestDetail_SoftDelete(t *testing.T) {
	t.Parallel()

	d := newTestDetail(t)
	require.NoError(t, d.SoftDelete("admin"))
	assert.True(t, d.IsDeleted())
	assert.False(t, d.IsActive())

	err := d.SoftDelete("admin")
	require.ErrorIs(t, err, rmgroup.ErrAlreadyDeleted)
}

func TestDetail_ActivateDeactivate(t *testing.T) {
	t.Parallel()

	d := newTestDetail(t)
	require.NoError(t, d.Deactivate("editor"))
	assert.False(t, d.IsActive())

	require.NoError(t, d.Activate("editor"))
	assert.True(t, d.IsActive())

	require.NoError(t, d.SoftDelete("admin"))
	err := d.Activate("editor")
	require.True(t, errors.Is(err, rmgroup.ErrAlreadyDeleted))
	err = d.Deactivate("editor")
	require.True(t, errors.Is(err, rmgroup.ErrAlreadyDeleted))
}

// ----------------------------------------------------------------------------
// ListFilter tests
// ----------------------------------------------------------------------------

func TestListFilter_Validate(t *testing.T) {
	t.Parallel()

	f := rmgroup.ListFilter{Page: 0, PageSize: 0}
	f.Validate()
	assert.Equal(t, 1, f.Page)
	assert.Equal(t, 10, f.PageSize)
	assert.Equal(t, "code", f.SortBy)
	assert.Equal(t, "asc", f.SortOrder)

	f = rmgroup.ListFilter{PageSize: 500}
	f.Validate()
	assert.Equal(t, 100, f.PageSize)

	f = rmgroup.ListFilter{Page: 3, PageSize: 20}
	f.Validate()
	assert.Equal(t, 40, f.Offset())
}
