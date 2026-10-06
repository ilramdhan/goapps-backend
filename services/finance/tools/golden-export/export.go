package main

// export.go holds the testable core of the golden fixture exporter (plan
// P4-T2, design Part 2 §6.5): the fixed read-only SELECT, the DSN user check,
// row normalisation and the deterministic gzip CSV writer. Every Oracle
// statement goes through oracle.ReadOnlyQuerier.QueryRO, so it passes
// oracle.CheckReadOnly before it can reach the database.

import (
	"compress/gzip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"sort"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/mutugading/goapps-backend/services/finance/internal/infrastructure/oracle"
)

// DefaultOutPath is where P4-T3 (derive_golden_test.go) reads the fixture,
// relative to goapps-backend/services/finance.
const DefaultOutPath = "internal/domain/erpintegration/testdata/golden_legacy_std_202608.csv.gz"

// DefaultSchema is the owner of the legacy ERP tables.
const DefaultSchema = "MGTDAT"

// ExpectedRows is the DEV row count recorded by recon §3 on 2026-09-25.
const ExpectedRows = 9602

// minScale is the minimum number of decimal places written for numeric cells.
const minScale int32 = 5

var (
	// ErrUserNotAllowed is returned when the DSN user is not in the read-only list.
	ErrUserNotAllowed = errors.New("golden-export: DSN user is not in the read-only user list")
	// ErrIdentityMismatch is returned when the session USER differs from the DSN user.
	ErrIdentityMismatch = errors.New("golden-export: session USER differs from the DSN user")
	// ErrBadSchema is returned for a schema name that is not a plain identifier.
	ErrBadSchema = errors.New("golden-export: invalid schema name")

	schemaRe = regexp.MustCompile(`^[A-Z][A-Z0-9_$#]{0,29}$`)

	// deniedUsers may never be used, even when listed: they own or write
	// the ERP objects.
	deniedUsers = map[string]struct{}{
		"SYS": {}, "SYSTEM": {}, "MGTDAT": {}, "GOAPPS_IF": {},
	}
)

// kind says how a column is normalised.
type kind int

const (
	kindText kind = iota // trimmed text, NULL -> ""
	kindNum              // decimal text, NULL -> "", >= 5 dp, never float
	kindRaw              // verbatim (legacy FLEX text), NULL -> ""
)

// column is one fixture column: its CSV name, SELECT expression and kind.
type column struct {
	name string
	expr string
	kind kind
}

// num renders a NUMBER as plain decimal text with a fixed '.' separator.
func num(expr string) string {
	return "TO_CHAR(" + expr + ", 'TM9', 'NLS_NUMERIC_CHARACTERS=''.,''')"
}

// flex renders the legacy FLEX text twin: TO_CHAR(ROUND(NVL(x,0),5),'FM990D00000').
func flex(expr string) string {
	return "TO_CHAR(ROUND(NVL(" + expr + ", 0), 5), 'FM990D00000', 'NLS_NUMERIC_CHARACTERS=''.,''')"
}

