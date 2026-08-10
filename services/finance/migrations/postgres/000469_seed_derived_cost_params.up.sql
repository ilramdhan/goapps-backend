-- Four derived cost-sheet params + their formulas (Group D1 of the product
-- cost sheet export). These template rows are pure arithmetic over params that
-- already exist, but no mst_parameter / mst_formula ever defined them, so they
-- can never enter cpc_param_snapshot and would print "-" forever.
--
--   CSV row 37 Duty, Inward, Waste          = RM_LANDED_COST - RM_RATE
--   CSV row 66 Forwarding Cost              = constant 0.024
--   CSV row 67 Domestic Cost (AX~AM)        = DELIVERY_COST_QLTY_LOSS + FORWARDING_COST
--   CSV row 95 Domestic cost w/ uneven pack = DOMESTIC_COST
--
-- Design decision (locked): these are engine params, NOT computed in the export
-- layer, so the value is auditable in the snapshot and reusable elsewhere.
--
-- FORWARDING_COST is seeded as a GLOBAL CONSTANT 0.024 per the template, where
-- all 7 route stages carry the same value. ⚠ FLAGGED FOR RE-CONFIRMATION with
-- the costing team: if the rate ever varies by destination or product, this
-- needs a master table and F_YARN_FORWARDING_COST becomes a LOOKUP.
--
-- Row 95 is currently an exact copy of row 67 in the template (1.376 / 1.771 /
-- … identical in every column). The "uneven packing" adjustment term is not
-- present in the Oracle package, so the formula is a pass-through until costing
-- supplies the delta. ⚠ ALSO FLAGGED.
--
-- display_order: DUTY_INWARD_WASTE takes free slot 11 next to its Raw Material
-- inputs (RM_RATE 8 / RM_LANDED_COST 10); the three cost rows take the free
-- 148-150 band adjacent to Analysis (125-142). Export row order comes from the
-- Go row manifest, not display_order — this only affects CAPP form ordering.

BEGIN;

-- ============================================================
-- PART 1: Insert the 4 params (idempotent)
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
    NOW(), 'seed_derived_000469'
FROM (VALUES
  ('DUTY_INWARD_WASTE','Duty, Inward, Waste','Duty Inward Waste','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'Raw Material',11),
  ('FORWARDING_COST','Forwarding Cost','Forwarding Cost','NUMBER','RATE','USD','0.024',NULL,NULL,'Analysis',148),
  ('DOMESTIC_COST','Domestic Cost','Domestic Cost','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'Analysis',149),
  ('DOMESTIC_COST_UNEVEN_PACK','Domestic Cost with Uneven Packing','Domestic Cost Uneven Pack','NUMBER','CALCULATED','USD',NULL,NULL,NULL,'Analysis',150)
) AS p(code, name, short_name, data_type, category, uom_code, default_val, min_val, max_val, display_group, display_order)
LEFT JOIN mst_uom u ON u.uom_code = p.uom_code AND u.deleted_at IS NULL
WHERE NOT EXISTS (
    SELECT 1 FROM mst_parameter WHERE param_code = p.code AND deleted_at IS NULL
);

-- ============================================================
-- PART 2: Insert the 4 formulas (idempotent)
-- ============================================================

INSERT INTO mst_formula (
    formula_code, formula_name, formula_type, expression,
    result_param_id, description, version, is_active, created_at, created_by
)
SELECT f.code, f.name, f.ftype, f.expr,
       (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1),
       f.descr, 1, TRUE, NOW(), 'seed_derived_000469'
FROM (VALUES
  ('F_YARN_DUTY_INWARD_WASTE','Duty, Inward, Waste','CALCULATION','RM_LANDED_COST - RM_RATE','DUTY_INWARD_WASTE','Landing premium over the base RM rate (CSV row 37)'),
  ('F_YARN_FORWARDING_COST','Forwarding Cost','CONSTANT','0.024','FORWARDING_COST','Global forwarding rate per kg — confirm source with costing (CSV row 66)'),
  ('F_YARN_DOMESTIC_COST','Domestic Cost','CALCULATION','DELIVERY_COST_QLTY_LOSS + FORWARDING_COST','DOMESTIC_COST','Delivery cost incl. quality loss plus forwarding (CSV row 67)'),
  ('F_YARN_DOMESTIC_COST_UNEVEN','Domestic Cost with Uneven Packing','CALCULATION','DOMESTIC_COST','DOMESTIC_COST_UNEVEN_PACK','Pass-through until costing supplies the uneven-packing delta (CSV row 95)')
) AS f(code, name, ftype, expr, result_code, descr)
WHERE (SELECT id FROM mst_parameter WHERE param_code = f.result_code AND deleted_at IS NULL LIMIT 1) IS NOT NULL
  AND NOT EXISTS (
      SELECT 1 FROM mst_formula WHERE formula_code = f.code AND deleted_at IS NULL
  );

-- ============================================================
-- PART 3: Link formula inputs (drives topo-sort order in the engine)
-- ============================================================

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1),
    (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1),
    fp.sort_order
FROM (VALUES
  ('F_YARN_DUTY_INWARD_WASTE','RM_LANDED_COST',1),('F_YARN_DUTY_INWARD_WASTE','RM_RATE',2),
  -- F_YARN_FORWARDING_COST is a CONSTANT — no inputs.
  ('F_YARN_DOMESTIC_COST','DELIVERY_COST_QLTY_LOSS',1),('F_YARN_DOMESTIC_COST','FORWARDING_COST',2),
  ('F_YARN_DOMESTIC_COST_UNEVEN','DOMESTIC_COST',1)
) AS fp(fcode, pcode, sort_order)
WHERE
    (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1) IS NOT NULL
AND NOT EXISTS (
    SELECT 1 FROM formula_param fp2
    WHERE fp2.formula_id = (SELECT id FROM mst_formula   WHERE formula_code = fp.fcode AND deleted_at IS NULL LIMIT 1)
      AND fp2.param_id   = (SELECT id FROM mst_parameter WHERE param_code   = fp.pcode AND deleted_at IS NULL LIMIT 1)
);

-- ============================================================
-- PART 4: Checklist the 4 params into CAPP
-- ============================================================
-- LoadFormulas only returns formulas whose result param has a CAPP row for that
-- product (see loadPerProductFormulas' JOIN). Without this the formulas would
-- exist but never run.
--
-- Scope: products that already carry the upstream inputs, i.e. the yarn cost
-- model set — NOT every row in cost_product_master. Bought-out / non-costed
-- products stay untouched.

INSERT INTO cost_product_applicable_param (
    capp_product_sys_id, capp_param_id,
    capp_is_required, capp_display_order, capp_created_by
)
SELECT DISTINCT
    src.capp_product_sys_id,
    np.id,
    FALSE, NULL::INT, 'seed_derived_000469'
FROM cost_product_applicable_param src
JOIN mst_parameter sp ON sp.id = src.capp_param_id AND sp.deleted_at IS NULL
CROSS JOIN mst_parameter np
WHERE sp.param_code IN ('RM_LANDED_COST', 'DELIVERY_COST_QLTY_LOSS')
  AND np.param_code IN ('DUTY_INWARD_WASTE', 'FORWARDING_COST', 'DOMESTIC_COST', 'DOMESTIC_COST_UNEVEN_PACK')
  AND np.deleted_at IS NULL
  AND NOT EXISTS (
      SELECT 1 FROM cost_product_applicable_param capp
      WHERE capp.capp_product_sys_id = src.capp_product_sys_id
        AND capp.capp_param_id       = np.id
  );

COMMIT;
