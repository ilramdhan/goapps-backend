-- Migration: create bulk-import v2 ETL staging tables.
-- All staging tables are UNLOGGED (no WAL — fast, transient; rows are scoped by
-- job_id and deleted at end of job). All data columns are TEXT (raw values;
-- casting/validation happens in the set-based resolve SQL). Each table carries
-- job_id BIGINT + row_num INT for precise error reporting. See design §4.
BEGIN;

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_product_master (
    job_id                  BIGINT NOT NULL,
    row_num                 INT    NOT NULL,
    legacy_oracle_sys_id    TEXT,
    product_type_code       TEXT,
    product_name            TEXT,
    shade_code              TEXT,
    shade_name              TEXT,
    grade_code              TEXT,
    description             TEXT,
    erp_item_code           TEXT,
    legacy_erp_compound_key TEXT,
    legacy_type_label       TEXT,
    is_active               TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_product_master_job
    ON stg_import_product_master (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_product_master_job_key
    ON stg_import_product_master (job_id, legacy_oracle_sys_id);

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_product_parameter (
    job_id               BIGINT NOT NULL,
    row_num              INT    NOT NULL,
    legacy_oracle_sys_id TEXT,
    param_code           TEXT,
    data_type            TEXT,
    value_numeric        TEXT,
    value_text           TEXT,
    value_flag           TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_product_parameter_job
    ON stg_import_product_parameter (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_product_parameter_job_key
    ON stg_import_product_parameter (job_id, legacy_oracle_sys_id);

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_applicable_param (
    job_id               BIGINT NOT NULL,
    row_num              INT    NOT NULL,
    legacy_oracle_sys_id TEXT,
    param_code           TEXT,
    is_required          TEXT,
    display_order        TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_applicable_param_job
    ON stg_import_applicable_param (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_applicable_param_job_key
    ON stg_import_applicable_param (job_id, legacy_oracle_sys_id);

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_route_head (
    job_id               BIGINT NOT NULL,
    row_num              INT    NOT NULL,
    legacy_oracle_sys_id TEXT,
    routing_status       TEXT,
    notes                TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_route_head_job
    ON stg_import_route_head (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_route_head_job_key
    ON stg_import_route_head (job_id, legacy_oracle_sys_id);

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_route_seq (
    job_id                      BIGINT NOT NULL,
    row_num                     INT    NOT NULL,
    route_head_legacy_product_id TEXT,
    node_product_legacy_id      TEXT,
    route_level                 TEXT,
    route_seq                   TEXT,
    route_name                  TEXT,
    route_item_code             TEXT,
    route_shade_code            TEXT,
    route_shade_name            TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_route_seq_job
    ON stg_import_route_seq (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_route_seq_job_key
    ON stg_import_route_seq (job_id, route_head_legacy_product_id);

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_route_rm (
    job_id                      BIGINT NOT NULL,
    row_num                     INT    NOT NULL,
    route_head_legacy_product_id TEXT,
    route_level                 TEXT,
    route_seq                   TEXT,
    rm_type                     TEXT,
    ratio                       TEXT,
    rm_product_legacy_id        TEXT,
    rm_item_code                TEXT,
    rm_group_code               TEXT,
    rm_name                     TEXT,
    rm_shade_code               TEXT,
    rm_shade_name               TEXT,
    sub_type                    TEXT,
    notes                       TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_route_rm_job
    ON stg_import_route_rm (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_route_rm_job_key
    ON stg_import_route_rm (job_id, route_head_legacy_product_id);

CREATE UNLOGGED TABLE IF NOT EXISTS stg_import_error (
    job_id        BIGINT NOT NULL,
    sheet         TEXT,
    row_num       INT,
    key_info      TEXT,
    error_message TEXT
);

CREATE INDEX IF NOT EXISTS idx_stg_import_error_job
    ON stg_import_error (job_id);
CREATE INDEX IF NOT EXISTS idx_stg_import_error_job_sheet
    ON stg_import_error (job_id, sheet);

COMMIT;
