-- WO grade actual: 1:N per grade (Phase 3). Packing actual from Oracle PPC_GRADE_ACTUAL.
CREATE TABLE IF NOT EXISTS wo_grade_actual (
    wga_id                BIGSERIAL     PRIMARY KEY,
    wga_wo_id             BIGINT        NOT NULL REFERENCES work_order(wo_id),
    wga_lot_no            VARCHAR(30)   NOT NULL,   -- original lot
    wga_grade             VARCHAR(5)    NOT NULL,   -- AX/AE/A9/A/AM/APQ/B/BB/C/JLT
    wga_dept              CHAR(3),                  -- TXT / TWT
    wga_total_qty_kg      DECIMAL(14,3),
    wga_bobbin_count      INT,
    wga_last_packing_date DATE,
    wga_synced_at         TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_wga_wo_lot_grade UNIQUE (wga_wo_id, wga_lot_no, wga_grade)
);

CREATE INDEX IF NOT EXISTS idx_wga_wo_id ON wo_grade_actual (wga_wo_id);
