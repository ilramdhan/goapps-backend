-- Common lot (Phase 3, PRD v1.2 §12): combine leftover bobbins across shades into
-- a new ERP identity. Original lots are tracked as components.
CREATE TABLE IF NOT EXISTS common_lot (
    cl_id             BIGSERIAL     PRIMARY KEY,
    cl_lot_no         VARCHAR(30)   NOT NULL UNIQUE,   -- new ERP lot number
    cl_item_code      VARCHAR(30),
    cl_shade_code     VARCHAR(20),
    cl_erp_grade_code VARCHAR(5),
    cl_created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS common_lot_component (
    clc_id                  BIGSERIAL     PRIMARY KEY,
    clc_common_lot_id       BIGINT        NOT NULL REFERENCES common_lot(cl_id) ON DELETE CASCADE,
    clc_original_lot_no     VARCHAR(30)   NOT NULL,
    clc_original_shade_code VARCHAR(20),
    clc_bobbin_count        INT,
    clc_qty_kg              DECIMAL(10, 3)
);

CREATE INDEX IF NOT EXISTS idx_clc_common_lot_id ON common_lot_component (clc_common_lot_id);
