package costproductmaster_test

import (
	"strings"
	"testing"
	"time"

	"github.com/shopspring/decimal"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domain "github.com/mutugading/goapps-backend/services/finance/internal/domain/costproductmaster"
)

var fixedAt = time.Date(2026, 9, 29, 10, 0, 0, 0, time.UTC)

func product(active bool, grade, shade, erpItem string) *domain.CostProductMaster {
	return domain.Reconstruct(
		42, "CSTPOY2609000001", 3,
		"POY test", shade, grade, "",
		erpItem, "", "",
		nil, "",
		active,
		fixedAt, "seed", fixedAt, "seed",
		"", "", "", "",
		"", false,
	)
}

func strp(s string) *string { return &s }

func TestCheckErpLink(t *testing.T) {
	tests := []struct {
		name     string
		p        *domain.CostProductMaster
		item     string
		shade    string
		isMB     bool
		wantErr  error
		wantNone bool
	}{
		{"ok yarn shaded", product(true, "AX", "X419T", ""), "POY0000275", "X419T", false, nil, true},
		{"ok case and space insensitive shade", product(true, "AX", " x419t ", ""), "POY0000275", "X419T", false, nil, true},
		{"ok empty grade counts as AX", product(true, "", "X419T", ""), "POY0000275", "X419T", false, nil, true},
		{"ok unshaded combo on unshaded product", product(true, "AX", "", ""), "POY0000275", "", false, nil, true},
		{"ok CMB on MB", product(true, "AX", "RED01", ""), "CMB0000001", "RED01", true, nil, true},
		{"inactive", product(false, "AX", "X419T", ""), "POY0000275", "X419T", false, domain.ErrInactive, false},
		{"non AX grade", product(true, "A", "X419T", ""), "POY0000275", "X419T", false, domain.ErrLinkNotAxGrade, false},
		{"CMB on yarn (V-12)", product(true, "AX", "RED01", ""), "CMB0000001", "RED01", false, domain.ErrLinkCmbRequiresMB, false},
		{"yarn item on MB (V-12)", product(true, "AX", "RED01", ""), "POY0000275", "RED01", true, domain.ErrLinkCmbRequiresMB, false},
		{"shade mismatch", product(true, "AX", "X419T", ""), "POY0000275", "Z444T", false, domain.ErrLinkShadeMismatch, false},
		{"shaded combo on unshaded product", product(true, "AX", "", ""), "POY0000275", "X419T", false, domain.ErrLinkProductHasNoShade, false},
		{"unshaded combo on shaded product", product(true, "AX", "X419T", ""), "POY0000275", "", false, domain.ErrLinkShadeMismatch, false},
		{"shade too long", product(true, "AX", "X419T", ""), "POY0000275", strings.Repeat("S", 51), false, domain.ErrLinkInvalidShadeCode, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.p.CheckErpLink(tc.item, tc.shade, tc.isMB)
			if tc.wantNone {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.wantErr)
		})
	}
}

func TestNormalizeErpItemCode(t *testing.T) {
	got, err := domain.NormalizeErpItemCode("  POY0000275 ")
	require.NoError(t, err)
	assert.Equal(t, "POY0000275", got)

	got, err = domain.NormalizeErpItemCode("   ")
	require.NoError(t, err)
	assert.Empty(t, got)

	_, err = domain.NormalizeErpItemCode(strings.Repeat("P", 51))
	require.ErrorIs(t, err, domain.ErrLinkInvalidItemCode)
	_, err = domain.NormalizeErpItemCode("POYé")
	require.ErrorIs(t, err, domain.ErrLinkInvalidItemCode)
}

func TestIsCmbItemAndIsAxGrade(t *testing.T) {
	assert.True(t, domain.IsCmbItem("CMB0001"))
	assert.True(t, domain.IsCmbItem(" cmb0001"))
	assert.False(t, domain.IsCmbItem("POY0001"))
	assert.True(t, domain.IsAxGrade(""))
	assert.True(t, domain.IsAxGrade("ax"))
	assert.False(t, domain.IsAxGrade("A"))
}

