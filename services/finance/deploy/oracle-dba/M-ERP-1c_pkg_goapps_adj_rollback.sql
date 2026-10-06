-- ============================================================================
-- Purpose        : M-ERP-1c rollback: drop the new package only
-- Target schema  : MGTDAT
-- M-ERP-1c rollback: drop the new package only
-- Executed by    : DBA. Drops MGTDAT.PKG_GOAPPS_ADJ only (new object); nothing legacy.
-- Note           : run only if no ADJ valuation needs RESTORE_ADJ first.
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
BEGIN EXECUTE IMMEDIATE 'DROP PACKAGE MGTDAT.PKG_GOAPPS_ADJ';
EXCEPTION WHEN OTHERS THEN IF SQLCODE != -4043 THEN RAISE; END IF; END;
/
