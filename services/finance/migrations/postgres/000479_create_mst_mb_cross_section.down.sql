-- 000479 rollback: drop the cross-section master.
--
-- ⚠ DESTRUCTIVE: DROP TABLE removes the 6 seeded rows AND every row a user
-- later added or edited through the Finance master UI (including any display
-- name / description Finance eventually supplies for 'RSD'). There is no
-- selective rollback — re-running the up migration restores only the 6 seed
-- rows in their original, un-edited form. The partial index idx_mbcs_active_order
-- is dropped together with the table.
--
-- 000480 holds FKs into this table; golang-migrate rolls 000480 back first, so
-- no explicit ordering is needed here.

BEGIN;

DROP TABLE IF EXISTS mst_mb_cross_section;

COMMIT;
