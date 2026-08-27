package postgres

import (
	"strings"
	"testing"
)

// TestRecipeFullQuery_ExcludesRejectedByParameter pins §11 item 140's fix on the second
// leaked export path: recipeFullQuery must carry a bound-parameter predicate that lets the
// caller exclude mbh_entry_status = 'REJECTED' rows, and it must NEVER hardcode the
// "REJECTED" literal directly into the SQL text — the value must travel as $6 (see
// ListRecipeFullRows' args, which pass domainmbhead.StatusRejected positionally).
//
// This is a structural, string-level test: the package has no SQL mock or test database
// (see recipeFullQuery's own doc comment and the precedent in
// mb_head_list_cost_product_filter_internal_test.go), so it cannot execute the query —
// only pin its shape.
func TestRecipeFullQuery_ExcludesRejectedByParameter(t *testing.T) {
	if !strings.Contains(recipeFullQuery, "$5::boolean OR h.mbh_entry_status != $6") {
		t.Fatalf("expected a parameterized include-rejected predicate on h.mbh_entry_status "+
			"using $5 (flag) and $6 (status value), got query:\n%s", recipeFullQuery)
	}
	if strings.Contains(recipeFullQuery, "'REJECTED'") || strings.Contains(recipeFullQuery, "\"REJECTED\"") {
		t.Fatalf("the REJECTED status literal must never be interpolated into the SQL text, "+
			"it must travel as bound parameter $6: query:\n%s", recipeFullQuery)
	}
}

// TestRecipeFullQuery_ExistingPredicatesUnchanged pins that adding the new $5/$6 predicate
// did not disturb the pre-existing $1..$4 predicates this query already relied on.
func TestRecipeFullQuery_ExistingPredicatesUnchanged(t *testing.T) {
	for _, want := range []string{
		"$1::boolean IS NULL OR h.mbh_is_active = $1::boolean",
		"$4 = '' OR h.mbh_check_status_calc = $4",
	} {
		if !strings.Contains(recipeFullQuery, want) {
			t.Errorf("expected recipeFullQuery to still contain %q", want)
		}
	}
}
