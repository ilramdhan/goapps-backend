#!/usr/bin/env python3
"""Migration lint (see migration-lint.sh). Python 3 stdlib only.

Rules (per violation: `path:line: [rule-id] message`):
  R0 file-naming          up without down (warning), duplicate version (error)
  R1 no-create-schema     CREATE/DROP/ALTER SCHEMA
  R2 no-search-path       SET search_path, set_config('search_path'
  R3 no-owner             OWNER TO, REASSIGN OWNED
  R4 no-grant-revoke      statement-start GRANT/REVOKE, ALTER DEFAULT PRIVILEGES
  R5 no-cross-db          dblink, fdw, servers, \\c, CREATE DATABASE/ROLE/USER ...
  R6 public-schema-only   refs into a non-public schema (denylist) or a.b.c names
  R7 unique-migration-table  x-migrations-table must be schema_migrations_<svc>
Comments, '...' literals are stripped before matching; dollar-quoted bodies are
real SQL and are linted (only their '...' literals are removed).
Allow-list: scripts/migration-lint.allow, `<rule> <path>[:<line>]  # reason`.
CREATE EXTENSION is intentionally not linted (pre-created by DBA scripts).
"""
import glob, os, re, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
SERVICES = ("iam", "finance", "ppc")
BASE_DENY = {"cmp", "auth", "export", "finance", "hr", "iam", "ppc", "goapps"}
SAFE_LEFT = {"pg_catalog", "information_schema", "public"}


def strip_sql(text, keep_strings=False):
    """Blank comments (and string contents unless keep_strings); keep newlines."""
    out, i, n = [], 0, len(text)
    while i < n:
        c, two = text[i], text[i:i + 2]
        if two == "--":
            while i < n and text[i] != "\n":
                i += 1
        elif two == "/*":
            j = text.find("*/", i + 2)
            j = n if j < 0 else j + 2
            out.append("".join(ch for ch in text[i:j] if ch == "\n"))
            i = j
        elif c == "'":
            j = i + 1
            while j < n:
                if text[j] == "'":
                    if text[j + 1:j + 2] == "'":
                        j += 2
                        continue
                    break
                j += 1
            j = min(j + 1, n)
            if keep_strings:
                out.append(text[i:j])
            else:
                out.append("''" + "".join(ch for ch in text[i:j] if ch == "\n"))
            i = j
        else:
            out.append(c)
            i += 1
    return "".join(out)


I = re.I
LINE_RULES = [  # (rule, regex on fully stripped text, message)
    ("R1", re.compile(r"\b(?:create|drop|alter)\s+schema\b", I), "schema DDL is forbidden (public only)"),
    ("R2", re.compile(r"\bset\s+(?:\w+\s+)?search_path\b|\balter\s.*?\bset\s+search_path\b", I), "search_path change is forbidden"),
    ("R3", re.compile(r"\bowner\s+to\b|\breassign\s+owned\b", I), "ownership change is forbidden"),
    ("R4", re.compile(r"(?:^|;|\bthen\b|\bbegin\b)\s*(?:grant|revoke)\b|\balter\s+default\s+privileges\b", I | re.M), "GRANT/REVOKE are managed by DBA scripts"),
    ("R5", re.compile(r"\bdblink\w*|\bpostgres_fdw\b|\bcreate\s+server\b|\bcreate\s+foreign\s+table\b|\bimport\s+foreign\s+schema\b|\bcreate\s+user\s+mapping\b|^\s*\\(?:c|connect)\b|\bcreate\s+database\b|\b(?:create|alter)\s+(?:role|user)\b", I | re.M), "cross-database / role management is forbidden"),
]
DYNAMIC_SQL_RULES = {"R1", "R2"}  # high-signal only; prose like "owner to" in strings is common
SEARCH_PATH_FN = re.compile(r"set_config\s*\(\s*'search_path'", I)
THREE_PART = re.compile(r"(?<![\w.\"])([A-Za-z_]\w*)\.([A-Za-z_]\w*)\.([A-Za-z_]\w*)\b")
TWO_PART = re.compile(r"(?<![\w.\"])([A-Za-z_]\w*)\.([A-Za-z_*]\w*)")
CREATE_SCHEMA = re.compile(r"\bcreate\s+schema\s+(?:if\s+not\s+exists\s+)?\"?(\w+)", I)


