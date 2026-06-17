-- 000386: Intermingling cost lookup. 19 Oracle rows. Feeds INTERMINGLE_COST CAPP param.
BEGIN;

CREATE TABLE IF NOT EXISTS mst_intermingling (
    intm_id          UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
    intm_code        VARCHAR(20)   NOT NULL,
    intm_name        VARCHAR(100)  NOT NULL,
    intm_cost_per_kg NUMERIC(15,6) NOT NULL DEFAULT 0,
    is_active        BOOLEAN       NOT NULL DEFAULT TRUE,
    notes            TEXT,
    created_at       TIMESTAMPTZ   NOT NULL DEFAULT NOW(),
    created_by       VARCHAR(100)  NOT NULL,
    updated_at       TIMESTAMPTZ,
    updated_by       VARCHAR(100),
    deleted_at       TIMESTAMPTZ,
    deleted_by       VARCHAR(100)
);

CREATE UNIQUE INDEX IF NOT EXISTS uix_mst_intermingling_code
    ON mst_intermingling (intm_code) WHERE deleted_at IS NULL;

-- Seed: 19 Oracle rows from CST_YARN_MST_INTERMINGLING. Values = Oracle_value / 100 (USD/kg).
INSERT INTO mst_intermingling (intm_code, intm_name, intm_cost_per_kg, created_by)
SELECT v.code, v.name, v.cost, 'seed_000386'
FROM (VALUES
    ('HIM',      'HIM',               0.0680),
    ('HTDH_HIM', 'HTDH HIM',          0.0780),
    ('HCDH_HIM', 'HCDH HIM',          0.0780),
    ('HT_HIM',   'HT HIM',            0.0780),
    ('LTY_HIM',  'LTY HIM',           0.0780),
    ('LTH',      'LTH',               0.0780),
    ('HCSH_NIM', 'HCSH NIM',          0.0100),
    ('LIM',      'LIM',               0.0240),
    ('IM',       'IM',                0.0544),
    ('HIMD',     'HIMD',              0.0750),
    ('HT_IM',    'HT IM',             0.0644),
    ('IM_BSY',   'IM BSY',            0.0680),
    ('BSY',      'BSY',               0.0680),
    ('NIM',      'NIM',               0.0000),
    ('SIM',      'SIM',               0.0300),
    ('ANN',      'ANN',               0.0700),
    ('ACD_CA',   'ACD-CA(1100-2500)', 0.1072),
    ('ARD_CA',   'ARD-CA(600-1100)',  0.1013),
    ('AMD_CA',   'AMD-CA(250-600)',   0.0826)
) AS v(code, name, cost)
WHERE NOT EXISTS (SELECT 1 FROM mst_intermingling m WHERE m.intm_code = v.code AND m.deleted_at IS NULL);

COMMIT;
