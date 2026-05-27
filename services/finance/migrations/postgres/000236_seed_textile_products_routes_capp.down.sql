-- 000236 down: remove TXFX_ textile products, their routes, CAPP, CPP, and
-- the seeded ITEM rm costs. ON DELETE CASCADE on cost_route_head -> _seq -> _rm
-- and on cost_product_master -> cost_product_parameter handles most of it;
-- CAPP also CASCADEs off product_sys_id.

BEGIN;

-- Route heads first (CASCADE drops seqs and rms).
DELETE FROM cost_route_head
 WHERE crh_created_by = 'seed_000236';

-- CPP + CAPP rows (CASCADE off product master would also catch these, but be
-- explicit so this can run independently if products were left in place).
DELETE FROM cost_product_parameter
 WHERE cpp_created_by = 'seed_000236';

DELETE FROM cost_product_applicable_param
 WHERE capp_created_by = 'seed_000236';

-- The product master rows themselves.
DELETE FROM cost_product_master
 WHERE cpm_created_by = 'seed_000236';

-- The seeded ITEM rm cost rows.
DELETE FROM cst_rm_cost
 WHERE created_by = 'seed_000236';

COMMIT;
