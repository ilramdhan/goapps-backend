-- 000579 (rm-ratio-mode-per-product) — per-product RM ratio mode.
--
-- Finance need (2026-10-19): the legacy denier seed (000580) switches only
-- the products whose RM ratio Finance overrides to DENIER mode, while the
-- rest of their product type (e.g. PTY/ATY) keeps INPUT mode. Today the mode
-- lives only on cost_product_type.cpt_rm_ratio_mode (000570).
--
-- cost_product_master.cpm_rm_ratio_mode: NULL = follow the product type
-- (today's behavior), 'INPUT' / 'DENIER' = per-product override. The engine
-- and route graph read COALESCE(cpm_rm_ratio_mode, cpt_rm_ratio_mode).
--
-- Purely additive: new nullable column, no data change, so every product
-- resolves exactly as before. Idempotent: ADD COLUMN IF NOT EXISTS and a
-- DO-block guarded ADD CONSTRAINT; a second run is a no-op.

BEGIN;

SET LOCAL lock_timeout = '10s';

ALTER TABLE cost_product_master
    ADD COLUMN IF NOT EXISTS cpm_rm_ratio_mode VARCHAR(10) NULL;

DO $$
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_cpm_rm_ratio_mode'
    ) THEN
        ALTER TABLE cost_product_master
            ADD CONSTRAINT chk_cpm_rm_ratio_mode
            CHECK (cpm_rm_ratio_mode IS NULL OR cpm_rm_ratio_mode IN ('INPUT', 'DENIER'));
    END IF;
END $$;

COMMENT ON COLUMN cost_product_master.cpm_rm_ratio_mode IS
    'Per-product RM ratio mode (000579): NULL = follow cost_product_type.cpt_rm_ratio_mode; INPUT or DENIER overrides it';

COMMIT;
