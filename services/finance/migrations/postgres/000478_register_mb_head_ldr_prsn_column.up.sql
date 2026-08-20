-- 000478: Register MB_HEAD.mbh_ldr_prsn in mst_lookup_master_column — the
-- MB_HEAD twin of the T7 divergence that 000477 fixed on the MB_SPIN side.
--
-- yarn_lookup_fill_handler.go resolves mbh_ldr_prsn today via
-- mbHeadNumericReaders (the D30 "planned LDR" reader), but no migration ever
-- inserted it into mst_lookup_master_column: 000416 registered only the
-- mbh_run_ldr_pct half of the LDR pair. So the reader is live while the column
-- is invisible to the "Source Column" dropdown and nothing validates a param
-- pointing at it. Exactly the shape 000477 cleared for mbs_ldr_prsn.
--
-- This is a code/migration gap, not a production data problem: the physical
-- column exists and the reader works. Registering it only makes existing
-- behaviour visible and selectable in the UI.
--
--   mbh_ldr_prsn  NUMERIC(10,4) (000417) → NUMBER
--
-- Display name: 'Planned LDR (%)', matching what 000477 used for the MB_SPIN
-- counterpart mbs_ldr_prsn. MB_HEAD already registers mbh_run_ldr_pct as
-- 'LDR (%)' (000416, same wording as MB_SPIN's mbs_run_ldr_pct), so the pair
-- reads identically on both masters and a user cannot confuse the planned value
-- with the actual production one.
--
-- Sort order continues from 70, the highest currently used for MB_HEAD
-- (10/20 from 000394, 30-70 from 000416), so nothing collides or reorders.

BEGIN;

INSERT INTO public.mst_lookup_master_column
  (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order)
VALUES
  ('MB_HEAD', 'mbh_ldr_prsn', 'Planned LDR (%)', 'NUMBER', 80)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

COMMIT;
