-- Efficiency snapshot (daily-perf v1.1). Computed per scope
-- (MACHINE_SHIFT -> MACHINE_DAY -> AREA_DAY), Including/Excluding variants. MTD is a
-- re-aggregation of components, not an average of percentages.
CREATE TABLE IF NOT EXISTS efficiency_snapshot (
    es_id                  BIGSERIAL     PRIMARY KEY,
    es_area                CHAR(3)       NOT NULL,   -- TXT / SPG / TWT
    es_scope               VARCHAR(20)   NOT NULL,   -- MACHINE_SHIFT / MACHINE_DAY / AREA_DAY
    es_machine_id          BIGINT,                   -- NULL for AREA_DAY
    es_wo_id               BIGINT,                   -- NULL for aggregate
    es_date                DATE          NOT NULL,
    es_shift               CHAR(1),                  -- NULL for _DAY
    es_segment             VARCHAR(10),              -- DTY / ACY / ATY / POY / NULL
    es_is_excluding        BOOLEAN       NOT NULL DEFAULT FALSE,
    es_qty_theoretical_100 DECIMAL(14,3),
    es_qty_theoretical_rng DECIMAL(14,3),
    es_qty_loss            DECIMAL(14,3),
    es_qty_waste           DECIMAL(14,3),
    es_qty_actual          DECIMAL(14,3),            -- SPG: doffed
    es_eff_production_pct  DECIMAL(6,2),
    es_eff_running_pct     DECIMAL(6,2),
    es_eff_plant_pct       DECIMAL(6,2),             -- SPG
    es_yield_pct           DECIMAL(6,2),             -- SPG
    es_waste_pct           DECIMAL(6,2),
    es_breaks_count        INT,
    es_breaks_per_ton      DECIMAL(8,2),
    es_calc_at             TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_es_area CHECK (es_area IN ('TXT', 'SPG', 'TWT')),
    CONSTRAINT chk_es_scope CHECK (es_scope IN ('MACHINE_SHIFT', 'MACHINE_DAY', 'AREA_DAY')),
    CONSTRAINT uq_es_scope UNIQUE (es_area, es_scope, es_machine_id, es_wo_id, es_date, es_shift, es_segment, es_is_excluding)
);

CREATE INDEX IF NOT EXISTS idx_es_area_date ON efficiency_snapshot (es_area, es_date, es_scope);
