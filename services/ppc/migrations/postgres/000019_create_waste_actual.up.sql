-- Waste actual (daily-perf v1.1). FKs to machine_shift_log and waste_category_master.
-- wa_machine_id / wa_wo_id nullable (waste may be recorded at area level, not tied to lot).
CREATE TABLE IF NOT EXISTS waste_actual (
    wa_id           BIGSERIAL     PRIMARY KEY,
    wa_area         CHAR(3)       NOT NULL,   -- TXT / SPG / TWT
    wa_machine_id   BIGINT,                   -- nullable (area-level waste)
    wa_wo_id        BIGINT,                   -- nullable
    wa_shift_log_id BIGINT        REFERENCES machine_shift_log(msl_id),
    wa_date         DATE          NOT NULL,
    wa_shift        CHAR(1),
    wa_category_id  BIGINT        NOT NULL REFERENCES waste_category_master(wcm_id),
    wa_qty_kg       DECIMAL(12,3) NOT NULL,
    wa_is_upset     BOOLEAN       NOT NULL DEFAULT FALSE, -- Reguler/Upsets (SPG)
    wa_notes        TEXT,
    wa_input_by     BIGINT        NOT NULL,
    wa_input_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_wa_area CHECK (wa_area IN ('TXT', 'SPG', 'TWT'))
);

CREATE INDEX IF NOT EXISTS idx_wa_area_date ON waste_actual (wa_area, wa_date, wa_shift);
CREATE INDEX IF NOT EXISTS idx_wa_category_id ON waste_actual (wa_category_id);
