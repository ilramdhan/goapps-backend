-- 000498: add mbs_lusture_code to mst_mb_spin.
--
-- Plain additive column, nullable, no default — mirrors the "column-only,
-- no backfill UPDATE needed" shape used elsewhere in this file's history
-- (e.g. 000486, 000496). VARCHAR(10) per task spec; no CHECK constraint,
-- no index, no seed data — none of that was requested, and none is added
-- here (scope: this column only).

BEGIN;

ALTER TABLE mst_mb_spin
    ADD COLUMN IF NOT EXISTS mbs_lusture_code VARCHAR(10);

COMMENT ON COLUMN mst_mb_spin.mbs_lusture_code IS
  'Lusture code for MB Spin.';

COMMIT;
