-- 000517 rollback — lepas kembali fk_mbh_cost_product.
--
-- ⚠ TIDAK destruktif terhadap data: yang dilepas hanya constraint. Nilai
-- mbh_cost_product_id tetap utuh.
--
-- ⚠ TIDAK REVERSIBEL SEPENUHNYA, dan ini perlu disadari: langkah 1 pada migrasi
-- UP menolkan nilai MENGGANTUNG menjadi NULL. Nilai-nilai itu TIDAK dikembalikan
-- oleh rollback ini — memang tidak bisa, karena baris cost_product_master yang
-- dulu ditunjuknya sudah tidak ada. Menjalankan UP lagi setelah DOWN aman dan
-- idempoten.
--
-- ⛔ TIDAK menyentuh milik migrasi lain: kolom mbh_cost_product_id (milik 000445)
-- dan index idx_mbh_cost_product_id (milik 000489) DIBIARKAN UTUH. Karena itu
-- berkas ini TIDAK memakai DROP COLUMN — berbeda dari 000490_*.down.sql yang
-- memang memiliki kolomnya sendiri.
--
-- COMMENT ON COLUMN sengaja TIDAK dikembalikan ke nilai sebelumnya: 000445 tidak
-- pernah menetapkan comment apa pun untuk kolom ini, jadi tidak ada nilai lama
-- yang bisa dipulihkan. Comment dibiarkan apa adanya — ia tidak memengaruhi
-- perilaku.

BEGIN;

ALTER TABLE mst_mb_head
    DROP CONSTRAINT IF EXISTS fk_mbh_cost_product;

COMMIT;
