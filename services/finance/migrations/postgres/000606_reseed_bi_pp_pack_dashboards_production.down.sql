-- P014: hapus hanya dashboard PP_PACK_* yang dibuat 000606 (penanda layout_config.seeded_by); dashboard dari 000603
-- (lingkungan yang grup OPERATIONS-nya ada) dibiarkan.
BEGIN;
DELETE FROM bi_dashboard
 WHERE dashboard_code IN ('PP_PACK_SUMMARY', 'PP_PACK_PRODUCT', 'PP_PACK_MACHINE', 'PP_PACK_POLY')
   AND layout_config->>'seeded_by' = '000606';
COMMIT;
