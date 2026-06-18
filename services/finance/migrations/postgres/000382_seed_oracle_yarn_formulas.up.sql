-- 000382: Seed Oracle yarn calculation formula chain.
-- All formulas write to CALCULATED params seeded in 000381.
-- Terminal formula produces COST_STAGE_OUT (required by ScopeKeyFinalCost).
-- Idempotency: NOT EXISTS guard on formula_code AND result_param_id.

BEGIN;

-- ─── SEQ 0: RM Norms ──────────────────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_RM_NORMS', 'RM Consumption Norms', 'CALCULATION',
       '1.0 / (1.0 - WASTE_PCT / 100.0)',
       p.id, 'Oracle SEQ 0: norms = 1/(1-waste%).', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'RM_NORMS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_RM_NORMS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.sort_order
  FROM mst_formula f
  JOIN (VALUES ('WASTE_PCT', 0)) AS v(param_code, sort_order) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.param_code AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_RM_NORMS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 1a: Grade Weights (AE, A9, A, B, C) ─────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_AE_WT', 'AE Bobbin Weight', 'CALCULATION',
       'AX_WT * AE_PERC / AX_PERC',
       p.id, 'Oracle SEQ 1: AE weight from AX proportionally.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'AE_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_AE_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('AX_WT',0),('AE_PERC',1),('AX_PERC',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_AE_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_A9_WT', 'A9 Bobbin Weight', 'CALCULATION',
       'AX_WT * A9_PERC / AX_PERC',
       p.id, 'Oracle SEQ 1.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'A9_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_A9_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('AX_WT',0),('A9_PERC',1),('AX_PERC',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_A9_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_A_WT', 'A Bobbin Weight', 'CALCULATION',
       'AX_WT * A_PERC / AX_PERC',
       p.id, 'Oracle SEQ 1.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'A_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_A_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('AX_WT',0),('A_PERC',1),('AX_PERC',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_A_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_B_WT', 'B Bobbin Weight', 'CALCULATION',
       'AX_WT * B_PERC / AX_PERC',
       p.id, 'Oracle SEQ 1.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'B_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_B_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('AX_WT',0),('B_PERC',1),('AX_PERC',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_B_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_C_WT', 'C Bobbin Weight', 'CALCULATION',
       'AX_WT * C_PERC / AX_PERC',
       p.id, 'Oracle SEQ 1.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'C_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_C_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('AX_WT',0),('C_PERC',1),('AX_PERC',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_C_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 1b: Net Bobbin Weight ────────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_NET_BOB_WT', 'Net Bobbin Weight', 'CALCULATION',
       'AX_WT + AE_WT + A9_WT + A_WT + B_WT + C_WT',
       p.id, 'Oracle SEQ 1: sum of all grade weights.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'NET_BOB_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_NET_BOB_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('AX_WT',0),('AE_WT',1),('A9_WT',2),('A_WT',3),('B_WT',4),('C_WT',5)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_NET_BOB_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 1c: Oil, MB, Intermingle costs ───────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_OIL_COST', 'Coning Oil Cost', 'CALCULATION',
       'OIL_RATE * OPU / 100.0',
       p.id, 'Oracle SEQ 1: oil cost per kg yarn.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'OIL_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_OIL_COST' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('OIL_RATE',0),('OPU',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_OIL_COST' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_MB_COST', 'Master Batch Cost', 'CALCULATION',
       'MB_RATE * MB_DOZING_PCT / 100.0',
       p.id, 'Oracle SEQ 1: MB cost per kg yarn.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'MB_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_MB_COST' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('MB_RATE',0),('MB_DOZING_PCT',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_MB_COST' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_RP_DOZING', 'Recipe Dozing Rate', 'CALCULATION',
       'MB_DOZING_PCT > 0 ? MB_DOZING_PCT : 0',
       p.id, 'Oracle SEQ 1: pass-through dozing rate.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'RP_DOZING' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_RP_DOZING' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 0 FROM mst_formula f
  JOIN mst_parameter p ON p.param_code = 'MB_DOZING_PCT' AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_RP_DOZING' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 2: Net Production + Per-kg Utilities ────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_NET_PROD', 'Net Production kg/day', 'CALCULATION',
       'NO_OF_POSITION * MC_SPEED * (MC_EFFICIENCY / 100.0) * 1440.0 * YARN_DENIER / 9000000.0',
       p.id, 'Oracle SEQ 2: machine output per day.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'NET_PRODUCTION' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_NET_PROD' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('NO_OF_POSITION',0),('MC_SPEED',1),('MC_EFFICIENCY',2),('YARN_DENIER',3)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_NET_PROD' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_POWER_KG', 'Power Cost per kg', 'CALCULATION',
       'NET_PRODUCTION > 0 ? POWER_PER_DAY / NET_PRODUCTION : 0',
       p.id, 'Oracle SEQ 2.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'POWER_PER_KG' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_POWER_KG' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('POWER_PER_DAY',0),('NET_PRODUCTION',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_POWER_KG' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_MANPOWER_KG', 'Manpower Cost per kg', 'CALCULATION',
       'NET_PRODUCTION > 0 ? MANPOWER_PER_DAY / NET_PRODUCTION : 0',
       p.id, 'Oracle SEQ 2.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'MANPOWER_PER_KG' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_MANPOWER_KG' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('MANPOWER_PER_DAY',0),('NET_PRODUCTION',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_MANPOWER_KG' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_OVERHEAD_KG', 'Overhead per kg', 'CALCULATION',
       'NET_PRODUCTION > 0 ? OVERHEAD_PER_DAY * NO_OF_END / NET_PRODUCTION : 0',
       p.id, 'Oracle SEQ 2.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'OVERHEAD_PER_KG' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_OVERHEAD_KG' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('OVERHEAD_PER_DAY',0),('NO_OF_END',1),('NET_PRODUCTION',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_OVERHEAD_KG' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_SPARES_KG', 'Spares per kg', 'CALCULATION',
       'NET_PRODUCTION > 0 ? SPARES_PER_DAY / NET_PRODUCTION : 0',
       p.id, 'Oracle SEQ 2.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'SPARES_PER_KG' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_SPARES_KG' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('SPARES_PER_DAY',0),('NET_PRODUCTION',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_SPARES_KG' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_TOTAL_FIXED', 'Total Fixed Cost per kg', 'CALCULATION',
       'POWER_PER_KG + MANPOWER_PER_KG + OVERHEAD_PER_KG + SPARES_PER_KG',
       p.id, 'Oracle SEQ 2.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'TOTAL_FIXED_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_TOTAL_FIXED' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('POWER_PER_KG',0),('MANPOWER_PER_KG',1),('OVERHEAD_PER_KG',2),('SPARES_PER_KG',3)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_TOTAL_FIXED' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 3: Box Weights ───────────────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_CAP_BOX_WT', 'Captive Box Weight', 'CALCULATION',
       'CAP_NO_OF_BOB * NET_BOB_WT * RM_NORMS',
       p.id, 'Oracle SEQ 3.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'CAP_BOX_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_CAP_BOX_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CAP_NO_OF_BOB',0),('NET_BOB_WT',1),('RM_NORMS',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_CAP_BOX_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_DEL_BOX_WT', 'Delivery Box Weight', 'CALCULATION',
       'DEL_NO_OF_BOB * NET_BOB_WT * RM_NORMS',
       p.id, 'Oracle SEQ 3.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'DEL_BOX_WT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_DEL_BOX_WT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('DEL_NO_OF_BOB',0),('NET_BOB_WT',1),('RM_NORMS',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_DEL_BOX_WT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 4: Packing Costs ─────────────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_CAP_PACK', 'Captive Packing Cost/kg', 'CALCULATION',
       'CAP_BOX_WT > 0 ? (CAP_NO_OF_BOB * CAP_BOB_RATE + CAP_BOX_RATE) / CAP_BOX_WT : 0',
       p.id, 'Oracle SEQ 4.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'CAP_PACK_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_CAP_PACK' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CAP_NO_OF_BOB',0),('CAP_BOB_RATE',1),('CAP_BOX_RATE',2),('CAP_BOX_WT',3)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_CAP_PACK' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_DEL_PACK', 'Delivery Packing Cost/kg', 'CALCULATION',
       'DEL_BOX_WT > 0 ? (DEL_NO_OF_BOB * DEL_BOB_RATE + DEL_BOX_RATE) / DEL_BOX_WT : 0',
       p.id, 'Oracle SEQ 4.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'DEL_PACK_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_DEL_PACK' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('DEL_NO_OF_BOB',0),('DEL_BOB_RATE',1),('DEL_BOX_RATE',2),('DEL_BOX_WT',3)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_DEL_PACK' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 5: Conversion Subtotals ──────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_CONV_CAP', 'Conversion Captive excl MB', 'CALCULATION',
       'TOTAL_FIXED_COST + CAP_PACK_COST + OIL_COST + INTERMINGLE_COST + SPECIAL_COST_1',
       p.id, 'Oracle SEQ 5.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'CONV_CAP_EX_MB' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_CONV_CAP' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('TOTAL_FIXED_COST',0),('CAP_PACK_COST',1),('OIL_COST',2),('INTERMINGLE_COST',3),('SPECIAL_COST_1',4)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_CONV_CAP' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_CONV_DEL', 'Conversion Delivery excl MB', 'CALCULATION',
       'TOTAL_FIXED_COST + DEL_PACK_COST + OIL_COST + INTERMINGLE_COST + SPECIAL_COST_1',
       p.id, 'Oracle SEQ 5.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'CONV_DEL_EX_MB' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_CONV_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('TOTAL_FIXED_COST',0),('DEL_PACK_COST',1),('OIL_COST',2),('INTERMINGLE_COST',3),('SPECIAL_COST_1',4)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_CONV_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 6: Pre-Quality-Loss Costs ────────────────────────────────────────────
-- COST_RM_TOTAL is injected by aggregateRMCost in the engine before formula eval.
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_CAP_PRE_QL', 'Captive Cost before Qloss', 'CALCULATION',
       'RM_NORMS * COST_RM_TOTAL + CONV_CAP_EX_MB',
       p.id, 'Oracle SEQ 6. COST_RM_TOTAL injected by engine from route RM.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'CAP_COST_PRE_QL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_CAP_PRE_QL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('RM_NORMS',0),('CONV_CAP_EX_MB',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_CAP_PRE_QL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_DEL_PRE_QL', 'Delivery Cost before Qloss', 'CALCULATION',
       'RM_NORMS * COST_RM_TOTAL + CONV_DEL_EX_MB',
       p.id, 'Oracle SEQ 6.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'DEL_COST_PRE_QL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_DEL_PRE_QL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('RM_NORMS',0),('CONV_DEL_EX_MB',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_DEL_PRE_QL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 7: Quality Losses ────────────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_NON_STD_LOSS', 'Non-Standard Value Loss', 'CALCULATION',
       'CAP_COST_PRE_QL * (NON_STD_PERC / 100.0) * (1.0 - BC_RECOVERY_RATE / 100.0)',
       p.id, 'Oracle SEQ 7.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'NON_STD_LOSS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_NON_STD_LOSS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CAP_COST_PRE_QL',0),('NON_STD_PERC',1),('BC_RECOVERY_RATE',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_NON_STD_LOSS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_BC_LOSS_CAP', 'BC Value Loss Captive', 'CALCULATION',
       'CAP_COST_PRE_QL * (BC_PERC / 100.0) * (1.0 - BC_RECOVERY_RATE / 100.0)',
       p.id, 'Oracle SEQ 7.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'BC_LOSS_CAP' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_BC_LOSS_CAP' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CAP_COST_PRE_QL',0),('BC_PERC',1),('BC_RECOVERY_RATE',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_BC_LOSS_CAP' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_BC_LOSS_DEL', 'BC Value Loss Delivery', 'CALCULATION',
       'DEL_COST_PRE_QL * (BC_PERC / 100.0) * (1.0 - BC_RECOVERY_RATE / 100.0)',
       p.id, 'Oracle SEQ 7.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'BC_LOSS_DEL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_BC_LOSS_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('DEL_COST_PRE_QL',0),('BC_PERC',1),('BC_RECOVERY_RATE',2)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_BC_LOSS_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 8: Quality Loss Totals ───────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_QLOSS_CAP', 'Quality Loss Captive', 'CALCULATION',
       'BC_LOSS_CAP + NON_STD_LOSS',
       p.id, 'Oracle SEQ 8.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'QLOSS_CAP' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_QLOSS_CAP' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('BC_LOSS_CAP',0),('NON_STD_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_QLOSS_CAP' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_QLOSS_DEL', 'Quality Loss Delivery', 'CALCULATION',
       'BC_LOSS_DEL + NON_STD_LOSS',
       p.id, 'Oracle SEQ 8.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'QLOSS_DEL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_QLOSS_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('BC_LOSS_DEL',0),('NON_STD_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_QLOSS_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 9: Final Costs ───────────────────────────────────────────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_CAP_FINAL', 'Final Captive Cost', 'CALCULATION',
       'CAP_COST_PRE_QL + QLOSS_CAP',
       p.id, 'Oracle SEQ 9.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'COST_CAP_FINAL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_CAP_FINAL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CAP_COST_PRE_QL',0),('QLOSS_CAP',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_CAP_FINAL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_DEL_FINAL', 'Final Delivery Cost', 'CALCULATION',
       'DEL_COST_PRE_QL + QLOSS_DEL',
       p.id, 'Oracle SEQ 9.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'COST_DEL_FINAL' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_DEL_FINAL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('DEL_COST_PRE_QL',0),('QLOSS_DEL',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_DEL_FINAL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── SEQ 10: Volume Bucket Costs (guard VOL_BUCKET_N_QTY > 0) ────────────────
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB1_LOSS', 'VB1 Change-Over Loss', 'CALCULATION',
       'VOL_BUCKET_1_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_1_QTY : 0',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB1_LOSS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB1_LOSS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CHANGE_OVER_KG',0),('VOL_BUCKET_1_QTY',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB1_LOSS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB2_LOSS', 'VB2 Change-Over Loss', 'CALCULATION',
       'VOL_BUCKET_2_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_2_QTY : 0',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB2_LOSS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB2_LOSS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CHANGE_OVER_KG',0),('VOL_BUCKET_2_QTY',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB2_LOSS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB3_LOSS', 'VB3 Change-Over Loss', 'CALCULATION',
       'VOL_BUCKET_3_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_3_QTY : 0',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB3_LOSS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB3_LOSS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CHANGE_OVER_KG',0),('VOL_BUCKET_3_QTY',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB3_LOSS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB4_LOSS', 'VB4 Change-Over Loss', 'CALCULATION',
       'VOL_BUCKET_4_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_4_QTY : 0',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB4_LOSS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB4_LOSS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CHANGE_OVER_KG',0),('VOL_BUCKET_4_QTY',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB4_LOSS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB5_LOSS', 'VB5 Change-Over Loss', 'CALCULATION',
       'VOL_BUCKET_5_QTY > 0 ? CHANGE_OVER_KG / VOL_BUCKET_5_QTY : 0',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB5_LOSS' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB5_LOSS' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('CHANGE_OVER_KG',0),('VOL_BUCKET_5_QTY',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB5_LOSS' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB1_DEL', 'VB1 Delivery Cost', 'CALCULATION',
       'COST_DEL_FINAL + VB1_LOSS',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB1_DEL_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB1_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('COST_DEL_FINAL',0),('VB1_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB1_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB2_DEL', 'VB2 Delivery Cost', 'CALCULATION',
       'COST_DEL_FINAL + VB2_LOSS',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB2_DEL_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB2_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('COST_DEL_FINAL',0),('VB2_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB2_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB3_DEL', 'VB3 Delivery Cost', 'CALCULATION',
       'COST_DEL_FINAL + VB3_LOSS',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB3_DEL_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB3_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('COST_DEL_FINAL',0),('VB3_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB3_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB4_DEL', 'VB4 Delivery Cost', 'CALCULATION',
       'COST_DEL_FINAL + VB4_LOSS',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB4_DEL_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB4_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('COST_DEL_FINAL',0),('VB4_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB4_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_VB5_DEL', 'VB5 Delivery Cost', 'CALCULATION',
       'COST_DEL_FINAL + VB5_LOSS',
       p.id, 'Oracle SEQ 10.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'VB5_DEL_COST' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_VB5_DEL' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, v.so FROM mst_formula f
  JOIN (VALUES ('COST_DEL_FINAL',0),('VB5_LOSS',1)) AS v(pc,so) ON TRUE
  JOIN mst_parameter p ON p.param_code = v.pc AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_VB5_DEL' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- ─── TERMINAL: COST_STAGE_OUT = COST_DEL_FINAL ───────────────────────────────
-- ScopeKeyFinalCost in compute.go reads "COST_STAGE_OUT" as the per-unit output.
INSERT INTO mst_formula (formula_code, formula_name, formula_type, expression, result_param_id, description, created_by)
SELECT 'F_YARN_STAGE_OUT', 'Terminal engine sink', 'CALCULATION',
       'COST_DEL_FINAL',
       p.id, 'Passthrough: COST_STAGE_OUT = COST_DEL_FINAL. Required by ScopeKeyFinalCost.', 'seed_000382'
  FROM mst_parameter p WHERE p.param_code = 'COST_STAGE_OUT' AND p.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM mst_formula f WHERE f.formula_code = 'F_YARN_STAGE_OUT' AND f.deleted_at IS NULL)
   AND NOT EXISTS (SELECT 1 FROM mst_formula fchk WHERE fchk.result_param_id = p.id AND fchk.deleted_at IS NULL);

INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 0 FROM mst_formula f
  JOIN mst_parameter p ON p.param_code = 'COST_DEL_FINAL' AND p.deleted_at IS NULL
 WHERE f.formula_code = 'F_YARN_STAGE_OUT' AND f.deleted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

COMMIT;