def lineno(text, pos):
    return text.count("\n", 0, pos) + 1


def lint_text(text, extra_deny=()):
    """Return [(line, rule, msg)] for one SQL file's text."""
    res = []
    nocomment = strip_sql(text, keep_strings=True)
    clean = strip_sql(text)
    for m in SEARCH_PATH_FN.finditer(nocomment):
        res.append((lineno(nocomment, m.start()), "R2", "set_config('search_path') is forbidden"))
    for rule, rx, msg in LINE_RULES:
        for m in rx.finditer(clean):
            pos = m.start() + (len(m.group(0)) - len(m.group(0).lstrip()))
            res.append((lineno(clean, pos), rule, msg))
        # dynamic SQL (EXECUTE 'SET search_path ...') hides in string literals:
        # re-check the keyword-only rules on the text with strings kept.
        if rule in DYNAMIC_SQL_RULES:
            for m in rx.finditer(nocomment):
                pos = m.start() + (len(m.group(0)) - len(m.group(0).lstrip()))
                res.append((lineno(nocomment, pos), rule, msg + " (in string / dynamic SQL)"))
    deny = BASE_DENY | {s.lower() for s in extra_deny}
    for m in CREATE_SCHEMA.finditer(clean):
        deny.add(m.group(1).lower())
    for m in THREE_PART.finditer(clean):
        if m.group(1).lower() not in SAFE_LEFT:
            res.append((lineno(clean, m.start()), "R6", "three-part name %s (cross-database?)" % m.group(0)))
    for m in TWO_PART.finditer(clean):
        if m.group(1).lower() in deny:
            res.append((lineno(clean, m.start()), "R6", "reference to non-public schema: %s" % m.group(0)))
    return sorted(set(res))


def all_denied_schemas(files):
    s = set()
    for f in files:
        s |= {m.group(1).lower() for m in CREATE_SCHEMA.finditer(strip_sql(open(f, encoding="utf-8", errors="replace").read()))}
    return s


MIG_NAME = re.compile(r"^(\d+)_.+\.(up|down)\.sql$")


def check_naming(svc, files):
    res, vers, ups, downs = [], {}, {}, set()
    for f in files:
        m = MIG_NAME.match(os.path.basename(f))
        rel = os.path.relpath(f, ROOT)
        if not m:
            res.append((rel, 1, "R0", "ERROR", "file name must be NNNNNN_name.{up,down}.sql"))
            continue
        v, kind = m.group(1), m.group(2)
        key = (v, kind)
        if key in vers:
            res.append((rel, 1, "R0", "ERROR", "duplicate %s version %s (also %s)" % (kind, v, vers[key])))
        vers[key] = rel
        (ups.__setitem__(v, rel) if kind == "up" else downs.add(v))
    for v, rel in sorted(ups.items()):
        if v not in downs:
            res.append((rel, 1, "R0", "WARN", "up migration without matching .down.sql"))
    return res


MT = re.compile(r"x-migrations-table=([^&'\"\s)]+)")
GOOD_MT = re.compile(r"^schema_migrations_(%s)$" % "|".join(SERVICES))
TEMPLATE_MT = re.compile(r"^schema_migrations_(%s|\$\{?\w+\}?|%%s|<\w+>)$" % "|".join(SERVICES))
SELF = ("migration_lint.py", "migration-lint.sh", "migration-lint.allow")


def check_r7_text(rel, text):
    """Return [(line, msg)] for x-migrations-table usages in one file."""
    res = []
    owner = re.match(r"services/(\w+)/", rel)
    for ln, line in enumerate(text.splitlines(), 1):
        for m in MT.finditer(line):
            name = m.group(1)
            if not TEMPLATE_MT.match(name):
                res.append((ln, "x-migrations-table '%s' must be schema_migrations_<%s>" % (name, "|".join(SERVICES))))
                continue
            mentioned = {s for s in SERVICES if re.search(r"(?<![a-z])%s(?![a-z])" % s, MT.sub("", line).lower())}
            if owner and owner.group(1) in SERVICES:
                mentioned = {owner.group(1)}
            want = GOOD_MT.match(name)
            if want and mentioned and mentioned != {want.group(1)} and len(mentioned) == 1:
                res.append((ln, "'%s' used for service %s (names must be unique per service)" % (name, next(iter(mentioned)))))
    return res


