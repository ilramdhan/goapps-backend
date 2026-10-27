-- H016 / HD-12: KPI "OT jam per headcount" di dashboard R1 (HR_OT_DEPT) = SUM(HR_OT_HOURS) / SUM(HR_HEADCOUNT)
-- lintas type (cross_ratio, HD-08), periode mengikuti viewer. Idempotent; label dipakai sebagai penanda.
-- Catatan: HR_HEADCOUNT MONTHLY dijumlah lintas periode terpilih (orang-bulan); angka benar untuk satu periode
-- (default THIS_MONTH). "% karyawan lembur" belum: butuh fakta jumlah karyawan ber-OT (lihat TASKS).
BEGIN;

UPDATE bi_dashboard
SET kpi_config = kpi_config || '[{"label":"Jam OT per Headcount","agg":"cross_ratio","cross_ratio_numerator_type":"HR_OT_HOURS","cross_ratio_denominator_type":"HR_HEADCOUNT","scale":1,"compare":"none","format":"raw","decimals":2}]'::jsonb
WHERE dashboard_code = 'HR_OT_DEPT'
  AND NOT (kpi_config @> '[{"label":"Jam OT per Headcount"}]'::jsonb);

COMMIT;
