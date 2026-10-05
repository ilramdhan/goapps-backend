-- 000548 down — delete only the rows this seed inserted (created_by marker).
--
-- cost_erp_grade.ceg_grade_group values applied by the up migration are left
-- in place: they cannot be told apart from the same value set by Costing or by
-- P0-T15b. The column itself is dropped by 000547 down.

BEGIN;

DELETE FROM cst_erp_valloss_rule     WHERE created_by = 'migration:000548';
DELETE FROM cst_erp_sell_price       WHERE created_by = 'migration:000548';
DELETE FROM cst_erp_grade_group_seed WHERE created_by = 'migration:000548';

COMMIT;