// columns is the stable fixture column order (design §6.5). "ax_*" are the
// legacy AX row inputs, "rule_*" the MICVL / ITEMSELLPRIC lookups the legacy
// std-value insert procedure makes, "legacy_*" the stored derived row (the expected
// values) and "flex_*" Oracle's own FM990D00000 text of those values.
var columns = []column{
	{"item_code", "der.FG_ITEM_CODE", kindText},
	{"grade_code", "der.FG_ITEM_GRADE", kindText},
	{"shade_code", "der.FG_ITEM_SHADE", kindText},
	{"item_type", "der.FG_ITEM_TYPE", kindText},
	{"fg_type", "ax.FG_TYPE", kindText},
	{"row_fg_type", "der.FG_TYPE", kindText},
	{"prod_type", "CASE SUBSTR(der.FG_ITEM_CODE, 1, 3) WHEN 'POY' THEN 'POY' WHEN 'ITY' THEN 'ITY' ELSE 'PTY' END", kindText},
	{"grade_group", "g.GRADE_BL_SHORT_NAME", kindText},
	{"rule_basis", "m.MICVL_BASIS", kindText},
	{"rule_val_loss", num("m.MICVL_VAL_LOSS"), kindNum},
	{"rule_sell_price_raw", "sp.VSSV_FIELD_01", kindRaw},
	{"ax_chp_item_code", "ax.FG_CHP_ITEM_CODE", kindText},
	{"ax_chp_cost", num("ax.FG_CHP_COST"), kindNum},
	{"ax_chp_con_kg", num("ax.FG_CHP_CON_KG"), kindNum},
	{"ax_conv", num("ax.FG_CONVER_COST"), kindNum},
	{"ax_conv1", num("ax.FG_CONVER_COST1"), kindNum},
	{"ax_conv2", num("ax.FG_CONVER_COST2"), kindNum},
	{"ax_conv4", num("ax.FG_CONVER_COST4"), kindNum},
	{"ax_conv5", num("ax.FG_CONVER_COST5"), kindNum},
	{"ax_ax_conv_cost", num("ax.FG_AX_CONV_COST"), kindNum},
	{"ax_std", num("ax.FG_COST_PER_KG"), kindNum},
	{"legacy_basis", "der.FG_BASIS", kindText},
	{"legacy_selling", num("der.FG_SELLING_PRICE"), kindNum},
	{"legacy_val_loss", num("der.FG_VALUE_LOSS"), kindNum},
	{"legacy_std", num("der.FG_COST_PER_KG"), kindNum},
	{"legacy_conv", num("der.FG_CONVER_COST"), kindNum},
	{"legacy_ax_cost", num("der.FG_AX_COST"), kindNum},
	{"legacy_ax_conv_cost", num("der.FG_AX_CONV_COST"), kindNum},
	{"legacy_pvl", num("der.FG_PROD_VALUE_LOSS"), kindNum},
	{"legacy_conv1", num("der.FG_CONVER_COST1"), kindNum},
	{"legacy_conv2", num("der.FG_CONVER_COST2"), kindNum},
	{"legacy_conv4", num("der.FG_CONVER_COST4"), kindNum},
	{"legacy_conv5", num("der.FG_CONVER_COST5"), kindNum},
	{"flex_std", flex("der.FG_COST_PER_KG"), kindRaw},
	{"flex_conv", flex("der.FG_CONVER_COST"), kindRaw},
	{"flex_ax_cost", flex("der.FG_AX_COST"), kindRaw},
	{"flex_pvl", flex("der.FG_PROD_VALUE_LOSS"), kindRaw},
}

// Header returns the fixture CSV header in column order.
func Header() []string {
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = c.name
	}
	return out
}

// BuildQuery returns the fixed golden SELECT (recon §3 at row level). The
// join shape mirrors recon §3 exactly (inner join to the AX row on item +
// shade, left joins to the grade group, rule and selling price), so the row
// count matches the recorded 9,602. schema "" leaves the tables unqualified.
func BuildQuery(schema string) (string, error) {
	prefix := ""
	if schema != "" {
		if !schemaRe.MatchString(schema) {
			return "", fmt.Errorf("%w: %q", ErrBadSchema, schema)
		}
		prefix = schema + "."
	}
	exprs := make([]string, len(columns))
	for i, c := range columns {
		exprs[i] = "       " + c.expr + " " + strings.ToUpper(c.name)
	}
	var b strings.Builder
	b.WriteString("SELECT\n")
	b.WriteString(strings.Join(exprs, ",\n"))
	b.WriteString("\n  FROM " + prefix + "OT_STD_COST_PRODUCTS_MGT der")
	b.WriteString("\n  JOIN " + prefix + "OT_STD_COST_PRODUCTS_MGT ax")
	b.WriteString("\n    ON ax.FG_ITEM_CODE = der.FG_ITEM_CODE AND ax.FG_ITEM_SHADE = der.FG_ITEM_SHADE AND ax.FG_ITEM_GRADE = 'AX'")
	b.WriteString("\n  LEFT JOIN " + prefix + "OM_GRADE_CODE_1 g ON g.GRADE_CODE = der.FG_ITEM_GRADE")
	b.WriteString("\n  LEFT JOIN " + prefix + "MGT_ITEM_COST_VAL_LOSS m")
	b.WriteString("\n    ON m.MICVL_TYPE = ax.FG_TYPE AND m.MICVL_GRADE_GROUP = g.GRADE_BL_SHORT_NAME")
	b.WriteString("\n   AND m.MICVL_PROD_TYPE = CASE SUBSTR(der.FG_ITEM_CODE, 1, 3) WHEN 'POY' THEN 'POY' WHEN 'ITY' THEN 'ITY' ELSE 'PTY' END")
	b.WriteString("\n  LEFT JOIN " + prefix + "IM_VS_STATIC_VALUE sp ON sp.VSSV_VS_CODE = 'ITEMSELLPRIC' AND sp.VSSV_CODE = m.MICVL_BASIS")
	b.WriteString("\n WHERE der.FG_ITEM_GRADE <> 'AX'")
	b.WriteString("\n ORDER BY der.FG_ITEM_CODE, der.FG_ITEM_GRADE, der.FG_ITEM_SHADE")
	q := b.String()
	if err := oracle.CheckReadOnly(q); err != nil {
		return "", fmt.Errorf("golden query failed the read-only guard: %w", err)
	}
	return q, nil
}

