-- MB Recipe: required fields, unique constraints, multi-shade.
--
-- Steps (order is load-bearing):
--   1. Add mbh_no_of_process to mst_mb_head (user-selected S/D/T; distinct from the
--      frozen mbh_param_no_of_process snapshot written by FreezeParams).
--   2. Create mst_mb_head_shade for up to 2 extra shade codes per MB (3 incl. header).
--   3. Clean up duplicate mbh_vs_number values -- MUST run before step 4 or the
--      unique index creation fails.
--   4. Add the two partial unique indexes on mbh_dev_code and mbh_vs_number.
--
-- !! DESTRUCTIVE — READ BEFORE APPLYING !!
-- Step 3 permanently destroys 179 mbh_vs_number values on live rows:
--   * 177 rows holding the sentinel '0' (a placeholder, never a real VS number)
--   * 2 rows genuinely colliding on '16728':
--       - MGT WOLFY BL 5106 N-D-04560-B  (WOLFY BL BR, created 2025-11-10)
--       - MGT WOLFY BL 5106-D-01014-C    (WOLFY BL,    created 2020-07-17)
-- The .down.sql CANNOT restore these values. Finance must decide which MB owns
-- 16728 and refill both MB costings above via re-import (decision D7).

-- ---------------------------------------------------------------------------
-- Step 1: new header column
-- ---------------------------------------------------------------------------
ALTER TABLE mst_mb_head ADD COLUMN IF NOT EXISTS mbh_no_of_process VARCHAR(10);

COMMENT ON COLUMN mst_mb_head.mbh_no_of_process IS 'User-selected number of process (S/D/T from mst_mb_param_option NO_OF_PROCESS); seeds the frozen mbh_param_no_of_process at Validate time';

-- ---------------------------------------------------------------------------
-- Step 2: child shade table (shades #2 and #3; shade #1 lives on the header)
-- ---------------------------------------------------------------------------
CREATE TABLE IF NOT EXISTS mst_mb_head_shade (
    mbhs_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    mbhs_mbh_id       UUID NOT NULL REFERENCES mst_mb_head (mbh_id) ON DELETE CASCADE,
    mbhs_seq_no       INTEGER NOT NULL,
    mbhs_shade_code   VARCHAR(20)  NOT NULL,
    mbhs_shade_name   VARCHAR(100) NOT NULL,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by        VARCHAR(100) NOT NULL,
    updated_at        TIMESTAMPTZ,
    updated_by        VARCHAR(100),
    deleted_at        TIMESTAMPTZ,
    deleted_by        VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uq_mbhs_seq
  ON mst_mb_head_shade (mbhs_mbh_id, mbhs_seq_no)
  WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_mbhs_code
  ON mst_mb_head_shade (mbhs_mbh_id, mbhs_shade_code)
  WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mbhs_mbh_id ON mst_mb_head_shade (mbhs_mbh_id);

COMMENT ON TABLE mst_mb_head_shade IS 'Extra shade codes for an MB recipe (max 2 rows, enforced at application layer); label-only metadata with no costing impact';

-- ---------------------------------------------------------------------------
-- Step 3: pre-index cleanup (IRREVERSIBLE — see header)
-- ---------------------------------------------------------------------------

-- '0' is a placeholder, never a real VS number
UPDATE mst_mb_head SET mbh_vs_number = NULL
WHERE deleted_at IS NULL AND mbh_vs_number = '0';

-- Genuine collision. Finance decides which MB owns 16728; both are cleared
-- so the index can be created, and the correct value returns via re-import (D7).
--   MGT WOLFY BL 5106 N-D-04560-B  (WOLFY BL BR, 2025-11-10)
--   MGT WOLFY BL 5106-D-01014-C    (WOLFY BL,    2020-07-17)
UPDATE mst_mb_head SET mbh_vs_number = NULL
WHERE deleted_at IS NULL AND mbh_vs_number = '16728';

-- ---------------------------------------------------------------------------
-- Step 4: partial unique indexes (excluding soft-deleted, NULL and empty string)
-- ---------------------------------------------------------------------------
CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_mb_head_dev_code
  ON mst_mb_head (mbh_dev_code)
  WHERE deleted_at IS NULL AND mbh_dev_code IS NOT NULL AND mbh_dev_code <> '';

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_mb_head_vs_number
  ON mst_mb_head (mbh_vs_number)
  WHERE deleted_at IS NULL AND mbh_vs_number IS NOT NULL AND mbh_vs_number <> '';
