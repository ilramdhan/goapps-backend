-- 000522 down — unregister RM_GROUP_OIL and drop v_rm_group_oil.
-- Only the registry row this migration created (marker seed_000522) is removed.
-- 000523's down (run first in reverse order) already reverts OIL_NAME's
-- lookup_master_code, so no parameter still points at RM_GROUP_OIL.

BEGIN;

DELETE FROM mst_lookup_master_column
WHERE lmc_master_code = 'RM_GROUP_OIL'
  AND EXISTS (
      SELECT 1 FROM mst_lookup_master
      WHERE lm_code = 'RM_GROUP_OIL' AND created_by = 'seed_000522'
  );

DELETE FROM mst_lookup_master
WHERE lm_code = 'RM_GROUP_OIL'
  AND created_by = 'seed_000522';

DROP VIEW IF EXISTS v_rm_group_oil;

COMMIT;
