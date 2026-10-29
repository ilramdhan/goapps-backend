-- H062b (DOWN): hapus permission status ETL HR beserta grant-nya.
BEGIN;
DELETE FROM role_permissions
WHERE permission_id IN (SELECT permission_id FROM mst_permission WHERE permission_code = 'finance.bi.etlstatus.view');
DELETE FROM mst_permission WHERE permission_code = 'finance.bi.etlstatus.view';
COMMIT;
