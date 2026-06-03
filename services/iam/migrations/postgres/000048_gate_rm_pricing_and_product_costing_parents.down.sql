-- Revert 000048: remove menu_permissions entries added for parent menus.
BEGIN;

DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0002-000000000014',  -- FINANCE_RM_PRICING
    '00000000-0000-0000-0002-000000000015'   -- FINANCE_PRODUCT_COSTING
)
AND assigned_by = 'seed';

COMMIT;
