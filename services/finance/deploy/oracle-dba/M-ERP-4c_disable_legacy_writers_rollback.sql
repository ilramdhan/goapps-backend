-- ============================================================================
-- Purpose        : M-ERP-4c rollback: re-enable the legacy writers (state change only)
-- Target schema  : MGTDAT
-- M-ERP-4c rollback: re-enable the legacy writers (state change only)
-- Executed by    : DBA
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
ALTER TRIGGER MGTDAT.ODBTRG_ITEMCOSTVALLOSS_MGT ENABLE;
ALTER TRIGGER MGTDAT.ODBTRG_SELLPRICECOST_MGT   ENABLE;
ALTER TRIGGER MGTDAT.ODBTRG_FG_STD_COST         ENABLE;
-- EXEC DBMS_SCHEDULER.ENABLE('<job name from 4a listing>');
