-- Seed: 4 default dashboard groups (Finance, Sales, Operations, HR).
BEGIN;

INSERT INTO bi_dashboard_group (group_code, group_name, icon, display_order, is_active, created_at)
VALUES
  ('FINANCE',    'Finance',    'Wallet',     10, TRUE, NOW()),
  ('SALES',      'Sales',      'TrendingUp', 20, TRUE, NOW()),
  ('OPERATIONS', 'Operations', 'Factory',    30, TRUE, NOW()),
  ('HR',         'HR',         'Users',      40, TRUE, NOW())
ON CONFLICT (group_code) DO NOTHING;

COMMIT;
