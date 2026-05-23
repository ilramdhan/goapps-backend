-- 000237: Remove TXFX_* invented rm_codes from cst_rm_cost.
--
-- cst_rm_cost is REAL production data sourced from Oracle sync. The earlier
-- S8e-fix seed (000236) polluted it with TXFX_PTA / TXFX_MEG / etc. invented
-- rm_codes for fixture purposes. Strip them out -- the upcoming 000239 deep
-- re-seed maps ITEM RM references to actual existing rm_codes already in the
-- table (e.g. pigments, dyes, masterbatches).

BEGIN;

DELETE FROM cst_rm_cost
 WHERE period = '202604'
   AND rm_code LIKE 'TXFX_%'
   AND created_by = 'seed_000236';

COMMIT;
