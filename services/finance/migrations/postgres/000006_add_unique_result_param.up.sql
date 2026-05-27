-- Add unique constraint on result_param_id (each param can only be output of one formula).
CREATE UNIQUE INDEX IF NOT EXISTS idx_mst_formula_result_param_unique
    ON mst_formula (result_param_id)
    WHERE deleted_at IS NULL;
