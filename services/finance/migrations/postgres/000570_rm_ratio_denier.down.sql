-- 000570 down — remove denier-based RM ratio schema/seed.
-- Drops every SPANDEX_DRAFT value/CAPP row (FK to mst_parameter), the
-- parameter itself, cst_rm_group_head.denier and cost_product_type.cpt_rm_ratio_mode.

BEGIN;

DELETE FROM cost_product_parameter
WHERE cpp_param_id IN (SELECT id FROM mst_parameter WHERE param_code = 'SPANDEX_DRAFT');

DELETE FROM cost_product_applicable_param
WHERE capp_param_id IN (SELECT id FROM mst_parameter WHERE param_code = 'SPANDEX_DRAFT');

DELETE FROM mst_parameter
WHERE param_code = 'SPANDEX_DRAFT';

ALTER TABLE cst_rm_group_head
    DROP CONSTRAINT IF EXISTS chk_rm_group_denier_positive;

ALTER TABLE cst_rm_group_head
    DROP COLUMN IF EXISTS denier;

ALTER TABLE cost_product_type
    DROP CONSTRAINT IF EXISTS chk_cpt_rm_ratio_mode;

ALTER TABLE cost_product_type
    DROP COLUMN IF EXISTS cpt_rm_ratio_mode;

COMMIT;
