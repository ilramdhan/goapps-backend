-- Migration: Create cst_rm_group_detail_period — period-scoped snapshot of the
-- editable cst_rm_group_detail fields, keyed on (period, group_detail_id).
--
-- cst_rm_group_detail remains the "current/latest" anchor row (its identity —
-- group_detail_id, item_code — is relied upon by cst_rm_cost_detail.group_detail_id,
-- the uk_rm_group_detail_item_active partial unique index, and
-- calculate_handler_v2.go's period-blind ListActiveDetailsByHeadID call). This table
-- lets an edit to an older period be recorded without mutating what "now" looks
-- like. See docs/superpowers/specs/2026-09-05-rm-group-period-versioning-design.md
-- §1.3 for the full rationale.

CREATE TABLE IF NOT EXISTS cst_rm_group_detail_period (
    group_detail_period_id UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Identity of the snapshot.
    period                  VARCHAR(6) NOT NULL,                 -- YYYYMM
    group_detail_id         UUID NOT NULL REFERENCES cst_rm_group_detail(group_detail_id),

    -- Snapshot of the same editable fields as cst_rm_group_detail.
    market_percentage DECIMAL(20,6),
    market_value_rp   DECIMAL(20,6),
    sort_order        INT     NOT NULL DEFAULT 0,
    is_active         BOOLEAN NOT NULL DEFAULT true,
    is_dummy          BOOLEAN NOT NULL DEFAULT false,

    -- V2 valuation inputs.
    valuation_freight_rate     DECIMAL(20,6),
    valuation_anti_dumping_pct DECIMAL(20,6),
    valuation_duty_pct         DECIMAL(20,6),
    valuation_transport_rate   DECIMAL(20,6),
    valuation_default_value    DECIMAL(20,6),

    -- Provenance: distinguishes a real per-period edit from a flat backfill copy.
    is_backfilled BOOLEAN NOT NULL DEFAULT false,

    -- Audit.
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_by VARCHAR(100) NOT NULL,
    updated_at TIMESTAMPTZ,
    updated_by VARCHAR(100),

    CONSTRAINT chk_rm_group_detail_period_format CHECK (period ~ '^[0-9]{6}$'),
    CONSTRAINT chk_rm_group_detail_period_market_percentage_nonneg CHECK (market_percentage IS NULL OR market_percentage >= 0),
    CONSTRAINT chk_rm_group_detail_period_market_value_nonneg      CHECK (market_value_rp   IS NULL OR market_value_rp   >= 0),
    CONSTRAINT chk_rm_group_detail_period_freight_rate_nonneg      CHECK (valuation_freight_rate     IS NULL OR valuation_freight_rate     >= 0),
    CONSTRAINT chk_rm_group_detail_period_anti_dumping_nonneg      CHECK (valuation_anti_dumping_pct IS NULL OR valuation_anti_dumping_pct >= 0),
    CONSTRAINT chk_rm_group_detail_period_duty_nonneg              CHECK (valuation_duty_pct         IS NULL OR valuation_duty_pct         >= 0),
    CONSTRAINT chk_rm_group_detail_period_transport_nonneg         CHECK (valuation_transport_rate   IS NULL OR valuation_transport_rate   >= 0),
    CONSTRAINT chk_rm_group_detail_period_default_value_nonneg     CHECK (valuation_default_value    IS NULL OR valuation_default_value    >= 0)
);

COMMENT ON TABLE cst_rm_group_detail_period IS 'Period-scoped snapshot of editable cst_rm_group_detail fields, one row per (period, group_detail_id) once that period has been explicitly edited or backfilled.';

-- UPSERT target: one snapshot per (period, group_detail_id).
CREATE UNIQUE INDEX IF NOT EXISTS uk_rm_group_detail_period
    ON cst_rm_group_detail_period (period, group_detail_id);

-- Lookup all period snapshots for a given detail row.
CREATE INDEX IF NOT EXISTS idx_rm_group_detail_period_detail
    ON cst_rm_group_detail_period (group_detail_id);
