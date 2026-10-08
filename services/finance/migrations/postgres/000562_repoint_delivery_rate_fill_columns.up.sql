-- 000562 — Repoint DELIVERY_BOB_RATE / DELIVERY_BOX_RATE fill columns to real master columns.
--
-- 000407 seeded lookup_source_column = 'bbcr_bob_rate_val' / 'bbcr_box_rate_val', which are not
-- columns of mst_box_bobbin_cost (000412 states the bbcr_* names never existed), so the fill
-- handler had no reader and changing DELIVERY_PACK_CODE filled nothing. Per the 000411 column
-- comments the VAL rates are bobin_cost_val / box_cost_val.
-- Guarded: only rows still on the legacy value are touched (hand-edited mappings are kept).
BEGIN;

UPDATE mst_parameter
   SET lookup_source_column = 'bobin_cost_val', updated_at = NOW(), updated_by = 'migration_000562'
 WHERE param_code = 'DELIVERY_BOB_RATE'
   AND lookup_fill_group_code = 'DELIVERY_PACK_CODE'
   AND lookup_source_column = 'bbcr_bob_rate_val'
   AND deleted_at IS NULL;

UPDATE mst_parameter
   SET lookup_source_column = 'box_cost_val', updated_at = NOW(), updated_by = 'migration_000562'
 WHERE param_code = 'DELIVERY_BOX_RATE'
   AND lookup_fill_group_code = 'DELIVERY_PACK_CODE'
   AND lookup_source_column = 'bbcr_box_rate_val'
   AND deleted_at IS NULL;

COMMIT;
