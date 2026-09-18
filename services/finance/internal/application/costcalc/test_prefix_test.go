package costcalc

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"testing"
)

// uniqueCodePrefix returns a short, collision-free prefix for the fixture codes a
// DB-backed suite inserts.
//
// It replaces fmt.Sprintf("XX%d", time.Now().UnixNano()%10000), which was not
// unique in either direction. On darwin/arm64 the monotonic clock only advances
// UnixNano in whole microseconds, so the low four digits are effectively always
// a multiple of 1000 — the expression yields one of ten values, not ten
// thousand. Two suites in the same `go test` binary therefore collide with each
// other, and a suite whose TearDownSuite did not complete collides with its own
// leftovers on the next run. Both showed up as
// uk_cost_product_master_code / idx_mst_parameter_code violations during setup,
// which reads as a product bug rather than as test residue.
//
// The result is kind + 6 hex chars. Keep kind at two characters: the longest
// consumer is cost_product_master.cpm_product_code at varchar(20), and the
// suites append suffixes of up to nine characters ("-UPMBNONE").
func uniqueCodePrefix(t *testing.T, kind string) string {
	t.Helper()
	var b [3]byte
	if _, err := rand.Read(b[:]); err != nil {
		t.Fatalf("generate unique code prefix: %v", err)
	}
	return fmt.Sprintf("%s%s", kind, hex.EncodeToString(b[:]))
}
