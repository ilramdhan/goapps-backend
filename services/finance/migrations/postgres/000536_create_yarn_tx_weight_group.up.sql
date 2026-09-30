-- 000536 (backlog1 follow-up, TX Weight multi product type) — one TX Weight
-- config (grade rules) shared by many product types.
--
-- Model:
--   mst_yarn_tx_weight_group       (ytwg_) one config: code/name/description.
--   mst_yarn_tx_weight_group_type  junction group -> cost_product_type;
--                                  UNIQUE(product_type_id) = a product type
--                                  belongs to AT MOST ONE config.
--   mst_yarn_tx_weight             rule rows gain ytw_group_id; a live rule is
--                                  unique per (group, grade). ytw_product_type_id
--                                  is KEPT but becomes nullable (rows written by
--                                  YarnTxWeightGroupService leave it NULL).
--
-- Data migration: every product type that has live rule rows gets its own
-- group (code = type code, name = type name), a 1:1 mapping, and its rows are
-- linked. The shared TTY/DTY mappings are seeded separately in 000537.
--
-- Idempotent: every DDL is IF [NOT] EXISTS and every data step only touches
-- rows not yet migrated, so a re-run is a no-op.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_yarn_tx_weight_group (
    ytwg_id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    ytwg_code        VARCHAR(30)   NOT NULL,
    ytwg_name        VARCHAR(100)  NOT NULL,
    ytwg_description VARCHAR(200),
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by       VARCHAR(100)  NOT NULL,
    updated_at       TIMESTAMPTZ,
    updated_by       VARCHAR(100),
    deleted_at       TIMESTAMPTZ,
    deleted_by       VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_yarn_tx_weight_group_code
    ON mst_yarn_tx_weight_group (ytwg_code) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS mst_yarn_tx_weight_group_type (
    ytwg_id         UUID          NOT NULL REFERENCES mst_yarn_tx_weight_group (ytwg_id) ON DELETE CASCADE,
    product_type_id INT           NOT NULL REFERENCES cost_product_type (cpt_type_id),
    created_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by      VARCHAR(100)  NOT NULL,
    CONSTRAINT pk_mst_yarn_tx_weight_group_type PRIMARY KEY (ytwg_id, product_type_id),
    CONSTRAINT uq_mst_yarn_tx_weight_group_type_product_type UNIQUE (product_type_id)
);

ALTER TABLE mst_yarn_tx_weight
    ADD COLUMN IF NOT EXISTS ytw_group_id UUID REFERENCES mst_yarn_tx_weight_group (ytwg_id);

ALTER TABLE mst_yarn_tx_weight ALTER COLUMN ytw_product_type_id DROP NOT NULL;

DO $$
DECLARE
    v_groups  INT;
    v_maps    INT;
    v_linked  INT;
BEGIN
    -- 1. One group per product type that has live, not yet migrated rules.
    INSERT INTO mst_yarn_tx_weight_group (ytwg_code, ytwg_name, created_by)
    SELECT DISTINCT pt.cpt_type_code, pt.cpt_type_name, 'seed_000536'
    FROM mst_yarn_tx_weight w
    JOIN cost_product_type pt ON pt.cpt_type_id = w.ytw_product_type_id
    WHERE w.deleted_at IS NULL
      AND w.ytw_group_id IS NULL
      AND NOT EXISTS (
          SELECT 1 FROM mst_yarn_tx_weight_group_type gt
          WHERE gt.product_type_id = pt.cpt_type_id
      )
      AND NOT EXISTS (
          SELECT 1 FROM mst_yarn_tx_weight_group g
          WHERE g.ytwg_code = pt.cpt_type_code
            AND g.deleted_at IS NULL
      );
    GET DIAGNOSTICS v_groups = ROW_COUNT;

    -- 2. 1:1 mapping type -> its own group (matched on code = type code).
    INSERT INTO mst_yarn_tx_weight_group_type (ytwg_id, product_type_id, created_by)
    SELECT g.ytwg_id, pt.cpt_type_id, 'seed_000536'
    FROM mst_yarn_tx_weight_group g
    JOIN cost_product_type pt ON pt.cpt_type_code = g.ytwg_code
    WHERE g.created_by = 'seed_000536'
      AND g.deleted_at IS NULL
      AND EXISTS (
          SELECT 1 FROM mst_yarn_tx_weight w
          WHERE w.ytw_product_type_id = pt.cpt_type_id
            AND w.deleted_at IS NULL
            AND w.ytw_group_id IS NULL
      )
    ON CONFLICT (product_type_id) DO NOTHING;
    GET DIAGNOSTICS v_maps = ROW_COUNT;

    -- 3. Link the live rule rows to their type's group.
    UPDATE mst_yarn_tx_weight w
    SET ytw_group_id = gt.ytwg_id
    FROM mst_yarn_tx_weight_group_type gt
    WHERE gt.product_type_id = w.ytw_product_type_id
      AND w.deleted_at IS NULL
      AND w.ytw_group_id IS NULL;
    GET DIAGNOSTICS v_linked = ROW_COUNT;

    RAISE NOTICE '000536: created % TX Weight group(s), % mapping(s), linked % rule row(s).', v_groups, v_maps, v_linked;

    IF EXISTS (SELECT 1 FROM mst_yarn_tx_weight WHERE deleted_at IS NULL AND ytw_group_id IS NULL) THEN
        RAISE NOTICE '000536: some live TX Weight rows are still without a group (their type is mapped elsewhere); the engine ignores them.';
    END IF;
END $$;

-- The old natural key (type, grade) gives way to (group, grade).
DROP INDEX IF EXISTS uix_mst_yarn_tx_weight_type_grade;

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_yarn_tx_weight_group_grade
    ON mst_yarn_tx_weight (ytw_group_id, ytw_grade) WHERE deleted_at IS NULL;

COMMENT ON TABLE  mst_yarn_tx_weight_group IS 'TX Weight config: one set of AE/A9/A/B/C grade rules shared by the product types mapped in mst_yarn_tx_weight_group_type.';
COMMENT ON TABLE  mst_yarn_tx_weight_group_type IS 'TX Weight group -> product type mapping. UNIQUE(product_type_id): a product type belongs to at most one group.';
COMMENT ON COLUMN mst_yarn_tx_weight.ytw_group_id IS 'Owning TX Weight group (000536). The engine reads rules by group, not by ytw_product_type_id.';
COMMENT ON COLUMN mst_yarn_tx_weight.ytw_product_type_id IS 'Legacy per-type key (000528). NULL for rows written by YarnTxWeightGroupService.';

COMMIT;
