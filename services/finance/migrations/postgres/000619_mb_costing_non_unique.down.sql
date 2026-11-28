-- 000619 down: restore the unique index.
-- WARNING: fails if two live heads now share an mb_costing; dedupe/rename them first.
DROP INDEX IF EXISTS idx_mst_mb_head_mb_costing;

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_mb_head_mb_costing
    ON mst_mb_head (mbh_mb_costing) WHERE deleted_at IS NULL;
