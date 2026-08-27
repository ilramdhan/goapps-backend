-- 000483 rollback: buang tabel shade tambahan.
--
-- ⚠ DESTRUKTIF: DROP TABLE membuang seluruh shade tambahan yang sudah diisi
-- user. Tidak ada rollback selektif — menjalankan ulang .up.sql hanya membuat
-- tabel kosong. Kedua index ikut terbuang bersama tabelnya.
--
-- mst_mb_head TIDAK disentuh: FK berada di sisi anak, dan shade header
-- (mbh_shade_code/mbh_shade_name) milik 000445.

BEGIN;

DROP TABLE IF EXISTS mst_mb_head_shade;

COMMIT;
