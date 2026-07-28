-- Downtime event (daily-perf v1.1). FKs to machine, machine_shift_log, and
-- downtime_reason_master (all created earlier). de_ce_id is a plain BIGINT: it points
-- at changeover_event which is created in a later migration (no forward FK possible),
-- and is used to avoid double-counting changeover downtime.
CREATE TABLE IF NOT EXISTS downtime_event (
    de_id           BIGSERIAL     PRIMARY KEY,
    de_machine_id   BIGINT        NOT NULL REFERENCES machine(machine_id),
    de_wo_id        BIGINT,                   -- nullable
    de_shift_log_id BIGINT        REFERENCES machine_shift_log(msl_id),
    de_ce_id        BIGINT,                   -- ref changeover_event (created later, no FK)
    de_date         DATE          NOT NULL,
    de_shift        CHAR(1),
    de_position_no  VARCHAR(10),              -- for idle position
    de_reason_id    BIGINT        NOT NULL REFERENCES downtime_reason_master(drm_id),
    de_start_at     TIMESTAMPTZ,
    de_end_at       TIMESTAMPTZ,
    de_duration_min INT,
    de_lost_kg      DECIMAL(12,3),            -- auto from theoretical rate, editable
    de_notes        TEXT,
    de_input_by     BIGINT        NOT NULL,
    de_input_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_de_machine_date ON downtime_event (de_machine_id, de_date, de_shift);
CREATE INDEX IF NOT EXISTS idx_de_shift_log_id ON downtime_event (de_shift_log_id);
