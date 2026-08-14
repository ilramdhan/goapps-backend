-- Reverse of 000477, steps 4 -> 2 in exact inverse order.
--
-- NOTE: step 3 of the up migration (NULLing 179 duplicate mbh_vs_number values —
-- 177 rows holding the sentinel '0' plus 2 rows colliding on '16728') is
-- IRREVERSIBLE BY DESIGN. The original values are not recoverable from this
-- script; they must be refilled via re-import (decision D7). Affected MB costings:
--   MGT WOLFY BL 5106 N-D-04560-B  (WOLFY BL BR, created 2025-11-10)
--   MGT WOLFY BL 5106-D-01014-C    (WOLFY BL,    created 2020-07-17)

-- Step 4 reversed: drop the partial unique indexes
DROP INDEX IF EXISTS uix_mst_mb_head_vs_number;
DROP INDEX IF EXISTS uix_mst_mb_head_dev_code;

-- Step 2 reversed: drop the child shade table (its indexes go with it)
DROP INDEX IF EXISTS idx_mbhs_mbh_id;
DROP INDEX IF EXISTS uq_mbhs_code;
DROP INDEX IF EXISTS uq_mbhs_seq;
DROP TABLE IF EXISTS mst_mb_head_shade;

-- Step 1 reversed: drop the header column
ALTER TABLE mst_mb_head DROP COLUMN IF EXISTS mbh_no_of_process;
