-- WO production actual: 1:N per (wo, date, shift). ETL-suggested from Oracle, editable.
-- TXT/TWT and SPG bobbin columns + two-axis qty model (v1.2) + operator KPI cols.
-- TRN_STS TXT: 0=Full, 1=Unfull (inverse of SPG). ADJUSTED rows keep bobbin baseline.
CREATE TABLE IF NOT EXISTS wo_production_actual (
    wpa_id                 BIGSERIAL     PRIMARY KEY,
    wpa_wo_id              BIGINT        NOT NULL REFERENCES work_order(wo_id),
    wpa_date               DATE          NOT NULL,
    wpa_shift              CHAR(1)       NOT NULL,   -- 1 / 2 / 3
    wpa_area               CHAR(3)       NOT NULL,   -- TXT / SPG / TWT

    -- TXT/TWT columns (PPC_TXT_PRODUCTION, aggregated per date+shift)
    wpa_total_bobbins      INT,
    wpa_full_bobbins       INT,           -- TRN_STS=0 (Full)
    wpa_unfull_bobbins     INT,           -- TRN_STS=1 (Unfull)
    wpa_normal_bobs        INT,           -- TQM passed (FINAL_TYPE!=7, APP_REL=2)
    wpa_downgrade_bobs     INT,           -- TQM final defect (TYPE=7)
    wpa_pending_bobs       INT,           -- still held by TQM
    wpa_pack_cek_bobs      INT,           -- handover to packing

    -- SPG columns (PPC_SPG_PRODUCTION, aggregated per date+shift)
    wpa_gross_bobbins      INT,           -- all off machine (DOFFCONT)
    wpa_transferred_bobs   INT,           -- TRN_TYPE!=4, TRN_STATUS=2
    wpa_cut_bobbins        INT,           -- TRN_TYPE=4 (cut)
    wpa_not_transfer       INT,           -- not yet in TRANSFER
    wpa_normal_bobs_spg    INT,           -- TQM_GRADE=1
    wpa_downgrade_bobs_spg INT,           -- TQM_GRADE=0
    wpa_not_checked_bobs   INT,           -- TRN_APP_REL_DT IS NULL
    wpa_weight_per_bob     DECIMAL(10,4), -- DOFF_WT (shift average)

    -- Two-axis model (v1.2). Audit axis: objective baseline + editable current.
    wpa_qty_bobbin         DECIMAL(18,3), -- ETL bobbin, immutable (baseline)
    wpa_qty_actual         DECIMAL(18,3), -- current, editable, default = qty_bobbin (efficiency basis)
    wpa_qty_source         VARCHAR(10)   DEFAULT 'BOBBIN', -- BOBBIN / ADJUSTED
    wpa_manual_reason      TEXT,          -- reason when ADJUSTED (detail log in wo_actual_log)
    -- SPG dual basis (doffed=efficiency, transferred=fulfillment)
    wpa_qty_doffed_kg      DECIMAL(18,3), -- SPG: qty_bobbin/efficiency basis
    wpa_qty_transferred_kg DECIMAL(18,3), -- SPG: fulfillment basis
    -- Scope axis: Incl/Excl DERIVED from qty_actual + wo_prod_category (not stored)

    -- KPI shift input operator (v1.2, page 13)
    wpa_breaks_shift1      INT,
    wpa_breaks_shift2      INT,
    wpa_breaks_shift3      INT,           -- breaks per shift (legacy YARNBREAK_1/2/3)
    wpa_doff_full_count    INT,
    wpa_doff_manual_count  INT,           -- revolving manual
    wpa_co_failure_count   INT,           -- change over failure

    -- ETL metadata
    wpa_sync_status        VARCHAR(20)   DEFAULT 'OK',  -- OK / SYNC_FAILED / PENDING
    wpa_synced_at          TIMESTAMPTZ,
    wpa_last_edited_by     BIGINT,
    wpa_last_edited_at     TIMESTAMPTZ,

    CONSTRAINT uq_wpa_wo_date_shift UNIQUE (wpa_wo_id, wpa_date, wpa_shift)
);

CREATE INDEX IF NOT EXISTS idx_wpa_wo_date ON wo_production_actual (wpa_wo_id, wpa_date);
