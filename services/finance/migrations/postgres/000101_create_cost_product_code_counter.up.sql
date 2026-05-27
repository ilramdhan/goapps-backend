-- Canonical PRD Phase B §7.2.2 — cost_product_code_counter (CPCC_).
-- Atomic per-(type, year_month) counter used by product code generator.
-- Format: CST + CPT_type_code + YYMM + LPAD(auto, 6, '0').
-- Concurrency: SELECT FOR UPDATE pattern in the generator function (see migration 106).

CREATE TABLE IF NOT EXISTS cost_product_code_counter (
    cpcc_counter_id      SERIAL PRIMARY KEY,
    cpcc_product_type_id INT NOT NULL
        REFERENCES cost_product_type (cpt_type_id) ON DELETE RESTRICT,
    cpcc_year_month      VARCHAR(4) NOT NULL,
    cpcc_last_number     INT NOT NULL DEFAULT 0
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_product_code_counter
    ON cost_product_code_counter (cpcc_product_type_id, cpcc_year_month);

CREATE INDEX IF NOT EXISTS idx_cost_product_code_counter_type
    ON cost_product_code_counter (cpcc_product_type_id);

COMMENT ON TABLE  cost_product_code_counter IS 'PRD Phase B §7.2.2 — Per-(type, year_month) sequence for product code generator.';
COMMENT ON COLUMN cost_product_code_counter.cpcc_year_month IS 'YYMM format (e.g. 2605 = May 2026).';
