-- H032 / HD-08: KPI "% Karyawan Lembur" di dashboard R1 (HR_OT_DEPT) = SUM(HR_OT_EMP_COUNT) / SUM(HR_HEADCOUNT) × 100
-- lintas type (cross_ratio), periode & filter dept mengikuti viewer. HR_OT_EMP_COUNT (MONTHLY, distinct NIK ber-OT
-- per dept) dihitung ETL OT (HD-09), dept memakai aturan yang sama dengan HR_HEADCOUNT. Lintas beberapa periode
-- keduanya orang-bulan; rasio hanya benar bila HR_HEADCOUNT ada untuk semua periode yang punya HR_OT_EMP_COUNT
-- (staging ATT di dev mulai 202609, OT 202608 → lihat TASKS). Down menghapus fakta; up ulang hanya mengisi
-- jendela 35 hari kecuali staging OT di-agregasi ulang.
-- Idempotent; label dipakai sebagai penanda.
BEGIN;

UPDATE bi_dashboard
SET kpi_config = kpi_config || '[{"label":"% Karyawan Lembur","agg":"cross_ratio","cross_ratio_numerator_type":"HR_OT_EMP_COUNT","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":100,"compare":"none","format":"raw","decimals":1}]'::jsonb
WHERE dashboard_code = 'HR_OT_DEPT'
  AND NOT (kpi_config @> '[{"label":"% Karyawan Lembur"}]'::jsonb);

COMMIT;
