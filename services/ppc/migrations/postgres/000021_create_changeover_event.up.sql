-- Changeover event (Phase 2). FKs to work_order (from/to) and machine (same DB).
CREATE TABLE IF NOT EXISTS changeover_event (
    ce_id                 BIGSERIAL     PRIMARY KEY,
    ce_from_wo_id         BIGINT        NOT NULL REFERENCES work_order(wo_id),
    ce_to_wo_id           BIGINT        NOT NULL REFERENCES work_order(wo_id),
    ce_machine_id         BIGINT        NOT NULL REFERENCES machine(machine_id),
    ce_duration_estimated INT,                       -- minutes
    ce_waste_estimated    DECIMAL(10,3),             -- kg
    ce_group              VARCHAR(10),               -- MINOR / MEDIUM / MAJOR / DEEP
    ce_duration_actual    INT,
    ce_waste_actual       DECIMAL(10,3),
    ce_status             VARCHAR(20),               -- PLANNED / IN_PROGRESS / DONE
    ce_started_at         TIMESTAMPTZ,
    ce_completed_at       TIMESTAMPTZ,
    ce_notes              TEXT
);

CREATE INDEX IF NOT EXISTS idx_ce_machine_id ON changeover_event (ce_machine_id);
CREATE INDEX IF NOT EXISTS idx_ce_from_wo_id ON changeover_event (ce_from_wo_id);
