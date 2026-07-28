-- Machine shift log (daily-perf v1.1). Per-machine per-shift positions + running
-- minutes feeding the efficiency engine. FK to machine (same DB).
CREATE TABLE IF NOT EXISTS machine_shift_log (
    msl_id              BIGSERIAL     PRIMARY KEY,
    msl_machine_id      BIGINT        NOT NULL REFERENCES machine(machine_id),
    msl_date            DATE          NOT NULL,
    msl_shift           CHAR(1)       NOT NULL,   -- 1 / 2 / 3
    msl_positions_total INT,
    msl_positions_running DECIMAL(8,2),           -- may be fractional
    msl_running_minutes INT,                       -- DERIVED from downtime (v1.2), not typed
    -- msl_activity_note removed (v1.2) -> replaced by shift_log_note (INSTRUKSI/ACTIVITY)
    msl_status          VARCHAR(20)   NOT NULL DEFAULT 'DRAFT', -- DRAFT / FINAL
    msl_input_by        BIGINT        NOT NULL,
    msl_input_at        TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    msl_updated_at      TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_msl_status CHECK (msl_status IN ('DRAFT', 'FINAL')),
    CONSTRAINT uq_msl_machine_date_shift UNIQUE (msl_machine_id, msl_date, msl_shift)
);

CREATE INDEX IF NOT EXISTS idx_msl_date ON machine_shift_log (msl_date, msl_shift);
