BEGIN;

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN ('finance.bi.deliverymargin.view', 'finance.bi.deliverymargin.export')
);
DELETE FROM mst_permission WHERE permission_code IN ('finance.bi.deliverymargin.view', 'finance.bi.deliverymargin.export');
DELETE FROM mst_menu WHERE menu_code = 'BI_DELIVERY_MARGIN';

COMMIT;
