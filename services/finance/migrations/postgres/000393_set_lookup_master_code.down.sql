-- Revert: clear lookup_master_code from params set in 000393.
BEGIN;
UPDATE mst_parameter
SET lookup_master_code = NULL,
    updated_at = NOW(), updated_by = 'migration_000393_down'
WHERE param_code IN ('MC_NAME', 'INTERMINGLE', 'STD_LOSS_GRADE', 'BC_LOSS_GRADE', 'MB_CODE', 'CAP_PACK_CODE', 'DEL_PACK_CODE')
  AND lookup_master_code IN ('MACHINE', 'INTERMINGLING', 'PRODUCT_GRADE', 'MB_HEAD', 'BOX_BOBBIN_COST')
  AND deleted_at IS NULL;
COMMIT;
