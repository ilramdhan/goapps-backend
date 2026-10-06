-- ============================================================================
-- Purpose        : 00_preflight_readonly.sql : SELECT-only reconnaissance (no DDL, no DML)
-- 00_preflight_readonly.sql : SELECT-only reconnaissance (no DDL, no DML)
-- Target schema  : MGTDAT (read via ALL_*/DBA_* views)
-- Executed by    : DBA. Order: this script runs FIRST (00 -> 1a -> 1b -> 1c -> 2 -> 99)
-- Rollback twin  : none (read-only)
-- RUNBOOK s0     : run on DEV and PROD and diff the two outputs before any deploy.
--                  SPOOL both to files, e.g. preflight_dev.txt / preflight_prod.txt.
-- ============================================================================
--
-- ============================================================================
-- DBA REVIEW CHECKLIST (sign-off required, AC-15)
-- Source: docs/superpowers/specs/2026-09-29-erp-cost-integration-design-part3.md s13.1
-- ============================================================================
-- Legacy-touching statements (L-1..L-13)
-- [ ] L-1  V_GOAPPS_ADJ_DEMAND reads OT_ADJ_HEAD/ITEM (APPR_STATUS = 3): read-only, keep; PERIOD filter mandatory in GoApps.
-- [ ] L-2  V_GOAPPS_STD_COST_CUR unions OT_STD_COST_PRODUCTS_MGT (LEGACY_FROZEN): read-only, keep.
-- [ ] L-3  VALUATE log insert (NULL APPR_STATUS excluded): per-head part withdrawn per U-1; period-wide accepted; compensated by GoApps gates G6/G7/G7a/G8/G9.
-- [ ] L-4  VALUATE UPDATE OT_ADJ_ITEM (INNER JOIN OM_ITEM, no row lock): per-head part withdrawn per U-1; period-wide accepted; compensated by GoApps gates G6/G7/G7a/G8/G9.
-- [ ] L-5  VALUATE NOT_COVERED count (NULL bug): per-head part withdrawn per U-1; period-wide accepted; compensated by GoApps gates G6/G7/G7a/G8/G9.
-- [ ] L-6  APPROVE_ADJ UPDATE OT_ADJ_HEAD (241441 aborts all heads on one bad rate): per-head part withdrawn per U-1; period-wide accepted; compensated by GoApps gates G6/G7/G7a/G8/G9; ERP users approve by default (adj_approve_enabled OFF).
-- [ ] L-7  APPROVE_ADJ sets STD_BATCH APPROVED unconditionally: withdrawn per U-1; period-wide accepted; compensated by GoApps gates G6/G7/G7a/G8/G9.
-- [ ] L-8  RESTORE_ADJ UPDATE OT_ADJ_ITEM period-wide: withdrawn per U-1; period-wide accepted; compensated by GoApps gates G6/G7/G7a/G8/G9.
-- [ ] L-9  Repoint of O_DGET_FG_ITEM_RATE_MGT, FUNC_CHIP_WAC_MGT, VIEW_MARGIN_REPORT*, PRODUCTS_LIST_COST: split into M-ERP-4b; 4a backup mandatory first; diff = reference lines only; DEV then PROD.
-- [ ] L-10 No UPDATE grant on CST_GOAPPS_STD_BATCH for GOAPPS_IF (AUTHID DEFINER does the updates): GRANT SELECT, INSERT only.
-- [ ] L-11 GOAPPS_IF gets no legacy SELECT grants; legacy SELECTs belong to the existing read-only user.
-- [ ] L-12 Plain CREATE TABLE/SEQUENCE/INDEX wrapped (ignore ORA-00955 / -01920): scripts re-runnable.
-- [ ] L-13 M-ERP-4c ALTER TRIGGER ... DISABLE (state change, not DROP) for ODBTRG_ITEMCOSTVALLOSS_MGT, ODBTRG_SELLPRICECOST_MGT, ODBTRG_FG_STD_COST + re-ENABLE script; cutover only.
-- Decisions and suggestions
-- [ ] D-S3 Minimal GOAPPS_IF grants (D-S2 withdrawn): SELECT, INSERT on the 2 interface tables; SELECT on CST_GOAPPS_ADJ_LOG/HEAD_LOG; EXECUTE on PKG_GOAPPS_ADJ.
-- [ ] (a) non-blocking: NVL(ADJH_APPR_STATUS,0) <> 3 instead of ADJH_APPR_STATUS != 3 (NULL status silently excluded today).
-- [ ] (b) non-blocking: LEFT JOIN instead of INNER JOIN to OM_ITEM in VALUATE (C-10).
-- [ ] (c) non-blocking: no UPDATE grant on CST_GOAPPS_STD_BATCH needed for GOAPPS_IF (AUTHID DEFINER).
-- [ ] (d) non-blocking: the package leaves GAL_ADJH_SYS_ID/OLD_ITEM_DESC NULL and never writes CST_GOAPPS_ADJ_HEAD_LOG; consider filling them for RESTORE_ADJ fidelity.
-- Open DBA questions
-- [ ] T-1 Tablespace: choose the value for DEFINE ts (&ts) in M-ERP-1a (see query 5 below).
-- [ ] T-3 VIEW_MARGIN_REPORT_DEV* variants: decide whether to repoint (flagged in M-ERP-1b).
-- [ ] Object types of O_DGET_FG_ITEM_RATE_MGT and PRODUCTS_LIST_COST confirmed via ALL_OBJECTS (FUNCTION/VIEW/PACKAGE?).
-- Execution order
-- [ ] Deploy:   00 -> M-ERP-1a -> 1b -> 1c -> M-ERP-2 -> 99
-- [ ] Cutover only: M-ERP-4a -> 4c -> 4b
-- [ ] Rollback (reverse): 2_rollback (revoke + LOCK, no DROP USER) -> 1c_rollback -> 1b_rollback -> 1a_rollback (drops NOTHING; tables kept); 4b_rollback, 4c_rollback
-- [ ] ABSOLUTE: this pack contains NO DROP TABLE and NO DELETE/TRUNCATE on any table (legacy or new). Confirm before running anything.
-- ORA codes (package + guard triggers)
--   -20901 period already has posted ADJ heads; integration refused
--   -20902 rate > 20 for prefixes guarded by ODBTRG_COST_VAL_MGT
--   -20903 batch incomplete/inconsistent (row count or control sums differ)
--   -20904 LOCK_BATCH: period has no posted ADJ yet
--   -20905 batch status wrong (not PUSHED in VALUATE; not VALUATED/APPROVED in LOCK)
--   -20910 guard trigger: STD_BATCH/STD_COST changed by a user other than GOAPPS_IF
--   -20911 guard trigger: STD_COST is insert-only; correction = new batch
--   -20912 guard trigger: STD_COST row inserted into a batch whose status is not PUSHED
-- Expected 99_verify output
-- [ ] All GOAPPS objects VALID; 2 guard triggers ENABLED
-- [ ] GOAPPS_IF grants are exactly the D-S3 list; the read-only user is unchanged
-- Guard tests (DBA, manual)
-- [ ] Insert into a guarded table as another user -> ORA-20910
-- [ ] Update of a STD_COST row -> ORA-20911
--
-- Reviewed by: ________  Date: ________
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
SET LINESIZE 250 PAGESIZE 500 TRIMSPOOL ON