func TestLinkErpItem_WritesItemOnlyNeverGradeCodes(t *testing.T) {
	p := domain.Reconstruct(
		1, "C1", 3, "n", "X419T", "AX", "",
		"", "G1-KEEP", "G2-KEEP", nil, "",
		true, fixedAt, "s", fixedAt, "s", "", "", "", "", "", false,
	)
	at := fixedAt.Add(time.Hour)
	require.True(t, p.LinkErpItem(" POY0000275 ", "alice", at))
	assert.Equal(t, "POY0000275", p.ErpItemCode())
	assert.Equal(t, "G1-KEEP", p.ErpGradeCode1(), "grade_code_1 untouched (D-LINK)")
	assert.Equal(t, "G2-KEEP", p.ErpGradeCode2(), "grade_code_2 untouched (D-LINK)")
	require.NotNil(t, p.ErpLinkedAt())
	assert.Equal(t, at, *p.ErpLinkedAt())
	assert.Equal(t, "alice", p.ErpLinkedBy())
	assert.Equal(t, "alice", p.UpdatedBy())

	assert.False(t, p.LinkErpItem("POY0000275", "bob", at), "same code is a no-op")
	assert.Equal(t, "alice", p.UpdatedBy())

	require.True(t, p.LinkErpItem("", "bob", at))
	assert.Empty(t, p.ErpItemCode())
	assert.Nil(t, p.ErpLinkedAt())
	assert.Empty(t, p.ErpLinkedBy())
	assert.Equal(t, "G2-KEEP", p.ErpGradeCode2())
}

func TestApplyErpAttributes(t *testing.T) {
	p := product(true, "AX", "X419T", "POY0000275")
	assert.Equal(t, domain.ErpAttributes{}, p.ErpAttributes(), "existing products start with all NULL")

	changed, err := p.ApplyErpAttributes(domain.ErpAttributesPatch{
		FgType: strp(" Type 1 "), ChpItemCode: strp("CHP0001"), MsBatchItem: strp("MSB000000001"),
		ItemType: strp("FG"), PrdPerDay: strp("123.45678"),
	}, "alice", fixedAt)
	require.NoError(t, err)
	assert.True(t, changed)
	a := p.ErpAttributes()
	assert.Equal(t, "Type 1", a.FgType)
	assert.Equal(t, "MSB000000001", a.MsBatchItem)
	require.True(t, a.PrdPerDay.Valid)
	assert.True(t, a.PrdPerDay.Decimal.Equal(decimal.RequireFromString("123.45678")))

	changed, err = p.ApplyErpAttributes(domain.ErpAttributesPatch{PrdPerDay: strp("123.456780")}, "bob", fixedAt)
	require.NoError(t, err)
	assert.False(t, changed, "equal decimal is a no-op")

	changed, err = p.ApplyErpAttributes(domain.ErpAttributesPatch{FgType: strp(""), PrdPerDay: strp("")}, "bob", fixedAt)
	require.NoError(t, err)
	assert.True(t, changed)
	assert.Empty(t, p.ErpAttributes().FgType)
	assert.False(t, p.ErpAttributes().PrdPerDay.Valid)
	assert.Equal(t, "CHP0001", p.ErpAttributes().ChpItemCode, "nil fields unchanged")
	assert.True(t, domain.ErpAttributesPatch{}.IsEmpty())
}

func TestApplyErpAttributes_Invalid(t *testing.T) {
	bad := []domain.ErpAttributesPatch{
		{FgType: strp(strings.Repeat("T", 16))},
		{ChpItemCode: strp(strings.Repeat("C", 51))},
		{MsBatchItem: strp("MSB0000000013")}, // 13 > FLEX_12
		{MsBatchItem: strp("MSBé")},
		{ItemType: strp(strings.Repeat("I", 21))},
		{PrdPerDay: strp("abc")},
		{PrdPerDay: strp("-1")},
		{PrdPerDay: strp("1.123456")},
		{PrdPerDay: strp("1000000000000000")},
	}
	for i, patch := range bad {
		p := product(true, "AX", "X419T", "")
		changed, err := p.ApplyErpAttributes(patch, "alice", fixedAt)
		require.ErrorIs(t, err, domain.ErrInvalidErpAttributes, "case %d", i)
		assert.False(t, changed)
		assert.Equal(t, domain.ErpAttributes{}, p.ErpAttributes(), "case %d leaves state untouched", i)
	}
	ok := product(true, "AX", "", "")
	_, err := ok.ApplyErpAttributes(domain.ErpAttributesPatch{PrdPerDay: strp("999999999999999.99999")}, "a", fixedAt)
	require.NoError(t, err, "max NUMERIC(20,5)")

	inactive := product(false, "AX", "", "")
	_, err = inactive.ApplyErpAttributes(domain.ErpAttributesPatch{FgType: strp("Type 1")}, "a", fixedAt)
	require.ErrorIs(t, err, domain.ErrInactive)
}

func TestRestoreErpAttributes(t *testing.T) {
	p := product(true, "AX", "", "")
	a := domain.ErpAttributes{FgType: "Type 2"}
	p.RestoreErpAttributes(a)
	assert.Equal(t, a, p.ErpAttributes())
	assert.Equal(t, "seed", p.UpdatedBy())
}
