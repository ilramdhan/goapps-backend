-- Migration 000048: Gate RM Pricing and Product Costing parent menus.
--
-- Both parent menus had no menu_permissions entries (intentionally visible to all
-- Finance users). However this caused them to appear in the sidebar for any user
-- with finance.module.root.view (e.g. "View Only BI" role) even though they have
-- no access to any children.
--
-- Fix: add permission requirements to the parent menus so they only appear when
-- the user has at least one relevant child permission.
--
-- RM Pricing parent  (00000000-0000-0000-0002-000000000014) → any finance.rmpricing.* view
-- Product Costing parent (00000000-0000-0000-0002-000000000015) → any finance.product.* view

BEGIN;

-- RM Pricing parent — show only when user has any RM Pricing view permission.
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000014', p.permission_id, 'seed'
FROM mst_permission p
WHERE p.permission_code IN (
    'finance.rmpricing.grouphead.view',
    'finance.rmpricing.groupdetail.view',
    'finance.rmpricing.cost.view',
    'finance.rmpricing.ungrouped.view'
)
  AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Product Costing parent — show only when user has any Product Costing view permission.
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0002-000000000015', p.permission_id, 'seed'
FROM mst_permission p
WHERE p.permission_code IN (
    'finance.product.request.view',
    'finance.product.master.view',
    'finance.product.route.view',
    'finance.product.order.view',
    'finance.transaction.cstproduct.view'
)
  AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

COMMIT;
