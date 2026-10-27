-- H004 (DOWN): hapus permission & role Workforce.
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code IN ('finance.bi.workforce.view', 'finance.bi.workforceemployee.view'))
   OR role_id IN (SELECT role_id FROM mst_role WHERE role_code IN ('DIREKSI_GM', 'HR') AND description LIKE '[H004]%');
DELETE FROM user_roles WHERE role_id IN (SELECT role_id FROM mst_role WHERE role_code IN ('DIREKSI_GM', 'HR') AND description LIKE '[H004]%');
DELETE FROM mst_role WHERE role_code IN ('DIREKSI_GM', 'HR') AND description LIKE '[H004]%';
DELETE FROM mst_permission WHERE permission_code IN ('finance.bi.workforce.view', 'finance.bi.workforceemployee.view');
COMMIT;
