-- 000490: mbs_cost_product_id pada mst_mb_spin — kolom + backfill + FK + index parsial.
--
-- Fase P8 (plan §5 P8, item A1 ≡ E3 — SATU pekerjaan, jangan dihitung dua kali).
-- Tujuan kolom ini: TRACEABILITY/OWNERSHIP saja — spin mana yang dimiliki oleh
-- cost product mana. ⛔ DIBATASI keputusan D18: kolom ini DILARANG menjadi jalur
-- aliran cost. Tidak ada perhitungan yang boleh membacanya sebagai sumber angka.
--
-- ⚠ NULLABLE PERMANEN (keputusan D23). ⛔ JANGAN pernah dijadikan NOT NULL dan
-- ⛔ JANGAN diberi DEFAULT. Alasannya bukan kemalasan migrasi: satu spin yang
-- ber-parent head DRAFT adalah kondisi SAH dan permanen — head DRAFT belum
-- pernah melewati transisi DRAFT->VALIDATED, jadi mbh_cost_product_id-nya masih
-- NULL dan tidak ada nilai yang benar untuk diturunkan ke spin-nya. NOT NULL
-- akan menjadikan keadaan sah itu sebagai error.
--
-- Tipe BIGINT, bukan UUID: target FK adalah cost_product_master.cpm_product_sys_id,
-- sebuah BIGSERIAL PRIMARY KEY (000106_create_cost_product_master.up.sql:7). Tipe
-- ini identik dengan mst_mb_head.mbh_cost_product_id BIGINT
-- (000445_extend_mst_mb_head_workflow.up.sql:17), yaitu kolom sumber backfill.
--
-- Relasi yang dipakai backfill: mst_mb_spin.mbs_mbh_id UUID NOT NULL REFERENCES
-- mst_mb_head (mbh_id) — 000389_create_mst_mb_spin.up.sql:8. Jadi setiap spin
-- selalu punya tepat satu head, dan nilai turunannya diambil dari head itu.
--
-- URUTAN SENGAJA (R10): kolom -> BACKFILL -> bersihkan nilai menggantung jadi
-- NULL -> BARU pasang FK -> index. Bila FK dipasang lebih dulu, backfill bisa
-- gagal; dan bila nilai yatim (mbh_cost_product_id yang menunjuk ke
-- cost_product_master yang sudah tidak ada) ikut tersalin, validasi FK memindai
-- tabel dan MEMBATALKAN seluruh migrasi. Karena itu langkah pembersihan berada
-- di ANTARA backfill dan FK, bukan sesudahnya.
--
-- ⛔ CREATE INDEX CONCURRENTLY TIDAK DIPAKAI. Bukti (sama seperti 000489):
-- runner golang-migrate mengirim seluruh berkas .sql sebagai SATU ExecContext
-- karena DSN migrate-up repo ini tidak menyetel x-multi-statement ⇒ PostgreSQL
-- menjalankannya sebagai blok transaksi implisit ⇒ CONCURRENTLY ditolak. Nol
-- preseden CONCURRENTLY untuk CREATE INDEX di direktori ini. Konsekuensi yang
-- perlu disadari: CREATE INDEX biasa mengambil lock ACCESS EXCLUSIVE singkat
-- pada mst_mb_spin — jalankan di jendela lalu lintas rendah.
--
-- BEGIN/COMMIT eksplisit, meniru 000489 dan 000484 (dua migrasi terdekat pada
-- area yang sama).

BEGIN;

-- Langkah 1 — kolom. Tanpa NOT NULL dan tanpa DEFAULT, sehingga ALTER TABLE ini
-- adalah perubahan katalog saja dan tidak menulis ulang satu baris pun (U-2:
-- updated_at tidak boleh bergerak karena migrasi).
ALTER TABLE mst_mb_spin
    ADD COLUMN IF NOT EXISTS mbs_cost_product_id BIGINT;

