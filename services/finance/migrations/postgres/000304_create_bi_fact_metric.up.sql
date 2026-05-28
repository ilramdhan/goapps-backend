-- Migration: create bi_fact_metric — single long-format fact table for all BI dashboards.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_fact_metric (
    metric_id      BIGSERIAL PRIMARY KEY,
    type           VARCHAR(40)  NOT NULL,
    group_1        VARCHAR(120) NOT NULL,
    group_2        VARCHAR(120),
    group_3        VARCHAR(120),
    group_1_order  INT,
    group_2_order  INT,
    group_3_order  INT,
    periode_grain  VARCHAR(10)  NOT NULL CHECK (periode_grain IN ('DAILY','MONTHLY','QUARTERLY','YEARLY')),
    periode_date   DATE         NOT NULL,
    periode_label  VARCHAR(20)  NOT NULL,
    value          NUMERIC(20,4) NOT NULL,
    display_value  NUMERIC(20,4) NOT NULL,
    uom            VARCHAR(20),
    scenario       VARCHAR(20)  NOT NULL DEFAULT 'ACTUAL',
    source_id      UUID NOT NULL REFERENCES bi_data_source(source_id),
    dimension_key  VARCHAR(200) NOT NULL DEFAULT '',
    uploaded_by    UUID,
    loaded_at      TIMESTAMP    NOT NULL DEFAULT NOW(),
    is_active      BOOLEAN      NOT NULL DEFAULT TRUE,
    -- NULLS NOT DISTINCT (Postgres 15+) is REQUIRED: group_2/group_3 are nullable, and
    -- without it SQL treats every NULL as distinct, so ON CONFLICT upsert would silently
    -- INSERT duplicate fact rows on every re-ingest (ETL/Excel) for any row with a NULL
    -- group level. With NULLS NOT DISTINCT, two rows that differ only by NULL==NULL collide
    -- and the UPSERT updates in place.
    CONSTRAINT uq_bi_fm_business_key UNIQUE NULLS NOT DISTINCT
      (type, group_1, group_2, group_3, periode_grain, periode_date, scenario, dimension_key)
);

CREATE INDEX IF NOT EXISTS idx_bi_fm_lookup     ON bi_fact_metric (type, group_1, periode_grain, periode_date) WHERE is_active;
CREATE INDEX IF NOT EXISTS idx_bi_fm_date_grain ON bi_fact_metric (periode_grain, periode_date)                WHERE is_active;
CREATE INDEX IF NOT EXISTS idx_bi_fm_g2         ON bi_fact_metric (type, group_1, group_2, periode_date)        WHERE is_active;
CREATE INDEX IF NOT EXISTS idx_bi_fm_source     ON bi_fact_metric (source_id, loaded_at);

COMMIT;
