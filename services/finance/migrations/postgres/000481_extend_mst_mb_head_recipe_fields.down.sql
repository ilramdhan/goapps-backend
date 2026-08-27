-- 000481 rollback.
--
-- ⚠ DESTRUKTIF: DROP COLUMN membuang seluruh nilai Number of Process yang sudah
-- dipilih user. Tidak ada cara memulihkannya selain restore backup.
-- Kolom-kolom recipe lain TIDAK disentuh di sini karena migrasi ini tidak
-- membuatnya (lihat daftar di berkas .up.sql).

BEGIN;

ALTER TABLE mst_mb_head
  DROP COLUMN IF EXISTS mbh_no_of_process;

COMMIT;
