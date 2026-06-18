-- 000394: Master lookup registry tables.
-- mst_lookup_master: registry of available master tables for MASTER_LOOKUP params.
-- mst_lookup_master_column: per-master list of fillable columns (drives lookup_source_column dropdown).

BEGIN;

CREATE TABLE IF NOT EXISTS mst_lookup_master (
    lm_code         VARCHAR(30)  PRIMARY KEY,
    lm_display_name VARCHAR(100) NOT NULL,
    lm_api_path     VARCHAR(200) NOT NULL,
    lm_code_field   VARCHAR(50)  NOT NULL,
    lm_label_field  VARCHAR(50)  NOT NULL,
    lm_is_active    BOOLEAN      NOT NULL DEFAULT TRUE,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    created_by      VARCHAR(100) NOT NULL
);

CREATE TABLE IF NOT EXISTS mst_lookup_master_column (
    lmc_id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
    lmc_master_code  VARCHAR(30)  NOT NULL REFERENCES mst_lookup_master(lm_code),
    lmc_column_name  VARCHAR(50)  NOT NULL,
    lmc_display_name VARCHAR(100) NOT NULL,
    lmc_data_type    VARCHAR(10)  NOT NULL CHECK (lmc_data_type IN ('NUMBER','TEXT')),
    lmc_sort_order   INTEGER      NOT NULL DEFAULT 0,
    UNIQUE (lmc_master_code, lmc_column_name)
);

-- Seed: 5 existing masters
INSERT INTO mst_lookup_master (lm_code, lm_display_name, lm_api_path, lm_code_field, lm_label_field, created_by) VALUES
    ('MACHINE',         'Machine Master',     '/api/v1/finance/machines',          'machineCode',       'machineName',       'seed_000394'),
    ('INTERMINGLING',   'Intermingling',      '/api/v1/finance/interminglings',    'interminglingCode', 'interminglingName', 'seed_000394'),
    ('PRODUCT_GRADE',   'Product Grade',      '/api/v1/finance/product-grades',    'pgCode',            'pgName',            'seed_000394'),
    ('MB_HEAD',         'MB Head (Melange)',  '/api/v1/finance/mb-heads',          'mbhMbCosting',      'mbhMgtName',        'seed_000394'),
    ('BOX_BOBBIN_COST', 'Box/Bobbin Cost',    '/api/v1/finance/box-bobbin-costs',  'bbcCode',           'bbcName',           'seed_000394')
ON CONFLICT (lm_code) DO NOTHING;

-- Seed: columns per master
INSERT INTO mst_lookup_master_column (lmc_master_code, lmc_column_name, lmc_display_name, lmc_data_type, lmc_sort_order) VALUES
    ('MACHINE', 'mc_speed',        'Machine Speed (m/min)',             'NUMBER', 10),
    ('MACHINE', 'mc_efficiency',   'Machine Efficiency (%)',            'NUMBER', 20),
    ('MACHINE', 'no_of_position',  'Number of Positions',               'NUMBER', 30),
    ('MACHINE', 'no_of_end',       'Number of Ends',                    'NUMBER', 40),
    ('MACHINE', 'machine_rpm',     'Machine RPM (optional)',            'NUMBER', 50),
    ('MACHINE', 'power_per_day',   'Power Cost per Day (USD, optional)','NUMBER', 60),
    ('INTERMINGLING', 'intm_cost_per_kg', 'Intermingling Cost (USD/kg)','NUMBER', 10),
    ('PRODUCT_GRADE', 'bc_perc',          'BC Grade (%)',                'NUMBER', 10),
    ('PRODUCT_GRADE', 'non_std_perc',     'Non-Standard (%)',            'NUMBER', 20),
    ('PRODUCT_GRADE', 'bc_recovery_rate', 'BC Recovery Rate (%)',        'NUMBER', 30),
    ('MB_HEAD', 'mbh_dozing',   'MB Dozing (%, optional)',             'NUMBER', 10),
    ('MB_HEAD', 'mbh_mgt_name', 'MB Management Name (optional)',       'TEXT',   20),
    ('BOX_BOBBIN_COST', 'no_of_bob',          'No. of Bobbins per Box',      'NUMBER', 10),
    ('BOX_BOBBIN_COST', 'bbcr_bob_rate_mkt',  'Bobbin Rate MKT (USD/bob)',   'NUMBER', 20),
    ('BOX_BOBBIN_COST', 'bbcr_box_rate_mkt',  'Box Rate MKT (USD/box)',      'NUMBER', 30)
ON CONFLICT (lmc_master_code, lmc_column_name) DO NOTHING;

COMMIT;
