-- Migration: Create aud_rm_cost_history — append-only audit trail for every RM cost calculation.
-- Written in the SAME transaction as the cst_rm_cost UPSERT so runs are traceable and diffable.

CREATE TABLE IF NOT EXISTS aud_rm_cost_history (
    history_id            UUID PRIMARY KEY DEFAULT gen_random_uuid(),

    -- Links to produced cost row + job (both nullable: cost row may be NULL if UPSERT fails
    -- after snapshot was captured, and job_id may be NULL for direct-call calculations).
    rm_cost_id            UUID,
    job_id                UUID REFERENCES job_execution(job_id) ON DELETE SET NULL,

    -- Identity (copied for standalone queryability).
    period                VARCHAR(6)  NOT NULL,
    rm_code               VARCHAR(30) NOT NULL,
    rm_type               VARCHAR(20) NOT NULL,
    group_head_id         UUID,

    -- Snapshot: pre-cascade per-stage rates aggregated from source data.
    cons_rate             DECIMAL(20,6),
    stores_rate           DECIMAL(20,6),
    dept_rate             DECIMAL(20,6),
    po_rate_1             DECIMAL(20,6),
    po_rate_2             DECIMAL(20,6),
    po_rate_3             DECIMAL(20,6),

    -- Snapshot: header configuration used for this calculation.
    cost_percentage       DECIMAL(20,6) NOT NULL,
    cost_per_kg           DECIMAL(20,6) NOT NULL,
    flag_valuation        VARCHAR(20) NOT NULL,
    flag_marketing        VARCHAR(20) NOT NULL,
    flag_simulation       VARCHAR(20) NOT NULL,
    init_val_valuation    DECIMAL(20,6),
    init_val_marketing    DECIMAL(20,6),
    init_val_simulation   DECIMAL(20,6),

    -- Snapshot: computed outputs.
    cost_val              DECIMAL(20,6),
    cost_mark             DECIMAL(20,6),
    cost_sim              DECIMAL(20,6),
    flag_valuation_used   VARCHAR(20) NOT NULL,
    flag_marketing_used   VARCHAR(20) NOT NULL,
    flag_simulation_used  VARCHAR(20) NOT NULL,

    -- Context for reproducibility / debugging.
    source_item_count     INT          NOT NULL DEFAULT 0,
    trigger_reason        VARCHAR(50)  NOT NULL,   -- 'oracle-sync-chain' | 'group-update' | 'detail-change' | 'manual-ui' | 'cron'
    calculated_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    calculated_by         VARCHAR(100) NOT NULL,

    CONSTRAINT chk_aud_rm_cost_rm_type CHECK (rm_type IN ('GROUP','ITEM')),
    CONSTRAINT chk_aud_rm_cost_period_format CHECK (period ~ '^[0-9]{6}$'),
    CONSTRAINT chk_aud_rm_cost_source_item_count_nonneg CHECK (source_item_count >= 0)
);

COMMENT ON TABLE aud_rm_cost_history IS 'Append-only audit trail: every RM cost calculation writes one row here (same transaction as cst_rm_cost UPSERT).';

-- Lookup "show me all runs for this cost row" (audit drawer).
CREATE INDEX IF NOT EXISTS idx_aud_rm_cost_period_rm
    ON aud_rm_cost_history (period, rm_code, calculated_at DESC);

-- Lookup by job_id for "what did this job produce?".
CREATE INDEX IF NOT EXISTS idx_aud_rm_cost_job
    ON aud_rm_cost_history (job_id) WHERE job_id IS NOT NULL;

-- Lookup by group for per-group recalc timeline.
CREATE INDEX IF NOT EXISTS idx_aud_rm_cost_group
    ON aud_rm_cost_history (group_head_id, calculated_at DESC) WHERE group_head_id IS NOT NULL;
