-- Add is_featured and feature_order to bi_dashboard for the Executive Dashboard landing page.
BEGIN;

ALTER TABLE bi_dashboard
  ADD COLUMN IF NOT EXISTS is_featured  BOOLEAN NOT NULL DEFAULT FALSE,
  ADD COLUMN IF NOT EXISTS feature_order INT     NOT NULL DEFAULT 99;

-- Seed: pin all 3 existing production dashboards as featured
UPDATE bi_dashboard
  SET is_featured = TRUE, feature_order = display_order
WHERE dashboard_code IN ('EBITDA', 'NET_PROFIT', 'DELIVERY_MARGIN');

COMMIT;
