-- Reverse 000512. Touches ONLY rows carrying this migration's audit marker, so a
-- pre-existing 000468 run (marker 'wire_group_c_000468') is left intact.
BEGIN;

-- 1. Un-wire params this migration only UPDATED (they pre-existed from 000468).
--    Rows it INSERTED are deleted in step 2, so exclude them here.
UPDATE mst_parameter
   SET lookup_master_code     = NULL,
       lookup_fill_group_code = NULL,
       lookup_source_column   = NULL,
       updated_at = NOW(),
       updated_by = 'reseed_group_c_000512_down'
 WHERE updated_by = 'reseed_group_c_000512'
   AND created_by <> 'reseed_group_c_000512'
   AND deleted_at IS NULL;

-- 2. Drop the params this migration inserted. The wiring lives on those same
--    rows, so deleting them removes it too.
DELETE FROM mst_parameter WHERE created_by = 'reseed_group_c_000512';

COMMIT;
