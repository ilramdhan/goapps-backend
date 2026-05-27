-- Canonical PRD Phase A §7.1.5 — cost_routing_draft (CRD_).
-- Shadow entity for Phase B Product Order; lenient (free-text RM refs allowed).
-- 1:N with cost_product_request. Promote sets CRD_linked_product_order_id and
-- transitions CRD_status to PROMOTED.
CREATE TABLE IF NOT EXISTS cost_routing_draft (
    crd_draft_id                BIGSERIAL    PRIMARY KEY,
    crd_request_id              BIGINT       NOT NULL
        REFERENCES cost_product_request (cpr_request_id) ON DELETE CASCADE,
    crd_product_top_2           VARCHAR(100),
    crd_item_code               VARCHAR(50),
    crd_cyl_type_id             INT,
    crd_shade_code              VARCHAR(50),
    crd_raw_material_type       VARCHAR(50),
    crd_status                  VARCHAR(20)  NOT NULL DEFAULT 'DRAFT',
    crd_linked_product_order_id BIGINT
        REFERENCES cost_product_order (cpo_order_id) ON DELETE SET NULL,
    crd_created_by              VARCHAR(64)  NOT NULL,
    crd_created_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    crd_updated_at              TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_crd_status CHECK (crd_status IN ('DRAFT', 'LOCKED', 'PROMOTED')),
    CONSTRAINT chk_crd_raw_material CHECK (
        crd_raw_material_type IS NULL
        OR crd_raw_material_type IN ('POY_BOUGHTOUT', 'CHIPS_SD', 'CHIPS_BRT', 'CHIPS_RECYCLE')
    ),
    CONSTRAINT chk_crd_promoted_link CHECK (
        crd_status <> 'PROMOTED' OR crd_linked_product_order_id IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_crd_request ON cost_routing_draft (crd_request_id);
CREATE INDEX IF NOT EXISTS idx_crd_status  ON cost_routing_draft (crd_status);

COMMENT ON TABLE cost_routing_draft IS 'PRD Phase A §7.1.5 — Routing draft (shadow entity for Phase B). Lenient: free-text RM refs allowed in CRDC.';
