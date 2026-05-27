-- 000241: Expanded textile master formulas (+9 formulas).
--
-- Brings active formula count from 13 -> 22. Each new formula targets a NEW
-- CALCULATED sink (introduced in 000240). result_param_id uniqueness on
-- mst_formula (WHERE deleted_at IS NULL AND is_active = TRUE) is satisfied
-- because each sink is dedicated to exactly one formula.
--
-- Constants used in expressions:
--   * 12       -- water utility IDR / L (proxy until WATER_L_RATE is rated)
--   * 800      -- steam IDR / kg (dyeing) (proxy)
--   * 1500     -- cone packaging IDR / cone (proxy)
--   * 1000     -- ton -> kg conversion
--
-- Idempotency: per-row NOT EXISTS on formula_code. formula_param uses
-- NOT EXISTS on (formula_id, param_id) (unique index).

BEGIN;

-- F_TX_DYE_CHEM: COST_DYE_CHEM = COST_RM_LOADED * (DYESTUFF_OWF_PCT / 100)
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_DYE_CHEM', 'Dye chemical cost proxy', 'CALCULATION',
       'COST_RM_LOADED * (DYESTUFF_OWF_PCT / 100)',
       p.id, 'Proxy: dye chemicals priced as loaded RM * OWF%.', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_DYE_CHEM' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_DYE_CHEM' AND f.deleted_at IS NULL);

-- F_TX_AUX_CHEM: COST_AUX_CHEM = COST_DYE_CHEM * 0.25
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_AUX_CHEM', 'Auxiliary chemical cost (25% of dye chem)', 'CALCULATION',
       'COST_DYE_CHEM * 0.25',
       p.id, 'Auxiliaries (leveling, dispersing, pH) approximated as 25% of dye chem.', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_AUX_CHEM' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_AUX_CHEM' AND f.deleted_at IS NULL);

-- F_TX_WATER_DYE: COST_WATER_DYE = WATER_L_PER_KG * 12
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_WATER_DYE', 'Water cost dyeing', 'CALCULATION',
       'WATER_L_PER_KG * 12',
       p.id, 'Process water cost in dyeing (IDR 12 per L).', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_WATER_DYE' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_WATER_DYE' AND f.deleted_at IS NULL);

-- F_TX_STEAM_DYE: COST_STEAM_DYE = STEAM_KG_DYE * 800
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_STEAM_DYE', 'Steam cost dyeing', 'CALCULATION',
       'STEAM_KG_DYE * 800',
       p.id, 'Steam cost in dyeing (IDR 800 per kg).', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_STEAM_DYE' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_STEAM_DYE' AND f.deleted_at IS NULL);

-- F_TX_GAS: COST_GAS = GAS_NM3_PER_KG * GAS_RATE_M3
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_GAS', 'Natural gas cost per kg', 'CALCULATION',
       'GAS_NM3_PER_KG * GAS_RATE_M3',
       p.id, 'Natural gas burn cost per kg of product.', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_GAS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_GAS' AND f.deleted_at IS NULL);

-- F_TX_CHILLER: COST_CHILLER = CHILL_PER_KG * CHILL_RATE
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_CHILLER', 'Chilled water cost per kg', 'CALCULATION',
       'CHILL_PER_KG * CHILL_RATE',
       p.id, 'Chiller / cooling cost per kg of product.', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_CHILLER' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_CHILLER' AND f.deleted_at IS NULL);

-- F_TX_INSPECT_QC: COST_INSPECT_QC = (INSPECT_HR_TON / 1000) * LBR_RATE_TECH
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_INSPECT_QC', 'Inspection QC labor cost per kg', 'CALCULATION',
       '(INSPECT_HR_TON / 1000) * LBR_RATE_TECH',
       p.id, 'QC inspection labor cost normalized per kg.', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_INSPECT_QC' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_INSPECT_QC' AND f.deleted_at IS NULL);

-- F_TX_PACKING_TOT: COST_PACKING_TOT = CONES_PER_KG * 1500 + (PACK_LBR_HR_TON / 1000) * LBR_RATE_TECH
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_PACKING_TOT', 'Total packing cost per kg', 'CALCULATION',
       'CONES_PER_KG * 1500 + (PACK_LBR_HR_TON / 1000) * LBR_RATE_TECH',
       p.id, 'Packing material (cones at IDR 1500 ea) plus packing labor per kg.', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_PACKING_TOT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_PACKING_TOT' AND f.deleted_at IS NULL);

-- F_TX_TRANSPORT: COST_TRANSPORT = INT_TRANSPORT
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_TX_TRANSPORT', 'Transport cost per kg', 'CALCULATION',
       'INT_TRANSPORT',
       p.id, 'Internal transport cost per kg (passthrough from input).', 'seed_000241'
  FROM mst_parameter p
 WHERE p.param_code = 'COST_TRANSPORT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_TX_TRANSPORT' AND f.deleted_at IS NULL);

-- formula_param links --------------------------------------------------------

-- F_TX_DYE_CHEM
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('COST_RM_LOADED', 0), ('DYESTUFF_OWF_PCT', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_DYE_CHEM' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_AUX_CHEM
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('COST_DYE_CHEM', 0)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_AUX_CHEM' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_WATER_DYE
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('WATER_L_PER_KG', 0)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_WATER_DYE' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_STEAM_DYE
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('STEAM_KG_DYE', 0)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_STEAM_DYE' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_GAS
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('GAS_NM3_PER_KG', 0), ('GAS_RATE_M3', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_GAS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_CHILLER
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('CHILL_PER_KG', 0), ('CHILL_RATE', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_CHILLER' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_INSPECT_QC
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('INSPECT_HR_TON', 0), ('LBR_RATE_TECH', 1)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_INSPECT_QC' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_PACKING_TOT
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('CONES_PER_KG', 0), ('PACK_LBR_HR_TON', 1), ('LBR_RATE_TECH', 2)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_PACKING_TOT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- F_TX_TRANSPORT
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, ord.sort_order
  FROM mst_formula f
  JOIN (VALUES ('INT_TRANSPORT', 0)) AS ord(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = ord.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_TX_TRANSPORT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

COMMIT;
