-- Canonical PRD Phase A §7.1.1 — cost_product_request (CPR_).
-- Aggregate root. Request-no generated via generate_cost_request_no() inserted below.

CREATE TABLE IF NOT EXISTS cost_product_request (
    cpr_request_id                     BIGSERIAL    PRIMARY KEY,
    cpr_request_no                     VARCHAR(30)  NOT NULL,
    cpr_request_type_id                INT          NOT NULL
        REFERENCES cost_request_type (crt_type_id) ON DELETE RESTRICT,
    cpr_title                          VARCHAR(255) NOT NULL,
    cpr_description                    TEXT,
    cpr_customer_name                  VARCHAR(255) NOT NULL,
    cpr_customer_code                  VARCHAR(50),
    cpr_product_classification         VARCHAR(20)  NOT NULL,
    cpr_verified_classification        VARCHAR(20),
    cpr_classification_override_reason TEXT,
    cpr_target_volume                  DECIMAL(18,4),
    cpr_target_price_range             VARCHAR(50),
    cpr_urgency_level                  VARCHAR(10)  NOT NULL DEFAULT 'medium',
    cpr_needed_by_date                 DATE,
    cpr_status                         VARCHAR(30)  NOT NULL DEFAULT 'DRAFT',
    cpr_closed_substatus               VARCHAR(20),
    cpr_feasibility_decision           VARCHAR(20),
    cpr_feasibility_note               TEXT,
    cpr_feasibility_by                 VARCHAR(64),
    cpr_feasibility_at                 TIMESTAMPTZ,
    cpr_reject_reason                  TEXT,
    cpr_cancel_reason                  TEXT,
    cpr_assigned_to_user_id            VARCHAR(64),
    cpr_requester_user_id              VARCHAR(64)  NOT NULL,
    cpr_created_at                     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpr_updated_at                     TIMESTAMPTZ  NOT NULL DEFAULT NOW(),

    CONSTRAINT uk_cpr_request_no UNIQUE (cpr_request_no),
    CONSTRAINT chk_cpr_classification CHECK (cpr_product_classification IN ('existing', 'new')),
    CONSTRAINT chk_cpr_verified_classification CHECK (
        cpr_verified_classification IS NULL
        OR cpr_verified_classification IN ('existing', 'new')
    ),
    CONSTRAINT chk_cpr_urgency CHECK (cpr_urgency_level IN ('low', 'medium', 'high')),
    CONSTRAINT chk_cpr_status CHECK (
        cpr_status IN (
            'DRAFT', 'SUBMITTED', 'UNDER_REVIEW', 'ROUTING_DEFINED',
            'PARAMETER_PENDING', 'PARAMETER_COMPLETE', 'COSTING_DONE',
            'QUOTED', 'QUOTE_READY', 'CLOSED', 'REJECTED'
        )
    ),
    CONSTRAINT chk_cpr_closed_substatus CHECK (
        cpr_closed_substatus IS NULL
        OR cpr_closed_substatus IN ('won', 'lost', 'cancelled', 'on_hold')
    ),
    CONSTRAINT chk_cpr_feasibility_decision CHECK (
        cpr_feasibility_decision IS NULL
        OR cpr_feasibility_decision IN ('FEASIBLE', 'NOT_FEASIBLE')
    ),
    CONSTRAINT chk_cpr_feasibility_note CHECK (
        cpr_feasibility_decision <> 'NOT_FEASIBLE'
        OR cpr_feasibility_note IS NOT NULL
    ),
    CONSTRAINT chk_cpr_verified_override CHECK (
        cpr_verified_classification IS NULL
        OR cpr_verified_classification = cpr_product_classification
        OR cpr_classification_override_reason IS NOT NULL
    ),
    CONSTRAINT chk_cpr_closed_substatus_required CHECK (
        cpr_status <> 'CLOSED' OR cpr_closed_substatus IS NOT NULL
    )
);

CREATE INDEX IF NOT EXISTS idx_cpr_status        ON cost_product_request (cpr_status);
CREATE INDEX IF NOT EXISTS idx_cpr_requester     ON cost_product_request (cpr_requester_user_id);
CREATE INDEX IF NOT EXISTS idx_cpr_assignee      ON cost_product_request (cpr_assigned_to_user_id);
CREATE INDEX IF NOT EXISTS idx_cpr_type          ON cost_product_request (cpr_request_type_id);
CREATE INDEX IF NOT EXISTS idx_cpr_search        ON cost_product_request
    USING GIN (to_tsvector('simple',
        COALESCE(cpr_title,'') || ' ' || COALESCE(cpr_customer_name,'') || ' ' || COALESCE(cpr_description,'')));

COMMENT ON TABLE cost_product_request IS 'PRD Phase A §7.1.1 — Product request aggregate root. State machine hard-coded in domain (G3 hybrid).';

-- Per-month counter for REQ-YYYYMM-NNNN generation.
CREATE TABLE IF NOT EXISTS cost_request_no_counter (
    crnc_year_month  VARCHAR(6) PRIMARY KEY,
    crnc_last_number INT        NOT NULL DEFAULT 0
);

-- Atomic next-number function. Format REQ-YYYYMM-NNNN.
CREATE OR REPLACE FUNCTION generate_cost_request_no(p_clock TIMESTAMPTZ DEFAULT NOW())
RETURNS VARCHAR LANGUAGE plpgsql AS $$
DECLARE
    v_ym VARCHAR(6);
    v_n  INT;
BEGIN
    v_ym := TO_CHAR(p_clock, 'YYYYMM');
    INSERT INTO cost_request_no_counter (crnc_year_month, crnc_last_number)
    VALUES (v_ym, 1)
    ON CONFLICT (crnc_year_month)
    DO UPDATE SET crnc_last_number = cost_request_no_counter.crnc_last_number + 1
    RETURNING crnc_last_number INTO v_n;
    RETURN 'REQ-' || v_ym || '-' || LPAD(v_n::TEXT, 4, '0');
END;
$$;

COMMENT ON FUNCTION generate_cost_request_no(TIMESTAMPTZ) IS
    'PRD Phase A — atomic REQ-YYYYMM-NNNN generator. Use inside an INSERT to avoid race.';