// identityQuery returns the session user; it is compared with the DSN user.
const identityQuery = "SELECT USER FROM DUAL"

// DSNUser extracts the upper-cased user name from an oracle:// DSN.
func DSNUser(dsn string) (string, error) {
	u, err := url.Parse(strings.TrimSpace(dsn))
	if err != nil {
		return "", fmt.Errorf("parse DSN: invalid URL")
	}
	if !strings.EqualFold(u.Scheme, "oracle") || u.User == nil || u.User.Username() == "" {
		return "", errors.New("parse DSN: expected oracle://<user>:<password>@<host>:<port>/<service>")
	}
	return strings.ToUpper(strings.TrimSpace(u.User.Username())), nil
}

// CheckUser refuses a user that is denied or not in the comma-separated
// read-only allow list (case-insensitive). An empty list refuses everyone.
func CheckUser(user, allowList string) error {
	user = strings.ToUpper(strings.TrimSpace(user))
	if _, denied := deniedUsers[user]; denied {
		return fmt.Errorf("%w: %s is an owner/writer account", ErrUserNotAllowed, user)
	}
	for _, a := range strings.Split(allowList, ",") {
		if a = strings.ToUpper(strings.TrimSpace(a)); a != "" && a == user {
			return nil
		}
	}
	return fmt.Errorf("%w: %s", ErrUserNotAllowed, user)
}

