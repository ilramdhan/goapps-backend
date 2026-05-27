-- IAM Service Database Migrations
-- 000030 (down): Drop user_company_mappings + mst_company_mapping.

DROP INDEX IF EXISTS unique_user_primary_mapping;
DROP INDEX IF EXISTS idx_ucm_mapping;
DROP TABLE IF EXISTS user_company_mappings;

DROP INDEX IF EXISTS unique_mapping_combo;
DROP INDEX IF EXISTS idx_company_mapping_section;
DROP INDEX IF EXISTS idx_company_mapping_department;
DROP INDEX IF EXISTS idx_company_mapping_division;
DROP INDEX IF EXISTS idx_company_mapping_company;
DROP INDEX IF EXISTS idx_company_mapping_active;
DROP INDEX IF EXISTS idx_company_mapping_code;
DROP TABLE IF EXISTS mst_company_mapping;