PROMPT == 1. Legacy objects used (existence / status)
SELECT OWNER, OBJECT_NAME, OBJECT_TYPE, STATUS
  FROM ALL_OBJECTS
 WHERE OWNER = 'MGTDAT'
   AND OBJECT_NAME IN ('OT_ADJ_HEAD','OT_ADJ_ITEM','OM_ITEM','OM_GRADE_CODE_1','OM_GRADE_CODE_2',
                       'OT_STD_COST_PRODUCTS_MGT')
 ORDER BY OBJECT_NAME;

PROMPT == 2. Column types/widths of legacy tables (compare with CST_GOAPPS_STD_COST, T-1/L-12)
SELECT TABLE_NAME, COLUMN_NAME, DATA_TYPE, DATA_LENGTH, DATA_PRECISION, DATA_SCALE, NULLABLE
  FROM ALL_TAB_COLUMNS
 WHERE OWNER = 'MGTDAT'
   AND TABLE_NAME IN ('OT_ADJ_HEAD','OT_ADJ_ITEM','OM_ITEM','OM_GRADE_CODE_1','OM_GRADE_CODE_2',
                      'OT_STD_COST_PRODUCTS_MGT')
 ORDER BY TABLE_NAME, COLUMN_ID;

PROMPT == 3. ADJI_FLEX_01..14 widths (FLEX overflow D-S5; expected VARCHAR2 >= 240?)
SELECT COLUMN_NAME, DATA_TYPE, DATA_LENGTH
  FROM ALL_TAB_COLUMNS
 WHERE OWNER = 'MGTDAT' AND TABLE_NAME = 'OT_ADJ_ITEM' AND COLUMN_NAME LIKE 'ADJI_FLEX\_%' ESCAPE '\'
 ORDER BY COLUMN_NAME;

