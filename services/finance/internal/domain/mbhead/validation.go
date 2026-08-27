// Package mbhead provides domain logic for Melange Batch Head (MEL product type) management.
package mbhead

import "fmt"

// RequiredRecipeField names one mandatory MB recipe field and how to read it off
// an entity. Kept as data so the rule set is inspectable and testable field by
// field instead of hiding behind a wall of if-statements.
type RequiredRecipeField struct {
	// Label is the user-facing form label, used to build the error message.
	Label string
	// Column is the database column the label corresponds to.
	Column string
	// isEmpty reports whether the field is unset on the given entity.
	isEmpty func(e *Entity) bool
}

// requiredRecipeFields is the mandatory-field list for the MB recipe form.
//
// TODO(U-A): the design (§4.1) and plan (§5 P5) both say "11 required fields",
// but the user's own enumerated list (plan §10, item B9) contains exactly TEN
// names: mb name, dev no, vs number, shade code, shade name, poy denier, poy
// filament, cross section, ldr %, final product. The eleventh field is NOT
// written down anywhere. The ten written fields are implemented here; the
// discrepancy is an open user decision and ⛔ must not be guessed at.
var requiredRecipeFields = []RequiredRecipeField{
	{"MB Name", "mbh_mgt_name", func(e *Entity) bool { return isBlankPtr(e.mgtName) }},
	{"Dev No", "mbh_dev_code", func(e *Entity) bool { return e.devCode == "" }},
	{"VS Number", "mbh_vs_number", func(e *Entity) bool { return isBlankPtr(e.vsNumber) }},
	{"Shade Code", "mbh_shade_code", func(e *Entity) bool { return e.shadeCode == "" }},
	{"Shade Name", "mbh_shade_name", func(e *Entity) bool { return e.shadeName == "" }},
	{"POY Denier", "mbh_denier", func(e *Entity) bool { return e.denier == nil }},
	{"POY Filament", "mbh_filament", func(e *Entity) bool { return e.filament == nil }},
	{"Cross Section", "mbh_cross_section", func(e *Entity) bool { return e.crossSection == "" }},
	{"LDR %", "mbh_ldr_prsn", func(e *Entity) bool { return e.mbhLdrPrsn == nil }},
	{"Final Product", "mbh_final_product", func(e *Entity) bool { return isBlankPtr(e.mbhFinalProduct) }},
}

// RequiredRecipeFields returns the mandatory-field rule set. Exposed so tests and
// callers can enumerate the rules rather than restate them.
func RequiredRecipeFields() []RequiredRecipeField {
	return append([]RequiredRecipeField(nil), requiredRecipeFields...)
}

// IsEmptyOn reports whether this required field is unset on the given entity.
func (f RequiredRecipeField) IsEmptyOn(e *Entity) bool { return f.isEmpty(e) }

// ValidateRecipeFields checks the mandatory MB recipe fields and returns an error
// wrapping ErrRecipeFieldRequired naming the first missing field.
//
// 🔴 NOT WIRED UP ON PURPOSE. As of 2026-08-22 this method is called from
// NOWHERE. Enforcing "required" on CREATE is blocked behind P4: the backfill of
// mbh_cross_section must land first (plan §5 P4), because 573 legacy heads still
// have it NULL (plan K6) and would be rejected by their own edit form. The rule
// set is written and tested now so that P4 only has to flip the call on.
//
// It is likewise never called from Reconstruct — legacy rows must load unvalidated
// (K8), which is why validation lives in an explicit method rather than in the
// hydration path.
func (e *Entity) ValidateRecipeFields() error {
	for _, f := range requiredRecipeFields {
		if f.isEmpty(e) {
			return fmt.Errorf("%w: %s (%s)", ErrRecipeFieldRequired, f.Label, f.Column)
		}
	}
	return nil
}

// isBlankPtr reports whether a string pointer is nil or points at an empty string.
func isBlankPtr(s *string) bool { return s == nil || *s == "" }
