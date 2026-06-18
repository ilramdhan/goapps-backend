-- 000393: Set lookup_master_code on INPUT/TEXT params that trigger master auto-fill.
-- These params drive the MasterLookupField dropdown + auto-populate in the product CAPP form.
-- No schema change — only data update on existing lookup_master_code column.

BEGIN;

-- MC_NAME → mst_machine: auto-fills MC_SPEED, MC_EFFICIENCY, NO_OF_POSITION, NO_OF_END, MACHINE_RPM, POWER_PER_DAY
UPDATE mst_parameter
SET lookup_master_code = 'MACHINE',
    updated_at = NOW(), updated_by = 'migration_000393'
WHERE param_code = 'MC_NAME' AND deleted_at IS NULL;

-- INTERMINGLE → mst_intermingling: auto-fills INTERMINGLE_COST
UPDATE mst_parameter
SET lookup_master_code = 'INTERMINGLING',
    updated_at = NOW(), updated_by = 'migration_000393'
WHERE param_code = 'INTERMINGLE' AND deleted_at IS NULL;

-- STD_LOSS_GRADE / BC_LOSS_GRADE → mst_product_grade: auto-fills BC_PERC, NON_STD_PERC, BC_RECOVERY_RATE
UPDATE mst_parameter
SET lookup_master_code = 'PRODUCT_GRADE',
    updated_at = NOW(), updated_by = 'migration_000393'
WHERE param_code IN ('STD_LOSS_GRADE', 'BC_LOSS_GRADE') AND deleted_at IS NULL;

-- MB_CODE → mst_mb_head: auto-fills MB_DOZING_PCT, MB_DYE_NAME (text)
UPDATE mst_parameter
SET lookup_master_code = 'MB_HEAD',
    updated_at = NOW(), updated_by = 'migration_000393'
WHERE param_code = 'MB_CODE' AND deleted_at IS NULL;

-- CAP_PACK_CODE → mst_box_bobbin_cost: auto-fills CAP_NO_OF_BOB, CAP_BOB_RATE, CAP_BOX_RATE
-- DEL_PACK_CODE → mst_box_bobbin_cost: auto-fills DEL_NO_OF_BOB, DEL_BOB_RATE, DEL_BOX_RATE
UPDATE mst_parameter
SET lookup_master_code = 'BOX_BOBBIN_COST',
    updated_at = NOW(), updated_by = 'migration_000393'
WHERE param_code IN ('CAP_PACK_CODE', 'DEL_PACK_CODE') AND deleted_at IS NULL;

COMMIT;
