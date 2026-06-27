-- 000414: Add Oracle source columns to mst_mb_spin (from CST_MST_BATCH_SPIN).
-- These columns were missing from the original seeder and are needed for:
--   mbs_run_ldr_pct  → CMBS_RUN_LDR_PRSN (LDR%, 100% filled) — key spinning parameter
--   mbs_mb_spg_orion → CMBS_MB_SPG_ORION  (Oracle SPG name, 100%)
--   mbs_lesture      → CMBS_LESTURE        (thread type: RND/TBL/etc, 100%)
--   mbs_d_f          → CMBS_D_F            (denier/filament spec, 84%)
--   mbs_vs_number    → CMBS_VS_NUMBER       (VS reference number, 84%)

ALTER TABLE public.mst_mb_spin
  ADD COLUMN IF NOT EXISTS mbs_run_ldr_pct  NUMERIC(10,4),
  ADD COLUMN IF NOT EXISTS mbs_mb_spg_orion VARCHAR(200),
  ADD COLUMN IF NOT EXISTS mbs_lesture      VARCHAR(50),
  ADD COLUMN IF NOT EXISTS mbs_d_f          VARCHAR(100),
  ADD COLUMN IF NOT EXISTS mbs_vs_number    VARCHAR(50);

COMMENT ON COLUMN public.mst_mb_spin.mbs_run_ldr_pct  IS 'Oracle CMBS_RUN_LDR_PRSN — LDR percentage (%)';
COMMENT ON COLUMN public.mst_mb_spin.mbs_mb_spg_orion IS 'Oracle CMBS_MB_SPG_ORION — Oracle SPG display name';
COMMENT ON COLUMN public.mst_mb_spin.mbs_lesture      IS 'Oracle CMBS_LESTURE — thread type (RND, TBL, etc)';
COMMENT ON COLUMN public.mst_mb_spin.mbs_d_f          IS 'Oracle CMBS_D_F — denier/filament/type spec string';
COMMENT ON COLUMN public.mst_mb_spin.mbs_vs_number    IS 'Oracle CMBS_VS_NUMBER — VS reference number';

-- Register new columns in lookup_master_column for costing engine
INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('MB_SPIN', 'mbs_run_ldr_pct',  'LDR (%)',        'NUMBER', 70),
  ('MB_SPIN', 'mbs_lesture',      'Lesture Type',   'TEXT',   80),
  ('MB_SPIN', 'mbs_d_f',          'D/F Spec',       'TEXT',   90),
  ('MB_SPIN', 'mbs_mb_spg_orion', 'SPG Oracle Name','TEXT',   100),
  ('MB_SPIN', 'mbs_vs_number',    'VS Number',      'TEXT',   110)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;
