-- Rollback Phase C (Calc Engine) S8a.8 sidebar menus seed.

BEGIN;

DELETE FROM menu_permissions
WHERE menu_id IN (
    SELECT menu_id FROM mst_menu
    WHERE menu_code IN ('FINANCE_CALC_JOBS', 'FINANCE_COST_RESULTS')
);

DELETE FROM mst_menu
WHERE menu_code IN ('FINANCE_CALC_JOBS', 'FINANCE_COST_RESULTS');

COMMIT;
