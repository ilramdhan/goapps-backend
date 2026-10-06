-- 000553 — extend chk_cal_operation with the ERP audit operations
-- (design Part 1 §4.6; plan-01 P0-T4).
--
-- Keeps every existing 000215 value and adds 10 ERP operations (all <= 30
-- chars; cal_operation is VARCHAR(30)):
--   ERP_LINK, ERP_PERIOD_LOCK, ERP_PERIOD_UNLOCK,
--   ERP_PUSH, ERP_VALUATE, ERP_ADJ_APPROVE, ERP_RESTORE, ERP_BATCH_LOCK,
--   ERP_ATTR_BACKFILL, ERP_WARN_ACK
--
-- Pure widening: every row valid before is still valid.

BEGIN;

ALTER TABLE cost_audit_log
    DROP CONSTRAINT IF EXISTS chk_cal_operation;

ALTER TABLE cost_audit_log
    ADD CONSTRAINT chk_cal_operation CHECK (
        cal_operation IN (
            'INSERT', 'UPDATE', 'DELETE',
            'STATUS_CHANGE', 'FEASIBILITY', 'CLASSIFICATION_OVERRIDE',
            'ASSIGN', 'PROMOTE', 'HIDE', 'UNHIDE',
            'RULE_CREATE', 'RULE_UPDATE', 'RULE_DELETE',
            'ERP_LINK', 'ERP_PERIOD_LOCK', 'ERP_PERIOD_UNLOCK',
            'ERP_PUSH', 'ERP_VALUATE', 'ERP_ADJ_APPROVE', 'ERP_RESTORE', 'ERP_BATCH_LOCK',
            'ERP_ATTR_BACKFILL', 'ERP_WARN_ACK'
        )
    );

COMMIT;
