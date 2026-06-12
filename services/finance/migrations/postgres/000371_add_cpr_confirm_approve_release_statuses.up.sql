-- Migration 000371: Add CONFIRMED, APPROVED, RELEASED statuses to cost_product_request.
-- These new states extend the approval chain after PARAMETER_COMPLETE:
--   PARAMETER_COMPLETE → CONFIRMED → APPROVED → RELEASED → COSTING_DONE

ALTER TABLE cost_product_request DROP CONSTRAINT IF EXISTS chk_cpr_status;
ALTER TABLE cost_product_request ADD CONSTRAINT chk_cpr_status CHECK (
    cpr_status IN (
        'DRAFT', 'SUBMITTED', 'UNDER_REVIEW', 'ROUTING_DEFINED',
        'PARAMETER_PENDING', 'PARAMETER_COMPLETE',
        'CONFIRMED', 'APPROVED', 'RELEASED',
        'COSTING_DONE', 'QUOTED', 'QUOTE_READY', 'CLOSED', 'REJECTED'
    )
);
