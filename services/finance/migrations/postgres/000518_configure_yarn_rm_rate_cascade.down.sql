-- 000518_configure_yarn_rm_rate_cascade.down.sql
--
-- Restores F_YARN_RM_RATE's expression to the original legacy Oracle-DSL text
-- verbatim from 000408_seed_oracle_formulas.up.sql.
UPDATE mst_formula
SET expression = 'sum(ratio * CASE rm_type WHEN ''GROUP'' THEN mst_rm_cost(rm_group_code,period,pricing_type) WHEN ''PRODUCT'' THEN upstream_product(rm_product_legacy_id).COST_CAP_FINAL END) for route_rms WHERE route_head_legacy_product_id=current_product AND route_level=current_level',
    updated_at = NOW(),
    updated_by = 'migration_000518_down'
WHERE formula_code = 'F_YARN_RM_RATE';
