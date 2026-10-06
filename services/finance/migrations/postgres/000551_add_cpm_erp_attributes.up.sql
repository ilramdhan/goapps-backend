-- 000551 — ERP attributes on cost_product_master (design Part 1 §4.4, M-7;
-- plan-01 P0-T4).
--
-- Five nullable columns pushed to Oracle with each derived std row:
--   cpm_erp_fg_type        -> GSC_FG_TYPE / FLEX_05 ('Type 1'..'Type 13')
--   cpm_erp_chp_item_code  -> GSC_CHP_ITEM_CODE / FLEX_04
--   cpm_erp_ms_batch_item  -> FLEX_12 (VARCHAR(12), C-11)
--   cpm_erp_item_type      -> compat
--   cpm_erp_prd_per_day    -> compat
--
-- No defaults, no backfill: every existing row gets NULL, so no existing
-- output changes. The ERP link key stays (cpm_erp_item_code, cpm_shade_code)
-- (D-LINK, sql-1 F-7/F-8); cpm_erp_grade_code_1/2 are untouched and unused.
-- The attribute backfill is a later job (P3-T6), not this migration.

BEGIN;

ALTER TABLE cost_product_master
    ADD COLUMN IF NOT EXISTS cpm_erp_fg_type       VARCHAR(15)   NULL,
    ADD COLUMN IF NOT EXISTS cpm_erp_chp_item_code VARCHAR(50)   NULL,
    ADD COLUMN IF NOT EXISTS cpm_erp_ms_batch_item VARCHAR(12)   NULL,
    ADD COLUMN IF NOT EXISTS cpm_erp_item_type     VARCHAR(20)   NULL,
    ADD COLUMN IF NOT EXISTS cpm_erp_prd_per_day   NUMERIC(20,5) NULL;

COMMENT ON COLUMN cost_product_master.cpm_erp_fg_type       IS 'ERP FG type (Type 1..Type 13), pushed as GSC_FG_TYPE / FLEX_05 (000551).';
COMMENT ON COLUMN cost_product_master.cpm_erp_chp_item_code IS 'ERP chip item code, pushed as GSC_CHP_ITEM_CODE / FLEX_04 (000551).';
COMMENT ON COLUMN cost_product_master.cpm_erp_ms_batch_item IS 'ERP master-batch item, pushed as FLEX_12, max 12 chars (000551, C-11).';
COMMENT ON COLUMN cost_product_master.cpm_erp_item_type     IS 'ERP item type, compatibility attribute (000551).';
COMMENT ON COLUMN cost_product_master.cpm_erp_prd_per_day   IS 'ERP production per day, compatibility attribute (000551).';

COMMIT;
