-- 000426: Fix mst_lookup_master for MB_SPIN — change lm_code_field from
-- mbs_mb_costing to mbs_orion_item_code.
--
-- Product params import stores MB_SPIN values as Oracle ORION item codes
-- (e.g. 'CMB0000003'). The validation in ListMasterOptions was comparing
-- against mbs_mb_costing ('BEIGE9B-64-A') causing false "missing_value" errors.
-- The correct lookup key field is mbs_orion_item_code.

UPDATE public.mst_lookup_master
SET lm_code_field = 'mbs_orion_item_code'
WHERE lm_code = 'MB_SPIN';
