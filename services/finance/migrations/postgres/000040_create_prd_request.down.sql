DROP TABLE IF EXISTS prd_request_sequence;

DROP INDEX IF EXISTS idx_prd_request_due_date;
DROP INDEX IF EXISTS idx_prd_request_fts;
DROP INDEX IF EXISTS idx_prd_request_specs_gin;
DROP INDEX IF EXISTS idx_prd_request_resolved_product;
DROP INDEX IF EXISTS idx_prd_request_assigned;
DROP INDEX IF EXISTS idx_prd_request_requester;
DROP INDEX IF EXISTS idx_prd_request_status;
DROP INDEX IF EXISTS uk_prd_request_ticket_no;
DROP TABLE IF EXISTS prd_request;
