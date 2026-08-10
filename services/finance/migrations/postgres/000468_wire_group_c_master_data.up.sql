-- Group-C master-data wiring for the product cost sheet export.
--
-- Three template rows read master columns that exist in the database but have
-- no mst_parameter reaching them, so they can never enter cpc_param_snapshot:
--
--   CSV row  9 Total Fixed Cost -> mst_machine.mc_tot_fxd_cst        (registered by 000425)
--   CSV row 73 STD SP AX        -> mst_product_grade.std_selling_price (registered by 000405)
--   CSV row 74 STD SP BC        -> mst_product_grade.sp_value          (registered by 000405)
--
-- Reaching rows 73/74 also requires re-introducing the PRODUCT_GRADE trigger
-- params. 000406 wiped every param and 000407 re-seeded 142 without restoring
-- the PRODUCT_GRADE fill-group that 000393 used to wire (STD_LOSS_GRADE /
-- BC_LOSS_GRADE), leaving the whole master unreachable.
--
-- The template needs TWO grade triggers, not one — proven by cross-referencing
-- the 000419 Oracle grade seed against the template:
--
--   row 70 NS Loss Type = 'Type 2 NS'    -> pg_name of GRD-20201014 (loss_pct 0.05)
--   row 72 NS Loss      = 0.05           -> that grade's loss_pct
--   row 71 BC Loss Type = 'Type 2POY BC' -> pg_name of GRD-20201002
--   row 73 STD SP AX    = 1.25           -> that grade's std_selling_price
--   row 74 STD SP BC    = 0.75           -> that grade's sp_value
--
-- New codes are used rather than reusing the orphaned STD_VALUE_LOSS /
-- VALUE_LOSS params: 000408 consumes VALUE_LOSS numerically as
-- (1.0 - VALUE_LOSS / 100.0) in F_YARN_BC_LOSS_CAP / _DEL / F_YARN_NON_STD_LOSS,
-- so it cannot also carry a grade name.
--
-- display_order uses the free 143-147 band; the export row order comes from the
-- Go row manifest, not display_order, so this only affects CAPP form ordering.

-- ============================================================
-- PART 1: Insert the 6 new params (idempotent)
-- ============================================================

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, default_value, min_value, max_value, display_group, display_order,
    is_active, created_at, created_by
)
SELECT
    p.code, p.name, p.short_name, p.data_type, p.category,
    u.uom_id, p.default_val::NUMERIC, p.min_val::NUMERIC, p.max_val::NUMERIC,
    p.display_group, p.display_order, TRUE,
    NOW(), 'wire_group_c_000468'
FROM (VALUES
  -- Fixed Cost group: CSV row 9. Grouped with its MC_NAME fill-group siblings
  -- POWER_PER_DAY (83) / MANPOWER_PER_DAY (84), on the free slot 81.
  ('TOTAL_FIXED_COST','Total Fixed Cost','Total Fixed Cost','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL,'Fixed Cost',81),
  -- Quality Loss group: CSV rows 70-74
  ('NS_LOSS_TYPE','NS Loss Type','NS Loss Type','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL,'Quality Loss',143),
  ('BC_LOSS_TYPE','BC Loss Type','BC Loss Type','TEXT','MASTER_LOOKUP',NULL,NULL,NULL,NULL,'Quality Loss',144),
  ('NS_LOSS','NS Loss','NS Loss','NUMBER','MASTER_LOOKUP',NULL,NULL,NULL,NULL,'Quality Loss',145),
  ('STD_SP_AX','STD SP AX','STD SP AX','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL,'Quality Loss',146),
  ('STD_SP_BC','STD SP BC','STD SP BC','NUMBER','MASTER_LOOKUP','USD',NULL,NULL,NULL,'Quality Loss',147)
) AS p(code, name, short_name, data_type, category, uom_code, default_val, min_val, max_val, display_group, display_order)
LEFT JOIN mst_uom u ON u.uom_code = p.uom_code AND u.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Set lookup triggers (lookup_master_code)
-- ============================================================

-- Both grade triggers source mst_product_grade (master registered by 000394).
UPDATE mst_parameter SET lookup_master_code = 'PRODUCT_GRADE',
    updated_at = NOW(), updated_by = 'wire_group_c_000468'
WHERE param_code IN ('NS_LOSS_TYPE', 'BC_LOSS_TYPE') AND deleted_at IS NULL;

-- ============================================================
-- PART 3: Set fill-group children (lookup_fill_group_code + lookup_source_column)
-- ============================================================

-- MC_NAME child — mirrors POWER_PER_DAY / MANPOWER_PER_DAY / OVERHEAD_PER_HEAD.
UPDATE mst_parameter SET lookup_fill_group_code='MC_NAME',      lookup_source_column='mc_tot_fxd_cst',    updated_at=NOW(), updated_by='wire_group_c_000468' WHERE param_code='TOTAL_FIXED_COST' AND deleted_at IS NULL;

-- NS_LOSS_TYPE child
UPDATE mst_parameter SET lookup_fill_group_code='NS_LOSS_TYPE', lookup_source_column='loss_pct',          updated_at=NOW(), updated_by='wire_group_c_000468' WHERE param_code='NS_LOSS'          AND deleted_at IS NULL;

-- BC_LOSS_TYPE children
UPDATE mst_parameter SET lookup_fill_group_code='BC_LOSS_TYPE', lookup_source_column='std_selling_price', updated_at=NOW(), updated_by='wire_group_c_000468' WHERE param_code='STD_SP_AX'        AND deleted_at IS NULL;
UPDATE mst_parameter SET lookup_fill_group_code='BC_LOSS_TYPE', lookup_source_column='sp_value',          updated_at=NOW(), updated_by='wire_group_c_000468' WHERE param_code='STD_SP_BC'        AND deleted_at IS NULL;
