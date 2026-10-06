package oracle

import (
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// ErrNonSelectRejected is returned by the read-only guard when a SQL text is
// not a single plain SELECT/WITH query (design §3.4, safety model §S).
var ErrNonSelectRejected = errors.New("oracle: non-SELECT statement rejected by read-only guard")

// readerRejectedTotal counts statements refused by the read-only guard.
var readerRejectedTotal = promauto.NewCounterVec(prometheus.CounterOpts{
	Name: "erp_oracle_reader_rejected_total",
	Help: "Number of SQL statements rejected by the ERP Oracle read-only guard.",
}, []string{"reason"})

// Guard rejection reasons (also used as the metric label).
const (
	rejectEmpty        = "empty"
	rejectUnterminated = "unterminated"
	rejectSemicolon    = "semicolon"
	rejectFirstToken   = "first_token"
	rejectForbidden    = "forbidden_token"
)

// forbiddenTokens are keywords that may never appear (outside comments and
// literals) in a statement sent through the read-only connection.
var forbiddenTokens = map[string]struct{}{
	"INSERT": {}, "UPDATE": {}, "DELETE": {}, "MERGE": {}, "DROP": {},
	"TRUNCATE": {}, "ALTER": {}, "CREATE": {}, "GRANT": {}, "REVOKE": {},
	"RENAME": {}, "COMMENT": {}, "LOCK": {}, "BEGIN": {}, "DECLARE": {},
	"CALL": {}, "EXEC": {}, "EXECUTE": {}, "COMMIT": {}, "ROLLBACK": {},
	"NOWAIT": {}, "SAVEPOINT": {}, "UPSERT": {},
}

// CheckReadOnly validates that query is a single SELECT or WITH statement.
// Comments and literals are stripped first; the remaining text must start
// with SELECT or WITH, contain no ';' and none of the forbidden keywords
// (this also rejects SELECT ... FOR UPDATE and NOWAIT). It returns an error
// wrapping ErrNonSelectRejected on refusal.
func CheckReadOnly(query string) error {
	reason := readOnlyViolation(query)
	if reason == "" {
		return nil
	}
	readerRejectedTotal.WithLabelValues(reason).Inc()
	return fmt.Errorf("%w (%s)", ErrNonSelectRejected, reason)
}

// readOnlyViolation returns the rejection reason, or "" when query is allowed.
func readOnlyViolation(query string) string {
	stripped, ok := stripCommentsAndLiterals(query)
	if !ok {
		return rejectUnterminated
	}
	if strings.ContainsRune(stripped, ';') {
		return rejectSemicolon
	}
	tokens := sqlWords(stripped)
	if len(tokens) == 0 {
		return rejectEmpty
	}
	if tokens[0] != "SELECT" && tokens[0] != "WITH" {
		return rejectFirstToken
	}
	for _, tok := range tokens {
		if _, bad := forbiddenTokens[tok]; bad {
			return rejectForbidden
		}
	}
	return ""
}

// firstSQLWord returns the first upper-cased keyword of query after comments
// and literals are stripped ("" when none). Exposed for the fuzz property.
func firstSQLWord(query string) string {
	stripped, ok := stripCommentsAndLiterals(query)
	if !ok {
		return ""
	}
	words := sqlWords(stripped)
	if len(words) == 0 {
		return ""
	}
	return words[0]
}

// stripCommentsAndLiterals replaces comments, string literals (including
// Oracle q'..' literals) and quoted identifiers with a single space. It
// returns ok=false when a comment or literal is unterminated.
func stripCommentsAndLiterals(query string) (string, bool) {
	src := []rune(query)
	var out strings.Builder
	out.Grow(len(query))
	for i := 0; i < len(src); {
		next, skipped, ok := skipNonCode(src, i)
		if !ok {
			return "", false
		}
		if skipped {
			out.WriteRune(' ')
			i = next
			continue
		}
		out.WriteRune(src[i])
		i++
	}
	return out.String(), true
}

// skipNonCode checks whether a comment, literal or quoted identifier starts at
// src[i]. When one does it returns the index just past it and skipped=true.
func skipNonCode(src []rune, i int) (next int, skipped, ok bool) {
	c := src[i]
	switch {
	case c == '-' && i+1 < len(src) && src[i+1] == '-':
		return skipLineComment(src, i+2)
	case c == '/' && i+1 < len(src) && src[i+1] == '*':
		return skipBlockComment(src, i+2)
	case isQQuoteStart(src, i):
		return skipQQuote(src, i)
	case c == '\'':
		return skipQuoted(src, i+1, '\'')
	case c == '"':
		return skipQuoted(src, i+1, '"')
	}
	return i, false, true
}

// skipLineComment skips a "--" comment whose body starts at src[start], up to
// (not including) the next line break.
func skipLineComment(src []rune, start int) (int, bool, bool) {
	j := start
	for j < len(src) && src[j] != '\n' && src[j] != '\r' {
		j++
	}
	return j, true, true
}

// skipBlockComment skips a "/* ... */" comment whose body starts at
// src[start]. An unterminated comment reports ok=false.
func skipBlockComment(src []rune, start int) (int, bool, bool) {
	for j := start; j+1 < len(src); j++ {
		if src[j] == '*' && src[j+1] == '/' {
			return j + 2, true, true
		}
	}
	return 0, false, false
}

// isQQuoteStart reports whether an Oracle alternative-quote literal
// (q'X...X', nq'X...X') starts at src[i].
func isQQuoteStart(src []rune, i int) bool {
	c := unicode.ToUpper(src[i])
	if c != 'Q' || i+2 >= len(src) || src[i+1] != '\'' {
		return false
	}
	if i > 0 {
		prev := unicode.ToUpper(src[i-1])
		if prev != 'N' && isWordRune(src[i-1]) {
			return false
		}
		if prev == 'N' && i > 1 && isWordRune(src[i-2]) {
			return false
		}
	}
	return true
}

// skipQQuote skips a q'X...X' literal starting at src[i] (the 'q').
func skipQQuote(src []rune, i int) (int, bool, bool) {
	open := src[i+2]
	closeRune := open
	switch open {
	case '[':
		closeRune = ']'
	case '{':
		closeRune = '}'
	case '(':
		closeRune = ')'
	case '<':
		closeRune = '>'
	}
	for j := i + 3; j+1 < len(src); j++ {
		if src[j] == closeRune && src[j+1] == '\'' {
			return j + 2, true, true
		}
	}
	return 0, false, false
}

// skipQuoted skips a quoted section whose body starts at src[start]; a doubled
// quote character is an escaped quote.
func skipQuoted(src []rune, start int, quote rune) (int, bool, bool) {
	for j := start; j < len(src); j++ {
		if src[j] != quote {
			continue
		}
		if j+1 < len(src) && src[j+1] == quote {
			j++
			continue
		}
		return j + 1, true, true
	}
	return 0, false, false
}

// isWordRune reports whether r can be part of an SQL keyword or identifier.
func isWordRune(r rune) bool {
	return r == '_' || r == '$' || r == '#' || unicode.IsLetter(r) || unicode.IsDigit(r)
}

// sqlWords splits stripped SQL into upper-cased words. Every non-word rune
// (including all unicode whitespace) is a separator.
func sqlWords(stripped string) []string {
	fields := strings.FieldsFunc(stripped, func(r rune) bool { return !isWordRune(r) })
	for i, f := range fields {
		fields[i] = strings.ToUpper(f)
	}
	return fields
}
