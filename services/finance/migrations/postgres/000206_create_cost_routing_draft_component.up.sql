-- Canonical PRD Phase A §7.1.6 — cost_routing_draft_component (CRDC_).
-- Single-level BOM line for the routing draft. Lenient: rm_ref_text is FREE-TEXT
-- (intermediate products may not exist yet in any master). Optional resolved id
-- points back to a Phase B product when known.
CREATE TABLE IF NOT EXISTS cost_routing_draft_component (
    crdc_component_id      BIGSERIAL    PRIMARY KEY,
    crdc_draft_id          BIGINT       NOT NULL
        REFERENCES cost_routing_draft (crd_draft_id) ON DELETE CASCADE,
    crdc_sequence_no       INT          NOT NULL,
    crdc_sub_sequence      INT,
    crdc_sub_type          VARCHAR(30),
    crdc_rm_type           VARCHAR(30)  NOT NULL,
    crdc_rm_ref_text       VARCHAR(255) NOT NULL,
    crdc_rm_ref_resolved_id BIGINT,
    crdc_notes             TEXT,
    CONSTRAINT uk_crdc_seq UNIQUE (crdc_draft_id, crdc_sequence_no, crdc_sub_sequence),
    CONSTRAINT chk_crdc_rm_type CHECK (
        crdc_rm_type IN ('STORE_RATE', 'CAPTIVE_COST', 'MULTI_YARN', 'UNEVEN_PACKING')
    )
);

CREATE INDEX IF NOT EXISTS idx_crdc_draft ON cost_routing_draft_component (crdc_draft_id, crdc_sequence_no);

COMMENT ON TABLE cost_routing_draft_component IS 'PRD Phase A §7.1.6 — BOM line for routing draft. rm_ref_text is FREE-TEXT (lenient vs Phase B which requires FK).';
