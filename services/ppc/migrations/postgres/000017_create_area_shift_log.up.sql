-- Area shift log (daily-perf v1.1). Per-area overtime + notes; asl_shift NULL = daily.
CREATE TABLE IF NOT EXISTS area_shift_log (
    asl_id       BIGSERIAL     PRIMARY KEY,
    asl_area     CHAR(3)       NOT NULL,   -- TXT / SPG / TWT
    asl_date     DATE          NOT NULL,
    asl_shift    CHAR(1),                  -- NULL = daily
    asl_ot_hours DECIMAL(6,2),
    asl_notes    TEXT,
    asl_input_by BIGINT        NOT NULL,
    asl_input_at TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_asl_area CHECK (asl_area IN ('TXT', 'SPG', 'TWT')),
    CONSTRAINT uq_asl_area_date_shift UNIQUE (asl_area, asl_date, asl_shift)
);

CREATE INDEX IF NOT EXISTS idx_asl_date ON area_shift_log (asl_date);
