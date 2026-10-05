-- ============================================================================
-- M-ERP-1b ROLLBACK (DBA ONLY, after sign-off)
-- Purpose        : drop ONLY the 3 new V_GOAPPS_* views. Legacy objects untouched.
--                  Run M-ERP-4b rollback (recompile backups) FIRST if repointed.
-- Target schema  : MGTDAT
-- Executed by    : DBA
-- Twin of        : M-ERP-1b_views.sql
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK

BEGIN EXECUTE IMMEDIATE 'DROP VIEW MGTDAT.V_GOAPPS_STD_COST_CUR';
EXCEPTION WHEN OTHERS THEN IF SQLCODE NOT IN (-942, -4080, -2289) THEN RAISE; END IF; END;
/
BEGIN EXECUTE IMMEDIATE 'DROP VIEW MGTDAT.V_GOAPPS_STD_COST_LATEST';
EXCEPTION WHEN OTHERS THEN IF SQLCODE NOT IN (-942, -4080, -2289) THEN RAISE; END IF; END;
/
BEGIN EXECUTE IMMEDIATE 'DROP VIEW MGTDAT.V_GOAPPS_ADJ_DEMAND';
EXCEPTION WHEN OTHERS THEN IF SQLCODE NOT IN (-942, -4080, -2289) THEN RAISE; END IF; END;
/
