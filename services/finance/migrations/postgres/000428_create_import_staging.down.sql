-- Rollback: drop bulk-import v2 ETL staging tables (reverse order of creation).
BEGIN;

DROP TABLE IF EXISTS stg_import_error;
DROP TABLE IF EXISTS stg_import_route_rm;
DROP TABLE IF EXISTS stg_import_route_seq;
DROP TABLE IF EXISTS stg_import_route_head;
DROP TABLE IF EXISTS stg_import_applicable_param;
DROP TABLE IF EXISTS stg_import_product_parameter;
DROP TABLE IF EXISTS stg_import_product_master;

COMMIT;
