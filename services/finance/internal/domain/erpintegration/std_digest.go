package erpintegration

// std_digest.go defines the batch control digest of the derived std rows
// (plan-05 P4-T4 step 3; design Part 1 §5.2, Part 3 recon §4a): row count,
// Σ std / conv / pvl over the OK rows (ControlTotals) plus the md5 of the
// canonical text of those rows. The same canonical text is produced in SQL by
// the postgres std-cost repository (StdDigest), so the digest can be
// recomputed from the stored rows and compared with the in-memory one; the
// Oracle recon later rebuilds it from the GSC_* columns.
//
// Canonical text (version 1):
//   - only rows with status OK (the pushed set);
//   - ordered by (item, grade, shade) byte-wise (Go string order = PG
//     COLLATE "C");
//   - one line per row, lines joined by "\n" (no trailing newline);
//   - fields joined by "|": item, grade, shade, source, basis, fg_type,
//     chp_cost, ax_conv_cost, conv_cost, selling_price, value_loss, ax_cost,
//     std_cost, prod_value_loss;
//   - decimals as fixed 5-dp text (PG NUMERIC(20,5)::text), NULL as "".
//
// The empty set hashes to md5("").

import (
	"crypto/md5" //nolint:gosec // G501: a recon checksum, not a security primitive
	"encoding/hex"
	"sort"
	"strings"

	"github.com/shopspring/decimal"
)

// StdDigestVersion is the version of the canonical std-row text.
const StdDigestVersion = 1

// StdDigest is the control digest of a batch's std rows.
type StdDigest struct {
	Totals  ControlTotals
	RowsMD5 string // lower-hex md5 of the canonical OK-row text
}

// Equal reports whether both digests are identical.
func (d StdDigest) Equal(o StdDigest) bool {
	return d.RowsMD5 == o.RowsMD5 && d.Totals.Equal(o.Totals)
}

// ComputeStdDigest computes the digest of rows in memory. The result does not
// depend on the order of rows.
func ComputeStdDigest(rows []StdRow) (StdDigest, error) {
	ok := make([]StdRow, 0, len(rows))
	sumStd, sumConv, sumPvl := decimal.Zero, decimal.Zero, decimal.Zero
	for _, r := range rows {
		if r.Status != DeriveOK {
			continue
		}
		ok = append(ok, r)
		sumStd = sumStd.Add(nullOrZero(r.StdCost))
		sumConv = sumConv.Add(nullOrZero(r.ConvCost))
		sumPvl = sumPvl.Add(nullOrZero(r.ProdValLoss))
	}
	sort.Slice(ok, func(i, j int) bool { return lessErpKey(ok[i].Key, ok[j].Key) })
	lines := make([]string, len(ok))
	for i, r := range ok {
		lines[i] = CanonicalStdLine(r)
	}
	totals, err := NewControlTotals(int64(len(ok)), sumStd, sumConv, sumPvl)
	if err != nil {
		return StdDigest{}, err
	}
	return StdDigest{Totals: totals, RowsMD5: MD5Hex(strings.Join(lines, "\n"))}, nil
}

// CanonicalStdLine renders one row in the canonical digest form.
func CanonicalStdLine(r StdRow) string {
	f := []string{
		r.Key.ItemCode, r.Key.GradeCode, r.Key.ShadeCode,
		string(r.Source), string(r.Basis), r.FgType,
		fixed5(r.ChpCost), fixed5(r.AxConvCost), fixed5(r.ConvCost),
		fixed5(r.SellingPrice), fixed5(r.ValueLoss), fixed5(r.AxCost),
		fixed5(r.StdCost), fixed5(r.ProdValLoss),
	}
	return strings.Join(f, "|")
}

// MD5Hex is the lower-hex md5 of s (PG md5(text)).
func MD5Hex(s string) string {
	sum := md5.Sum([]byte(s)) //nolint:gosec // G401: recon checksum only
	return hex.EncodeToString(sum[:])
}

func lessErpKey(a, b ErpKey) bool {
	if a.ItemCode != b.ItemCode {
		return a.ItemCode < b.ItemCode
	}
	if a.GradeCode != b.GradeCode {
		return a.GradeCode < b.GradeCode
	}
	return a.ShadeCode < b.ShadeCode
}

func fixed5(d decimal.NullDecimal) string {
	if !d.Valid {
		return ""
	}
	return d.Decimal.StringFixed(ScaleR5)
}

func nullOrZero(d decimal.NullDecimal) decimal.Decimal {
	if !d.Valid {
		return decimal.Zero
	}
	return d.Decimal
}
