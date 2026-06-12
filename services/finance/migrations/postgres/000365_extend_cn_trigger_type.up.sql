-- 000365: Extend chk_cn_trigger_type to include fill-assignment reminder triggers.
-- SLA_OVERDUE       — fill task missed its SLA deadline.
-- PENDING_FILL      — task is ACTIVE/FILLING, filler reminder.
-- PENDING_APPROVAL  — task is APPROVAL_PENDING, approver reminder.
-- MISSING_PARAM     — required parameter values not yet filled.
-- NEW_PARAM         — new required params added after task activation.

ALTER TABLE cost_notification
    DROP CONSTRAINT IF EXISTS chk_cn_trigger_type;

ALTER TABLE cost_notification
    ADD CONSTRAINT chk_cn_trigger_type CHECK (
        cn_trigger_type IN (
            'STATUS_CHANGE', 'MENTION', 'ASSIGNED', 'FEASIBILITY',
            'COMMENT_ADDED', 'ROUTING_PROMOTED', 'REQUEST_REJECTED', 'REQUEST_CLOSED',
            'SLA_OVERDUE', 'PENDING_FILL', 'PENDING_APPROVAL',
            'MISSING_PARAM', 'NEW_PARAM'
        )
    );
