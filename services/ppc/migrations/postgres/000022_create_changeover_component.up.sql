-- Changeover component (Phase 2). Per-component breakdown of a changeover event.
CREATE TABLE IF NOT EXISTS changeover_component (
    cc_id               BIGSERIAL     PRIMARY KEY,
    cc_event_id         BIGINT        NOT NULL REFERENCES changeover_event(ce_id),
    cc_component_code   CHAR(5)       NOT NULL,   -- BASE / C1-C7
    cc_duration_applied INT           NOT NULL,
    cc_waste_applied    DECIMAL(10,3) NOT NULL,
    cc_is_auto_detected BOOLEAN       NOT NULL DEFAULT TRUE,
    cc_override_by      BIGINT,
    cc_override_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_cc_event_id ON changeover_component (cc_event_id);
