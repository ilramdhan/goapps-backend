-- ============================================================================
-- M-ERP-1a ROLLBACK (DBA ONLY, after sign-off)
-- Purpose        : rollback of M-ERP-1a WITHOUT dropping anything.
--                  ABSOLUTE RULE: no DROP TABLE and no DELETE/TRUNCATE in this pack,
--                  on legacy tables or on the new interface tables.
--                  The CST_GOAPPS_* tables, SEQ_GOAPPS_ADJ_LOG and the guard
--                  triggers are KEPT (data preserved). GoApps access is cut by
--                  M-ERP-2 rollback (revoke + account lock) and the app writer
--                  stays disabled. Removing any object later is a separate
--                  change-control decision by the lead/DBA, outside this pack.
-- Target schema  : MGTDAT
-- Executed by    : DBA
-- Prerequisite   : M-ERP-2 rollback already run
-- Twin of        : M-ERP-1a_interface_tables.sql
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK

PROMPT M-ERP-1a rollback: nothing is dropped. Interface tables are kept; verify GOAPPS_IF is locked:
SELECT USERNAME, ACCOUNT_STATUS FROM DBA_USERS WHERE USERNAME = 'GOAPPS_IF';
