-- 000481: MB Recipe — kolom recipe yang belum ada pada mst_mb_head.
--
-- FASE P3 (gelombang AMAN): aditif murni. Hanya SATU kolom yang benar-benar
-- belum ada. Seluruh field lain pada spec §6 "Tabel Field Form MB Recipe"
-- sudah ada di skema dan SENGAJA tidak di-ADD ulang:
--
--   mbh_mb_costing, mbh_mgt_name, mbh_oracle_sys_id, mbh_denier,
--   mbh_filament, mbh_dozing, mbh_is_active   -> 000388
--   mbh_vs_number                             -> 000416   (VARCHAR(50))
--   mbh_code, mbh_status, mbh_check_status,
--   mbh_ldr_prsn, mbh_final_product           -> 000417
--   mbh_dev_code, mbh_shade_code, mbh_shade_name,
--   mbh_cross_section, mbh_lusture_code,
--   mbh_is_boughtout                          -> 000445
--   mbh_machine_id                            -> 000458
--
-- ATURAN FASE: kolom baru pada tabel EKSISTING wajib NULLable, tanpa DEFAULT,
-- tanpa NOT NULL — supaya ALTER TABLE tidak menulis ulang satu baris pun dan
-- updated_at pada 4000+ baris impor Oracle tidak bergerak (U-2).
--
-- ⛔ NOL UPDATE / NOL backfill di migrasi ini.

BEGIN;

ALTER TABLE mst_mb_head
  ADD COLUMN IF NOT EXISTS mbh_no_of_process VARCHAR(10);

COMMENT ON COLUMN mst_mb_head.mbh_no_of_process IS
  'Kode opsi Number of Process pilihan user (S/D/T), soft-link ke mst_mb_param_option(mbpo_code) '
  'dengan mbpo_mbp_code=''NO_OF_PROCESS'' (seed 000444). ⛔ BERBEDA dari mbh_param_no_of_process '
  '(000445) yang merupakan SNAPSHOT BEKU, hanya ditulis pada jalur VALIDATE. Jangan digabung: '
  'menggabungkan membuat snapshot cost historis berubah retroaktif (plan §5.2 / B6).';

-- Tanpa FK ke mst_mb_param_option: PK-nya mbpo_id dan unique-nya komposit
-- (mbpo_mbp_code, mbpo_code) (000443), sehingga FK menuntut kolom pasangan.
-- Mengikuti pola soft-link mbh_lusture_code (000445). Validasi di domain Go.

COMMIT;
