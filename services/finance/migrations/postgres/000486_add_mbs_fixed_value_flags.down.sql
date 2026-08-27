-- Down 000486: lepas kedua penanda fix/actual.
-- Aman: kolom aditif nullable, tidak ada FK/index/CHECK yang bergantung padanya,
-- dan tidak ada backfill yang perlu dibatalkan.
-- ⚠ Menjatuhkan kolom ini MENGHAPUS informasi "nilai ini diisi manusia sebagai
-- actual" secara permanen — hanya jalankan bila P12b memang dibatalkan.

BEGIN;

ALTER TABLE mst_mb_spin
  DROP COLUMN IF EXISTS mbs_dozing_is_fixed,
  DROP COLUMN IF EXISTS mbs_ldr_is_fixed;

COMMIT;
