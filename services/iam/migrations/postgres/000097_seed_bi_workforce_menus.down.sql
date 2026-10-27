-- H060 (DOWN): hapus menu Workforce dan lepas mst_permission.menu_id yang menunjuk ke menu tersebut.
BEGIN;
UPDATE mst_permission
SET menu_id = NULL, updated_by = 'seed', updated_at = NOW()
WHERE menu_id IN (SELECT menu_id FROM mst_menu WHERE menu_code IN ('BI_HR_OT_DEPT', 'BI_HR_OT_OVERLIMIT', 'BI_HR_OT_EMPLOYEES'));
DELETE FROM menu_permissions WHERE menu_id IN (SELECT menu_id FROM mst_menu WHERE menu_code IN ('BI_HR_OT_DEPT', 'BI_HR_OT_OVERLIMIT', 'BI_HR_OT_EMPLOYEES'));
DELETE FROM mst_menu WHERE menu_code IN ('BI_HR_OT_DEPT', 'BI_HR_OT_OVERLIMIT', 'BI_HR_OT_EMPLOYEES');
COMMIT;
