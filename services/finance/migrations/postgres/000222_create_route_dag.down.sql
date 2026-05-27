-- 000222 down: recreate cost_product_order family and drop the new route_* tables.
-- This is a best-effort restore -- multi-level routings (> 1 SEQ) cannot be
-- represented in the old flat schema, so the down migration only restores
-- single-level routings. Run the pre-migration pg_dump from
-- .backup-pre-s7.16/ if richer data needs to be brought back.

BEGIN;

ALTER TABLE cost_routing_draft DROP CONSTRAINT IF EXISTS fk_crd_route_head;
ALTER TABLE cost_routing_draft ADD COLUMN IF NOT EXISTS crd_linked_product_order_id BIGINT;

CREATE TABLE IF NOT EXISTS cost_product_order (
    cpo_order_id        BIGSERIAL PRIMARY KEY,
    cpo_product_sys_id  BIGINT NOT NULL,
    cpo_cyl_type_id     INTEGER,
    cpo_current_version_id BIGINT,
    cpo_is_active       BOOLEAN NOT NULL DEFAULT TRUE,
    cpo_created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    cpo_created_by      VARCHAR(64) NOT NULL,
    cpo_updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    cpo_updated_by      VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS cost_product_order_version (
    cpov_version_id     BIGSERIAL PRIMARY KEY,
    cpov_order_id       BIGINT NOT NULL REFERENCES cost_product_order(cpo_order_id) ON DELETE CASCADE,
    cpov_version_no     INTEGER NOT NULL,
    cpov_status         VARCHAR(20) NOT NULL,
    cpov_effective_from TIMESTAMPTZ,
    cpov_effective_to   TIMESTAMPTZ,
    cpov_cycle_override BOOLEAN NOT NULL DEFAULT FALSE,
    cpov_created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    cpov_created_by     VARCHAR(64) NOT NULL
);

CREATE TABLE IF NOT EXISTS cost_product_order_component (
    cpoc_component_id      BIGSERIAL PRIMARY KEY,
    cpoc_version_id        BIGINT NOT NULL REFERENCES cost_product_order_version(cpov_version_id) ON DELETE CASCADE,
    cpoc_sequence_no       INTEGER NOT NULL,
    cpoc_sub_sequence      INTEGER,
    cpoc_sub_type          VARCHAR(30),
    cpoc_rm_type_id        INTEGER NOT NULL,
    cpoc_rm_product_sys_id BIGINT,
    cpoc_rm_master_item_id BIGINT,
    cpoc_rm_description    VARCHAR(255)
);

-- Best-effort backfill (single-level only)
DO $$
DECLARE
    h RECORD;
    new_order_id   BIGINT;
    new_version_id BIGINT;
    seq_row        RECORD;
    rm_row         RECORD;
    next_seq       INTEGER;
BEGIN
    FOR h IN SELECT * FROM cost_route_head WHERE crh_deleted_at IS NULL LOOP
        INSERT INTO cost_product_order (cpo_product_sys_id, cpo_cyl_type_id, cpo_is_active, cpo_created_at, cpo_created_by, cpo_updated_by)
        VALUES (h.crh_product_sys_id, h.crh_cyl_type_id, TRUE, h.crh_created_at, h.crh_created_by, h.crh_created_by)
        RETURNING cpo_order_id INTO new_order_id;

        INSERT INTO cost_product_order_version (cpov_order_id, cpov_version_no, cpov_status, cpov_created_by)
        VALUES (new_order_id, h.crh_version, CASE h.crh_routing_status WHEN 'LOCKED' THEN 'active' ELSE 'draft' END, h.crh_created_by)
        RETURNING cpov_version_id INTO new_version_id;

        next_seq := 1;
        FOR seq_row IN SELECT * FROM cost_route_seq WHERE crs_head_id = h.crh_head_id AND crs_route_level = 1 ORDER BY crs_route_seq LOOP
            FOR rm_row IN SELECT * FROM cost_route_rm WHERE crm_seq_id = seq_row.crs_seq_id ORDER BY crm_rm_id LOOP
                INSERT INTO cost_product_order_component (
                    cpoc_version_id, cpoc_sequence_no, cpoc_sub_type,
                    cpoc_rm_type_id, cpoc_rm_product_sys_id, cpoc_rm_master_item_id, cpoc_rm_description
                ) VALUES (
                    new_version_id, next_seq, rm_row.crm_sub_type,
                    1,
                    rm_row.crm_rm_product_sys_id,
                    NULL,
                    rm_row.crm_route_rm_name
                );
                next_seq := next_seq + 1;
            END LOOP;
        END LOOP;

        UPDATE cost_product_order SET cpo_current_version_id = new_version_id WHERE cpo_order_id = new_order_id;
        UPDATE cost_routing_draft SET crd_linked_product_order_id = new_order_id WHERE crd_linked_route_head_id = h.crh_head_id;
    END LOOP;
END $$;

ALTER TABLE cost_routing_draft DROP COLUMN IF EXISTS crd_linked_route_head_id;
DROP TABLE IF EXISTS cost_route_rm   CASCADE;
DROP TABLE IF EXISTS cost_route_seq  CASCADE;
DROP TABLE IF EXISTS cost_route_head CASCADE;

COMMIT;
