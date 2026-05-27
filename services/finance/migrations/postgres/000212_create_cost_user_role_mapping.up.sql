-- Canonical PRD Phase A §7.1.11 — cost_user_role_mapping (CURM_).
-- Maps SSO user_id → (tier, functional_role) for routing-rule evaluation + access control.
CREATE TABLE IF NOT EXISTS cost_user_role_mapping (
    curm_mapping_id     BIGSERIAL    PRIMARY KEY,
    curm_user_id        VARCHAR(64)  NOT NULL,
    curm_tier           VARCHAR(20)  NOT NULL,
    curm_functional_role VARCHAR(30) NOT NULL,
    curm_is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    curm_effective_from TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    curm_effective_to   TIMESTAMPTZ,
    CONSTRAINT chk_curm_tier CHECK (curm_tier IN ('User', 'Dept Lead', 'Manager', 'Admin')),
    CONSTRAINT chk_curm_functional_role CHECK (
        curm_functional_role IN ('Marketing', 'Engineering', 'Produksi', 'RND', 'Finance', 'Admin')
    )
);

CREATE INDEX IF NOT EXISTS idx_curm_user_active
    ON cost_user_role_mapping (curm_user_id)
    WHERE curm_is_active = TRUE;

CREATE INDEX IF NOT EXISTS idx_curm_role_active
    ON cost_user_role_mapping (curm_functional_role, curm_tier)
    WHERE curm_is_active = TRUE;

COMMENT ON TABLE cost_user_role_mapping IS 'PRD Phase A §7.1.11 — user_id → (tier, functional_role) for routing + access.';
