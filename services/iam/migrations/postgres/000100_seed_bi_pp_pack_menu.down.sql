BEGIN;
DELETE FROM menu_permissions WHERE menu_id = (SELECT menu_id FROM mst_menu WHERE menu_code = 'BI_PP_PACK');
DELETE FROM mst_menu WHERE menu_code = 'BI_PP_PACK';
COMMIT;
