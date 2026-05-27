-- Canonical PRD Phase B §7.4.1 — cost_product_master (CPM_).
-- Product identity in costing system, separate from ERP.
-- Code format: CST + CPT_type_code + YYMM + LPAD(auto_number, 6, '0').
-- Includes generate_cost_product_code() function for atomic auto-code via FOR UPDATE.

CREATE TABLE IF NOT EXISTS cost_product_master (
    cpm_product_sys_id     BIGSERIAL    PRIMARY KEY,
    cpm_product_code       VARCHAR(20)  NOT NULL,
    cpm_product_type_id    INT          NOT NULL
        REFERENCES cost_product_type (cpt_type_id) ON DELETE RESTRICT,
    cpm_product_name       TEXT         NOT NULL,
    cpm_shade_code         VARCHAR(50),
    cpm_grade_code         VARCHAR(20)  NOT NULL DEFAULT 'AX',
    cpm_description        TEXT,
    cpm_erp_item_code      VARCHAR(20),
    cpm_erp_grade_code_1   VARCHAR(20),
    cpm_erp_grade_code_2   VARCHAR(20),
    cpm_erp_linked_at      TIMESTAMPTZ,
    cpm_erp_linked_by      VARCHAR(64),
    cpm_is_active          BOOLEAN      NOT NULL DEFAULT TRUE,
    cpm_created_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpm_created_by         VARCHAR(64)  NOT NULL,
    cpm_updated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    cpm_updated_by         VARCHAR(64)  NOT NULL
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_product_master_code
    ON cost_product_master (cpm_product_code);

CREATE INDEX IF NOT EXISTS idx_cost_product_master_type
    ON cost_product_master (cpm_product_type_id)
    WHERE cpm_is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_cost_product_master_erp_item
    ON cost_product_master (cpm_erp_item_code)
    WHERE cpm_erp_item_code IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_cost_product_master_name_search
    ON cost_product_master USING GIN (to_tsvector('simple', COALESCE(cpm_product_name, '')));

COMMENT ON TABLE cost_product_master IS 'PRD Phase B §7.4.1 — Costing product identity (CST-prefixed code). Separate from ERP.';

-- =============================================================================
-- generate_cost_product_code(type_id, clock) → VARCHAR
-- Atomic per (type, year_month) counter increment via cost_product_code_counter.
-- =============================================================================
CREATE OR REPLACE FUNCTION generate_cost_product_code(
    p_type_id INT,
    p_clock   TIMESTAMPTZ DEFAULT NOW()
) RETURNS VARCHAR LANGUAGE plpgsql AS $$
DECLARE
    v_type_code     VARCHAR(5);
    v_year_month    VARCHAR(4);
    v_next_number   INT;
BEGIN
    SELECT cpt_type_code INTO v_type_code
    FROM cost_product_type
    WHERE cpt_type_id = p_type_id AND cpt_is_active = TRUE;

    IF v_type_code IS NULL THEN
        RAISE EXCEPTION 'cost_product_type id % not found or inactive', p_type_id;
    END IF;

    v_year_month := TO_CHAR(p_clock, 'YYMM');

    INSERT INTO cost_product_code_counter (cpcc_product_type_id, cpcc_year_month, cpcc_last_number)
    VALUES (p_type_id, v_year_month, 1)
    ON CONFLICT (cpcc_product_type_id, cpcc_year_month)
    DO UPDATE SET cpcc_last_number = cost_product_code_counter.cpcc_last_number + 1
    RETURNING cpcc_last_number INTO v_next_number;

    RETURN 'CST' || v_type_code || v_year_month || LPAD(v_next_number::TEXT, 6, '0');
END;
$$;

COMMENT ON FUNCTION generate_cost_product_code(INT, TIMESTAMPTZ)
    IS 'Atomic product code generator. Format: CST + CPT_type_code + YYMM + LPAD(seq, 6, 0).';
