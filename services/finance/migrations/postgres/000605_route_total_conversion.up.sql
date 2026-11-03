-- 000605 - Route total conversion cost (Excel conv-cost-calc.xlsx cols EF/EG/EH/EI).
-- Additive only: 5 params, 4 CALCULATION formulas, their edges, and CAPP backfill
-- for non-MB products that already carry CAPTIVE_COST_QLTY_LOSS.
-- UPSTREAM_TOTAL_CAP_CONV is engine-injected (costcalc conversion inheritance), no formula.
-- Markers: seed_000605 (params/formulas), seed_route_total_conv_000605 (CAPP).

BEGIN;

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, is_active, created_at, created_by
)
SELECT
    'CAP_CONV_STAGE', 'Captive Conversion (Stage)', 'Cap Conv Stage', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'Analysis', 143,
    'Stage captive conversion of this route step = CAPTIVE_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS. Excel conv-cost-calc.xlsx column EF.',
    TRUE, NOW(), 'seed_000605'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'CAP_CONV_STAGE' AND deleted_at IS NULL
);

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, is_active, created_at, created_by
)
SELECT
    'DEL_CONV_STAGE', 'Delivery Conversion (Stage)', 'Del Conv Stage', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'Analysis', 144,
    'Stage delivery conversion of this route step = DELIVERY_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS. Excel conv-cost-calc.xlsx column EG.',
    TRUE, NOW(), 'seed_000605'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'DEL_CONV_STAGE' AND deleted_at IS NULL
);

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, is_active, created_at, created_by
)
SELECT
    'UPSTREAM_TOTAL_CAP_CONV', 'Upstream Total Captive Conversion', 'Upstream Tot Cap Conv', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'Analysis', 145,
    'Engine-injected by costcalc conversion inheritance (upstream route total captive conversion); intentionally NO formula. Used by TOTAL_CAP_CONV / TOTAL_DEL_CONV.',
    TRUE, NOW(), 'seed_000605'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'UPSTREAM_TOTAL_CAP_CONV' AND deleted_at IS NULL
);

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, is_active, created_at, created_by
)
SELECT
    'TOTAL_CAP_CONV', 'Total Captive Conversion (Route)', 'Total Cap Conv', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'Analysis', 146,
    'Route total captive conversion = CAP_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV. Excel conv-cost-calc.xlsx column EH.',
    TRUE, NOW(), 'seed_000605'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'TOTAL_CAP_CONV' AND deleted_at IS NULL
);

INSERT INTO mst_parameter (
    param_code, param_name, param_short_name, data_type, param_category,
    uom_id, owner_department, is_required_for_costing, is_period_dependent,
    display_group, display_order, notes, is_active, created_at, created_by
)
SELECT
    'TOTAL_DEL_CONV', 'Total Delivery Conversion (Route)', 'Total Del Conv', 'NUMBER', 'CALCULATED',
    (SELECT u.uom_id FROM mst_uom u WHERE u.uom_code = 'USD' AND u.deleted_at IS NULL LIMIT 1),
    'Finance', FALSE, FALSE,
    'Analysis', 147,
    'Route total delivery conversion = DEL_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV. Excel conv-cost-calc.xlsx column EI.',
    TRUE, NOW(), 'seed_000605'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = 'TOTAL_DEL_CONV' AND deleted_at IS NULL
);

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT 'F_YARN_CAP_CONV_STAGE', 'Captive Conversion (Stage)', 'CALCULATION', 'CAPTIVE_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS',
       p.id, 'CAP_CONV_STAGE = CAPTIVE_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS', 1, TRUE, NOW(), 'seed_000605'
FROM mst_parameter p
WHERE p.param_code = 'CAP_CONV_STAGE'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = 'F_YARN_CAP_CONV_STAGE' AND deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE result_param_id = p.id AND deleted_at IS NULL
  );

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT 'F_YARN_DEL_CONV_STAGE', 'Delivery Conversion (Stage)', 'CALCULATION', 'DELIVERY_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS',
       p.id, 'DEL_CONV_STAGE = DELIVERY_COST_QLTY_LOSS - RM_RATE + VOLUME_BUCKET_4_LOSS', 1, TRUE, NOW(), 'seed_000605'
FROM mst_parameter p
WHERE p.param_code = 'DEL_CONV_STAGE'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = 'F_YARN_DEL_CONV_STAGE' AND deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE result_param_id = p.id AND deleted_at IS NULL
  );

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT 'F_YARN_TOTAL_CAP_CONV', 'Total Captive Conversion (Route)', 'CALCULATION', 'CAP_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV',
       p.id, 'TOTAL_CAP_CONV = CAP_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV', 1, TRUE, NOW(), 'seed_000605'
