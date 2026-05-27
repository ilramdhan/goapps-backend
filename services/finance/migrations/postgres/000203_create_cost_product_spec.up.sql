-- Canonical PRD Phase A §7.1.2 — cost_product_spec (CPS_).
-- 1:1 conditional with cost_product_request (only when product_classification = new).
CREATE TABLE IF NOT EXISTS cost_product_spec (
    cps_spec_id             BIGSERIAL    PRIMARY KEY,
    cps_request_id          BIGINT       NOT NULL
        REFERENCES cost_product_request (cpr_request_id) ON DELETE CASCADE,
    cps_raw_material_type   VARCHAR(50)  NOT NULL,
    cps_product_description TEXT         NOT NULL,
    cps_shade_id            INT,
    cps_shade_custom_text   VARCHAR(100),
    cps_paper_tube_type_id  INT          NOT NULL
        REFERENCES cost_paper_tube_type (cptt_paper_tube_type_id) ON DELETE RESTRICT,
    cps_weight_per_bobbin_kg DECIMAL(10,3) NOT NULL,
    cps_box_type            VARCHAR(20)  NOT NULL,
    cps_created_at          TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cps_created_by          VARCHAR(64)  NOT NULL,
    CONSTRAINT uk_cps_request UNIQUE (cps_request_id),
    CONSTRAINT chk_cps_raw_material CHECK (
        cps_raw_material_type IN ('POY_BOUGHTOUT', 'CHIPS_SD', 'CHIPS_BRT', 'CHIPS_RECYCLE')
    ),
    CONSTRAINT chk_cps_box_type CHECK (cps_box_type IN ('JUMBO', 'NORMAL', 'PALLET')),
    CONSTRAINT chk_cps_shade_present CHECK (
        cps_shade_id IS NOT NULL OR cps_shade_custom_text IS NOT NULL
    ),
    CONSTRAINT chk_cps_weight_positive CHECK (cps_weight_per_bobbin_kg > 0)
);

CREATE INDEX IF NOT EXISTS idx_cps_request ON cost_product_spec (cps_request_id);

COMMENT ON TABLE cost_product_spec IS 'PRD Phase A §7.1.2 — Product specification for requests with classification=new.';
