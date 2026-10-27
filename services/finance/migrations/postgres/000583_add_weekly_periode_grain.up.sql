-- HD-05: grain WEEKLY (periode_date = Senin, label IYYY-"W"IW).
-- Satu berkas = satu Exec = satu transaksi implisit, jadi lock ACCESS EXCLUSIVE dari DROP/ADD tetap
-- dipegang selama VALIDATE men-scan bi_fact_metric. NOT VALID + VALIDATE hanya memisahkan pemasangan
-- dari scan; kalau tabel prod sangat besar, jalankan di jam sepi. Grain lama tetap lolos CHECK baru.
ALTER TABLE bi_dashboard DROP CONSTRAINT IF EXISTS bi_dashboard_periode_grain_check;
ALTER TABLE bi_dashboard ADD CONSTRAINT bi_dashboard_periode_grain_check
    CHECK (periode_grain IN ('DAILY','WEEKLY','MONTHLY','QUARTERLY','YEARLY'));

ALTER TABLE bi_fact_metric DROP CONSTRAINT IF EXISTS bi_fact_metric_periode_grain_check;
ALTER TABLE bi_fact_metric ADD CONSTRAINT bi_fact_metric_periode_grain_check
    CHECK (periode_grain IN ('DAILY','WEEKLY','MONTHLY','QUARTERLY','YEARLY')) NOT VALID;
ALTER TABLE bi_fact_metric VALIDATE CONSTRAINT bi_fact_metric_periode_grain_check;