FROM mst_parameter p
WHERE p.param_code = 'TOTAL_CAP_CONV'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = 'F_YARN_TOTAL_CAP_CONV' AND deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE result_param_id = p.id AND deleted_at IS NULL
  );

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT 'F_YARN_TOTAL_DEL_CONV', 'Total Delivery Conversion (Route)', 'CALCULATION', 'DEL_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV',
       p.id, 'TOTAL_DEL_CONV = DEL_CONV_STAGE + UPSTREAM_TOTAL_CAP_CONV', 1, TRUE, NOW(), 'seed_000605'
FROM mst_parameter p
WHERE p.param_code = 'TOTAL_DEL_CONV'
  AND p.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = 'F_YARN_TOTAL_DEL_CONV' AND deleted_at IS NULL
  )
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE result_param_id = p.id AND deleted_at IS NULL
  );

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 1
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'CAPTIVE_COST_QLTY_LOSS' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_CAP_CONV_STAGE' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 2
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'RM_RATE' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_CAP_CONV_STAGE' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 3
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'VOLUME_BUCKET_4_LOSS' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_CAP_CONV_STAGE' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 1
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'DELIVERY_COST_QLTY_LOSS' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_DEL_CONV_STAGE' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 2
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'RM_RATE' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_DEL_CONV_STAGE' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 3
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'VOLUME_BUCKET_4_LOSS' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_DEL_CONV_STAGE' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 1
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'CAP_CONV_STAGE' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_TOTAL_CAP_CONV' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 2
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'UPSTREAM_TOTAL_CAP_CONV' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_TOTAL_CAP_CONV' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 1
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'DEL_CONV_STAGE' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_TOTAL_DEL_CONV' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 2
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'UPSTREAM_TOTAL_CAP_CONV' AND p.deleted_at IS NULL
WHERE f.formula_code = 'F_YARN_TOTAL_DEL_CONV' AND f.deleted_at IS NULL AND f.created_by = 'seed_000605'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- CAPP backfill: non-MB products that already carry CAPTIVE_COST_QLTY_LOSS.
-- Only 4 params are attached. UPSTREAM_TOTAL_CAP_CONV is deliberately NOT attached:
-- it is CALCULATED with no formula (engine-injected), and costcalc
-- rejectCalculatedParamsWithoutFormula (calculated_param_formula_guard.go) fails any
-- product whose CAPP holds a consumed CALCULATED param without an active formula.
INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id, capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT base.capp_product_sys_id, np.id, FALSE, CAST(NULL AS INT), 'seed_route_total_conv_000605'
FROM cost_product_applicable_param base
JOIN mst_parameter bp ON bp.id = base.capp_param_id
 AND bp.param_code = 'CAPTIVE_COST_QLTY_LOSS' AND bp.deleted_at IS NULL
JOIN cost_product_master pm ON pm.cpm_product_sys_id = base.capp_product_sys_id
JOIN cost_product_type pt ON pt.cpt_type_id = pm.cpm_product_type_id AND pt.cpt_type_code <> 'MB'
CROSS JOIN mst_parameter np
WHERE np.deleted_at IS NULL
  -- Only products whose final cost is pinned by the explicit COST_STAGE_OUT sink
  -- (000515). Without it, resolveFinalCost falls back to findTerminalFormula
  -- (deepest terminal formula), and F_YARN_TOTAL_DEL_CONV could become that
  -- terminal and change cpc_cost_per_unit. With the sink present the new
  -- formulas are informational only and no existing cost number can move.
  AND EXISTS (
      SELECT 1
      FROM cost_product_applicable_param so
      JOIN mst_parameter sp ON sp.id = so.capp_param_id
       AND sp.param_code = 'COST_STAGE_OUT' AND sp.deleted_at IS NULL
      WHERE so.capp_product_sys_id = base.capp_product_sys_id
  )
  AND np.param_code IN ('CAP_CONV_STAGE','DEL_CONV_STAGE','TOTAL_CAP_CONV','TOTAL_DEL_CONV')
ON CONFLICT ON CONSTRAINT capp_unique_product_param DO NOTHING;

DO $$
DECLARE
    v_params INT;
    v_formulas INT;
    v_capp INT;
BEGIN
    SELECT COUNT(*) INTO v_params FROM mst_parameter WHERE created_by = 'seed_000605' AND deleted_at IS NULL;
    SELECT COUNT(*) INTO v_formulas FROM mst_formula WHERE created_by = 'seed_000605' AND deleted_at IS NULL;
    SELECT COUNT(*) INTO v_capp FROM cost_product_applicable_param WHERE capp_created_by = 'seed_route_total_conv_000605';
    RAISE NOTICE '000605: params=%, formulas=%, capp rows=%', v_params, v_formulas, v_capp;
END $$;

COMMIT;
