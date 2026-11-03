-- P002 / ETL§B: staging Packed Production harian (view ASPAK.VIEW_BI_PACK_PROD_DAILY).
-- Idempotent.
BEGIN;

CREATE TABLE IF NOT EXISTS bi_pp_stg_pack_prod_daily (
    stg_id               BIGSERIAL      PRIMARY KEY,
    pack_date            DATE           NOT NULL,
    week_start           DATE           NOT NULL,
    month_start          DATE           NOT NULL,
    period               CHAR(6)        NOT NULL,
    week_period          CHAR(6)        NOT NULL,
    dept_id              VARCHAR(3)     NOT NULL,
    dept_name            VARCHAR(20)    NOT NULL,
    prod_group           VARCHAR(20)    NOT NULL,
    pallet_code          VARCHAR(1),
    prd_type_name        VARCHAR(40),
    prd_type_order       INT,
    grp_rep_name         VARCHAR(40),
    transaction_type     VARCHAR(120),
    transaction_type_mc  VARCHAR(120),
    group_type           VARCHAR(60),
    group_type_order     INT,
    merge_reporting      VARCHAR(10),
    poly_type            VARCHAR(20),
    mc_no                VARCHAR(10),
    mach_group           VARCHAR(40),
    mach_order_rep       INT,
    merge_no             VARCHAR(10),
    prod_descr           VARCHAR(200),
    shade                VARCHAR(80),
    poy_line             VARCHAR(20),
    grade_eligible       CHAR(1)        NOT NULL,
    ax_qty               NUMERIC(12,6)  NOT NULL DEFAULT 0,
    fq_qty               NUMERIC(12,6)  NOT NULL DEFAULT 0,
    dg_qty               NUMERIC(12,6)  NOT NULL DEFAULT 0,
    jlt_qty              NUMERIC(12,6)  NOT NULL DEFAULT 0,
    total_qty            NUMERIC(12,6)  NOT NULL DEFAULT 0,
    loaded_at            TIMESTAMP      NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bi_pp_stg_pack_prod_daily_date  ON bi_pp_stg_pack_prod_daily (pack_date);
CREATE INDEX IF NOT EXISTS idx_bi_pp_stg_pack_prod_daily_week  ON bi_pp_stg_pack_prod_daily (week_start);
CREATE INDEX IF NOT EXISTS idx_bi_pp_stg_pack_prod_daily_month ON bi_pp_stg_pack_prod_daily (month_start);

COMMIT;
