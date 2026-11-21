-- Reverse 000613 PART 1/2: drop the IS_BOUGHTOUT edges, restore the 000452 expressions.
-- PART 3 (CPP IS_BOUGHTOUT re-sync) is intentionally NOT reverted: the old value was stale.
-- Edges are deleted first, while the marker still identifies the rewritten formulas.

BEGIN;

DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id
  AND fp.param_id = p.id
  AND p.param_code = 'IS_BOUGHTOUT'
  AND f.formula_code IN ('F_MB_WASTE_VAL', 'F_MB_FIXED_COST', 'F_MB_COST_OTHERS', 'F_MB_CONV_COST')
  AND f.deleted_at IS NULL
  AND f.updated_by = 'mb_boughtout_000613';

UPDATE mst_formula f
SET expression = v.old_expr,
    updated_at = NOW(),
    updated_by = 'SYSTEM'
FROM (VALUES
  ('F_MB_WASTE_VAL',   '(MB_RM_COST / (1 - MB_WASTE/100)) * (MB_WASTE/100)'),
  ('F_MB_FIXED_COST',  'MB_NET_PROD > 0 ? MACHINE_MB_FIXED_TOTAL / MB_NET_PROD : 0'),
  ('F_MB_COST_OTHERS', '((MB_RM_COST + MB_WASTE_VAL + MB_FIXED_TOTAL) * ((MB_QUALITY_LOSS + MB_DEV_EXPENSE)/100)) + MB_PACKING'),
  ('F_MB_CONV_COST',   '(MB_NO_PROCESS * MB_FIXED_TOTAL) + MB_COST_OTHERS + MB_WASTE_VAL')
) AS v(code, old_expr)
WHERE f.formula_code = v.code
  AND f.deleted_at IS NULL
  AND f.updated_by = 'mb_boughtout_000613'
  AND f.expression = 'IS_BOUGHTOUT == 1 ? 0 : (' || v.old_expr || ')';

COMMIT;
