-- Revert to the original eight trigger types.
ALTER TABLE cost_notification
    DROP CONSTRAINT IF EXISTS chk_cn_trigger_type;

ALTER TABLE cost_notification
    ADD CONSTRAINT chk_cn_trigger_type CHECK (
        cn_trigger_type IN (
            'STATUS_CHANGE', 'MENTION', 'ASSIGNED', 'FEASIBILITY',
            'COMMENT_ADDED', 'ROUTING_PROMOTED', 'REQUEST_REJECTED', 'REQUEST_CLOSED'
        )
    );
