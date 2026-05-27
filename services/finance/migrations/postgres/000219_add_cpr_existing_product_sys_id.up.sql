-- Add cpr_existing_product_sys_id so QUOTE_READY-via-UseExistingCosting traces
-- back to a concrete cost_product_master row. NULL when the request isn't
-- reusing an existing product (classification = "new" or not yet decided).

ALTER TABLE cost_product_request
    ADD COLUMN IF NOT EXISTS cpr_existing_product_sys_id BIGINT
        REFERENCES cost_product_master(cpm_product_sys_id);

CREATE INDEX IF NOT EXISTS idx_cpr_existing_product
    ON cost_product_request(cpr_existing_product_sys_id)
    WHERE cpr_existing_product_sys_id IS NOT NULL;

COMMENT ON COLUMN cost_product_request.cpr_existing_product_sys_id IS
    'When request classification is verified as EXISTING and UseExistingCosting is invoked, points to the product master whose costing is reused.';
