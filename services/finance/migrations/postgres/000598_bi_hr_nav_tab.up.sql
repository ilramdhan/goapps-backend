-- H065: tab navigasi Workforce dideklarasikan di layout_config.nav_tab {label, permission}
-- (urutan = display_order). FE (workforce-nav) membaca ini, bukan konstanta. Idempotent.
BEGIN;
UPDATE bi_dashboard d
SET layout_config = COALESCE(d.layout_config, '{}'::jsonb) || jsonb_build_object(
      'nav_tab', jsonb_build_object('label', v.label, 'permission', v.permission))
FROM (VALUES
  ('HR_OT_DEPT',      'Overtime per Dept',  'finance.bi.workforce.view'),
  ('HR_OT_TREND',     'Tren OT',            'finance.bi.workforce.view'),
  ('HR_OT_OVERLIMIT', 'Kepatuhan 14 Jam',   'finance.bi.workforce.view'),
  ('HR_OT_EMPLOYEES', 'Employee Overtime',  'finance.bi.workforceemployee.view')
) AS v(code, label, permission)
WHERE d.dashboard_code = v.code;
COMMIT;
