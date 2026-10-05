# Oracle DBA script pack

The DBA executes these scripts. GoApps never runs them and never connects to Oracle with them.
Every script has a header (purpose, target schema MGTDAT, executing role, prerequisite, rollback twin) and starts with `WHENEVER SQLERROR EXIT FAILURE ROLLBACK`.

## Order
`00_preflight_readonly.sql` -> `M-ERP-1a_interface_tables.sql` -> `M-ERP-1b_views.sql` -> `M-ERP-1c_pkg_goapps_adj.sql` -> `M-ERP-2_goapps_if_user.sql` -> `99_verify_readonly.sql`

Cutover only (after the above, in this order): `M-ERP-3_operating_agreement.sql` (comments only) -> `M-ERP-4a_backup_ddl.sql <PERIOD>` -> `M-ERP-4b_repoint.sql` (guided template) -> `M-ERP-4c_disable_legacy_writers.sql`. 4a has no rollback (backups are kept); 4b rollback = `@m_erp_4a_ddl_backup_<PERIOD>.sql`; 4c rollback = ENABLE.

| Script | Kind | Rollback twin |
|---|---|---|
| `00_preflight_readonly.sql` | SELECT only. Run on DEV and PROD and diff (runbook s0) | none |
| `M-ERP-1a_interface_tables.sql` | New tables, guard triggers, ADJ logs | `M-ERP-1a_interface_tables_rollback.sql` |
| `M-ERP-1b_views.sql` | New views (CREATE OR REPLACE) | `M-ERP-1b_views_rollback.sql` |
| `M-ERP-1c_pkg_goapps_adj.sql` | Lead's package, unchanged | `..._rollback.sql` |
| `M-ERP-2_goapps_if_user.sql` | New user GOAPPS_IF, minimal grants | `..._rollback.sql` |
| `M-ERP-3/4a/4b/4c` | Cutover-only (see above) | 4b, 4c have twins |
| `99_verify_readonly.sql` | SELECT only. Objects VALID, triggers ENABLED, grants | none |

Rules: no DML on legacy tables, no COMMIT, plain CREATEs are wrapped to be re-runnable (ORA-00955 ignored), **ABSOLUTE: no `DROP TABLE`, no `DELETE`, no `TRUNCATE` anywhere in this pack, on legacy tables or on the new interface tables, including rollbacks.** Rollbacks never remove tables or data: 1a keeps the CST_GOAPPS_* tables, 2 revokes + locks GOAPPS_IF (no DROP USER). The only DROPs allowed are of the new view/package code objects (`V_GOAPPS_*`, `PKG_GOAPPS_ADJ`), which hold no data. Enforced by `dba_scripts_test.go`.

## Open DBA questions
- T-1: tablespace for the new tables/indexes. Set `DEFINE ts = <TABLESPACE>` in `M-ERP-1a_interface_tables.sql`; candidates are listed by the preflight (section 5).
- T-3: the `VIEW_MARGIN_REPORT_DEV*` margin variants. Not in 1b; handled in M-ERP-4b once answered (flagged for the checklist).
