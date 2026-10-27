-- H003 / HD-06: kalender periode per dashboard. CALENDAR = bulan kalender (default,
-- perilaku lama), PAYROLL_26_25 = periode payroll 26 bulan lalu s.d. 25 (dashboard HR).
ALTER TABLE bi_dashboard
    ADD COLUMN IF NOT EXISTS period_calendar VARCHAR(20) NOT NULL DEFAULT 'CALENDAR';

ALTER TABLE bi_dashboard DROP CONSTRAINT IF EXISTS bi_dashboard_period_calendar_check;
ALTER TABLE bi_dashboard ADD CONSTRAINT bi_dashboard_period_calendar_check
    CHECK (period_calendar IN ('CALENDAR','PAYROLL_26_25'));