def r7_files():
    pats = ["Makefile", "services/*/Makefile", ".github/workflows/*.yml", ".github/workflows/*.yaml",
            "docker-compose*.yaml", "docker-compose*.yml", "scripts/**/*.sh", "scripts/**/*.py",
            "scripts/**/*.yaml", "scripts/**/*.yml", "scripts/**/Makefile"]
    found = set()
    for p in pats:
        found |= set(glob.glob(os.path.join(ROOT, p), recursive=True))
    return sorted(f for f in found if os.path.isfile(f) and os.path.basename(f) not in SELF)


def load_allow():
    path = os.path.join(ROOT, "scripts", "migration-lint.allow")
    entries, errs = [], []
    if os.path.exists(path):
        for n, raw in enumerate(open(path, encoding="utf-8"), 1):
            if not raw.strip() or raw.lstrip().startswith("#"):
                continue
            body, _, why = raw.partition("#")
            parts = body.split()
            if len(parts) != 2 or not why.strip():
                errs.append((n, "allow entry needs '<rule> <path>[:<line>]  # justification'"))
                continue
            p, _, ln = parts[1].partition(":")
            entries.append({"n": n, "rule": parts[0], "path": p, "line": int(ln) if ln.isdigit() else None, "used": False})
    return entries, errs


def run():
    sql_by_svc = {s: sorted(glob.glob(os.path.join(ROOT, "services", s, "migrations", "postgres", "*.sql"))) for s in SERVICES}
    # also pick up any other service dir with migrations
    for d in glob.glob(os.path.join(ROOT, "services", "*", "migrations", "postgres")):
        s = d.split(os.sep)[-3]
        sql_by_svc.setdefault(s, sorted(glob.glob(os.path.join(d, "*.sql"))))
    denied = all_denied_schemas(sum(sql_by_svc.values(), []))
    violations = []  # (rel, line, rule, level, msg)
    for svc, files in sorted(sql_by_svc.items()):
        violations += check_naming(svc, files)
        for f in files:
            rel = os.path.relpath(f, ROOT)
            for ln, rule, msg in lint_text(open(f, encoding="utf-8", errors="replace").read(), denied):
                violations.append((rel, ln, rule, "ERROR", msg))
    for f in r7_files():
        rel = os.path.relpath(f, ROOT)
        for ln, msg in check_r7_text(rel, open(f, encoding="utf-8", errors="replace").read()):
            violations.append((rel, ln, "R7", "ERROR", msg))
    entries, allow_errs = load_allow()
    kept = []
    for v in violations:
        hit = False
        for e in entries:
            if e["rule"] == v[2] and e["path"] == v[0] and e["line"] in (None, v[1]):
                e["used"] = hit = True
        if not hit:
            kept.append(v)
    errors = warns = 0
    for rel, ln, rule, lvl, msg in kept:
        print("%s:%d: [%s] %s%s" % (rel, ln, rule, "warning: " if lvl == "WARN" else "", msg))
        errors += lvl == "ERROR"
        warns += lvl == "WARN"
    for n, msg in allow_errs:
        print("scripts/migration-lint.allow:%d: [allow] %s" % (n, msg)); errors += 1
    for e in entries:
        if not e["used"]:
            print("scripts/migration-lint.allow:%d: [allow] stale entry (matches nothing): %s %s" % (e["n"], e["rule"], e["path"])); errors += 1
    nfiles = sum(len(v) for v in sql_by_svc.values())
    print("migration-lint: %d file(s), %d error(s), %d warning(s)" % (nfiles, errors, warns))
    return 1 if errors else 0


