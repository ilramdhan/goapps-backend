-- 000385: Box/bobbin packing cost master. MKT/VAL rates per period in child table.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_box_bobbin_cost (
    bbc_id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    bbc_code       VARCHAR(30)  NOT NULL,
    bbc_name       VARCHAR(100) NOT NULL,
    bbc_type       VARCHAR(20)  NOT NULL,
    no_of_bob      INTEGER      NOT NULL DEFAULT 6,
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    notes          TEXT,
    created_at     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by     VARCHAR(100) NOT NULL,
    updated_at     TIMESTAMPTZ,
    updated_by     VARCHAR(100),
    deleted_at     TIMESTAMPTZ,
    deleted_by     VARCHAR(100)
);

CREATE TABLE IF NOT EXISTS mst_box_bobbin_cost_rate (
    bbcr_id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    bbcr_bbc_id        UUID          NOT NULL REFERENCES mst_box_bobbin_cost (bbc_id),
    bbcr_period        CHAR(6)       NOT NULL,
    bbcr_bob_rate_mkt  NUMERIC(15,6) NOT NULL DEFAULT 0,
    bbcr_box_rate_mkt  NUMERIC(15,6) NOT NULL DEFAULT 0,
    bbcr_bob_rate_val  NUMERIC(15,6),
    bbcr_box_rate_val  NUMERIC(15,6),
    created_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by         VARCHAR(100)  NOT NULL,
    updated_at         TIMESTAMPTZ,
    updated_by         VARCHAR(100),
    deleted_at         TIMESTAMPTZ,
    deleted_by         VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_bbc_code
    ON mst_box_bobbin_cost (bbc_code) WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_bbcr_period
    ON mst_box_bobbin_cost_rate (bbcr_bbc_id, bbcr_period) WHERE deleted_at IS NULL;

COMMIT;
