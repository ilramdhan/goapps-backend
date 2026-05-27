-- 000222: introduce cost_route_head / cost_route_seq / cost_route_rm to replace
-- the flat cost_product_order_* family with a multi-level routing DAG aligned
-- with mermaid-erd-costing.md (CST_PRD_ROUTE_HEAD/SEQ/RM).
--
-- Migration steps:
--   1. Create the three new tables with constraints + indexes.
--   2. Backfill from cost_product_order + cost_product_order_version +
--      cost_product_order_component into a single-level (route_level=1) route
--      per existing order.
--   3. Add cost_routing_draft.crd_linked_route_head_id (replaces
--      crd_linked_product_order_id) and backfill via the existing FK chain.
--   4. Drop the legacy cost_product_order_component / _version / order tables.
--
-- Tables created here are the persisted "released routing". The drafting
-- workflow (cost_routing_draft + cost_routing_draft_component) is unchanged --
-- only its terminal target switches from product_order to route_head.

BEGIN;

-- 1. New tables -------------------------------------------------------------

CREATE TABLE IF NOT EXISTS cost_route_head (
    crh_head_id                 BIGSERIAL PRIMARY KEY,
    crh_product_sys_id          BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE RESTRICT,
    crh_routing_status          VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
    crh_version                 INTEGER NOT NULL DEFAULT 1,
    crh_promoted_from_draft_id  BIGINT,
    crh_cyl_type_id             INTEGER,
    crh_notes                   TEXT,
    crh_created_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    crh_created_by              VARCHAR(100) NOT NULL,
    crh_updated_at              TIMESTAMPTZ NOT NULL DEFAULT now(),
    crh_updated_by              VARCHAR(100),
    crh_deleted_at              TIMESTAMPTZ,
    crh_deleted_by              VARCHAR(100),
    CONSTRAINT chk_crh_status   CHECK (crh_routing_status IN ('DRAFT', 'COMPLETE', 'LOCKED')),
    CONSTRAINT chk_crh_version  CHECK (crh_version > 0)
);

-- One non-LOCKED routing per product (matches ERD UK semantics with soft-delete + lock awareness)
CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_route_head_active_per_product
    ON cost_route_head (crh_product_sys_id)
    WHERE crh_deleted_at IS NULL AND crh_routing_status <> 'LOCKED';

CREATE INDEX IF NOT EXISTS idx_cost_route_head_status ON cost_route_head (crh_routing_status);
CREATE INDEX IF NOT EXISTS idx_cost_route_head_draft  ON cost_route_head (crh_promoted_from_draft_id) WHERE crh_promoted_from_draft_id IS NOT NULL;

CREATE TABLE IF NOT EXISTS cost_route_seq (
    crs_seq_id            BIGSERIAL PRIMARY KEY,
    crs_head_id           BIGINT NOT NULL REFERENCES cost_route_head(crh_head_id) ON DELETE CASCADE,
    crs_product_sys_id    BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE RESTRICT,
    crs_route_level       INTEGER NOT NULL,
    crs_route_seq         INTEGER NOT NULL,
    crs_route_name        VARCHAR(200),
    crs_route_item_code   VARCHAR(30),
    crs_route_shade_code  VARCHAR(30),
    crs_route_shade_name  VARCHAR(100),
    crs_position_x        NUMERIC(10,2) NOT NULL DEFAULT 0,
    crs_position_y        NUMERIC(10,2) NOT NULL DEFAULT 0,
    crs_created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    crs_created_by        VARCHAR(100) NOT NULL,
    crs_updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
    crs_updated_by        VARCHAR(100),
    crs_deleted_at        TIMESTAMPTZ,
    crs_deleted_by        VARCHAR(100),
    CONSTRAINT chk_crs_level     CHECK (crs_route_level >= 1),
    CONSTRAINT chk_crs_seq       CHECK (crs_route_seq >= 1),
    CONSTRAINT uk_crs_head_level_seq UNIQUE (crs_head_id, crs_route_level, crs_route_seq)
);

CREATE INDEX IF NOT EXISTS idx_crs_head_level ON cost_route_seq (crs_head_id, crs_route_level);
CREATE INDEX IF NOT EXISTS idx_crs_product    ON cost_route_seq (crs_product_sys_id);

