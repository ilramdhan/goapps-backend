-- 000547 down — drop only the objects 000547 created.

BEGIN;

DROP TABLE IF EXISTS cst_erp_valloss_rule;
DROP TABLE IF EXISTS cst_erp_sell_price;
DROP TABLE IF EXISTS cst_erp_grade_group_seed;

DROP INDEX IF EXISTS idx_ceg_grade_group;
ALTER TABLE cost_erp_grade DROP COLUMN IF EXISTS ceg_grade_group;

COMMIT;
