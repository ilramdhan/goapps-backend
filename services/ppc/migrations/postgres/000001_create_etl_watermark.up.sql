-- ETL watermark tracks the last-processed timestamp per Oracle source table so
-- incremental pulls (LAST_UPDATED > watermark) resume where they left off.
CREATE TABLE IF NOT EXISTS etl_watermark (
    ewm_table_name VARCHAR(50)  PRIMARY KEY,
    ewm_last_run   TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    ewm_updated_at TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);
