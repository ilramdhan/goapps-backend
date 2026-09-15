-- Reverse 000510: strip the OIL_GAIN term back out of F_YARN_CONV_CAP /
-- F_YARN_CONV_DEL and drop the two formula_param edges that 000510 added.
--
-- Everything is keyed on the 'wire_oil_gain_000510' marker, so a formula this
-- migration never touched — one edited by hand, or by a later migration that
-- overwrote updated_by — is left completely alone.
--
-- Order matters: the edges are deleted FIRST, while the marker is still on the
-- formula rows and can still identify exactly which edges 000510 created.
-- Reversing the order would leave the formula_param rows unreachable.
--
-- Idempotent: once the marker is cleared nothing matches and re-running is a
-- no-op.
--
-- Note on updated_at / updated_by: they are restored to NULL because that is the
-- state 000408_seed_oracle_formulas.up.sql left these rows in — its INSERT sets
-- neither column.

BEGIN;

-- ============================================================
-- PART 1: Remove the OIL_GAIN input edges added by 000510
-- ============================================================

DELETE FROM formula_param fp
USING mst_formula f, mst_parameter p
WHERE fp.formula_id = f.id
  AND fp.param_id = p.id
  AND p.param_code = 'OIL_GAIN'
  AND f.formula_code IN ('F_YARN_CONV_CAP', 'F_YARN_CONV_DEL')
  AND f.deleted_at IS NULL
  AND f.updated_by = 'wire_oil_gain_000510';

-- ============================================================
-- PART 2: Restore the 000408 expressions
-- ============================================================
-- The WHERE clause pins the post-000510 expression text, so a formula whose
-- expression has since been changed to something else is not reverted to a
-- stale value.

UPDATE mst_formula
SET expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_CONV_CAP'
  AND deleted_at IS NULL
  AND updated_by = 'wire_oil_gain_000510'
  AND expression = 'TOTAL_FIXEDCOST_PER_KG + CAPTIVE_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN';

UPDATE mst_formula
SET expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1',
    updated_at = NULL,
    updated_by = NULL
WHERE formula_code = 'F_YARN_CONV_DEL'
  AND deleted_at IS NULL
  AND updated_by = 'wire_oil_gain_000510'
  AND expression = 'TOTAL_FIXEDCOST_PER_KG + DELIVERY_PACK_COST + OIL_COST + INTERMINGLING + SPECIAL_COST_1 + OIL_GAIN';

COMMIT;
