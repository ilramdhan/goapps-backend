-- Boughtout MB: MB_RM_COST is the whole cost (Finance feedback 2026-11).
--
-- Before: F_MB_FINAL_COST already returned MB_RM_COST for IS_BOUGHTOUT = 1, but
-- MB_WASTE_VAL / MB_FIXED_TOTAL / MB_COST_OTHERS / MB_CONV_COST were still computed
-- and shown (snapshot + cost-calc-detail export) as if the MB were produced in-house.
-- After: those four are 0 when IS_BOUGHTOUT = 1, so MB_FINAL_COST = MB_RM_COST and
-- MB_RM_COST + MB_CONV_COST is the same number. For IS_BOUGHTOUT = 0 every expression evaluates exactly as before.
--
-- PART 1  Wrap the four expressions in `IS_BOUGHTOUT == 1 ? 0 : (<old>)`. Guarded on the
--   exact 000452 text (change policy, docs/costing-formulas.md §2): a hand edit is never
--   clobbered. Rows are tagged updated_by = 'mb_boughtout_000613'.
-- PART 2  Add the IS_BOUGHTOUT input edge to each rewritten formula (every param an
--   expression references needs a formula_param edge). IS_BOUGHTOUT is an INPUT, so no cycle.
-- PART 3  Data fix: re-sync the frozen IS_BOUGHTOUT CPP value of each MB cost product
--   from mst_mb_head.mbh_is_boughtout (the source of truth). mbFreezeCostParams only writes
--   it at validate time, and 58 heads (local snapshot 2026-11) carry a stale TRUE although
--   the head is no longer boughtout — their costs silently dropped the conversion cost.
--   Only products owned by exactly one live head are touched. NOT reverted by the down
--   migration: the stale value was wrong, not a state worth restoring.
--
-- Existing cst_product_cost rows are NOT rewritten here (frozen history). Re-run the MB
-- batch for each open period to produce corrected rows (old ones become SUPERSEDED).
--
-- Every statement is guarded; re-running is a no-op.

BEGIN;

-- PART 1
UPDATE mst_formula f
SET expression = 'IS_BOUGHTOUT == 1 ? 0 : (' || f.expression || ')',
    updated_at = NOW(),
    updated_by = 'mb_boughtout_000613'
FROM (VALUES
  ('F_MB_WASTE_VAL',   '(MB_RM_COST / (1 - MB_WASTE/100)) * (MB_WASTE/100)'),
  ('F_MB_FIXED_COST',  'MB_NET_PROD > 0 ? MACHINE_MB_FIXED_TOTAL / MB_NET_PROD : 0'),
  ('F_MB_COST_OTHERS', '((MB_RM_COST + MB_WASTE_VAL + MB_FIXED_TOTAL) * ((MB_QUALITY_LOSS + MB_DEV_EXPENSE)/100)) + MB_PACKING'),
  ('F_MB_CONV_COST',   '(MB_NO_PROCESS * MB_FIXED_TOTAL) + MB_COST_OTHERS + MB_WASTE_VAL')
) AS v(code, old_expr)
WHERE f.formula_code = v.code
  AND f.deleted_at IS NULL
  AND f.expression = v.old_expr
  AND EXISTS (SELECT 1 FROM mst_parameter p WHERE p.param_code = 'IS_BOUGHTOUT' AND p.deleted_at IS NULL);

-- PART 2
INSERT INTO formula_param (formula_id, param_id, sort_order)
SELECT f.id, p.id, 99
FROM mst_formula f
JOIN mst_parameter p ON p.param_code = 'IS_BOUGHTOUT' AND p.deleted_at IS NULL
WHERE f.formula_code IN ('F_MB_WASTE_VAL', 'F_MB_FIXED_COST', 'F_MB_COST_OTHERS', 'F_MB_CONV_COST')
  AND f.deleted_at IS NULL
  AND f.updated_by = 'mb_boughtout_000613'
  AND NOT EXISTS (SELECT 1 FROM formula_param fp WHERE fp.formula_id = f.id AND fp.param_id = p.id);

-- PART 3
UPDATE cost_product_parameter cpp
SET cpp_value_flag = h.mbh_is_boughtout,
    cpp_updated_at = NOW(),
    cpp_updated_by = 'mb_boughtout_000613'
FROM mst_mb_head h, mst_parameter p
WHERE p.param_code = 'IS_BOUGHTOUT' AND p.deleted_at IS NULL
  AND cpp.cpp_param_id = p.id
  AND cpp.cpp_product_sys_id = h.mbh_cost_product_id
  AND h.deleted_at IS NULL
  AND cpp.cpp_value_flag IS DISTINCT FROM h.mbh_is_boughtout
  AND cpp.cpp_value_numeric IS NULL
  AND cpp.cpp_value_text IS NULL
  AND (SELECT count(*) FROM mst_mb_head h2
       WHERE h2.mbh_cost_product_id = h.mbh_cost_product_id AND h2.deleted_at IS NULL) = 1;

COMMIT;
