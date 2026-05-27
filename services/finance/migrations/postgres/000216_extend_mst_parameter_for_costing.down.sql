DROP INDEX IF EXISTS idx_mst_parameter_display_group;
DROP INDEX IF EXISTS idx_mst_parameter_required_active;

ALTER TABLE mst_parameter
    DROP COLUMN IF EXISTS display_group,
    DROP COLUMN IF EXISTS display_order,
    DROP COLUMN IF EXISTS lookup_master_code,
    DROP COLUMN IF EXISTS is_period_dependent,
    DROP COLUMN IF EXISTS is_required_for_costing,
    DROP COLUMN IF EXISTS owner_department;
