-- Migration: Create aud_rm_group + aud_rm_group_detail — append-only audit trail
-- for RM group head and detail mutations. Every create/update/delete records a
-- full snapshot of the row as it appeared after the operation, together with
-- the actor and the action type.

CREATE TABLE IF NOT EXISTS aud_rm_group (
    history_id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_head_id        UUID NOT NULL,
    action               VARCHAR(20) NOT NULL,

    -- Snapshot of head fields after the operation.
    group_code           VARCHAR(30)  NOT NULL,
    group_name           VARCHAR(200) NOT NULL,
    description          TEXT,
    colourant            VARCHAR(30),
    ci_name              VARCHAR(30),
    cost_percentage      DECIMAL(20,6) NOT NULL DEFAULT 0,
    cost_per_kg          DECIMAL(20,6) NOT NULL DEFAULT 0,
    flag_valuation       VARCHAR(20)  NOT NULL,
    flag_marketing       VARCHAR(20)  NOT NULL,
    flag_simulation      VARCHAR(20)  NOT NULL,
    init_val_valuation   DECIMAL(20,6),
    init_val_marketing   DECIMAL(20,6),
    init_val_simulation  DECIMAL(20,6),
    is_active            BOOLEAN NOT NULL,

    -- Actor + timestamp.
    changed_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    changed_by           VARCHAR(100) NOT NULL,

    CONSTRAINT chk_aud_rm_group_action CHECK (action IN ('CREATE','UPDATE','DELETE'))
);

COMMENT ON TABLE aud_rm_group IS 'Append-only audit log of cst_rm_group_head mutations. Each row captures a snapshot of head fields after the operation.';

CREATE INDEX IF NOT EXISTS idx_aud_rm_group_head
    ON aud_rm_group (group_head_id, changed_at DESC);

CREATE INDEX IF NOT EXISTS idx_aud_rm_group_changed_at
    ON aud_rm_group (changed_at DESC);

CREATE TABLE IF NOT EXISTS aud_rm_group_detail (
    history_id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    group_detail_id    UUID NOT NULL,
    group_head_id      UUID NOT NULL,
    action             VARCHAR(20) NOT NULL,

    -- Snapshot of detail fields after the operation.
    item_code          VARCHAR(20) NOT NULL,
    item_name          VARCHAR(240),
    item_type_code     VARCHAR(60),
    grade_code         VARCHAR(40),
    item_grade         VARCHAR(240),
    uom_code           VARCHAR(12),
    market_percentage  DECIMAL(20,6),
    market_value_rp    DECIMAL(20,6),
    sort_order         INT     NOT NULL DEFAULT 0,
    is_active          BOOLEAN NOT NULL,
    is_dummy           BOOLEAN NOT NULL,

    -- Actor + timestamp.
    changed_at         TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    changed_by         VARCHAR(100) NOT NULL,

    CONSTRAINT chk_aud_rm_group_detail_action CHECK (action IN ('CREATE','UPDATE','DELETE'))
);

COMMENT ON TABLE aud_rm_group_detail IS 'Append-only audit log of cst_rm_group_detail mutations. Each row captures a snapshot of detail fields after the operation.';

CREATE INDEX IF NOT EXISTS idx_aud_rm_group_detail_detail
    ON aud_rm_group_detail (group_detail_id, changed_at DESC);

CREATE INDEX IF NOT EXISTS idx_aud_rm_group_detail_head
    ON aud_rm_group_detail (group_head_id, changed_at DESC);

CREATE INDEX IF NOT EXISTS idx_aud_rm_group_detail_changed_at
    ON aud_rm_group_detail (changed_at DESC);
