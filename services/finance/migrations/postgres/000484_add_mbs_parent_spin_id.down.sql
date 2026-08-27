-- 000484 rollback.
--
-- ⚠ DESTRUKTIF: DROP COLUMN membuang seluruh jejak asal-usul duplikasi dan
-- jejak recalc. Baris spin hasil duplikasi TIDAK ikut terhapus — hanya
-- kaitannya ke induk yang hilang, dan kaitan itu tidak bisa disusun ulang.
--
-- Urutan: index dan constraint dilepas lebih dulu agar kegagalan (bila ada)
-- informatif; DROP COLUMN sebenarnya sudah otomatis membuang keduanya.

BEGIN;

DROP INDEX IF EXISTS idx_mbs_parent_spin;

ALTER TABLE mst_mb_spin
  DROP CONSTRAINT IF EXISTS fk_mbs_parent_spin;

ALTER TABLE mst_mb_spin
  DROP COLUMN IF EXISTS mbs_last_recalc_by,
  DROP COLUMN IF EXISTS mbs_last_recalc_at,
  DROP COLUMN IF EXISTS mbs_duplicated_by,
  DROP COLUMN IF EXISTS mbs_duplicated_at,
  DROP COLUMN IF EXISTS mbs_parent_spin_id;

COMMIT;
