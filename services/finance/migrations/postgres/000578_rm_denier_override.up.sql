-- 000578 (rm-denier-override) — per-product RM denier override.
--
-- Finance need (2026-10-17): a DENIER-mode product A whose route uses RM
-- group B and RM product AA must be able to use a different denier for B
-- and/or AA when computing ITS OWN RM ratio, without changing the DENIER of
-- AA or the denier of group B (that would change every product using them).
--
-- 1. cost_product_rm_denier_override: one row per (owner product, RM
--    identity). The key is the RM identity (rm_product_sys_id for PRODUCT,
--    rm_group_code for GROUP), not a route line id, so an override survives
--    route edits / forks / attaches. cpdo_denier is the FINAL line denier
--    (for spandex groups it is NOT divided by SPANDEX_DRAFT). Rows are
--    hard-deleted; every change is written to the cost audit log by the app.
-- 2. cost_product_master.cpm_duplicated_from_sys_id: parent → copy link set
--    by product duplicate / route fork, so an override on a parent can be
--    applied to its direct copies. Products duplicated before this migration
--    have no link (NULL).
--
-- Purely additive: new table + new nullable column, no data change. No
-- literal ids. Idempotent: IF NOT EXISTS everywhere, a second run is a no-op.

BEGIN;

SET LOCAL lock_timeout = '10s';

CREATE TABLE IF NOT EXISTS cost_product_rm_denier_override (
    cpdo_id                BIGSERIAL     PRIMARY KEY,
    cpdo_product_sys_id    BIGINT        NOT NULL
        REFERENCES cost_product_master (cpm_product_sys_id) ON DELETE CASCADE,
    cpdo_rm_type           VARCHAR(10)   NOT NULL,
    cpdo_rm_product_sys_id BIGINT        NULL
        REFERENCES cost_product_master (cpm_product_sys_id) ON DELETE CASCADE,
    cpdo_rm_group_code     VARCHAR(30)   NULL,
    cpdo_denier            NUMERIC(18,6) NOT NULL,
    cpdo_reason            TEXT          NOT NULL,
    cpdo_created_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    cpdo_created_by        VARCHAR(64)   NOT NULL,
    cpdo_updated_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    cpdo_updated_by        VARCHAR(64)   NOT NULL,
    CONSTRAINT chk_cpdo_rm_type CHECK (cpdo_rm_type IN ('PRODUCT', 'GROUP')),
    CONSTRAINT chk_cpdo_denier_positive CHECK (cpdo_denier > 0),
    CONSTRAINT chk_cpdo_reason_not_blank CHECK (btrim(cpdo_reason) <> ''),
    CONSTRAINT chk_cpdo_rm_identity CHECK (
        (cpdo_rm_type = 'PRODUCT' AND cpdo_rm_product_sys_id IS NOT NULL AND cpdo_rm_group_code IS NULL)
     OR (cpdo_rm_type = 'GROUP'   AND cpdo_rm_group_code IS NOT NULL AND cpdo_rm_product_sys_id IS NULL)
    )
);

COMMENT ON TABLE cost_product_rm_denier_override IS
    'Per-product RM denier override (000578): final line denier used by the owner product''s DENIER RM ratio';
COMMENT ON COLUMN cost_product_rm_denier_override.cpdo_denier IS
    'Final line denier (GROUP spandex: NOT divided by SPANDEX_DRAFT)';

CREATE UNIQUE INDEX IF NOT EXISTS uk_cpdo_product_rm_product
    ON cost_product_rm_denier_override (cpdo_product_sys_id, cpdo_rm_product_sys_id)
    WHERE cpdo_rm_type = 'PRODUCT';

CREATE UNIQUE INDEX IF NOT EXISTS uk_cpdo_product_rm_group
    ON cost_product_rm_denier_override (cpdo_product_sys_id, cpdo_rm_group_code)
    WHERE cpdo_rm_type = 'GROUP';

CREATE INDEX IF NOT EXISTS idx_cpdo_product
    ON cost_product_rm_denier_override (cpdo_product_sys_id);

CREATE INDEX IF NOT EXISTS idx_cpdo_rm_product
    ON cost_product_rm_denier_override (cpdo_rm_product_sys_id)
    WHERE cpdo_rm_product_sys_id IS NOT NULL;

ALTER TABLE cost_product_master
    ADD COLUMN IF NOT EXISTS cpm_duplicated_from_sys_id BIGINT NULL
        REFERENCES cost_product_master (cpm_product_sys_id) ON DELETE SET NULL;

COMMENT ON COLUMN cost_product_master.cpm_duplicated_from_sys_id IS
    'Source product of a duplicate / route fork (000578); NULL for originals and pre-000578 copies';

CREATE INDEX IF NOT EXISTS idx_cpm_duplicated_from
    ON cost_product_master (cpm_duplicated_from_sys_id)
    WHERE cpm_duplicated_from_sys_id IS NOT NULL;

COMMIT;
