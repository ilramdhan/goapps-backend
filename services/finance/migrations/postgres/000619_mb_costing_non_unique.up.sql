-- 000619: MB costing names may repeat across live heads (match the Finance Excel exactly).
-- mbh_oracle_sys_id stays UNIQUE. Lookups by name stay fast via a plain partial index.
DROP INDEX IF EXISTS uix_mst_mb_head_mb_costing;

CREATE INDEX IF NOT EXISTS idx_mst_mb_head_mb_costing
    ON mst_mb_head (mbh_mb_costing) WHERE deleted_at IS NULL;
