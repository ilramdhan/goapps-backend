-- Shift log book, two types (INSTRUKSI/ACTIVITY) — replaces msl_activity_note (v1.2).
-- FK to machine (same DB). sln_wo_id is an optional soft ref to work_order context.
CREATE TABLE IF NOT EXISTS shift_log_note (
    sln_id         BIGSERIAL     PRIMARY KEY,
    sln_machine_id BIGINT        NOT NULL REFERENCES machine(machine_id),
    sln_date       DATE          NOT NULL,
    sln_shift      CHAR(1)       NOT NULL,   -- 1 / 2 / 3
    sln_type       VARCHAR(15)   NOT NULL,   -- INSTRUKSI / ACTIVITY
    sln_note       TEXT          NOT NULL,
    sln_wo_id      BIGINT,                   -- optional soft ref to work_order
    sln_input_by   BIGINT        NOT NULL,
    sln_input_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_sln_type CHECK (sln_type IN ('INSTRUKSI', 'ACTIVITY'))
);

CREATE INDEX IF NOT EXISTS idx_sln_machine_date ON shift_log_note (sln_machine_id, sln_date, sln_shift);
