-- 000416: Add Oracle source columns to mst_mb_head (from CST_MST_BATCH_HEAD).
-- Mirrors the MB Spin pattern (000414).

ALTER TABLE public.mst_mb_head
  ADD COLUMN IF NOT EXISTS mbh_orion_item_code VARCHAR(200),
  ADD COLUMN IF NOT EXISTS mbh_mb_spg_orion    VARCHAR(200),
  ADD COLUMN IF NOT EXISTS mbh_run_ldr_pct     NUMERIC(10,4),
  ADD COLUMN IF NOT EXISTS mbh_d_f             VARCHAR(100),
  ADD COLUMN IF NOT EXISTS mbh_lesture         VARCHAR(50),
  ADD COLUMN IF NOT EXISTS mbh_vs_number       VARCHAR(50),
  ADD COLUMN IF NOT EXISTS notes               TEXT;

COMMENT ON COLUMN public.mst_mb_head.mbh_orion_item_code IS 'Oracle CMBH_ORION_ITEM_CODE — ERP item code (34%)';
COMMENT ON COLUMN public.mst_mb_head.mbh_mb_spg_orion    IS 'Oracle CMBH_MB_SPG_ORION — Oracle SPG display name (99%)';
COMMENT ON COLUMN public.mst_mb_head.mbh_run_ldr_pct     IS 'Oracle CMBH_RUN_LDR_PRSN — LDR percentage (89%)';
COMMENT ON COLUMN public.mst_mb_head.mbh_d_f             IS 'Oracle CMBH_D_F — denier/filament spec string (76%)';
COMMENT ON COLUMN public.mst_mb_head.mbh_lesture         IS 'Oracle CMBH_LESTURE — thread type RND/TBL/etc (86%)';
COMMENT ON COLUMN public.mst_mb_head.mbh_vs_number       IS 'Oracle CMBH_VS_NUMBER — VS reference number (78%)';

INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('MB_HEAD', 'mbh_run_ldr_pct',  'LDR (%)',         'NUMBER', 30),
  ('MB_HEAD', 'mbh_lesture',      'Lesture Type',    'TEXT',   40),
  ('MB_HEAD', 'mbh_d_f',          'D/F Spec',        'TEXT',   50),
  ('MB_HEAD', 'mbh_mb_spg_orion', 'SPG Oracle Name', 'TEXT',   60),
  ('MB_HEAD', 'mbh_vs_number',    'VS Number',       'TEXT',   70)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;
