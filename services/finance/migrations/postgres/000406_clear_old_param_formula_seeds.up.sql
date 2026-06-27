-- Delete in FK dependency order.
-- WARNING: removes ALL per-product parameter values (cost_product_parameter).
-- Re-fill required after migration.

DELETE FROM formula_param;
DELETE FROM mst_formula;
DELETE FROM cost_product_parameter;
DELETE FROM cost_product_applicable_param;
DELETE FROM mst_parameter;
