BEGIN;
UPDATE mst_parameter SET lookup_source_column = 'bbcr_bob_rate_val', updated_at = NOW(), updated_by = 'migration_000562_down'
 WHERE param_code = 'DELIVERY_BOB_RATE' AND lookup_fill_group_code = 'DELIVERY_PACK_CODE' AND lookup_source_column = 'bobin_cost_val' AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_source_column = 'bbcr_box_rate_val', updated_at = NOW(), updated_by = 'migration_000562_down'
 WHERE param_code = 'DELIVERY_BOX_RATE' AND lookup_fill_group_code = 'DELIVERY_PACK_CODE' AND lookup_source_column = 'box_cost_val' AND deleted_at IS NULL;
COMMIT;
