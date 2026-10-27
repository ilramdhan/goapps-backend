-- H023 (HD-18): division di bi_hr_employee untuk "Kelompokkan per" tabel Employee Overtime.
-- Backfill dari baris staging dengan work_date terbaru per NIK (atribut saat ini, HD-15).
BEGIN;

ALTER TABLE bi_hr_employee ADD COLUMN IF NOT EXISTS division VARCHAR(200);

UPDATE bi_hr_employee e
SET division = s.division
FROM (
    SELECT DISTINCT ON (nik) nik, division
    FROM bi_hr_stg_ot_daily
    ORDER BY nik, work_date DESC, stg_id DESC
) s
WHERE s.nik = e.nik AND e.division IS DISTINCT FROM s.division;

COMMIT;
