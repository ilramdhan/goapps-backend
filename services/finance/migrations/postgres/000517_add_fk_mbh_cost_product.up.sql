-- 000517: FK untuk mst_mb_head.mbh_cost_product_id -> cost_product_master.
--
-- MENUTUP DEFEK H-1. Kolomnya lahir di
-- 000445_extend_mst_mb_head_workflow.up.sql:17 sebagai `ADD COLUMN IF NOT EXISTS
-- mbh_cost_product_id BIGINT` — POLOS, tanpa REFERENCES. Akibatnya nilai
-- MENGGANTUNG (menunjuk cpm_product_sys_id yang sudah tidak ada) bisa hidup di
-- kolom ini tanpa terdeteksi.
--
-- Mengapa itu berbahaya, bukan sekadar tidak rapi: kolom ini disalin turun ke
-- mst_mb_spin.mbs_cost_product_id, dan DI SANA FK-nya ADA (fk_mbs_cost_product,
-- 000490). Jalur salinannya:
--   * internal/infrastructure/postgres/mb_spin_duplicate.go:218 (lockSourceSpin)
--     membaca h.mbh_cost_product_id dari head;
--   * :352 (insertClone) menuliskannya ke mbs_cost_product_id.
-- Jadi satu head menggantung berubah menjadi pelanggaran FK (SQLSTATE 23503)
-- pada operasi DUPLICATE SPIN — gagal di hilir, jauh dari sumber masalahnya.
-- Kegagalan itu memang ditemui nyata pada finance_db lokal (99 head menggantung).
--
-- Migrasi ini TIDAK menambah kolom dan TIDAK menambah index:
--   * kolom milik 000445;
--   * index parsial idx_mbh_cost_product_id SUDAH ada dari
--     000489_add_idx_mbh_cost_product_id.up.sql — membuatnya lagi akan mubazir.
-- Yang ditambahkan HANYA constraint-nya.
--
-- ⚠ NULLABLE PERMANEN, sama seperti kolom turunannya di spin (D23). ⛔ JANGAN
-- dijadikan NOT NULL dan ⛔ JANGAN diberi DEFAULT: head berstatus DRAFT belum
-- pernah melewati transisi DRAFT->VALIDATED, jadi NULL adalah keadaan SAH dan
-- permanen baginya. Penulisnya satu-satunya adalah mbWriteBackCostProduct
-- (internal/infrastructure/postgres/mb_autogen_repository.go:706), yang hanya
-- berjalan pada transisi itu.
--
-- URUTAN SENGAJA (R10): bersihkan nilai menggantung jadi NULL DULU, BARU pasang
-- FK. Bila FK dipasang lebih dulu, validasinya memindai seluruh tabel dan satu
-- baris yatim saja akan MEMBATALKAN seluruh migrasi. Pada finance_db lokal
-- langkah 1 sekarang menyentuh 0 baris (sudah dibersihkan manual saat H-1
-- didiagnosis), tetapi pada staging/produksi langkah itu WAJIB ada — di sanalah
-- data impor legacy berada dan jumlah sebenarnya HARUS dibaca dari hasil
-- eksekusi, ⛔ bukan disimpulkan dari komentar ini.
--
-- ⛔ CREATE INDEX CONCURRENTLY tidak relevan di sini (tidak ada index yang
-- dibuat), namun catatan lock tetap berlaku: ADD CONSTRAINT ... FOREIGN KEY
-- mengambil lock SHARE ROW EXCLUSIVE pada mst_mb_head dan lock pada
-- cost_product_master selama validasi — jalankan di jendela lalu lintas rendah.
--
-- BEGIN/COMMIT eksplisit, meniru 000489/000490 pada area yang sama.

BEGIN;

-- Langkah 1 — bersihkan nilai MENGGANTUNG jadi NULL, SEBELUM FK dipasang.
-- Nilainya yang dinolkan, ⛔ bukan barisnya yang dihapus: sebuah MB Head adalah
-- data bisnis; yang rusak hanya penandanya. NULL sah menurut D23.
--
-- ⚠ SENGAJA tidak menyaring deleted_at: head yang sudah soft-deleted pun ikut
-- divalidasi FK oleh PostgreSQL (FK tidak mengenal soft delete), jadi
-- membiarkannya akan tetap membatalkan langkah 2.
--
-- ⚠ Pembersihan ini TIDAK merambat ke mst_mb_spin.mbs_cost_product_id. Itu
-- disengaja: nilai di spin sudah punya FK sendiri sejak 000490 sehingga dijamin
-- tidak menggantung, dan menimpanya akan menghapus jejak ownership yang sah.
UPDATE mst_mb_head h
   SET mbh_cost_product_id = NULL
 WHERE h.mbh_cost_product_id IS NOT NULL
   AND NOT EXISTS (
       SELECT 1
         FROM cost_product_master m
        WHERE m.cpm_product_sys_id = h.mbh_cost_product_id
   );

-- Langkah 2 — FK, setelah data dijamin bersih.
-- ON DELETE SET NULL, menyalin persis fk_mbs_cost_product (000490) dan preseden
-- kaitan lunak 000438_add_cpr_reference_product.up.sql:21. RESTRICT keliru di
-- sini: kolom ini traceability, bukan fakta bisnis yang mengikat, sehingga ia
-- TIDAK boleh memblokir penghapusan cost product. Pilihan ini juga tidak
-- memperkenalkan keadaan NULL baru yang tak tertangani kode — jalur baca sudah
-- NULL-safe: mbResolveRefProductSysID memindai ke sql.NullInt64 dengan pesan
-- error jelas (mb_autogen_repository.go:535) dan mb_head_repository.go:305
-- memakai COALESCE(mbh_cost_product_id, 0).
--
-- Idempoten lewat DO-block: ADD CONSTRAINT tidak punya IF NOT EXISTS di
-- PostgreSQL. Bila constraint sudah ada blok ini no-op; bila gagal karena sebab
-- lain ia GAGAL BERISIK dan transaksi dibatalkan (⛔ bukan silent-skip).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_mbh_cost_product'
  ) THEN
    ALTER TABLE mst_mb_head
      ADD CONSTRAINT fk_mbh_cost_product
        FOREIGN KEY (mbh_cost_product_id)
        REFERENCES cost_product_master (cpm_product_sys_id)
        ON DELETE SET NULL;
  END IF;
END
$$;

COMMENT ON COLUMN mst_mb_head.mbh_cost_product_id IS
  'Traceability/ownership: cost_product_master.cpm_product_sys_id hasil auto-generate '
  'head ini, ditulis sekali oleh mbWriteBackCostProduct pada transisi DRAFT->VALIDATED. '
  'NULLABLE PERMANEN (D23) — NULL sah untuk head yang belum pernah tervalidasi. '
  'Diturunkan ke mst_mb_spin.mbs_cost_product_id. DILARANG dipakai sebagai jalur '
  'aliran cost (D18).';

COMMIT;
