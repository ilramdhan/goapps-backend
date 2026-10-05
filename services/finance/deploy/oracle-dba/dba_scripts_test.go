package oracledba_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const packageFile = "M-ERP-1c_pkg_goapps_adj.sql"

// Files that intentionally have no rollback twin (read-only, or backups are kept).
var noRollbackTwin = map[string]bool{
	"00_preflight_readonly.sql":       true,
	"99_verify_readonly.sql":          true,
	"M-ERP-3_operating_agreement.sql": true,
	"M-ERP-4a_backup_ddl.sql":         true,
}

var (
	reCreate = regexp.MustCompile(`(?i)\bCREATE\s+(TABLE|INDEX|UNIQUE\s+INDEX|SEQUENCE|USER)\b`)
	reDrop   = regexp.MustCompile(`(?i)\b(DROP|TRUNCATE)\s+(?:(?:TABLE|VIEW|SEQUENCE|TRIGGER|PACKAGE|INDEX|USER|SYNONYM)\s+)?("?[A-Za-z0-9_$.]+)?`)
	// reNeverDrop: user rule, absolute. No table/user/sequence is ever dropped, and no
	// table is ever emptied, anywhere in the pack (rollbacks included). Applies to every file.
	reNeverDrop = regexp.MustCompile(`(?i)\b(DROP\s+(TABLE|USER|SEQUENCE|TRIGGER|INDEX)|TRUNCATE\s+TABLE|DELETE\s+FROM)\b`)
	reLegacyDML = regexp.MustCompile(`(?i)\b(DELETE\s+FROM|MERGE\s+INTO|INSERT\s+INTO|UPDATE)\s+(?:MGTDAT\.)?(OT_|OM_|MGT_|IM_)[A-Za-z0-9_]*`)
	reIdent     = regexp.MustCompile(`(?i)IDENTIFIED\s+BY\s+(\S+)`)
	reCommit    = regexp.MustCompile(`(?im)^\s*COMMIT\s*;`)
	reLineCmt   = regexp.MustCompile(`--.*`)
)

// dropAllow lists the only DROP targets: new code objects that hold no data.
var dropAllow = []string{"V_GOAPPS_", "PKG_GOAPPS_ADJ"}

func stripComments(s string) string { return reLineCmt.ReplaceAllString(s, "") }

// checkScript returns one "rule: detail" message per violation of the DBA script lint (S-T5).
func checkScript(name, content string, isRollback bool) []string {
	var v []string
	lines := strings.Split(content, "\n")
	head := strings.ToLower(strings.Join(lines[:min(30, len(lines))], "\n"))
	for _, labels := range [][]string{{"purpose"}, {"schema"}, {"executing role", "executed by", "role"}, {"rollback"}} {
		ok := false
		for _, l := range labels {
			ok = ok || strings.Contains(head, l)
		}
		if !ok {
			v = append(v, "header: missing "+labels[0])
		}
	}
	code := stripComments(content)
	codeLines := strings.Split(code, "\n")

	for i, l := range codeLines {
		if reCreate.MatchString(l) {
			win := strings.Join(codeLines[i:min(i+40, len(codeLines))], "\n")
			if !strings.Contains(win, "-955") && !strings.Contains(win, "-1920") {
				v = append(v, "create: unguarded CREATE at line "+strconv.Itoa(i+1))
			}
		}
	}

	if m := reNeverDrop.FindString(code); m != "" {
		v = append(v, "never-drop: forbidden statement "+strings.ToUpper(m))
	}
	for _, m := range reDrop.FindAllStringSubmatch(code, -1) {
		if !isRollback {
			v = append(v, "drop: "+strings.ToUpper(m[1])+" outside *_rollback.sql")
			continue
		}
		target := strings.ToUpper(strings.Trim(m[2], `"`))
		if i := strings.LastIndex(target, "."); i >= 0 {
			target = target[i+1:]
		}
		allowed := false
		for _, a := range dropAllow {
			allowed = allowed || strings.Contains(target, a)
		}
		if !allowed {
			v = append(v, "drop-target: not in allowlist: "+target)
		}
	}

	if name != packageFile && reLegacyDML.MatchString(code) {
		v = append(v, "legacy-dml: DML on legacy table outside "+packageFile)
	}
	for _, m := range reIdent.FindAllStringSubmatch(code, -1) {
		if !strings.Contains(m[1], "&") {
			v = append(v, "password: literal IDENTIFIED BY")
		}
	}
	if !strings.Contains(strings.ToUpper(content), "WHENEVER SQLERROR") {
		v = append(v, "whenever: missing WHENEVER SQLERROR")
	}
	if name != packageFile && reCommit.MatchString(code) {
		v = append(v, "commit: COMMIT outside package")
	}
	return v
}

func TestDBAScripts(t *testing.T) {
	files, err := filepath.Glob("*.sql")
	require.NoError(t, err)
	require.NotEmpty(t, files)
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			b, err := os.ReadFile(f)
			require.NoError(t, err)
			isRB := strings.HasSuffix(f, "_rollback.sql")
			assert.Empty(t, checkScript(f, string(b), isRB))
			if !isRB && !noRollbackTwin[f] {
				_, err := os.Stat(strings.TrimSuffix(f, ".sql") + "_rollback.sql")
				assert.NoError(t, err, "rollback twin missing")
			}
		})
	}
}

func TestBadFixtureIsRejected(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("testdata", "bad_script.sql"))
	require.NoError(t, err)
	got := checkScript("bad_script.sql", string(b), false)
	kinds := map[string]bool{}
	for _, m := range got {
		kinds[strings.SplitN(m, ":", 2)[0]] = true
	}
	assert.GreaterOrEqual(t, len(kinds), 5, "violations: %v", got)
}
