-- Revert migration 000349: Remove landing_sections from featured dashboards.
BEGIN;

UPDATE bi_dashboard
SET layout_config = layout_config - 'landing_sections'
WHERE dashboard_code IN ('EBITDA', 'NET_PROFIT', 'DELIVERY_MARGIN');

COMMIT;
