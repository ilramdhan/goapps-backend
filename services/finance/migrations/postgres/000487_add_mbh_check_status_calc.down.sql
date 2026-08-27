-- Down 000487: lepas kolom turunan mbh_check_status_calc beserta CHECK-nya.
--
-- Aman: kolom aditif nullable tanpa FK/index, dan ⛔ tidak ada backfill yang
-- perlu dibatalkan (butir 44 — 207 baris legacy memang tidak pernah disentuh).
-- ⚠ Menjatuhkan kolom ini MENGHAPUS seluruh nilai turunan yang sudah dihitung
-- aplikasi secara permanen. Kolom LAMA `mbh_check_status` ⛔ TIDAK disentuh di
-- sini maupun di up.sql — ia beku, dan tetap beku.

BEGIN;

ALTER TABLE mst_mb_head
  DROP CONSTRAINT IF EXISTS chk_mbh_check_status_calc;

ALTER TABLE mst_mb_head
  DROP COLUMN IF EXISTS mbh_check_status_calc;

COMMIT;
