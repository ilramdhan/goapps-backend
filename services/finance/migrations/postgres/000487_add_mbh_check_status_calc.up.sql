-- 000487: kolom turunan `mbh_check_status_calc` pada mst_mb_head (fase P10).
--
-- ⭐ DASAR KEPUTUSAN USER — ⛔ JANGAN diubah tanpa keputusan user baru:
--   * `K-1` (2026-08-21) — OPSI 2: kolom lama `mbh_check_status` DIBIARKAN BEKU
--     sebagai jejak impor Oracle (000417:10,16, VARCHAR(50) tanpa CHECK). Nilai
--     TURUNAN ditulis ke KOLOM BARU ini. ⛔ TIDAK ADA jalur tulis aplikasi ke
--     kolom lama dari mesin turunan.
--   * plan §11 butir 41 (2026-08-22) — NAMA kolom = `mbh_check_status_calc`.
--     Akhiran `_calc` menegaskan nilainya DIHITUNG APLIKASI, kontras dengan
--     passthrough Oracle. Kandidat `_derived` / `mbh_workflow_state` DITOLAK.
--   * plan §11 butir 43 (2026-08-23) — NOMOR migrasi = 000487 (nomor bebas
--     berikutnya; tertinggi di disk saat itu = 000486). Menyisip di antara
--     000481-000486 DITOLAK (§11 butir 62: migrasi yang sudah pernah jalan di DB
--     mana pun ⛔ tidak boleh diedit).
--   * plan §11 butir 44 (2026-08-22) — 207 baris legacy DIBIARKAN NULL PERMANEN.
--     ⛔ TIDAK ADA BACKFILL. NULL berarti "belum pernah dihitung aplikasi" —
--     jujur, dan DAPAT DIBEDAKAN dari hasil hitung nyata.
--   * plan §11 butir 42 (2026-08-23) — OPSI (2) BERDAMPINGAN: kolom BARU ini
--     jadi kolom utama di tabel/filter/export; kolom LAMA tetap tampil HANYA di
--     halaman detail, READ-ONLY, sebagai jejak Oracle.
--
-- BENTUK: VARCHAR(50) NULLable, ⛔ TANPA DEFAULT, ⛔ TANPA NOT NULL, ⛔ TANPA
-- backfill. ALTER TABLE ini ⛔ TIDAK menulis ulang satu baris pun dan updated_at
-- ⛔ tidak bergerak (`U-2`: jangan sentuh tabel Oracle).
--
-- 🔴 CHECK constraint WAJIB mengizinkan NULL (butir 44 — NULL kini PERMANEN
-- by decision). Ejaan WAJIB Title Case PERSIS produksi: 'Waiting', 'Current',
-- 'Boughtout', 'Approved', 'Outdated', 'Rejected'. ⛔ Huruf kecil akan menolak
-- ribuan baris yang ada di kolom lama bila nilai itu pernah disalin.
-- ⚠ CHECK ini SENGAJA memuat SELURUH enam nilai target walaupun mesin turunan
-- hari ini baru menulis SEBAGIAN (lihat komentar kolom) — supaya perluasan
-- mesin di fase berikutnya ⛔ tidak butuh migrasi CHECK lagi.
--
-- ⚠ CONSTRAINT DIBERI NAMA EKSPLISIT (`chk_mbh_check_status_calc`) — ⛔ sengaja
-- BUKAN CHECK inline tanpa nama seperti 000445:6-7, yang namanya auto-generate
-- PostgreSQL dan karenanya butuh query verifikasi sebelum bisa di-DROP.

BEGIN;

ALTER TABLE mst_mb_head
  ADD COLUMN IF NOT EXISTS mbh_check_status_calc VARCHAR(50);

ALTER TABLE mst_mb_head
  DROP CONSTRAINT IF EXISTS chk_mbh_check_status_calc;

ALTER TABLE mst_mb_head
  ADD CONSTRAINT chk_mbh_check_status_calc
  CHECK (
    mbh_check_status_calc IS NULL
    OR mbh_check_status_calc IN ('Waiting', 'Current', 'Boughtout', 'Approved', 'Outdated', 'Rejected')
  );

COMMENT ON COLUMN mst_mb_head.mbh_check_status_calc IS
  'Check status TURUNAN, DIHITUNG APLIKASI dari mbh_entry_status + mbh_is_boughtout '
  '(keputusan user K-1 opsi 2). NULL = BELUM PERNAH DIHITUNG APLIKASI — bukan '
  '"tidak ada status", dan UI WAJIB menampilkannya eksplisit (mis. "Belum '
  'dihitung"), BUKAN string kosong. 207 baris legacy tetap NULL PERMANEN: tidak '
  'ada backfill (butir 44). Kolom ini MENGGANTIKAN mbh_check_status sebagai kolom '
  'utama di tabel/filter/export; mbh_check_status tetap BEKU sebagai jejak impor '
  'Oracle dan hanya tampil read-only di halaman detail (butir 42). CHECK memuat 6 '
  'nilai, tetapi mesin turunan hari ini baru menulis Waiting/Approved/Boughtout — '
  'Current/Outdated/Rejected menunggu keputusan user (aturan 5,6,8 desain §c dan '
  'gerbang G12-REJECT).';

COMMIT;
