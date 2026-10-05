-- ============================================================================
-- Purpose        : M-ERP-4c: disable legacy cost writers (state change only, nothing dropped)
-- Target schema  : MGTDAT
-- M-ERP-4c: disable legacy cost writers (state change only, nothing dropped)
-- Executed by    : DBA, at CUTOVER only, after 4a and 4b
-- Rollback twin  : M-ERP-4c_disable_legacy_writers_rollback.sql
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
ALTER TRIGGER MGTDAT.ODBTRG_ITEMCOSTVALLOSS_MGT DISABLE;
ALTER TRIGGER MGTDAT.ODBTRG_SELLPRICECOST_MGT   DISABLE;
ALTER TRIGGER MGTDAT.ODBTRG_FG_STD_COST         DISABLE;
-- Legacy cost jobs: take names from the 4a scheduler listing, then uncomment.
-- EXEC DBMS_SCHEDULER.DISABLE('<job name from 4a listing>');
