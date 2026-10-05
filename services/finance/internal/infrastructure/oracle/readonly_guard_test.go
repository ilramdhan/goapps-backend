package oracle

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestCheckReadOnly_Table(t *testing.T) {
	cases := []struct {
		name  string
		query string
		ok    bool
	}{
		// Accepted.
		{"plain select", "SELECT ITEM_CODE FROM MGTDAT.OM_ITEM", true},
		{"lower case select", "select item_code from mgtdat.om_item", true},
		{"mixed case select", "SeLeCt 1 FrOm DUAL", true},
		{"with cte", "WITH a AS (SELECT 1 x FROM DUAL) SELECT x FROM a", true},
		{"leading whitespace", "  \n\t SELECT 1 FROM DUAL", true},
		{"leading line comment", "-- header\nSELECT 1 FROM DUAL", true},
		{"leading block comment", "/* hint */ SELECT 1 FROM DUAL", true},
		{"hint comment", "SELECT /*+ INDEX(t) */ 1 FROM DUAL t", true},
		{"literal with semicolon", "SELECT 'a;b' FROM DUAL", true},
		{"literal with delete", "SELECT 'DELETE FROM X' FROM DUAL", true},
		{"literal with escaped quote", "SELECT 'it''s; DROP' FROM DUAL", true},
		{"q quote literal", "SELECT q'[DROP TABLE; x]' FROM DUAL", true},
		{"nq quote literal", "SELECT nq'{UPDATE ;}' FROM DUAL", true},
		{"comment with semicolon", "SELECT 1 FROM DUAL -- ; DELETE", true},
		{"block comment with update", "SELECT 1 /* UPDATE X SET Y=1; */ FROM DUAL", true},
		{"quoted identifier update", "SELECT \"UPDATE\" FROM DUAL", true},
		{"column containing keyword", "SELECT UPDATED_AT, DELETED_FLAG, CREATED_BY FROM T", true},
		{"bind placeholders", "SELECT * FROM MGTDAT.OM_ITEM WHERE ITEM_CODE = :1", true},
		{"unicode nbsp separator", "SELECT 1 FROM DUAL", true},
		{"unicode ideographic space", "SELECT　1 FROM DUAL", true},
		{"union select", "SELECT 1 FROM DUAL UNION ALL SELECT 2 FROM DUAL", true},
		{"subquery", "SELECT * FROM (SELECT ITEM_CODE FROM MGTDAT.OM_ITEM) WHERE ROWNUM <= 10", true},

		// Rejected.
		{"empty", "", false},
		{"only comment", "-- nothing", false},
		{"only whitespace", "   \n ", false},
		{"insert", "INSERT INTO T VALUES (1)", false},
		{"update", "UPDATE OT_ADJ_ITEM SET ADJI_RATE=0", false},
		{"lower update", "update ot_adj_item set adji_rate=0", false},
		{"delete", "DELETE FROM MGTDAT.OM_ITEM", false},
		{"merge", "MERGE INTO T USING S ON (1=1) WHEN MATCHED THEN UPDATE SET A=1", false},
		{"drop", "DROP TABLE T", false},
		{"truncate", "TRUNCATE TABLE T", false},
		{"alter session", "ALTER SESSION SET NLS_DATE_FORMAT='YYYY'", false},
		{"create", "CREATE TABLE T (A NUMBER)", false},
		{"grant", "GRANT SELECT ON T TO U", false},
		{"revoke", "REVOKE SELECT ON T FROM U", false},
		{"rename", "RENAME T TO U", false},
		{"comment on", "COMMENT ON TABLE T IS 'x'", false},
		{"lock table", "LOCK TABLE T IN EXCLUSIVE MODE", false},
		{"plsql block", "BEGIN MGTDAT.STD_FG_UPD; END;", false},
		{"declare block", "DECLARE x NUMBER; BEGIN NULL; END;", false},
		{"call", "CALL MGTDAT.CHP_WAC_UPD()", false},
		{"exec", "EXEC MGTDAT.X", false},
		{"commit", "COMMIT", false},
		{"rollback", "ROLLBACK", false},
		{"trailing semicolon", "SELECT 1 FROM DUAL;", false},
		{"stacked statements", "SELECT 1 FROM DUAL; DELETE FROM T", false},
		{"with delete", "WITH a AS (SELECT 1 FROM DUAL) DELETE FROM T", false},
		{"with insert", "WITH a AS (SELECT 1 x FROM DUAL) INSERT INTO T SELECT x FROM a", false},
		{"select for update", "SELECT * FROM OT_ADJ_ITEM FOR UPDATE", false},
		{"select for update nowait", "SELECT * FROM T FOR UPDATE NOWAIT", false},
		{"select for update mixed", "select * from t For\tUpdate", false},
		{"comment then delete", "/* SELECT */ DELETE FROM T", false},
		{"line comment then delete", "-- SELECT\nDELETE FROM T", false},
		{"comment hiding separator", "SELECT 1 FROM DUAL/**/;/**/DROP TABLE T", false},
		{"unicode space before update", "SELECT 1 FROM T FOR UPDATE", false},
		{"unterminated literal", "SELECT 'abc FROM DUAL", false},
		{"unterminated block comment", "SELECT 1 /* FROM DUAL", false},
		{"unterminated q literal", "SELECT q'[abc FROM DUAL", false},
		{"execute immediate", "SELECT 1 FROM DUAL WHERE EXECUTE IMMEDIATE", false},
		{"select into dual update", "SELECT 1 FROM DUAL WHERE 1 = (UPDATE T SET A = 1)", false},
	}
	if len(cases) < 40 {
		t.Fatalf("guard table must hold at least 40 cases, has %d", len(cases))
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := CheckReadOnly(tc.query)
			if tc.ok && err != nil {
				t.Fatalf("expected %q to be accepted, got %v", tc.query, err)
			}
			if !tc.ok {
				if err == nil {
					t.Fatalf("expected %q to be rejected", tc.query)
				}
				if !errors.Is(err, ErrNonSelectRejected) {
					t.Fatalf("expected ErrNonSelectRejected, got %v", err)
				}
			}
		})
	}
}

