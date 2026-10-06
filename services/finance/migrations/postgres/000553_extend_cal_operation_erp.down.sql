-- 000553 down — restore the 000215 chk_cal_operation list.
--
-- cost_audit_log is append-only (000215 triggers), so ERP_* rows cannot be
-- deleted. If any exist, re-adding the narrow CHECK fails and this down
-- migration aborts with nothing changed. That is intended: rolling back the
-- constraint must not silently invalidate audit history.

BEGIN;

ALTER TABLE cost_audit_log
    DROP CONSTRAINT IF EXISTS chk_cal_operation;

ALTER TABLE cost_audit_log
    ADD CONSTRAINT chk_cal_operation CHECK (
        cal_operation IN (
            'INSERT', 'UPDATE', 'DELETE',
            'STATUS_CHANGE', 'FEASIBILITY', 'CLASSIFICATION_OVERRIDE',
            'ASSIGN', 'PROMOTE', 'HIDE', 'UNHIDE',
            'RULE_CREATE', 'RULE_UPDATE', 'RULE_DELETE'
        )
    );

COMMIT;
