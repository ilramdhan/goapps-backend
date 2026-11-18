package mblegacyfill

import (
	"math/big"
	"strings"
)

// Normalize trims a raw value; empty means NULL/”.
func Normalize(v string) string { return strings.TrimSpace(v) }

// IsEmpty reports whether v is NULL/” after trimming.
func IsEmpty(v string) bool { return Normalize(v) == "" }

// Equal compares two values by kind: empty == empty, numerics as numbers
// (2.6 == 2.60), text trimmed and case-sensitive. Unparseable numerics fall back to text.
func Equal(kind Kind, a, b string) bool {
	a, b = Normalize(a), Normalize(b)
	if a == "" || b == "" {
		return a == b
	}
	if kind == KindNum || kind == KindInt {
		x, okx := new(big.Rat).SetString(a)
		y, oky := new(big.Rat).SetString(b)
		if okx && oky {
			return x.Cmp(y) == 0
		}
	}
	return a == b
}

func upper(s string) string { return strings.ToUpper(s) }
