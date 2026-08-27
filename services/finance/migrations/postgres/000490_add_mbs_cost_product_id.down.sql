-- 000490 rollback — kebalikan aman dari mbs_cost_product_id pada mst_mb_spin.
--
-- ⚠ DESTRUKTIF: DROP COLUMN membuang seluruh kaitan traceability spin -> cost
-- product yang dihasilkan backfill. Kaitan itu MASIH bisa disusun ulang dengan
-- menjalankan kembali migrasi UP, karena sumbernya (mst_mb_head.mbh_cost_product_id)
-- tidak ikut disentuh — kecuali untuk baris yang nilainya sudah diubah kode
-- aplikasi setelah migrasi berjalan; nilai itu hilang permanen.
--
-- Urutan: index -> constraint -> column, agar kegagalan (bila ada) informatif.
-- DROP COLUMN sebenarnya sudah otomatis membuang index dan constraint-nya; kedua
-- baris pertama ditulis eksplisit meniru 000484_*.down.sql.
--
-- ⛔ TIDAK menyentuh milik migrasi lain: mst_mb_head.mbh_cost_product_id (kolom
-- milik 000445), idx_mbh_cost_product_id (milik 000489), fk_mbs_parent_spin dan
-- idx_mbs_parent_spin (milik 000484), serta tabel cost_product_master (milik
-- 000106) semuanya DIBIARKAN UTUH.

BEGIN;

DROP INDEX IF EXISTS idx_mbs_cost_product_id;

ALTER TABLE mst_mb_spin
    DROP CONSTRAINT IF EXISTS fk_mbs_cost_product;

ALTER TABLE mst_mb_spin
    DROP COLUMN IF EXISTS mbs_cost_product_id;

COMMIT;
