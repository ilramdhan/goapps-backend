-- Fix FINANCE_PRODUCTS URL (/finance/products → /finance/product-master).
-- Soft-delete duplicate plural FINANCE_PARAMETERS (mock prototype); FINANCE_PARAMETER (singular) remains.

UPDATE mst_menu
SET menu_url = '/finance/product-master',
    updated_at = NOW(),
    updated_by = 'seed'
WHERE menu_code = 'FINANCE_PRODUCTS'
  AND deleted_at IS NULL;

UPDATE mst_menu
SET deleted_at = NOW(),
    deleted_by = 'seed',
    is_active = FALSE,
    is_visible = FALSE
WHERE menu_code = 'FINANCE_PARAMETERS'
  AND deleted_at IS NULL;
