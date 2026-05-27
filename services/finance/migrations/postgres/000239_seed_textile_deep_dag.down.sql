-- 000239 down: remove everything seeded by 000239 up.

BEGIN;

DELETE FROM cost_route_rm
 WHERE crm_created_by = 'seed_000239';

DELETE FROM cost_route_seq
 WHERE crs_created_by = 'seed_000239';

DELETE FROM cost_route_head
 WHERE crh_created_by = 'seed_000239';

DELETE FROM cost_product_parameter
 WHERE cpp_created_by = 'seed_000239';

DELETE FROM cost_product_applicable_param
 WHERE capp_created_by = 'seed_000239';

DELETE FROM cost_product_master
 WHERE cpm_created_by = 'seed_000239';

COMMIT;