CREATE TABLE IF NOT EXISTS cost_route_rm (
    crm_rm_id                  BIGSERIAL PRIMARY KEY,
    crm_seq_id                 BIGINT NOT NULL REFERENCES cost_route_seq(crs_seq_id) ON DELETE CASCADE,
    crm_parent_product_sys_id  BIGINT NOT NULL REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE RESTRICT,
    crm_rm_product_sys_id      BIGINT REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE RESTRICT,
    crm_rm_item_code           VARCHAR(30),
    crm_rm_group_code          VARCHAR(30),
    crm_rm_type                VARCHAR(20) NOT NULL,
    crm_route_rm_name          VARCHAR(200),
    crm_route_rm_item_code     VARCHAR(30),
    crm_route_rm_shade_code    VARCHAR(30),
    crm_route_rm_shade_name    VARCHAR(100),
    crm_route_rm_ratio         NUMERIC(10,6) NOT NULL,
    crm_uom_id                 INTEGER,
    crm_sub_type               VARCHAR(30),
    crm_notes                  TEXT,
    crm_created_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    crm_created_by             VARCHAR(100) NOT NULL,
    crm_updated_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    crm_updated_by             VARCHAR(100),
    CONSTRAINT chk_crm_type    CHECK (crm_rm_type IN ('PRODUCT', 'ITEM', 'GROUP')),
    CONSTRAINT chk_crm_ratio   CHECK (crm_route_rm_ratio > 0),
    CONSTRAINT chk_crm_one_ref CHECK (
        (CASE WHEN crm_rm_product_sys_id IS NOT NULL THEN 1 ELSE 0 END
         + CASE WHEN crm_rm_item_code    IS NOT NULL THEN 1 ELSE 0 END
         + CASE WHEN crm_rm_group_code   IS NOT NULL THEN 1 ELSE 0 END) = 1
    ),
    CONSTRAINT chk_crm_type_ref_match CHECK (
        (crm_rm_type = 'PRODUCT' AND crm_rm_product_sys_id IS NOT NULL) OR
        (crm_rm_type = 'ITEM'    AND crm_rm_item_code      IS NOT NULL) OR
        (crm_rm_type = 'GROUP'   AND crm_rm_group_code     IS NOT NULL)
    )
);

CREATE INDEX IF NOT EXISTS idx_crm_seq        ON cost_route_rm (crm_seq_id);
CREATE INDEX IF NOT EXISTS idx_crm_rm_product ON cost_route_rm (crm_rm_product_sys_id) WHERE crm_rm_product_sys_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_crm_rm_item    ON cost_route_rm (crm_rm_item_code)      WHERE crm_rm_item_code IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_crm_rm_group   ON cost_route_rm (crm_rm_group_code)     WHERE crm_rm_group_code IS NOT NULL;

-- 2. Backfill from cost_product_order + version + component -----------------
-- We create one route_head per existing order, one level-1 SEQ per order's
-- product, and copy each component as a route_rm row on that SEQ.

DO $$
DECLARE
    r RECORD;
    new_head_id BIGINT;
    new_seq_id  BIGINT;
    c RECORD;
