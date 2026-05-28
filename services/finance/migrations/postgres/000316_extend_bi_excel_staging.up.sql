-- Migration: extend bi_excel_staging with group orders, scenario, periode_label, display_value.
-- Required by the Excel upload commit path (spec 1C) to carry the full fact-metric shape
-- through staging into bi_fact_metric via a set-based INSERT...SELECT.
BEGIN;

ALTER TABLE bi_excel_staging ADD COLUMN IF NOT EXISTS group_1_order INT;
ALTER TABLE bi_excel_staging ADD COLUMN IF NOT EXISTS group_2_order INT;
ALTER TABLE bi_excel_staging ADD COLUMN IF NOT EXISTS group_3_order INT;
ALTER TABLE bi_excel_staging ADD COLUMN IF NOT EXISTS scenario VARCHAR(20);
ALTER TABLE bi_excel_staging ADD COLUMN IF NOT EXISTS periode_label VARCHAR(20);
ALTER TABLE bi_excel_staging ADD COLUMN IF NOT EXISTS display_value NUMERIC(20,4);

COMMIT;
