-- Reverse 000536: back to per product type TX Weight rules.
--
-- Before dropping the group model, every live group rule is fanned out to a
-- per-type row for each type the group maps (skipped when that type already
-- has a live row for the grade), so the pre-000536 engine keeps producing the
-- same weights. Rows without a product type (only ever written through the
-- group service) are then removed so ytw_product_type_id can be NOT NULL again.
--
-- Guarded: a no-op when 000536 was never applied (no ytw_group_id column).
BEGIN;

DO $$
DECLARE
    v_fanned  INT;
    v_removed INT;
BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM information_schema.columns
        WHERE table_schema = 'public'
          AND table_name   = 'mst_yarn_tx_weight'
          AND column_name  = 'ytw_group_id'
    ) THEN
        RAISE NOTICE '000536 down: ytw_group_id absent — nothing to reverse.';
        RETURN;
    END IF;

    INSERT INTO mst_yarn_tx_weight (
        ytw_product_type_id, ytw_grade, ytw_mode, ytw_value, ytw_description, created_by
    )
    SELECT gt.product_type_id, w.ytw_grade, w.ytw_mode, w.ytw_value, w.ytw_description, 'rollback_000536'
    FROM mst_yarn_tx_weight w
    JOIN mst_yarn_tx_weight_group g
         ON g.ytwg_id = w.ytw_group_id
        AND g.deleted_at IS NULL
    JOIN mst_yarn_tx_weight_group_type gt ON gt.ytwg_id = g.ytwg_id
    WHERE w.deleted_at IS NULL
      AND NOT EXISTS (
          SELECT 1 FROM mst_yarn_tx_weight x
          WHERE x.ytw_product_type_id = gt.product_type_id
            AND x.ytw_grade = w.ytw_grade
            AND x.deleted_at IS NULL
      );
    GET DIAGNOSTICS v_fanned = ROW_COUNT;

    DELETE FROM mst_yarn_tx_weight WHERE ytw_product_type_id IS NULL;
    GET DIAGNOSTICS v_removed = ROW_COUNT;

    RAISE NOTICE '000536 down: fanned out % per-type rule row(s), removed % group-only row(s).', v_fanned, v_removed;
END $$;

DROP INDEX IF EXISTS uix_mst_yarn_tx_weight_group_grade;

ALTER TABLE mst_yarn_tx_weight ALTER COLUMN ytw_product_type_id SET NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_yarn_tx_weight_type_grade
    ON mst_yarn_tx_weight (ytw_product_type_id, ytw_grade) WHERE deleted_at IS NULL;

ALTER TABLE mst_yarn_tx_weight DROP COLUMN IF EXISTS ytw_group_id;

DROP TABLE IF EXISTS mst_yarn_tx_weight_group_type;
DROP TABLE IF EXISTS mst_yarn_tx_weight_group;

COMMENT ON COLUMN mst_yarn_tx_weight.ytw_product_type_id IS NULL;

COMMIT;
