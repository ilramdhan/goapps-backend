-- 000388: Melange Batch head master (MEL product type only). Feeds MB_RATE_MKT CAPP param.
-- No seed — data from one-time Oracle import (CST_MST_MB_HEAD, ~4169 rows).
BEGIN;

CREATE TABLE IF NOT EXISTS mst_mb_head (
    mbh_id             UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    mbh_oracle_sys_id  VARCHAR(30)   UNIQUE,
    mbh_mb_costing     VARCHAR(100)  NOT NULL,
    mbh_mgt_name       VARCHAR(100),
    mbh_denier         NUMERIC(10,2),
    mbh_filament       INTEGER,
    mbh_dozing         NUMERIC(10,4),
    mbh_is_active      BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by         VARCHAR(100)  NOT NULL,
    updated_at         TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    deleted_at         TIMESTAMPTZ,
    deleted_by         VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_mb_head_mb_costing
    ON mst_mb_head (mbh_mb_costing) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mst_mb_head_mgt_name
    ON mst_mb_head (mbh_mgt_name) WHERE mbh_is_active = TRUE;

COMMIT;