-- Langkah 2 — BACKFILL dari head. Plan memperkirakan ~2699 baris tersentuh.
-- Angka sebenarnya WAJIB dilaporkan dari hasil eksekusi, ⛔ bukan disimpulkan
-- dari komentar ini.
--
-- Syarat WHERE, satu per satu:
--   * h.mbh_cost_product_id IS NOT NULL   — hanya head yang sudah tervalidasi
--     punya nilai untuk diturunkan; head DRAFT sengaja dibiarkan NULL (D23).
--   * s.mbs_cost_product_id IS NULL       — idempoten: menjalankan ulang migrasi
--     tidak menimpa nilai yang sudah diisi kode aplikasi.
--   * s.deleted_at IS NULL                — baris yang sudah di-soft-delete tidak
--     ikut ditulis, agar jejak historisnya tidak berubah.
-- ⚠ h.deleted_at SENGAJA TIDAK disaring: bila head sudah soft-deleted namun
-- spin-nya masih hidup, kaitan ownership-nya justru tetap perlu tercatat.
UPDATE mst_mb_spin s
   SET mbs_cost_product_id = h.mbh_cost_product_id
  FROM mst_mb_head h
 WHERE h.mbh_id = s.mbs_mbh_id
   AND h.mbh_cost_product_id IS NOT NULL
   AND s.mbs_cost_product_id IS NULL
   AND s.deleted_at IS NULL;

-- Langkah 3 — bersihkan nilai MENGGANTUNG jadi NULL (R10), SEBELUM FK dipasang.
-- mbh_cost_product_id pada mst_mb_head adalah soft link TANPA FK (000445 hanya
-- ADD COLUMN, tidak ada REFERENCES), jadi nilai yang tidak punya pasangan di
-- cost_product_master bisa saja ada pada data produksi/impor. Nilai seperti itu
-- akan membuat ADD CONSTRAINT di langkah 4 GAGAL dan membatalkan migrasi.
-- Di sini nilainya dinolkan menjadi NULL — sah menurut D23 — bukan barisnya
-- yang dihapus.
UPDATE mst_mb_spin s
   SET mbs_cost_product_id = NULL
 WHERE s.mbs_cost_product_id IS NOT NULL
   AND NOT EXISTS (
       SELECT 1
         FROM cost_product_master m
        WHERE m.cpm_product_sys_id = s.mbs_cost_product_id
   );

-- Langkah 4 — FK, setelah data dijamin bersih.
-- ON DELETE SET NULL, mengikuti preseden 000438_add_cpr_reference_product.up.sql:21
-- untuk kaitan lunak: bila cost product-nya dihapus, spin kehilangan penandanya
-- dan TIDAK memblokir penghapusan master. RESTRICT keliru di sini karena kolom
-- ini hanya traceability (D18), bukan fakta bisnis yang mengikat.
--
-- Idempoten lewat DO-block: ADD CONSTRAINT tidak punya IF NOT EXISTS di
-- PostgreSQL. Pola ini menyalin 000484_*.up.sql — bila constraint sudah ada
-- blok ini no-op; bila gagal karena sebab lain, ia GAGAL BERISIK dan transaksi
-- dibatalkan (⛔ bukan pola silent-skip).
DO $$
BEGIN
  IF NOT EXISTS (
    SELECT 1 FROM pg_constraint WHERE conname = 'fk_mbs_cost_product'
  ) THEN
    ALTER TABLE mst_mb_spin
      ADD CONSTRAINT fk_mbs_cost_product
        FOREIGN KEY (mbs_cost_product_id)
        REFERENCES cost_product_master (cpm_product_sys_id)
        ON DELETE SET NULL;
  END IF;
END
$$;

-- Langkah 5 — index PARSIAL, NON-UNIQUE, mengikuti preseden
-- 000489_add_idx_mbh_cost_product_id.up.sql:48 dan idx_mbs_parent_spin
-- (000484) pada tabel yang sama.
--   * PARSIAL (WHERE ... IS NOT NULL): kolomnya nullable dan setiap spin dari
--     head DRAFT bernilai NULL; tidak ada query yang mencari NULL lewat index.
--   * NON-UNIQUE, SENGAJA: banyak spin BERBAGI satu cost product karena semuanya
--     turun dari head yang sama — relasinya memang N:1, jadi UNIQUE akan salah
--     secara semantik dan akan membatalkan backfill di atas.
CREATE INDEX IF NOT EXISTS idx_mbs_cost_product_id
    ON mst_mb_spin (mbs_cost_product_id)
    WHERE mbs_cost_product_id IS NOT NULL;

COMMENT ON COLUMN mst_mb_spin.mbs_cost_product_id IS
  'Traceability/ownership: cost_product_master.cpm_product_sys_id milik spin ini, '
  'diturunkan dari mst_mb_head.mbh_cost_product_id head induknya. NULLABLE PERMANEN '
  '(D23) — NULL sah untuk spin dari head DRAFT yang belum tervalidasi. '
  'DILARANG dipakai sebagai jalur aliran cost (D18).';

COMMIT;
