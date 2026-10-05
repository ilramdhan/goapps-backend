-- ============================================================================
-- Purpose        : 99_verify_readonly.sql : post-deploy SELECT-only verification (no DDL, no DML)
-- 99_verify_readonly.sql : post-deploy SELECT-only verification (no DDL, no DML)
-- Target schema  : MGTDAT
-- Executed by    : DBA. Order: LAST (00 -> 1a -> 1b -> 1c -> 2 -> 99)
-- Rollback twin  : none (read-only)
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
SET LINESIZE 250 PAGESIZE 500 TRIMSPOOL ON

PROMPT == 1. New objects (all STATUS must be VALID)
SELECT OWNER, OBJECT_NAME, OBJECT_TYPE, STATUS
  FROM ALL_OBJECTS
 WHERE OBJECT_NAME LIKE 'CST\_GOAPPS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'V\_GOAPPS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'PKG\_GOAPPS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'TRG\_GS%' ESCAPE '\'
    OR OBJECT_NAME LIKE 'SEQ\_GOAPPS%' ESCAPE '\'
 ORDER BY OBJECT_TYPE, OBJECT_NAME;
PROMPT -- any row below = problem (expect no rows)
SELECT OBJECT_NAME, OBJECT_TYPE, STATUS FROM ALL_OBJECTS
 WHERE STATUS <> 'VALID'
   AND (OBJECT_NAME LIKE 'CST\_GOAPPS%' ESCAPE '\' OR OBJECT_NAME LIKE 'V\_GOAPPS%' ESCAPE '\'
     OR OBJECT_NAME LIKE 'PKG\_GOAPPS%' ESCAPE '\' OR OBJECT_NAME LIKE 'TRG\_GS%' ESCAPE '\'
     OR OBJECT_NAME LIKE 'SEQ\_GOAPPS%' ESCAPE '\');

PROMPT == 2. Triggers (STATUS must be ENABLED)
SELECT OWNER, TRIGGER_NAME, TABLE_NAME, TRIGGERING_EVENT, STATUS
  FROM ALL_TRIGGERS WHERE TRIGGER_NAME LIKE 'TRG\_GS%' ESCAPE '\';

PROMPT == 3. GOAPPS_IF grants. EXPECTED (M-ERP-2, exactly this list, nothing more):
PROMPT --   sys : CREATE SESSION
PROMPT --   SELECT, INSERT on CST_GOAPPS_STD_BATCH and CST_GOAPPS_STD_COST
PROMPT --   EXECUTE on PKG_GOAPPS_ADJ
SELECT GRANTEE, PRIVILEGE FROM DBA_SYS_PRIVS WHERE GRANTEE = 'GOAPPS_IF';
SELECT GRANTEE, OWNER, TABLE_NAME, PRIVILEGE
  FROM DBA_TAB_PRIVS WHERE GRANTEE = 'GOAPPS_IF' ORDER BY TABLE_NAME, PRIVILEGE;
SELECT GRANTED_ROLE FROM DBA_ROLE_PRIVS WHERE GRANTEE = 'GOAPPS_IF';  -- expect no rows

PROMPT == 4. Read-only user: no NEW privileges (replace &RO_USER; expect no GOAPPS objects, no write privs)
SELECT GRANTEE, TABLE_NAME, PRIVILEGE FROM DBA_TAB_PRIVS
 WHERE GRANTEE = UPPER('&RO_USER')
   AND (PRIVILEGE IN ('INSERT','UPDATE','DELETE','EXECUTE')
        OR TABLE_NAME LIKE 'CST\_GOAPPS%' ESCAPE '\' OR TABLE_NAME LIKE 'PKG\_GOAPPS%' ESCAPE '\');

-- ----------------------------------------------------------------------------
-- 5. Guard tests: INSTRUCTIONS ONLY, NOT EXECUTED by this script. Run manually by the DBA
--    on DEV only, as shown; each statement must fail as noted, then ROLLBACK.
--   a) connected as any user other than GOAPPS_IF (e.g. the RO user), try an INSERT into
--      MGTDAT.CST_GOAPPS_STD_BATCH  -> expect ORA-20910 (or ORA-01031 if no grant).
--   b) connected as GOAPPS_IF, change any row in CST_GOAPPS_STD_COST (UPDATE or DELETE)
--      -> expect ORA-20911.
--   c) connected as GOAPPS_IF, insert a CST_GOAPPS_STD_COST row whose batch status is not
--      PUSHED -> expect ORA-20912.
--   Never run these on PROD data; use a throwaway batch id.
-- ----------------------------------------------------------------------------
