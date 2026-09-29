-- Reverse 000532: strip the MB_COST_MKT term back out of F_YARN_CAP_PRE_QL /
-- F_YARN_DEL_PRE_QL and drop the two formula_param edges 000532 added.
--
-- Everything is keyed on the 'add_mb_cost_000532' marker, so a formula this
-- migration never touched (edited by hand, or by a later migration that
-- overwrote updated_by) is left alone.
--
-- Order matters: the edges are deleted FIRST, while the marker still
-- identifies exactly which formulas 000532 rewrote.
--
-- updated_at / updated_by are restored to NULL: that is the state 000408 left
-- these rows in (its INSERT sets neither column). Idempotent.

BEGIN;

DO $$
DECLARE
    v_deleted  INTEGER;
    v_reverted INTEGER;
BEGIN
    DELETE FROM formula_param fp
    USING mst_formula f, mst_parameter p
    WHERE fp.formula_id = f.id
      AND fp.param_id = p.id
      AND p.param_code = 'MB_COST_MKT'
      AND f.formula_code IN ('F_YARN_CAP_PRE_QL', 'F_YARN_DEL_PRE_QL')
      AND f.deleted_at IS NULL
      AND f.updated_by = 'add_mb_cost_000532';
    GET DIAGNOSTICS v_deleted = ROW_COUNT;

    UPDATE mst_formula f
    SET expression = CASE f.formula_code
            WHEN 'F_YARN_CAP_PRE_QL' THEN 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB'
            WHEN 'F_YARN_DEL_PRE_QL' THEN 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB'
        END,
        updated_at = NULL,
        updated_by = NULL
    WHERE f.deleted_at IS NULL
      AND f.updated_by = 'add_mb_cost_000532'
      AND (
          (f.formula_code = 'F_YARN_CAP_PRE_QL'
           AND f.expression = 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_CAP_PACK_EXCL_MB + MB_COST_MKT')
       OR (f.formula_code = 'F_YARN_DEL_PRE_QL'
           AND f.expression = 'RM_NORMS * RM_LANDED_COST + ONLY_CONV_DEL_PACK_EXCL_MB + MB_COST_MKT')
      );
    GET DIAGNOSTICS v_reverted = ROW_COUNT;

    RAISE NOTICE '000532 down: % formula_param edge(s) deleted, % formula(s) reverted',
        v_deleted, v_reverted;
END $$;

COMMIT;
