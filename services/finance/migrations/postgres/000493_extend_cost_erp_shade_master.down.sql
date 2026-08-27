-- Reverse 000493. Column widenings (VARCHAR -> TEXT) are NOT narrowed back:
-- narrowing could fail on data synced in the meantime (Oracle values already
-- exceed the original VARCHAR(20)/VARCHAR(100) bounds), so this rollback is
-- best-effort like other finance migrations that widen a column.
DROP INDEX IF EXISTS idx_cost_erp_shade_code_lower;
DROP INDEX IF EXISTS idx_cost_erp_shade_name_lower;

ALTER TABLE cost_erp_shade
    DROP CONSTRAINT IF EXISTS chk_cost_erp_shade_source;

ALTER TABLE cost_erp_shade
    DROP COLUMN IF EXISTS ces_shade_short_name,
    DROP COLUMN IF EXISTS ces_shade_source,
    DROP COLUMN IF EXISTS ces_source_created_at,
    DROP COLUMN IF EXISTS ces_source_updated_at,
    DROP COLUMN IF EXISTS ces_source_created_by,
    DROP COLUMN IF EXISTS ces_source_updated_by,
    DROP COLUMN IF EXISTS created_at,
    DROP COLUMN IF EXISTS created_by,
    DROP COLUMN IF EXISTS updated_at,
    DROP COLUMN IF EXISTS updated_by;

COMMENT ON TABLE cost_erp_shade IS 'PRD Phase B §7.3.3 — Read-only replica of Oracle ERP shade master. Also used by Phase A cost_product_spec.';
