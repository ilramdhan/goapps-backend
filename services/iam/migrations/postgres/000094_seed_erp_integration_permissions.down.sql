-- 000094 down: remove ERP integration permissions, menus, roles; restore 000092's 40-value constraint.
DELETE FROM role_permissions WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'finance.cost.erpintegration.%' OR permission_code LIKE 'finance.cost.erprule.%');
DELETE FROM role_permissions WHERE role_id IN (SELECT role_id FROM mst_role WHERE role_code IN ('ERP_COSTING','ERP_FIN_APPROVER'));
DELETE FROM user_roles WHERE role_id IN (SELECT role_id FROM mst_role WHERE role_code IN ('ERP_COSTING','ERP_FIN_APPROVER'));
DELETE FROM menu_permissions WHERE menu_id IN ('00000000-0000-0000-0003-000000000053', '00000000-0000-0000-0003-000000000054', '00000000-0000-0000-0003-000000000055');
DELETE FROM menu_permissions WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code LIKE 'finance.cost.erpintegration.%' OR permission_code LIKE 'finance.cost.erprule.%');
DELETE FROM mst_permission WHERE permission_code LIKE 'finance.cost.erpintegration.%' OR permission_code LIKE 'finance.cost.erprule.%';
DELETE FROM mst_menu WHERE menu_id IN ('00000000-0000-0000-0003-000000000053', '00000000-0000-0000-0003-000000000054', '00000000-0000-0000-0003-000000000055');
DELETE FROM mst_role WHERE role_code IN ('ERP_COSTING','ERP_FIN_APPROVER');

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke','bulkunvalidate','bulksubmit','bulkvalidate'
  ));
