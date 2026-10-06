-- 000558 — ERP integration settings row (plan-06 P5-T11; design Part 2 §9.3;
-- User decision 2026-09-29 U-2).
--
-- One row (ceis_setting_id = 1) holding the admin-editable schedule of the
-- monthly read/compute run (LoadDemand -> Coverage -> Derive -> Validate).
-- Push, valuation, approve and restore are never scheduled.
--
-- Resolution order (application layer): (1) this row once ceis_schedule_mode
-- is set, (2) erp_integration.schedule.cron, (3) run_day_of_month + run_time.
-- The seeded row is disabled with no mode, so the config applies until an
-- admin edits it through UpdateErpIntegrationSchedule (audited). The env
-- ERP_SCHEDULE_ENABLED stays the master switch (default false).
--
-- New table only; nothing existing changes.

BEGIN;

CREATE TABLE IF NOT EXISTS cst_erp_integration_setting (
    ceis_setting_id       SMALLINT     NOT NULL DEFAULT 1,
    ceis_schedule_enabled BOOLEAN      NOT NULL DEFAULT FALSE,
    ceis_schedule_mode    VARCHAR(16)  NULL,
    ceis_run_day          SMALLINT     NULL,
    ceis_run_time         VARCHAR(5)   NULL,
    ceis_run_date         DATE         NULL,
    ceis_cron             VARCHAR(100) NULL,
    ceis_timezone         VARCHAR(64)  NOT NULL DEFAULT 'Asia/Jakarta',
    created_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by            VARCHAR(64)  NOT NULL,
    updated_at            TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    updated_by            VARCHAR(64)  NULL,
    CONSTRAINT pk_cst_erp_integration_setting PRIMARY KEY (ceis_setting_id),
    CONSTRAINT chk_ceis_single_row CHECK (ceis_setting_id = 1),
    CONSTRAINT chk_ceis_schedule_mode CHECK (ceis_schedule_mode IS NULL OR ceis_schedule_mode IN (
        'END_OF_MONTH', 'START_OF_MONTH', 'DAY_OF_MONTH', 'SPECIFIC_DATE', 'CRON')),
    CONSTRAINT chk_ceis_run_day CHECK (ceis_run_day IS NULL OR ceis_run_day BETWEEN 1 AND 31),
    CONSTRAINT chk_ceis_run_time CHECK (ceis_run_time IS NULL OR ceis_run_time ~ '^([01][0-9]|2[0-3]):[0-5][0-9]$'),
    CONSTRAINT chk_ceis_enabled_needs_mode CHECK (NOT ceis_schedule_enabled OR ceis_schedule_mode IS NOT NULL),
    CONSTRAINT chk_ceis_mode_fields CHECK (
        (ceis_schedule_mode IS DISTINCT FROM 'DAY_OF_MONTH' OR ceis_run_day IS NOT NULL)
        AND (ceis_schedule_mode IS DISTINCT FROM 'SPECIFIC_DATE' OR ceis_run_date IS NOT NULL)
        AND (ceis_schedule_mode IS DISTINCT FROM 'CRON' OR NULLIF(TRIM(ceis_cron), '') IS NOT NULL))
);

COMMENT ON TABLE cst_erp_integration_setting IS
    'ERP integration settings, one row (design Part 2 §9.3, U-2). Schedule of the read/compute run only; never push/valuation/approve/restore.';

INSERT INTO cst_erp_integration_setting (ceis_setting_id, ceis_schedule_enabled, created_by)
VALUES (1, FALSE, 'migration:000558')
ON CONFLICT (ceis_setting_id) DO NOTHING;

COMMIT;
