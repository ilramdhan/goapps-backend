-- cost_product_parameter (CPP_) — per-product static parameter values.
-- This is the DETAIL table for cost_product_master (CPM_). One row = one param
-- value for one product. UNIQUE(product_sys_id, param_id) enforces single value
-- per pair. Period-dependent params (mst_parameter.is_period_dependent = TRUE)
-- belong to Phase C and are NOT stored here.
--
-- Value storage follows the canonical PRD §7.9.4 pattern: exactly ONE of
-- value_numeric / value_text / value_flag is populated per row, matching the
-- data_type of the referenced mst_parameter row. The CHECK constraint enforces
-- this at write time.

CREATE TABLE IF NOT EXISTS cost_product_parameter (
    cpp_value_id        BIGSERIAL PRIMARY KEY,
    cpp_product_sys_id  BIGINT      NOT NULL
        REFERENCES cost_product_master(cpm_product_sys_id) ON DELETE CASCADE,
    cpp_param_id        UUID        NOT NULL
        REFERENCES mst_parameter(id),
    cpp_value_numeric   NUMERIC(20,6),
    cpp_value_text      TEXT,
    cpp_value_flag      BOOLEAN,

    cpp_filled_at       TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpp_filled_by       VARCHAR(100) NOT NULL,
    cpp_created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    cpp_created_by      VARCHAR(100) NOT NULL,
    cpp_updated_at      TIMESTAMPTZ,
    cpp_updated_by      VARCHAR(100),

    CONSTRAINT cpp_unique_product_param UNIQUE (cpp_product_sys_id, cpp_param_id),

    -- Exactly one value column populated per row.
    CONSTRAINT cpp_one_value_chk CHECK (
        (CASE WHEN cpp_value_numeric IS NOT NULL THEN 1 ELSE 0 END)
      + (CASE WHEN cpp_value_text    IS NOT NULL THEN 1 ELSE 0 END)
      + (CASE WHEN cpp_value_flag    IS NOT NULL THEN 1 ELSE 0 END)
      = 1
    )
);

CREATE INDEX IF NOT EXISTS idx_cpp_product ON cost_product_parameter(cpp_product_sys_id);
CREATE INDEX IF NOT EXISTS idx_cpp_param   ON cost_product_parameter(cpp_param_id);

COMMENT ON TABLE  cost_product_parameter IS 'Phase B: per-product static parameter values. Detail of cost_product_master.';
COMMENT ON COLUMN cost_product_parameter.cpp_value_numeric IS 'Populated when referenced mst_parameter.data_type = NUMBER.';
COMMENT ON COLUMN cost_product_parameter.cpp_value_text    IS 'Populated when referenced mst_parameter.data_type = TEXT (also stores LOOKUP key strings).';
COMMENT ON COLUMN cost_product_parameter.cpp_value_flag    IS 'Populated when referenced mst_parameter.data_type = BOOLEAN.';
