DELETE FROM bi_fact_metric WHERE periode_grain = 'WEEKLY';
UPDATE bi_dashboard SET periode_grain = 'DAILY' WHERE periode_grain = 'WEEKLY';

ALTER TABLE bi_dashboard DROP CONSTRAINT IF EXISTS bi_dashboard_periode_grain_check;
ALTER TABLE bi_dashboard ADD CONSTRAINT bi_dashboard_periode_grain_check
    CHECK (periode_grain IN ('DAILY','MONTHLY','QUARTERLY','YEARLY'));

ALTER TABLE bi_fact_metric DROP CONSTRAINT IF EXISTS bi_fact_metric_periode_grain_check;
ALTER TABLE bi_fact_metric ADD CONSTRAINT bi_fact_metric_periode_grain_check
    CHECK (periode_grain IN ('DAILY','MONTHLY','QUARTERLY','YEARLY'));
