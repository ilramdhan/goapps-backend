-- 000486: penanda fix/actual per-nilai pada mst_mb_spin (P12b, prasyarat keras P13).
--
-- ⭐ HASIL LANGKAH NOL (2026-08-22) — mbs_status TERBUKTI TIDAK CUKUP:
--   (a) mbs_status adalah PASSTHROUGH ORACLE murni, VARCHAR(100) TANPA CHECK
--       (000418:11,15 — komentar kolom: 'Oracle CMBS_STATUS (Spinning/R and D/
--       Boughtout)'). Sebaran nyata di seed 2700 baris: 'R and D' 1662 ·
--       'Spinning' 975 · 'Boughtout' 62. ⛔ TIDAK SATU PUN nilai itu berarti
--       "nilai ini diisi manusia sebagai actual" — ketiganya menyatakan JENIS
--       PROSES SPINNING, bukan provenance sebuah angka.
--   (b) 🔴 ALASAN YANG MENENTUKAN — SALAH DERAJAT: aturan recalc #3 berbicara
--       PER-NILAI ("LDR *atau* dozing sudah diisi fix/actual"), sedangkan
--       mbs_status adalah PER-BARIS. Satu spin bisa punya dozing actual
--       sementara LDR masih hasil hitung. Satu kolom per-baris SECARA STRUKTURAL
--       tidak dapat menyatakan dua fakta independen. ⇒ butuh DUA penanda.
--   (c) mbh_check_status (langkah 2) GUGUR: ia milik mst_mb_head (per-HEAD,
--       bukan per-spin — derajatnya makin jauh), juga VARCHAR tanpa CHECK
--       (000417:10,16), dan sedang berubah menjadi TURUNAN OTOMATIS (keputusan
--       user K-1, docs/desain-check-status-turunan.md). Nilai yang DITURUNKAN
--       dari kolom lain ⛔ tidak dapat menyatakan "diisi manusia sebagai actual".
-- ⇒ Langkah 3 tercapai. Kolom baru dibuat — sesuai izin user 2026-08-21
--   ("jika perlu kolom flagging untuk fix actual supaya tidak ikut ter
--   recalculate silahkan"). ⛔ mbs_status TIDAK DISENTUH, tetap beku.
--
-- BENTUK: dua BOOLEAN, NULLable, TANPA DEFAULT, TANPA NOT NULL — meniru pola
-- 000484 (P3). ALTER TABLE ini ⛔ TIDAK menulis ulang satu baris pun dari ~2699
-- baris impor Oracle, dan updated_at ⛔ tidak bergerak (U-2).
--
-- 🔴 SEMANTIK NULL — DITETAPKAN SADAR, WAJIB DIBACA SEBELUM P13:
--   NULL  = tidak diketahui → DIPERLAKUKAN SEBAGAI FIX (⛔ JANGAN direcalc).
--   FALSE = eksplisit boleh direcalc (di-set oleh jalur duplicate spin).
--   TRUE  = eksplisit fix/actual, diisi manusia (⛔ JANGAN direcalc).
-- Alasan: seluruh 2699 baris lama menjadi NULL ⇒ SELURUHNYA dianggap fix ⇒
-- eksekusi pertama recalc P13 ⛔ TIDAK menggerakkan satu angka pun secara
-- diam-diam. Ini kesalahan ke arah TIDAK MENGGERAKKAN ANGKA — arah aman yang
-- diwajibkan kriteria selesai P12b. ⛔ JANGAN mengubahnya menjadi
-- "NULL = boleh recalc" tanpa keputusan user baru.
-- ⚠ Konsekuensi yang diterima sadar: baris baru yang dibuat lewat Create biasa
-- juga NULL ⇒ dianggap fix. Jalur duplicate spin (P8) WAJIB menulis FALSE
-- secara eksplisit agar child hasil duplikasi ikut recalc.
--
-- ⛔ NOL backfill, NOL UPDATE: tidak ada baris lama yang ditandai apa pun.

BEGIN;

ALTER TABLE mst_mb_spin
  ADD COLUMN IF NOT EXISTS mbs_ldr_is_fixed    BOOLEAN,
  ADD COLUMN IF NOT EXISTS mbs_dozing_is_fixed BOOLEAN;

COMMENT ON COLUMN mst_mb_spin.mbs_ldr_is_fixed IS
  'Penanda fix/actual untuk NILAI LDR (mbs_run_ldr_pct). TRUE = diisi manusia '
  'sebagai actual, jangan direcalc. FALSE = boleh direcalc. NULL = tidak '
  'diketahui, DIPERLAKUKAN SEBAGAI FIX (aman). Aturan recalc #3, fase P12b.';
COMMENT ON COLUMN mst_mb_spin.mbs_dozing_is_fixed IS
  'Penanda fix/actual untuk NILAI DOZING (mbs_dozing). Semantik identik dengan '
  'mbs_ldr_is_fixed. Terpisah karena aturan #3 bersifat PER-NILAI, bukan per-baris.';

COMMIT;
