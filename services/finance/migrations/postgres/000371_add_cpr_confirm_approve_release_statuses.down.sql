-- Revert CONFIRMED/APPROVED/RELEASED status values.
-- WARNING: will fail if any rows have these statuses.
ALTER TABLE cost_product_request DROP CONSTRAINT IF EXISTS chk_cpr_status;
ALTER TABLE cost_product_request ADD CONSTRAINT chk_cpr_status CHECK (
    cpr_status IN (
        'DRAFT', 'SUBMITTED', 'UNDER_REVIEW', 'ROUTING_DEFINED',
        'PARAMETER_PENDING', 'PARAMETER_COMPLETE', 'COSTING_DONE',
        'QUOTED', 'QUOTE_READY', 'CLOSED', 'REJECTED'
    )
);
