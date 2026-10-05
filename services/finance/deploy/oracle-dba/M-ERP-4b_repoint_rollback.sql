-- ============================================================================
-- Purpose        : M-ERP-4b rollback: recompile the original DDL backed up by M-ERP-4a
-- Target schema  : MGTDAT
-- M-ERP-4b rollback: recompile the original DDL backed up by M-ERP-4a
-- Executed by    : DBA. Requires the spool file from 4a for the same &&PERIOD.
-- ============================================================================
WHENEVER SQLERROR EXIT FAILURE ROLLBACK
SET DEFINE ON
DEFINE PERIOD = &1
@m_erp_4a_ddl_backup_&&PERIOD..sql
