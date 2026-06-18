-- 000395: Extend mst_parameter for MASTER_LOOKUP type and fill-group system.

BEGIN;

-- 1. Drop old CHECK, add extended CHECK including MASTER_LOOKUP
ALTER TABLE mst_parameter
    DROP CONSTRAINT IF EXISTS mst_parameter_param_category_check;
ALTER TABLE mst_parameter
    ADD CONSTRAINT mst_parameter_param_category_check
        CHECK (param_category IN ('INPUT', 'RATE', 'CALCULATED', 'MASTER_LOOKUP'));

-- 2. Add 2 new columns
ALTER TABLE mst_parameter
    ADD COLUMN IF NOT EXISTS lookup_fill_group_code VARCHAR(20),
    ADD COLUMN IF NOT EXISTS lookup_source_column   VARCHAR(50);

COMMENT ON COLUMN mst_parameter.lookup_fill_group_code IS
    'If set, this param is a child of the MASTER_LOOKUP param with this param_code. Auto-added when parent is added to CAPP.';
COMMENT ON COLUMN mst_parameter.lookup_source_column IS
    'Column name in the master entity to read value from (e.g., mc_speed). References mst_lookup_master_column.lmc_column_name.';

-- 3. Set trigger params to MASTER_LOOKUP
UPDATE mst_parameter SET param_category = 'MASTER_LOOKUP', updated_at = NOW(), updated_by = 'migration_000395'
WHERE param_code IN ('MC_NAME','INTERMINGLE','STD_LOSS_GRADE','BC_LOSS_GRADE','MB_CODE','CAP_PACK_CODE','DEL_PACK_CODE')
  AND deleted_at IS NULL;

-- 4. Set child params: lookup_fill_group_code + lookup_source_column
UPDATE mst_parameter SET lookup_fill_group_code = 'MC_NAME', lookup_source_column = 'mc_speed'        WHERE param_code = 'MC_SPEED'         AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MC_NAME', lookup_source_column = 'mc_efficiency'   WHERE param_code = 'MC_EFFICIENCY'    AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MC_NAME', lookup_source_column = 'no_of_position'  WHERE param_code = 'NO_OF_POSITION'   AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MC_NAME', lookup_source_column = 'no_of_end'       WHERE param_code = 'NO_OF_END'        AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MC_NAME', lookup_source_column = 'machine_rpm'     WHERE param_code = 'MACHINE_RPM'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MC_NAME', lookup_source_column = 'power_per_day'   WHERE param_code = 'POWER_PER_DAY'    AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'INTERMINGLE',    lookup_source_column = 'intm_cost_per_kg'  WHERE param_code = 'INTERMINGLE_COST'  AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'STD_LOSS_GRADE', lookup_source_column = 'bc_perc'           WHERE param_code = 'BC_PERC'           AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'STD_LOSS_GRADE', lookup_source_column = 'non_std_perc'      WHERE param_code = 'NON_STD_PERC'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'STD_LOSS_GRADE', lookup_source_column = 'bc_recovery_rate'  WHERE param_code = 'BC_RECOVERY_RATE'  AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MB_CODE',        lookup_source_column = 'mbh_dozing'        WHERE param_code = 'MB_DOZING_PCT'     AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'MB_CODE',        lookup_source_column = 'mbh_mgt_name'      WHERE param_code = 'MB_DYE_NAME'       AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'CAP_PACK_CODE',  lookup_source_column = 'no_of_bob'         WHERE param_code = 'CAP_NO_OF_BOB'     AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'CAP_PACK_CODE',  lookup_source_column = 'bbcr_bob_rate_mkt' WHERE param_code = 'CAP_BOB_RATE'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'CAP_PACK_CODE',  lookup_source_column = 'bbcr_box_rate_mkt' WHERE param_code = 'CAP_BOX_RATE'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'DEL_PACK_CODE',  lookup_source_column = 'no_of_bob'         WHERE param_code = 'DEL_NO_OF_BOB'     AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'DEL_PACK_CODE',  lookup_source_column = 'bbcr_bob_rate_mkt' WHERE param_code = 'DEL_BOB_RATE'      AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code = 'DEL_PACK_CODE',  lookup_source_column = 'bbcr_box_rate_mkt' WHERE param_code = 'DEL_BOX_RATE'      AND deleted_at IS NULL;

COMMIT;
