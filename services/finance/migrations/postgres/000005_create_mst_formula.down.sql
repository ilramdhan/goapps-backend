-- Rollback: Drop formula_param and mst_formula tables
-- Drop junction table first (FK dependency on mst_formula)
DROP TABLE IF EXISTS formula_param;
DROP TABLE IF EXISTS mst_formula;
