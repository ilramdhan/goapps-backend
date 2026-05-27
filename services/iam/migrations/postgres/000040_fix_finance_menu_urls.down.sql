-- Revert menu URL fix and re-enable FINANCE_PARAMETERS duplicate.

UPDATE mst_menu
SET menu_url = '/finance/products',
    updated_at = NOW(),
    updated_by = 'seed'
WHERE menu_code = 'FINANCE_PRODUCTS';

UPDATE mst_menu
SET deleted_at = NULL,
    deleted_by = NULL,
    is_active = TRUE,
    is_visible = TRUE
WHERE menu_code = 'FINANCE_PARAMETERS';
