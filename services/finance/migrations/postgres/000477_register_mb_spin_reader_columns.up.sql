-- 000477: Register the five MB_SPIN columns that have a live Go reader but were
-- never inserted into mst_lookup_master_column (R30 divergence, direction T7).
--
-- yarn_lookup_fill_handler.go resolves all five today via mbSpinNumericReaders /
-- mbSpinTextReaders, and 000407 already wires several of them as
-- lookup_source_column on real params — so they are actively filling values.
-- But because no migration ever registered them, they are invisible to the
-- "Source Column" dropdown and nothing validates a param pointing at them.
--
-- This is a code/migration gap, not a production data problem: the columns
-- exist physically and the fills work. Registering them only makes the existing
-- behaviour visible and selectable in the UI.
--
--   mbs_denier    NUMERIC(10,2) (000389) → NUMBER
--   mbs_filament  INTEGER       (000389) → NUMBER
--   mbs_dozing    NUMERIC(10,4) (000389) → NUMBER
--   mbs_ldr_prsn  NUMERIC(10,4) (000418) → NUMBER
--   mbs_mgt_name  VARCHAR(100)  (000389) → TEXT
--
-- Sort orders continue from 110, the highest currently used for MB_SPIN
-- (50/60 from 000404, 70-110 from 000414), so nothing collides or reorders.

BEGIN;

INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('MB_SPIN', 'mbs_denier',   'Denier',                'NUMBER', 120),
  ('MB_SPIN', 'mbs_filament', 'Filament',              'NUMBER', 130),
  ('MB_SPIN', 'mbs_ldr_prsn', 'Planned LDR (%)',       'NUMBER', 140),
  ('MB_SPIN', 'mbs_dozing',   'MB Dozing (%, legacy)', 'NUMBER', 150),
  ('MB_SPIN', 'mbs_mgt_name', 'MB Management Name',    'TEXT',   160)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

COMMIT;
