package oracle

import (
	"database/sql"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParsePackQty(t *testing.T) {
	cases := []struct {
		name string
		in   sql.NullString
		want string
	}{
		{"fraction", sql.NullString{String: "1234.567891", Valid: true}, "1234.567891"},
		{"leading dot (TM9 < 1)", sql.NullString{String: ".25", Valid: true}, "0.25"},
		{"negative", sql.NullString{String: "-3.5", Valid: true}, "-3.5"},
		{"zero", sql.NullString{String: "0", Valid: true}, "0"},
		{"null", sql.NullString{}, "0"},
		{"empty", sql.NullString{String: "", Valid: true}, "0"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parsePackQty(c.in)
			require.NoError(t, err)
			assert.Equal(t, c.want, got.String())
		})
	}
}

func TestParsePackQty_Invalid(t *testing.T) {
	_, err := parsePackQty(sql.NullString{String: "1,5", Valid: true})
	assert.Error(t, err)
}

func TestPackProdDailyQuery_HasWindowFilter(t *testing.T) {
	assert.Contains(t, packProdDailyQuery, "WHERE PACK_DATE >= TO_DATE(:1,'YYYY-MM-DD') AND PACK_DATE < TO_DATE(:2,'YYYY-MM-DD') + 1")
	assert.Contains(t, packProdDailyQuery, "FROM ASPAK.VIEW_BI_PACK_PROD_DAILY")
}

func TestPackProdDailyQuery_ColumnCount(t *testing.T) {
	sel := packProdDailyQuery[len("SELECT"):strings.Index(packProdDailyQuery, "FROM ")]
	// TO_CHAR(x,'TM9','NLS_NUMERIC_CHARACTERS=''.,''') holds 3 commas: 2 argument separators + 1 inside the literal.
	items := strings.Count(sel, ",") + 1 - 3*strings.Count(sel, "TO_CHAR(")
	assert.Equal(t, packProdColumns, items)
}
