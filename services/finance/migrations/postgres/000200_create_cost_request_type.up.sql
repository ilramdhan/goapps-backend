-- Canonical PRD Phase A §7.1.3 — cost_request_type (CRT_).
-- Lookup: QUOTE / DEVELOPMENT.
CREATE TABLE IF NOT EXISTS cost_request_type (
    crt_type_id                SERIAL       PRIMARY KEY,
    crt_code                   VARCHAR(30)  NOT NULL,
    crt_display_name           VARCHAR(80)  NOT NULL,
    crt_state_machine_variant  VARCHAR(30)  NOT NULL,
    crt_required_field_config  JSONB        NOT NULL DEFAULT '{}'::jsonb,
    crt_default_urgency        VARCHAR(10)  NOT NULL DEFAULT 'medium',
    crt_is_active              BOOLEAN      NOT NULL DEFAULT TRUE,
    CONSTRAINT chk_crt_variant   CHECK (crt_state_machine_variant IN ('FULL', 'SHORTCUT_CAPABLE')),
    CONSTRAINT chk_crt_urgency   CHECK (crt_default_urgency IN ('low', 'medium', 'high'))
);

CREATE UNIQUE INDEX IF NOT EXISTS uk_cost_request_type_code ON cost_request_type (crt_code);

-- Seed canonical types.
INSERT INTO cost_request_type (crt_code, crt_display_name, crt_state_machine_variant, crt_default_urgency)
VALUES
    ('QUOTE',       'Quote inquiry',         'SHORTCUT_CAPABLE', 'medium'),
    ('DEVELOPMENT', 'Development / new RnD', 'FULL',             'medium')
ON CONFLICT (crt_code) DO NOTHING;

COMMENT ON TABLE cost_request_type IS 'PRD Phase A §7.1.3 — Request type lookup. QUOTE allows existing/shortcut; DEVELOPMENT forces full flow.';