# ----------------------------------------------------------------- self-test
BAD = {  # rule -> snippets that MUST be flagged with that rule
    "R1": ["CREATE SCHEMA IF NOT EXISTS foo;", "drop schema foo cascade;", "ALTER SCHEMA a RENAME TO b;"],
    "R2": ["SET search_path TO finance, public;", "SET LOCAL search_path = x;", "SELECT set_config('search_path', 'x', false);",
           "ALTER ROLE app SET search_path = x;", "CREATE FUNCTION f() RETURNS void AS $$ BEGIN SET search_path = x; END $$ LANGUAGE plpgsql;",
           "DO $$ BEGIN EXECUTE 'SET search_path = cmp'; END $$;"],
    "R3": ["ALTER TABLE t OWNER TO bob;", "REASSIGN OWNED BY a TO b;"],
    "R4": ["GRANT SELECT ON t TO app;", "SELECT 1;\n  revoke all on t from app;", "ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO x;",
           "DO $$ BEGIN GRANT ALL ON t TO x; END $$;"],
    "R5": ["CREATE EXTENSION dblink; SELECT * FROM dblink('x','y') AS t(a int);", "CREATE EXTENSION postgres_fdw;", "CREATE SERVER s FOREIGN DATA WRAPPER postgres_fdw;",
           "CREATE FOREIGN TABLE ft (a int) SERVER s;", "IMPORT FOREIGN SCHEMA x FROM SERVER s INTO public;", "CREATE USER MAPPING FOR u SERVER s;",
           "\\c other_db", "\\connect other_db", "CREATE DATABASE x;", "CREATE ROLE r;", "ALTER USER u WITH PASSWORD 'x';"],
    "R6": ["SELECT * FROM finance.items;", "INSERT INTO iam.users(id) VALUES (1);", "SELECT * FROM otherdb.public.t;",
           "CREATE SCHEMA zzz; SELECT * FROM zzz.t;", "DO $$ BEGIN PERFORM 1 FROM cmp.t; END $$;"],
}
GOOD = [  # must produce no violations
    "-- grant access to finance.items; SET search_path\n/* OWNER TO x; GRANT y */ SELECT 1;",
    "INSERT INTO perms(code) VALUES ('finance.mb.head.view'), ('iam.user.view');",
    "SELECT 'it''s GRANT SELECT ON x; OWNER TO y; finance.items' AS s;",
    "ALTER TABLE t ADD COLUMN grant_type text;",
    "ALTER TABLE a ADD CONSTRAINT fk FOREIGN KEY (b) REFERENCES public.b(id);",
    "SELECT f.id, t.name FROM public.foo f JOIN pg_catalog.pg_class t ON t.oid = f.id;",
    "SELECT information_schema.x.y FROM information_schema.tables;",
    "CREATE EXTENSION IF NOT EXISTS pgcrypto;",
    "-- Penjelasan: grant diberikan oleh DBA\nCREATE TABLE t(id int);",
    "CREATE FUNCTION f() RETURNS text AS $body$ BEGIN RETURN 'finance.x'; END $body$ LANGUAGE plpgsql;",
    "COMMENT ON COLUMN public.t.c IS 'x';",
]


def self_test():
    fails = 0
    for rule, snippets in BAD.items():
        for s in snippets:
            got = {r for _, r, _ in lint_text(s)}
            if rule not in got:
                print("SELF-TEST FAIL: expected %s for %r, got %s" % (rule, s, sorted(got))); fails += 1
    for s in GOOD:
        got = lint_text(s)
        if got:
            print("SELF-TEST FAIL: false positive for %r: %s" % (s, got)); fails += 1
    r7_bad = ["migrate -database 'x?x-migrations-table=schema_migrations'", "x-migrations-table=public_mig_ppc",
              "FINANCE_DB_URL='postgres://h/finance_db?x-migrations-table=schema_migrations_iam'"]
    r7_good = ["x-migrations-table=schema_migrations_%s' $$s", "x-migrations-table=schema_migrations_ppc",
               "IAM_URL='postgres://h/iam_db?x-migrations-table=schema_migrations_iam'"]
    for s in r7_bad:
        if not check_r7_text("Makefile", s):
            print("SELF-TEST FAIL: R7 missed %r" % s); fails += 1
    for s in r7_good:
        if check_r7_text("Makefile", s):
            print("SELF-TEST FAIL: R7 false positive %r" % s); fails += 1
    if not check_r7_text("services/ppc/Makefile", "x-migrations-table=schema_migrations_iam"):
        print("SELF-TEST FAIL: R7 owner mismatch missed"); fails += 1
    # line numbers survive stripping
    if lint_text("-- a\n/* b\nc */\nGRANT x;")[0][0] != 4:
        print("SELF-TEST FAIL: line numbering"); fails += 1
    print("migration-lint self-test: %s" % ("FAILED (%d)" % fails if fails else "ok"))
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(self_test() if "--self-test" in sys.argv[1:] else run())
