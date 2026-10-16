package lookupsync

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/domain/parameter"
)

// childChange mirrors computeLookupFillPatches (frontend lookup-fill.ts) for
// one applicable child: a numeric fill SETs the numeric value, else a text
// fill SETs the text, else the child is CLEARed. It reports a change only when
// the stored value differs: numerics compare at NUMERIC(20,6) scale, and a
// CLEAR is a change only when a stored row exists.
//
// An empty text fill is treated as CLEAR: the form never persists an empty
// value, and cpp_one_value_chk forbids storing one as "set".
func childChange(child *parameter.Parameter, res FillResult, state ChildState) (ValueChange, bool) {
	code := child.Code().String()
	c := ValueChange{
		ParamID:   child.ID(),
		ParamCode: code,
		ParamName: child.Name(),
		OldValue:  storedDisplay(state.Stored),
	}
	if num, ok := res.Numeric[code]; ok {
		d := normalizeNumeric(decimal.NewFromFloat(num))
		if st := state.Stored; st != nil && st.Numeric != nil && normalizeNumeric(*st.Numeric).Equal(d) {
			return ValueChange{}, false
		}
		c.Kind, c.NewNumeric, c.NewValue = ChangeSet, &d, d.String()
		return c, true
	}
	if txt, ok := res.Text[code]; ok && txt != "" {
		if st := state.Stored; st != nil && st.Text != nil && *st.Text == txt {
			return ValueChange{}, false
		}
		t := txt
		c.Kind, c.NewText, c.NewValue = ChangeSet, &t, txt
		return c, true
	}
	if state.Stored == nil {
		return ValueChange{}, false
	}
	c.Kind = ChangeClear
	return c, true
}

// normalizeNumeric rounds to the cpp_value_numeric scale.
func normalizeNumeric(d decimal.Decimal) decimal.Decimal {
	return d.Round(numericScale)
}

// storedDisplay renders a stored value as text ("" when absent).
func storedDisplay(v *StoredValue) string {
	switch {
	case v == nil:
		return ""
	case v.Numeric != nil:
		return normalizeNumeric(*v.Numeric).String()
	case v.Text != nil:
		return *v.Text
	case v.Flag != nil:
		return strconv.FormatBool(*v.Flag)
	default:
		return ""
	}
}

// changeDigest is the canonical line of one applicable change in the plan
// fingerprint: "<product_sys_id>|<param_id>|<kind>|<new value>". Numerics use
// the normalized decimal string so 1.5 and 1.500000 hash identically.
func changeDigest(productSysID int64, c ValueChange) []byte {
	var b strings.Builder
	b.WriteString(strconv.FormatInt(productSysID, 10))
	b.WriteByte('|')
	b.WriteString(c.ParamID.String())
	b.WriteByte('|')
	b.WriteString(c.Kind)
	b.WriteByte('|')
	switch {
	case c.NewNumeric != nil:
		b.WriteString("N:")
		b.WriteString(normalizeNumeric(*c.NewNumeric).String())
	case c.NewText != nil:
		b.WriteString("T:")
		b.WriteString(*c.NewText)
	}
	return []byte(b.String())
}

// fingerprint is the hex sha256 of the sorted change digests, each followed by
// '\n'. It is independent of reader order and page size; an empty plan hashes
// the empty input. Skipped items and old values do not contribute, so Apply's
// drift check fires only when what would be WRITTEN changes.
func fingerprint(digests [][]byte) string {
	sorted := make([]string, len(digests))
	for i, d := range digests {
		sorted[i] = string(d)
	}
	sort.Strings(sorted)
	h := sha256.New()
	for _, d := range sorted {
		h.Write([]byte(d))
		h.Write([]byte{'\n'})
	}
	return hex.EncodeToString(h.Sum(nil))
}