func TestReadOnlyDB_NilConnection(t *testing.T) {
	var r *ReadOnlyDB
	if _, err := r.QueryRO(context.Background(), "SELECT 1 FROM DUAL"); !errors.Is(err, ErrNoReadConnection) {
		t.Fatalf("expected ErrNoReadConnection, got %v", err)
	}
	var c *Client
	if _, err := c.QueryRO(context.Background(), "SELECT 1 FROM DUAL"); !errors.Is(err, ErrNoReadConnection) {
		t.Fatalf("expected ErrNoReadConnection, got %v", err)
	}
}

// FuzzReadOnlyGuard checks the core property: an accepted statement always
// starts with SELECT/WITH and contains no ';' outside comments and literals.
func FuzzReadOnlyGuard(f *testing.F) {
	seeds := []string{
		"SELECT 1 FROM DUAL",
		"WITH a AS (SELECT 1 FROM DUAL) SELECT * FROM a",
		"SELECT 'x;y' FROM DUAL",
		"SELECT q'[;]' FROM DUAL",
		"/* c */ SELECT 1 FROM DUAL -- ;",
		"DELETE FROM T",
		"SELECT 1 FROM DUAL; DROP TABLE T",
		"SELECT * FROM T FOR UPDATE",
		"BEGIN NULL; END;",
		"SELECT 1 FROM DUAL",
	}
	for _, s := range seeds {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, query string) {
		if CheckReadOnly(query) != nil {
			return
		}
		first := firstSQLWord(query)
		if first != "SELECT" && first != "WITH" {
			t.Fatalf("accepted %q but first token is %q", query, first)
		}
		stripped, ok := stripCommentsAndLiterals(query)
		if !ok {
			t.Fatalf("accepted %q with unterminated literal/comment", query)
		}
		if strings.ContainsRune(stripped, ';') {
			t.Fatalf("accepted %q containing ';'", query)
		}
		for _, w := range sqlWords(stripped) {
			if _, bad := forbiddenTokens[w]; bad {
				t.Fatalf("accepted %q containing forbidden token %q", query, w)
			}
		}
	})
}
