-- 000552 down — drop only the approval-trail columns. cpc_verified_* was
-- never modified by the up migration, so nothing else needs restoring.

BEGIN;

ALTER TABLE cst_product_cost
    DROP COLUMN IF EXISTS cpc_approved_by,
    DROP COLUMN IF EXISTS cpc_approved_at;

COMMIT;
