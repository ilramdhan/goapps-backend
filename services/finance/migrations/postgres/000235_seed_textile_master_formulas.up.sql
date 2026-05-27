-- 000235: Textile master formulas catalog (S8e-fix 2/3).
--
-- Extends the canonical formula chain with textile-specific cost components.
-- The existing chain in production is:
--     COST_RM_LOADED = COST_RM_TOTAL * (1 + MAT_OVERHEAD_PCT / 100)
--     COST_ELEC      = ELEC_KWH * ELEC_RATE
--     COST_LABOR     = LABOR_HRS * LABOR_RATE
--     COST_LABOR_FULL = COST_LABOR * (1 + LABOR_OVERHEAD_PCT / 100)
--     COST_CONVERSION = COST_ELEC + COST_LABOR_FULL + DEPREC_PER_KG
--     COST_STAGE_OUT  = (COST_RM_LOADED + COST_CONVERSION) * (1 + WASTE_PCT / 100)
--
-- The engine reads COST_RM_TOTAL (populated from upstream RM) and COST_STAGE_OUT
-- (the terminal sink). We leave that chain UNCHANGED. The new formulas below
-- chain into NEW calculated sinks that exercise additional code paths
-- (more inputs per formula, longer topological chain) without conflicting
-- with the existing result_param_id uniqueness in the calc engine flow.
--
-- Idempotency: each INSERT uses NOT EXISTS guard on formula_code.
-- formula_param rows are inserted only when the parent formula was just
-- created in this run.

BEGIN;

-- F_TEXTILE_STEAM: COST_STEAM = STEAM_KG * STEAM_RATE
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TEXTILE_STEAM', 'Steam cost per kg', 'CALCULATION',
       'STEAM_KG * STEAM_RATE',
       p.id, 'Cost of steam per kg of product (textile dyeing / finishing).', 'seed_000235'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_STEAM' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TEXTILE_STEAM' AND f.deleted_at IS NULL);

-- F_TEXTILE_WATER: COST_WATER = WATER_M3 * WATER_RATE
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TEXTILE_WATER', 'Water cost per kg', 'CALCULATION',
       'WATER_M3 * WATER_RATE',
       p.id, 'Cost of process water per kg of product.', 'seed_000235'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_WATER' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TEXTILE_WATER' AND f.deleted_at IS NULL);

-- F_TEXTILE_UTIL: COST_UTIL = COST_ELEC + COST_STEAM + COST_WATER + AIR_PER_KG
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TEXTILE_UTIL', 'Total utility cost', 'CALCULATION',
       'COST_ELEC + COST_STEAM + COST_WATER + AIR_PER_KG',
       p.id, 'Sum of electricity, steam, water, compressed air per kg.', 'seed_000235'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_UTIL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TEXTILE_UTIL' AND f.deleted_at IS NULL);

-- F_TEXTILE_OVERHEAD: COST_OVERHEAD = MAINT_PER_KG + FACTORY_OH + QC_PER_KG + PACK_PER_KG
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TEXTILE_OVERHEAD', 'Total overhead cost', 'CALCULATION',
       'MAINT_PER_KG + FACTORY_OH + QC_PER_KG + PACK_PER_KG',
       p.id, 'Sum of maintenance, factory OH, QC, packing per kg.', 'seed_000235'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_OVERHEAD' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TEXTILE_OVERHEAD' AND f.deleted_at IS NULL);

-- F_TEXTILE_AFTER_YLD: COST_AFTER_YLD = COST_STAGE_OUT / (YIELD_PCT / 100)
-- Divides the engine's terminal cost by yield to get yield-corrected cost.
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TEXTILE_AFTER_YLD', 'Yield-corrected cost', 'CALCULATION',
       'COST_STAGE_OUT / (YIELD_PCT / 100)',
       p.id, 'Yield-adjusted total cost per kg.', 'seed_000235'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_AFTER_YLD' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TEXTILE_AFTER_YLD' AND f.deleted_at IS NULL);

-- F_TEXTILE_SELLING: SELLING_PRICE = COST_AFTER_YLD * (1 + MARGIN_PCT / 100)
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TEXTILE_SELLING', 'Selling price (cost + margin)', 'CALCULATION',
       'COST_AFTER_YLD * (1 + MARGIN_PCT / 100)',
       p.id, 'Suggested selling price for FG products.', 'seed_000235'
  FROM mst_parameter p
 WHERE p.param_code = 'SELLING_PRICE' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TEXTILE_SELLING' AND f.deleted_at IS NULL);

-- formula_param join rows for each new formula -------------------------------
-- We resolve formula_id + param_id by code and skip insert if the pair already
-- exists (idempotent re-run).

-- F_TEXTILE_STEAM inputs: STEAM_KG, STEAM_RATE
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('STEAM_KG', 0), ('STEAM_RATE', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TEXTILE_STEAM' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TEXTILE_WATER inputs: WATER_M3, WATER_RATE
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('WATER_M3', 0), ('WATER_RATE', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TEXTILE_WATER' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TEXTILE_UTIL inputs: COST_ELEC, COST_STEAM, COST_WATER, AIR_PER_KG
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('COST_ELEC', 0), ('COST_STEAM', 1), ('COST_WATER', 2), ('AIR_PER_KG', 3)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TEXTILE_UTIL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TEXTILE_OVERHEAD inputs
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('MAINT_PER_KG', 0), ('FACTORY_OH', 1), ('QC_PER_KG', 2), ('PACK_PER_KG', 3)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TEXTILE_OVERHEAD' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TEXTILE_AFTER_YLD inputs
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('COST_STAGE_OUT', 0), ('YIELD_PCT', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TEXTILE_AFTER_YLD' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TEXTILE_SELLING inputs
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('COST_AFTER_YLD', 0), ('MARGIN_PCT', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TEXTILE_SELLING' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

COMMIT;
