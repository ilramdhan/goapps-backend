-- 000485 rollback.
--
-- ⚠ DESTRUKTIF GANDA:
--   (1) DROP TABLE membuang SELURUH riwayat audit lock/unlock — tidak dapat
--       direkonstruksi dari sumber lain.
--   (2) DROP COLUMN membuang status terkunci setiap recipe; setelah .up.sql
--       dijalankan ulang, semua head kembali NULL (= tidak terkunci).
-- Rollback data yang sesungguhnya = restore backup, bukan migrasi ini.
--
-- Tabel log dibuang lebih dulu karena ia yang memegang FK ke mst_mb_head.

BEGIN;

DROP TABLE IF EXISTS mst_mb_head_lock_log;

ALTER TABLE mst_mb_head
  DROP COLUMN IF EXISTS mbh_unlock_reason,
  DROP COLUMN IF EXISTS mbh_unlock_requested_by,
  DROP COLUMN IF EXISTS mbh_unlock_requested_at,
  DROP COLUMN IF EXISTS mbh_locked_by,
  DROP COLUMN IF EXISTS mbh_locked_at,
  DROP COLUMN IF EXISTS mbh_is_locked;

COMMIT;
