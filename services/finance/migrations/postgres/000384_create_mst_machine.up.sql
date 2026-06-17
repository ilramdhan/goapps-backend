-- 000384: Machine master table. Feeds NO_OF_POSITION, NO_OF_END, MC_SPEED, MC_EFFICIENCY CAPP params.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_machine (
    mc_id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    mc_code        VARCHAR(30)   NOT NULL,
    mc_name        VARCHAR(100)  NOT NULL,
    mc_type        VARCHAR(30),
    mc_location    VARCHAR(100),
    no_of_position INTEGER       NOT NULL DEFAULT 0,
    no_of_end      INTEGER       NOT NULL DEFAULT 1,
    mc_speed       NUMERIC(10,2) NOT NULL DEFAULT 0,
    machine_rpm    NUMERIC(10,2),
    mc_efficiency  NUMERIC(5,2)  NOT NULL DEFAULT 95,
    power_per_day  NUMERIC(15,4),
    is_active      BOOLEAN       NOT NULL DEFAULT TRUE,
    notes          TEXT,
    created_at     TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by     VARCHAR(100)  NOT NULL,
    updated_at     TIMESTAMPTZ,
    updated_by     VARCHAR(100),
    deleted_at     TIMESTAMPTZ,
    deleted_by     VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_machine_code
    ON mst_machine (mc_code) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_mst_machine_type
    ON mst_machine (mc_type) WHERE deleted_at IS NULL;

INSERT INTO mst_machine (mc_code, mc_name, mc_type, no_of_position, no_of_end, mc_speed, mc_efficiency, created_by)
SELECT v.mc_code, v.mc_name, v.mc_type, v.positions, v.ends, v.speed, v.efficiency, 'seed_000384'
FROM (VALUES
    ('BT-D',  'Barmag DTY Line D', 'DTY', 504, 1, 800,  92.0),
    ('BT-E',  'Barmag DTY Line E', 'DTY', 504, 1, 800,  91.5),
    ('TMT-A', 'TMT POY Machine A', 'POY', 288, 1, 4500, 94.0),
    ('TMT-B', 'TMT POY Machine B', 'POY', 288, 1, 4500, 93.5)
) AS v(mc_code, mc_name, mc_type, positions, ends, speed, efficiency)
WHERE NOT EXISTS (SELECT 1 FROM mst_machine m WHERE m.mc_code = v.mc_code AND m.deleted_at IS NULL);

COMMIT;
