-- Migration: Create cst_rm_group_head_period — period-scoped snapshot of the
-- editable cst_rm_group_head fields, keyed on (period, group_head_id).
--
-- cst_rm_group_head remains the "current/latest" anchor row (its identity —
-- group_head_id, group_code — is relied upon by cst_rm_cost.group_head_id,
-- cst_rm_cost_detail.group_detail_id, ExistsHeadByCode/ExistsHeadByID, AddDetail,
-- and calculate_handler_v2.go's period-blind GetHeadByID/ListActiveDetailsByHeadID
-- calls). This table lets an edit to an older period be recorded without mutating
-- what "now" looks like. See docs/superpowers/specs/2026-09-05-rm-group-period-
-- versioning-design.md §1.2 for the full rationale.

CREATE TABLE IF NOT EXISTS cst_rm_group_head_period (
    group_head_period_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity of the snapshot.
    period                VARCHAR(6) NOT NULL,                 -- YYYYMM
    group_head_id         UUID NOT NULL REFERENCES cst_rm_group_head(group_head_id),

    -- Snapshot of the same editable fields as cst_rm_group_head.
    name                  VARCHAR(200) NOT NULL,
    description           TEXT,
    colorant              VARCHAR(100),
    ci_name               VARCHAR(100),
    cost_percentage       DECIMAL(20,6) NOT NULL DEFAULT 0,
    cost_per_kg           DECIMAL(20,6) NOT NULL DEFAULT 0,
    flag_valuation        VARCHAR(20) NOT NULL,
    flag_marketing        VARCHAR(20) NOT NULL,
    flag_simulation       VARCHAR(20) NOT NULL,
    init_val_valuation    DECIMAL(20,6),
    init_val_marketing    DECIMAL(20,6),
    init_val_simulation   DECIMAL(20,6),
    is_active             BOOLEAN NOT NULL DEFAULT true,

    -- V2 marketing-projection inputs.
    marketing_freight_rate     DECIMAL(20,6),
    marketing_anti_dumping_pct DECIMAL(20,6),
    marketing_default_value    DECIMAL(20,6),
    valuation_flag_v2          VARCHAR(20),
    marketing_flag_v2          VARCHAR(20),

    -- Provenance: distinguishes a real per-period edit from a flat backfill copy.
    is_backfilled BOOLEAN NOT NULL DEFAULT false,

    -- Audit.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),

    CONSTRAINT chk_rm_group_head_period_format CHECK (period ~ '^[0-9]{6}$'),
    CONSTRAINT chk_rm_group_head_period_flag_valuation  CHECK (flag_valuation  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_group_head_period_flag_marketing  CHECK (flag_marketing  IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_group_head_period_flag_simulation CHECK (flag_simulation IN ('CONS','STORES','DEPT','PO_1','PO_2','PO_3','INIT')),
    CONSTRAINT chk_rm_group_head_period_valuation_flag_v2 CHECK (valuation_flag_v2 IS NULL OR valuation_flag_v2 IN ('AUTO','CR','SR','PR','CL','SL','FL')),
    CONSTRAINT chk_rm_group_head_period_marketing_flag_v2 CHECK (marketing_flag_v2 IS NULL OR marketing_flag_v2 IN ('AUTO','SP','PP','FP'))
);

COMMENT ON TABLE cst_rm_group_head_period IS 'Period-scoped snapshot of editable cst_rm_group_head fields, one row per (period, group_head_id) once that period has been explicitly edited or backfilled.';

-- UPSERT target: one snapshot per (period, group_head_id).
CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_group_head_period
    ON cst_rm_group_head_period (period, group_head_id);

-- Lookup all period snapshots for a given group.
CREATE INDEX IF NOT EXISTS idx_rm_group_head_period_head
    ON cst_rm_group_head_period (group_head_id);
