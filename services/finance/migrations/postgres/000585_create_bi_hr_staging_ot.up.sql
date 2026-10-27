-- H010 / HD-03: staging OT harian (cermin VIEW_BI_OT_DAILY) + master karyawan kecil untuk label drill (HD-07).
-- Staging tanpa nama karyawan. Jam = NUMERIC (tanpa float). Histori tidak dipangkas (HD-04).
BEGIN;

CREATE TABLE IF NOT EXISTS bi_hr_employee (
    nik        VARCHAR(30)  PRIMARY KEY,
    emp_sys_id VARCHAR(40)  NOT NULL,
    emp_name   VARCHAR(200) NOT NULL,
    dept_id    VARCHAR(40),
    dept_name  VARCHAR(200),
    updated_at TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS bi_hr_stg_ot_daily (
    stg_id          BIGSERIAL PRIMARY KEY,
    work_date       DATE          NOT NULL,
    week_start      DATE          NOT NULL,
    att_period      CHAR(6)       NOT NULL,
    week_att_period CHAR(6)       NOT NULL,
    emp_sys_id      VARCHAR(40)   NOT NULL,
    nik             VARCHAR(30)   NOT NULL,
    division        VARCHAR(200),
    dept_id         VARCHAR(40),
    dept_name       VARCHAR(200),
    schedule_code   VARCHAR(40),
    ot_type         VARCHAR(40)   NOT NULL,
    ot_tier_1       NUMERIC(12,6) NOT NULL DEFAULT 0,
    ot_tier_2       NUMERIC(12,6) NOT NULL DEFAULT 0,
    ot_tier_3       NUMERIC(12,6) NOT NULL DEFAULT 0,
    ot_actual_hours NUMERIC(12,6) NOT NULL DEFAULT 0,
    ot_calc_hours   NUMERIC(12,6) NOT NULL DEFAULT 0,
    ot_limit_hours  NUMERIC(12,6) NOT NULL DEFAULT 0,
    loaded_at       TIMESTAMP     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bi_hr_ot_date   ON bi_hr_stg_ot_daily (work_date);
CREATE INDEX IF NOT EXISTS idx_bi_hr_ot_week   ON bi_hr_stg_ot_daily (week_start, emp_sys_id);
CREATE INDEX IF NOT EXISTS idx_bi_hr_ot_period ON bi_hr_stg_ot_daily (att_period, dept_id);

COMMIT;
