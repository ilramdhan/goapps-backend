-- Down 000497 — drop exactly the index this migration added, nothing else.
-- Safe: additive index only, no data written, no other object depends on it.

BEGIN;

DROP INDEX IF EXISTS idx_mbs_ldr_type;

COMMIT;
