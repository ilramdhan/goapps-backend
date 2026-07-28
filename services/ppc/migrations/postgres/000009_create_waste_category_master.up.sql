-- Waste category master. PPC-owned, per-area, configurable + seeded (daily-perf v1.1).
-- Type WASTE or DOWNGRADE; DOWNGRADE rows carry a target grade (B/C).
CREATE TABLE IF NOT EXISTS waste_category_master (
    wcm_id           BIGSERIAL     PRIMARY KEY,
    wcm_area         CHAR(3)       NOT NULL,   -- TXT / SPG / TWT
    wcm_type         VARCHAR(15)   NOT NULL,   -- WASTE / DOWNGRADE
    wcm_code         VARCHAR(30)   NOT NULL,   -- SPINNING / TAKE_UP / DTY / POY / FLYING_FILAMENT / ...
    wcm_name         VARCHAR(100)  NOT NULL,
    wcm_grade_target VARCHAR(5),               -- DOWNGRADE: B / C
    wcm_is_active    BOOLEAN       NOT NULL DEFAULT TRUE,
    wcm_sort_order   INT           NOT NULL DEFAULT 0,
    wcm_created_at   TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    wcm_created_by   VARCHAR(100)  NOT NULL DEFAULT 'system',
    wcm_updated_at   TIMESTAMPTZ,
    wcm_updated_by   VARCHAR(100),
    CONSTRAINT chk_wcm_area CHECK (wcm_area IN ('TXT', 'SPG', 'TWT')),
    CONSTRAINT chk_wcm_type CHECK (wcm_type IN ('WASTE', 'DOWNGRADE')),
    CONSTRAINT uq_wcm_area_type_code UNIQUE (wcm_area, wcm_type, wcm_code)
);

CREATE INDEX IF NOT EXISTS idx_wcm_area ON waste_category_master (wcm_area) WHERE wcm_is_active = TRUE;
