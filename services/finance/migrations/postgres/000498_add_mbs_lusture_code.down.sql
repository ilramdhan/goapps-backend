-- Down 000498 — drop exactly what the up migration added, nothing more.
--
-- ⛔ DESTRUCTIVE for mbs_lusture_code: any values written into this column
-- after 000498 ran are permanently lost. Only run this if the change is
-- truly being rolled back.

BEGIN;

ALTER TABLE mst_mb_spin
    DROP COLUMN IF EXISTS mbs_lusture_code;

COMMIT;
