-- 000389: Melange Batch spin detail — child of mst_mb_head. MEL type only.
-- No seed — data from one-time Oracle import (CST_MST_MB_SPIN, ~2679 rows).
BEGIN;

CREATE TABLE IF NOT EXISTS mst_mb_spin (
    mbs_id             UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    mbs_oracle_sys_id  VARCHAR(30)   UNIQUE,
    mbs_mbh_id         UUID          NOT NULL REFERENCES mst_mb_head (mbh_id),
    mbs_mgt_name       VARCHAR(100)  NOT NULL,
    mbs_denier         NUMERIC(10,2),
    mbs_filament       INTEGER,
    mbs_dozing         NUMERIC(10,4),
    mbs_mb_costing     VARCHAR(100),
    mbs_is_active      BOOLEAN       NOT NULL DEFAULT TRUE,
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by         VARCHAR(100)  NOT NULL,
    updated_at         TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    deleted_at         TIMESTAMPTZ,
    deleted_by         VARCHAR(100)
);

CREATE INDEX IF NOT EXISTS idx_mst_mb_spin_mbh_id
    ON mst_mb_spin (mbs_mbh_id) WHERE mbs_is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_mst_mb_spin_mgt_name
    ON mst_mb_spin (mbs_mgt_name) WHERE mbs_is_active = TRUE;

COMMIT;
