// Package mbactivesync plans and applies the synchronization of MB (masterbatch)
// active state with the Finance "mb-active" file. This package contains no SQL.
package mbactivesync

import (
	"regexp"
	"strings"
)

var (
	familyCutRe    = regexp.MustCompile(`[ -]*(D-|ND-|N-D-|-B$|-C$).*$`)
	familySuffixRe = regexp.MustCompile(`[ -]+(CS|N|ND|NEW|REV[0-9]*)$`)
	nonAlnumRe     = regexp.MustCompile(`[^A-Z0-9]`)
)

// NormShadeLoose is the matching key for shades (port of pg_temp.ns): UPPER(TRIM),
// with '-' and ' ' removed. Empty result means "no shade". "Y145T" == "Y-145-T".
func NormShadeLoose(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, " ", "")
	return s
}

// NormShadeExact is UPPER(TRIM), identical to mbsource.NormalizeShade; used where
// the DB uniqueness/validation rules compare shades exactly.
func NormShadeExact(s string) string {
	return strings.ToUpper(strings.TrimSpace(s))
}

// FamilyKey is the recipe "family" key of an MB costing name (port of pg_temp.nk
// in L-10): strips version/variant tails (D-, ND-, N-D-, trailing -B/-C, then
// trailing CS/N/ND/NEW/REVn) and keeps only A-Z0-9. Empty result means no key.
func FamilyKey(name string) string {
	x := strings.ToUpper(strings.TrimSpace(name))
	x = familyCutRe.ReplaceAllString(x, "")
	x = familySuffixRe.ReplaceAllString(x, "")
	return nonAlnumRe.ReplaceAllString(x, "")
}
