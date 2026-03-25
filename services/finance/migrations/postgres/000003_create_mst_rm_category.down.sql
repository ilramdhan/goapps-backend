-- Drop Raw Material Category table and indexes
DROP INDEX IF EXISTS idx_mst_rm_category_search;
DROP INDEX IF EXISTS idx_mst_rm_category_created_at;
DROP INDEX IF EXISTS idx_mst_rm_category_active;
DROP INDEX IF EXISTS idx_mst_rm_category_code;
DROP TABLE IF EXISTS mst_rm_category;
