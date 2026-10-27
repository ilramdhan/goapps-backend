-- H016 / HD-12: staging absensi harian (cermin VIEW_BI_ATT_DAILY, kolom lengkap; dipakai lagi R3/R4 fase 2)
-- + seed job ETL_HR_ATT_DAILY. Fase 1 hanya mengagregasi HR_HEADCOUNT. Tanpa nama karyawan (HD-07).
-- Jam/menit NUMERIC (tanpa float). Histori tidak dipangkas (HD-04).
BEGIN;

CREATE TABLE IF NOT EXISTS bi_hr_stg_att_daily (
    stg_id             BIGSERIAL PRIMARY KEY,
    work_date          DATE          NOT NULL,
    week_start         DATE          NOT NULL,
    att_period         CHAR(6)       NOT NULL,
    emp_sys_id         VARCHAR(40)   NOT NULL,
    nik                VARCHAR(30)   NOT NULL,
    division           VARCHAR(200),
    dept_id            VARCHAR(40),
    dept_name          VARCHAR(200),
    emp_status         VARCHAR(100),
    emp_group          VARCHAR(100),
    schedule_code      VARCHAR(40),
    sched_status       VARCHAR(40),
    sched_type         VARCHAR(40),
    sched_in           TIMESTAMP,
    sched_out          TIMESTAMP,
    actual_in          TIMESTAMP,
    actual_out         TIMESTAMP,
    actual_hour        NUMERIC(12,6),
    day_cat_rekap      VARCHAR(40),
    day_cat            VARCHAR(40),
    is_scheduled_work  SMALLINT      NOT NULL DEFAULT 0,
    late_minutes       NUMERIC(12,2),
    early_leave_minutes NUMERIC(12,2),
    loaded_at          TIMESTAMP     NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_bi_hr_att_date   ON bi_hr_stg_att_daily (work_date);
CREATE INDEX IF NOT EXISTS idx_bi_hr_att_period ON bi_hr_stg_att_daily (att_period, dept_id);
CREATE INDEX IF NOT EXISTS idx_bi_hr_att_nik    ON bi_hr_stg_att_daily (nik, work_date);

WITH src AS (SELECT source_id FROM bi_data_source WHERE source_code = 'HRIS_ORACLE')
INSERT INTO bi_job (job_name, source_id, target_type, schedule_cron, oracle_procedure, config, is_active)
SELECT 'ETL_HR_ATT_DAILY', src.source_id, 'HR_ATT_STAGING', '10 5 * * *', NULL,
       '{"kind": "etl_hr_att_staging", "source_view": "MGTHRIS.VIEW_BI_ATT_DAILY", "target_type": "HR_ATT_STAGING", "source_code": "HRIS_ORACLE"}'::jsonb,
       TRUE
FROM src
ON CONFLICT (job_name) DO NOTHING;

COMMIT;