BEGIN
    FOR r IN
        SELECT cpo.cpo_order_id,
               cpo.cpo_product_sys_id,
               cpo.cpo_cyl_type_id,
               cpo.cpo_created_at,
               cpo.cpo_created_by,
               cpm.cpm_product_code,
               cpm.cpm_product_name,
               cpov.cpov_version_id
        FROM cost_product_order cpo
        JOIN cost_product_order_version cpov ON cpov.cpov_order_id = cpo.cpo_order_id
        JOIN cost_product_master cpm ON cpm.cpm_product_sys_id = cpo.cpo_product_sys_id
        WHERE cpo.cpo_is_active = TRUE
    LOOP
        INSERT INTO cost_route_head (
            crh_product_sys_id, crh_routing_status, crh_version,
            crh_cyl_type_id, crh_created_at, crh_created_by, crh_updated_by
        ) VALUES (
            r.cpo_product_sys_id, 'DRAFT', 1,
            r.cpo_cyl_type_id, r.cpo_created_at, r.cpo_created_by, r.cpo_created_by
        ) RETURNING crh_head_id INTO new_head_id;

        INSERT INTO cost_route_seq (
            crs_head_id, crs_product_sys_id, crs_route_level, crs_route_seq,
            crs_route_name, crs_route_item_code, crs_route_shade_code, crs_route_shade_name,
            crs_position_x, crs_position_y,
            crs_created_by, crs_updated_by
        ) VALUES (
            new_head_id, r.cpo_product_sys_id, 1, 1,
            r.cpm_product_name, NULL, NULL, NULL,
            0, 0,
            r.cpo_created_by, r.cpo_created_by
        ) RETURNING crs_seq_id INTO new_seq_id;

        FOR c IN
            SELECT cpoc.cpoc_rm_type_id,
                   cpoc.cpoc_rm_product_sys_id,
                   cpoc.cpoc_rm_master_item_id,
                   cpoc.cpoc_rm_description,
                   cpoc.cpoc_sub_type,
                   cei.cei_item_code,
                   cei.cei_item_name
            FROM cost_product_order_component cpoc
            LEFT JOIN cost_erp_item cei ON cei.cei_item_id = cpoc.cpoc_rm_master_item_id
            WHERE cpoc.cpoc_version_id = r.cpov_version_id
        LOOP
            IF c.cpoc_rm_product_sys_id IS NOT NULL THEN
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_product_sys_id, crm_rm_type,
                    crm_route_rm_name, crm_route_rm_ratio, crm_sub_type,
                    crm_created_by, crm_updated_by
                ) VALUES (
                    new_seq_id, r.cpo_product_sys_id,
                    c.cpoc_rm_product_sys_id, 'PRODUCT',
                    NULL, 1.0, c.cpoc_sub_type,
                    r.cpo_created_by, r.cpo_created_by
                );
            ELSIF c.cpoc_rm_master_item_id IS NOT NULL THEN
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_item_code, crm_rm_type,
                    crm_route_rm_name, crm_route_rm_item_code, crm_route_rm_ratio, crm_sub_type,
                    crm_created_by, crm_updated_by
                ) VALUES (
                    new_seq_id, r.cpo_product_sys_id,
                    c.cei_item_code, 'ITEM',
                    c.cei_item_name, c.cei_item_code, 1.0, c.cpoc_sub_type,
                    r.cpo_created_by, r.cpo_created_by
                );
            ELSE
                -- free-text description -> map to ITEM with synthesised item_code so DB CHECK is satisfied
                INSERT INTO cost_route_rm (
                    crm_seq_id, crm_parent_product_sys_id,
                    crm_rm_item_code, crm_rm_type,
                    crm_route_rm_name, crm_route_rm_ratio, crm_sub_type, crm_notes,
                    crm_created_by, crm_updated_by
                ) VALUES (
                    new_seq_id, r.cpo_product_sys_id,
                    COALESCE(NULLIF(LEFT(c.cpoc_rm_description, 30), ''), 'LEGACY'),
                    'ITEM',
                    c.cpoc_rm_description, 1.0, c.cpoc_sub_type, c.cpoc_rm_description,
                    r.cpo_created_by, r.cpo_created_by
                );
            END IF;
        END LOOP;
    END LOOP;
END $$;

-- 3. cost_routing_draft: add crd_linked_route_head_id, backfill, drop the old FK
ALTER TABLE cost_routing_draft
    ADD COLUMN IF NOT EXISTS crd_linked_route_head_id BIGINT;

UPDATE cost_routing_draft crd
SET crd_linked_route_head_id = crh.crh_head_id
FROM cost_route_head crh
WHERE crh.crh_product_sys_id IN (
    SELECT cpo.cpo_product_sys_id
    FROM cost_product_order cpo
    WHERE cpo.cpo_order_id = crd.crd_linked_product_order_id
);

ALTER TABLE cost_routing_draft
    DROP CONSTRAINT IF EXISTS cost_routing_draft_crd_linked_product_order_id_fkey;
ALTER TABLE cost_routing_draft
    DROP COLUMN IF EXISTS crd_linked_product_order_id;

ALTER TABLE cost_routing_draft
    ADD CONSTRAINT fk_crd_route_head
        FOREIGN KEY (crd_linked_route_head_id) REFERENCES cost_route_head(crh_head_id) ON DELETE SET NULL;

-- Wire promote-from-draft pointer back the other way too
UPDATE cost_route_head crh
SET crh_promoted_from_draft_id = crd.crd_draft_id
FROM cost_routing_draft crd
WHERE crd.crd_linked_route_head_id = crh.crh_head_id;

-- 4. Drop legacy tables (CASCADE drops component → version → order constraints) -
DROP TABLE IF EXISTS cost_product_order_component CASCADE;
DROP TABLE IF EXISTS cost_product_order_version   CASCADE;
DROP TABLE IF EXISTS cost_product_order           CASCADE;

COMMIT;
