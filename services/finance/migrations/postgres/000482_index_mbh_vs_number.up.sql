-- 000482: index pencarian VS Number pada mst_mb_head.
--
-- 🔴 NON-UNIQUE — DISENGAJA. JANGAN diubah menjadi UNIQUE.
-- Rencana unique index DIBATALKAN (design §2.5 revisi 2, keputusan B9). Bukti
-- produksi (spec §8.4.4 / SQL-4b):
--   mbh_vs_number = '0'      -> 177 head (placeholder massal, tidak saling terkait)
--   mbh_vs_number = '16728'  -> 2 head  (dua varian MGT WOLFY BL 5106)
-- CREATE UNIQUE INDEX akan GAGAL seketika dan membuat schema dirty.
-- ⛔ Data itu TIDAK dibersihkan di sini — normalisasi '0' -> NULL adalah
-- migrasi data terpisah di luar batch ini (menunggu OQ-16).
--
-- Keunikan VS Number ditegakkan HANYA di lapisan aplikasi, untuk data BARU,
-- dan WAJIB mengecualikan '0' serta string kosong; kalau tidak setiap create
-- head akan bentrok dengan 177 baris legacy (design §2.5, plan P5).
--
-- Tidak ada index lain atas mbh_vs_number di repo ini (grep pada seluruh
-- migrasi: 0 hit), jadi tidak ada duplikasi dengan 000477 — 000477 hanya
-- mendaftarkan kolom MB_SPIN ke mst_lookup_master_column, bukan membuat index.
--
-- Kolom mbh_vs_number sendiri sudah ada sejak 000416; ⛔ tidak di-ADD ulang.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_mst_mb_head_vs_number
  ON mst_mb_head (mbh_vs_number)
  WHERE mbh_vs_number IS NOT NULL AND deleted_at IS NULL;

COMMIT;
