package lookupsync

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestBoxBobbinSwitchColumnsMatchSource keeps boxBobbinCostColumns honest.
// fillFromBoxBobbinCost resolves columns with a literal switch, so unlike the
// reader maps it cannot be enumerated at runtime — boxBobbinCostColumns is a copy, and
// a copy is exactly the kind of hand-maintained duplicate R30 is about. This
// reads resolver_box_bobbin.go and asserts the copy still matches the switch.
func TestBoxBobbinSwitchColumnsMatchSource(t *testing.T) {
	raw, err := os.ReadFile("resolver_box_bobbin.go")
	require.NoError(t, err, "reading resolver_box_bobbin.go")

	src := string(raw)
	start := strings.Index(src, "func (r *Resolver) fillFromBoxBobbinCost(")
	require.NotEqual(t, -1, start, "fillFromBoxBobbinCost not found — was it renamed? Update this test.")
	end := strings.Index(src[start:], "\nfunc ")
	require.NotEqual(t, -1, end, "could not delimit fillFromBoxBobbinCost body")
	body := src[start : start+end]

	caseRe := regexp.MustCompile(`(?m)^\s*case\s+"([a-z0-9_]+)":`)
	found := caseRe.FindAllStringSubmatch(body, -1)
	require.NotEmpty(t, found, "no `case \"...\":` labels found in fillFromBoxBobbinCost — the switch shape "+
		"changed and this check would otherwise pass vacuously")

	inSource := make([]string, 0, len(found))
	for _, m := range found {
		inSource = append(inSource, m[1])
	}
	sort.Strings(inSource)
	copied := append([]string(nil), boxBobbinCostColumns...)
	sort.Strings(copied)

	assert.Equal(t, inSource, copied,
		"boxBobbinCostColumns is out of sync with the switch in fillFromBoxBobbinCost. Update the slice in "+
			"readers.go to match the case labels, otherwise the BOX_BOBBIN_COST side of the R30 "+
			"drift check — and the startup divergence check that shares this inventory — is wrong.")
}
