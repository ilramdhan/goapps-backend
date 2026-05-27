BEGIN;

CREATE TABLE IF NOT EXISTS cost_routing_draft (
    crd_draft_id            BIGSERIAL PRIMARY KEY,
    crd_request_id          BIGINT NOT NULL,
    crd_product_top_2       VARCHAR(50),
    crd_item_code           VARCHAR(30),
    crd_cyl_type_id         INTEGER,
    crd_shade_code          VARCHAR(30),
    crd_raw_material_type   VARCHAR(30),
    crd_status              VARCHAR(20) NOT NULL DEFAULT 'DRAFT',
    crd_linked_route_head_id BIGINT REFERENCES cost_route_head(crh_head_id) ON DELETE SET NULL,
    crd_created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    crd_created_by          VARCHAR(100) NOT NULL,
    crd_updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
    crd_updated_by          VARCHAR(100)
);

CREATE TABLE IF NOT EXISTS cost_routing_draft_component (
    crdc_component_id      BIGSERIAL PRIMARY KEY,
    crdc_draft_id          BIGINT NOT NULL REFERENCES cost_routing_draft(crd_draft_id) ON DELETE CASCADE,
    crdc_sequence_no       INTEGER NOT NULL,
    crdc_sub_sequence      INTEGER,
    crdc_sub_type          VARCHAR(30),
    crdc_rm_type           VARCHAR(30) NOT NULL,
    crdc_rm_ref_text       VARCHAR(255) NOT NULL,
    crdc_rm_ref_resolved_id BIGINT,
    crdc_notes             TEXT
);

DROP INDEX IF EXISTS idx_cpr_linked_route_head;
ALTER TABLE cost_product_request DROP COLUMN IF EXISTS cpr_linked_route_head_id;

COMMIT;
