DROP TRIGGER IF EXISTS cal_no_update ON cost_audit_log;
DROP TRIGGER IF EXISTS cal_no_delete ON cost_audit_log;
DROP FUNCTION IF EXISTS cost_audit_log_forbid_mutation();
DROP TABLE IF EXISTS cost_audit_log;