// VerifyIdentity checks via a guarded SELECT that the session runs as want.
func VerifyIdentity(ctx context.Context, q oracle.ReadOnlyQuerier, want string) (err error) {
	rows, err := q.QueryRO(ctx, identityQuery)
	if err != nil {
		return fmt.Errorf("read session user: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	var got sql.NullString
	if rows.Next() {
		if err := rows.Scan(&got); err != nil {
			return fmt.Errorf("scan session user: %w", err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read session user: %w", err)
	}
	if !strings.EqualFold(strings.TrimSpace(got.String), want) {
		return fmt.Errorf("%w: session=%q dsn=%q", ErrIdentityMismatch, got.String, want)
	}
	return nil
}

// Summary describes an export for the ledger.
type Summary struct {
	Rows          int
	DuplicateKeys int            // extra rows sharing (item, grade, shade)
	WideCells     int            // numeric cells with more than 5 dp (kept exact)
	PerBasis      map[string]int // by rule_basis ("<none>" when NULL)
	PerMatch      map[string]int // "std_match" / "std_diff" vs legacy at 5 dp (informational)
}

// Fetch runs the golden SELECT and returns normalised, deterministically
// sorted rows plus a summary.
func Fetch(ctx context.Context, q oracle.ReadOnlyQuerier, schema string) (out [][]string, sum Summary, err error) {
	query, err := BuildQuery(schema)
	if err != nil {
		return nil, sum, err
	}
	rows, err := q.QueryRO(ctx, query)
	if err != nil {
		return nil, sum, fmt.Errorf("run golden query: %w", err)
	}
	defer func() {
		if cerr := rows.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close rows: %w", cerr)
		}
	}()
	for rows.Next() {
		rec, wide, serr := scanRow(rows)
		if serr != nil {
			return nil, sum, fmt.Errorf("row %d: %w", len(out)+1, serr)
		}
		sum.WideCells += wide
		out = append(out, rec)
	}
	if err := rows.Err(); err != nil {
		return nil, sum, fmt.Errorf("iterate golden query: %w", err)
	}
	sortRows(out)
	return out, summarize(out, sum.WideCells), nil
}

// scanRow scans one row in column order and normalises every cell.
func scanRow(rows oracle.Rows) ([]string, int, error) {
	cells := make([]sql.NullString, len(columns))
	dest := make([]any, len(columns))
	for i := range cells {
		dest[i] = &cells[i]
	}
	if err := rows.Scan(dest...); err != nil {
		return nil, 0, fmt.Errorf("scan: %w", err)
	}
	rec := make([]string, len(columns))
	wide := 0
	for i, c := range columns {
		v, w, err := normalize(c, cells[i])
		if err != nil {
			return nil, 0, err
		}
		rec[i] = v
		if w {
			wide++
		}
	}
	return rec, wide, nil
}

// normalize converts one scanned cell to its fixture text. wide reports a
// numeric value with more than 5 decimal places (written exactly, not cut).
func normalize(c column, cell sql.NullString) (s string, wide bool, err error) {
	if !cell.Valid {
		return "", false, nil
	}
	switch c.kind {
	case kindText:
		return strings.TrimSpace(cell.String), false, nil
	case kindRaw:
		return cell.String, false, nil
	case kindNum:
		return normalizeNum(c, cell.String)
	}
	return "", false, fmt.Errorf("column %s: unknown kind %d", c.name, c.kind)
}

// normalizeNum parses decimal text and formats it with FormatNum.
func normalizeNum(c column, raw string) (s string, wide bool, err error) {
	t := strings.TrimSpace(raw)
	if t == "" {
		return "", false, nil
	}
	d, perr := decimal.NewFromString(t)
	if perr != nil {
		return "", false, fmt.Errorf("column %s: invalid number %q", c.name, raw)
	}
	s, wide = FormatNum(d)
	return s, wide, nil
}

// FormatNum writes d with at least 5 decimal places and never drops digits.
func FormatNum(d decimal.Decimal) (string, bool) {
	// A value exact at 5 dp (including an over-long zero tail) is not "wide".
	if d.Equal(d.Truncate(minScale)) {
		return d.StringFixed(minScale), false
	}
	return d.String(), true
}

// sortRows orders rows by every column in byte order (independent of the
// Oracle NLS_SORT and of duplicate-row order).
func sortRows(rows [][]string) {
	sort.SliceStable(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		for k := range a {
			if a[k] != b[k] {
				return a[k] < b[k]
			}
		}
		return false
	})
}

// colIndex returns the index of the named column (it must exist).
func colIndex(name string) int {
	for i, c := range columns {
		if c.name == name {
			return i
		}
	}
	panic("golden-export: unknown column " + name)
}

// summarize computes the ledger summary of sorted rows.
func summarize(rows [][]string, wide int) Summary {
	iItem, iGrade, iShade := colIndex("item_code"), colIndex("grade_code"), colIndex("shade_code")
	iBasis := colIndex("rule_basis")
	sum := Summary{Rows: len(rows), WideCells: wide, PerBasis: map[string]int{}, PerMatch: map[string]int{}}
	seen := map[string]struct{}{}
	for _, r := range rows {
		key := r[iItem] + "|" + r[iGrade] + "|" + r[iShade]
		if _, dup := seen[key]; dup {
			sum.DuplicateKeys++
		}
		seen[key] = struct{}{}
		basis := r[iBasis]
		if basis == "" {
			basis = "<none>"
		}
		sum.PerBasis[basis]++
		sum.PerMatch[legacyMatch(r)]++
	}
	return sum
}

// legacyMatch re-applies the recon §3 check (legacy std vs the expected std
// from the AX row and the rule) at 5 dp. It is informational only: the real
// assertion is the Go Derive in P4-T3.
func legacyMatch(r []string) string {
	get := func(name string) (decimal.Decimal, bool) {
		s := r[colIndex(name)]
		if s == "" {
			return decimal.Zero, false
		}
		d, err := decimal.NewFromString(s)
		return d, err == nil
	}
	std, ok := get("legacy_std")
	if !ok {
		return "std_null"
	}
	loss, _ := get("rule_val_loss") // NVL(loss, 0) as in recon §3
	var exp decimal.Decimal
	if r[colIndex("rule_basis")] == "COST" {
		chp, _ := get("ax_chp_cost")
		kg, _ := get("ax_chp_con_kg")
		conv, _ := get("ax_conv")
		exp = chp.Mul(kg).Add(conv.Sub(loss).Round(minScale)).Round(minScale)
	} else {
		sell, err := decimal.NewFromString(strings.TrimSpace(r[colIndex("rule_sell_price_raw")]))
		if err != nil {
			sell = decimal.Zero
		}
		exp = sell.Sub(loss).Round(minScale)
	}
	if std.Round(minScale).Equal(exp) {
		return "std_match"
	}
	return "std_diff"
}

// WriteFixture writes header + rows as gzip CSV with a zero gzip mod-time and
// no name, so identical data always yields identical bytes. It returns the
// lower-hex sha256 of the compressed bytes.
func WriteFixture(w io.Writer, rows [][]string) (string, error) {
	h := sha256.New()
	gz, err := gzip.NewWriterLevel(io.MultiWriter(w, h), gzip.BestCompression)
	if err != nil {
		return "", fmt.Errorf("gzip: %w", err)
	}
	cw := csv.NewWriter(gz)
	if err := cw.Write(Header()); err != nil {
		return "", fmt.Errorf("write header: %w", err)
	}
	if err := cw.WriteAll(rows); err != nil {
		return "", fmt.Errorf("write rows: %w", err)
	}
	if err := gz.Close(); err != nil {
		return "", fmt.Errorf("close gzip: %w", err)
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
