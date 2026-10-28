-- =============================================================================
-- 90 — Verify roles/grants (read-only). Run as `postgres` on each DB; paste output back.
-- psql -h 127.0.0.1 -U postgres -d <db> -f 90-verify.sql
-- =============================================================================
\pset pager off
\echo '== roles'
SELECT rolname, rolsuper, rolinherit, rolcanlogin, rolconnlimit,
       ARRAY(SELECT b.rolname || CASE WHEN NOT m.inherit_option THEN '(noinherit)' ELSE '' END
             FROM pg_auth_members m JOIN pg_roles b ON b.oid = m.roleid WHERE m.member = r.oid) AS member_of
FROM pg_roles r WHERE rolname !~ '^pg_' ORDER BY 1;

\echo '== objects NOT owned by the service owner (expect 0 rows)'
SELECT c.relkind, c.relname, pg_get_userbyid(c.relowner) owner
FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname !~ '^(pg_|information_schema$)' AND c.relkind IN ('r','p','v','m','S')
  AND pg_get_userbyid(c.relowner) <> regexp_replace(current_database(), '_db$', '_owner')
  AND NOT EXISTS (SELECT 1 FROM pg_depend d WHERE d.objid = c.oid AND d.deptype = 'e');

\echo '== tables the app role CANNOT write (expect 0 rows)'
SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
WHERE n.nspname = 'public' AND c.relkind IN ('r','p')
  AND NOT has_table_privilege(regexp_replace(current_database(), '_db$', '_app'), c.oid, 'INSERT,UPDATE,DELETE,SELECT');

\echo '== who can read restricted IAM columns/tables (expect only *_app, *_owner, backup_ro, postgres)'
SELECT t.tbl, t.col, r.rolname
FROM (VALUES ('mst_user','password_hash'),('mst_user','two_factor_secret'),('mst_user_detail','phone'),
             ('api_keys','key_hash'),('audit_logs','new_data'),('user_sessions','refresh_token_hash'),
             ('password_reset_tokens','token_hash'),('user_recovery_codes','code_hash'),
             ('chat_message','body_encrypted')) t(tbl,col)
JOIN pg_roles r ON r.rolname IN ('iam_ro','report_ro','dba_read','ilham','indra','iam_app','backup_ro')
WHERE current_database() = 'iam_db'
  AND to_regclass('public.' || t.tbl) IS NOT NULL
  AND has_column_privilege(r.rolname, ('public.' || t.tbl)::regclass, t.col, 'SELECT')
ORDER BY 1,2,3;

\echo '== cross-database CONNECT (each *_app must connect only to its own DB)'
SELECT r.rolname, d.datname
FROM pg_roles r CROSS JOIN pg_database d
WHERE r.rolname ~ '_(app|owner)$' AND d.datname ~ '_db$'
  AND has_database_privilege(r.rolname, d.datname, 'CONNECT')
ORDER BY 1,2;

\echo '== schemas other than public (finance_db prod: cmp owned by finance_owner; everything else: 0 rows)'
SELECT nspname, pg_get_userbyid(nspowner) AS owner,
       (SELECT count(*) FROM pg_class c WHERE c.relnamespace = n.oid) AS objects
FROM pg_namespace n WHERE nspname !~ '^(pg_|information_schema$)' AND nspname <> 'public' ORDER BY 1;

\echo '== default privileges'
SELECT pg_get_userbyid(defaclrole) AS for_role, defaclnamespace::regnamespace, defaclobjtype, defaclacl
FROM pg_default_acl ORDER BY 1,3;