PROMPT == 4. ADJI_ITEM_DESC width (sizes CST_GOAPPS_ADJ_LOG.OLD_ITEM_DESC)
SELECT COLUMN_NAME, DATA_TYPE, DATA_LENGTH
  FROM ALL_TAB_COLUMNS
 WHERE OWNER = 'MGTDAT' AND TABLE_NAME = 'OT_ADJ_ITEM' AND COLUMN_NAME = 'ADJI_ITEM_DESC';

PROMPT == 5. Tablespaces (T-1: choose the value for DEFINE ts in M-ERP-1a)
SELECT TABLESPACE_NAME, STATUS, CONTENTS, EXTENT_MANAGEMENT, SEGMENT_SPACE_MANAGEMENT
  FROM DBA_TABLESPACES
 ORDER BY TABLESPACE_NAME;
SELECT USERNAME, DEFAULT_TABLESPACE FROM DBA_USERS WHERE USERNAME IN ('MGTDAT','GOAPPS_IF');

PROMPT == 6. New objects already present? (expect none on first deploy)
SELECT OWNER, OBJECT_NAME, OBJECT_TYPE, STATUS
  FROM ALL_OBJECTS
 WHERE OBJECT_NAME LIKE 'CST\_GOAPPS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'V\_GOAPPS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'PKG\_GOAPPS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'TRG\_GS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'SEQ\_GOAPPS%' ESCAPE '\'
 ORDER BY OBJECT_TYPE, OBJECT_NAME;

PROMPT == 7. Dependents of OT_STD_COST_PRODUCTS_MGT (the M-ERP-4b repoint list)
SELECT OWNER, NAME, TYPE, REFERENCED_OWNER, REFERENCED_NAME
  FROM ALL_DEPENDENCIES
 WHERE REFERENCED_NAME = 'OT_STD_COST_PRODUCTS_MGT'
 ORDER BY OWNER, TYPE, NAME;

PROMPT == 8. Current grants for GOAPPS_IF
SELECT GRANTEE, OWNER, TABLE_NAME, PRIVILEGE, GRANTABLE
  FROM DBA_TAB_PRIVS WHERE GRANTEE = 'GOAPPS_IF' ORDER BY TABLE_NAME, PRIVILEGE;
SELECT GRANTEE, PRIVILEGE, ADMIN_OPTION FROM DBA_SYS_PRIVS WHERE GRANTEE = 'GOAPPS_IF';
SELECT USERNAME, ACCOUNT_STATUS FROM DBA_USERS WHERE USERNAME = 'GOAPPS_IF';

PROMPT == 9. Current grants for the read-only user (replace &RO_USER; no new grants expected)
SELECT GRANTEE, OWNER, TABLE_NAME, PRIVILEGE
  FROM DBA_TAB_PRIVS WHERE GRANTEE = UPPER('&RO_USER') ORDER BY TABLE_NAME, PRIVILEGE;
SELECT GRANTEE, PRIVILEGE FROM DBA_SYS_PRIVS WHERE GRANTEE = UPPER('&RO_USER');
