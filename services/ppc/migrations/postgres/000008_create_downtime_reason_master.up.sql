-- Downtime reason master. PPC-owned, per-area, configurable + seeded (daily-perf v1.1).
-- drm_is_exclude_from_eff excludes reasons like POWER_FAILURE from efficiency calc.
CREATE TABLE IF NOT EXISTS downtime_reason_master (
    drm_id                  BIGSERIAL     PRIMARY KEY,
    drm_area                CHAR(3)       NOT NULL,   -- TXT / SPG / TWT
    drm_code                VARCHAR(20)   NOT NULL,   -- XST / LB / TP / CPF / POWER_FAILURE / ...
    drm_name                VARCHAR(100)  NOT NULL,
    drm_category            VARCHAR(20)   NOT NULL,   -- IDLE_POSITION / MACHINE_DOWN / PRODUCTION_LOSS
    drm_is_exclude_from_eff BOOLEAN       NOT NULL DEFAULT FALSE,
    drm_is_active           BOOLEAN       NOT NULL DEFAULT TRUE,
    drm_sort_order          INT           NOT NULL DEFAULT 0,
    drm_created_at          TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    drm_created_by          VARCHAR(100)  NOT NULL DEFAULT 'system',
    drm_updated_at          TIMESTAMPTZ,
    drm_updated_by          VARCHAR(100),
    CONSTRAINT chk_drm_area CHECK (drm_area IN ('TXT', 'SPG', 'TWT')),
    CONSTRAINT chk_drm_category CHECK (drm_category IN ('IDLE_POSITION', 'MACHINE_DOWN', 'PRODUCTION_LOSS')),
    CONSTRAINT uq_drm_area_code UNIQUE (drm_area, drm_code)
);

CREATE INDEX IF NOT EXISTS idx_drm_area ON downtime_reason_master (drm_area) WHERE drm_is_active = TRUE;
