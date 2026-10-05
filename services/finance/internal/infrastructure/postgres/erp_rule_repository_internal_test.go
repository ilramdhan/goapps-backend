package postgres

import (
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/stretchr/testify/assert"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/erprule"
)

func TestEscapeLikePattern(t *testing.T) {
	cases := map[string]string{
		"abc":    "abc",
		"a_b":    `a\_b`,
		"100%":   `100\%`,
		`back\s`: `back\\s`,
		`%_\`:    `\%\_\\`,
	}
	for in, want := range cases {
		assert.Equal(t, want, escapeLikePattern(in), in)
	}
}

func TestAppendPage(t *testing.T) {
	q, args := appendPage("SELECT 1", []any{"x"}, 3, 10)
	assert.Equal(t, "SELECT 1 LIMIT $2 OFFSET $3", q)
	assert.Equal(t, []any{"x", 10, 20}, args)

	q, args = appendPage("SELECT 1", nil, 0, 5)
	assert.Equal(t, "SELECT 1 LIMIT $1 OFFSET $2", q)
	assert.Equal(t, []any{5, 0}, args, "page < 1 is page 1")

	q, args = appendPage("SELECT 1", []any{"x"}, 2, 0)
	assert.Equal(t, "SELECT 1", q, "pageSize <= 0 is unlimited")
	assert.Equal(t, []any{"x"}, args)
}

func TestVallossRuleWhere(t *testing.T) {
	where, args := vallossRuleWhere(erprule.VallossRuleFilter{})
	assert.Equal(t, " WHERE cevr_is_active", where)
	assert.Empty(t, args)

	where, args = vallossRuleWhere(erprule.VallossRuleFilter{IncludeInactive: true})
	assert.Empty(t, where)
	assert.Empty(t, args)

	where, args = vallossRuleWhere(erprule.VallossRuleFilter{
		FgType: "Type 1", ProdType: erprule.ProdTypePTY, GradeGroup: erprule.GradeGroupBC, Basis: erprule.BasisCost,
	})
	assert.Equal(t, " WHERE cevr_fg_type = $1 AND cevr_prod_type = $2 AND cevr_grade_group = $3 AND cevr_basis = $4 AND cevr_is_active", where)
	assert.Equal(t, []any{"Type 1", "PTY", "BC", "COST"}, args)
}

func TestIsVallossKeyViolation(t *testing.T) {
	assert.False(t, isVallossKeyViolation(assert.AnError))
	assert.True(t, isVallossKeyViolation(&pgconn.PgError{Code: "23505", ConstraintName: "uk_cevr_key"}))
	assert.True(t, isVallossKeyViolation(&pgconn.PgError{Code: "23505"}), "no constraint name: only unique index")
	assert.False(t, isVallossKeyViolation(&pgconn.PgError{Code: "23505", ConstraintName: "cst_erp_valloss_rule_pkey"}))
	assert.False(t, isVallossKeyViolation(&pgconn.PgError{Code: "23514", ConstraintName: "chk_cevr_val_loss"}))
}
