-- IAM Service Database Migrations
-- 000026: Seed RM Grouping (RM Pricing) menu entries and permissions.
--
-- Adds "RM Pricing" parent menu under Finance, with three children:
--   - RM Groups     → /finance/rm-pricing/groups
--   - RM Costs      → /finance/rm-pricing/costs
--   - Ungrouped Items → /finance/rm-pricing/ungrouped
--
-- Seeds 11 granular permissions (grouphead/groupdetail/cost/ungrouped split).
-- Extends chk_permission_action to include 'recalculate' for the manual recalc permission.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- EXTEND chk_permission_action — allow 'recalculate' action
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate'
    ));

-- =============================================================================
-- PERMISSIONS — finance.rmpricing.{grouphead|groupdetail|cost|ungrouped}.*
-- Permission code format constraint: ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- (no underscores or hyphens in segments; multi-word entities concatenated: grouphead, groupdetail, rmpricing)
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    -- Group header CRUD
    ('finance.rmpricing.grouphead.view',    'View RM Group Headers',   'List and view RM group headers',               'finance', 'rmpricing', 'view',   true, 'seed'),
    ('finance.rmpricing.grouphead.create',  'Create RM Group Header',  'Create new RM group headers',                  'finance', 'rmpricing', 'create', true, 'seed'),
    ('finance.rmpricing.grouphead.update',  'Update RM Group Header',  'Update RM group header fields (flags, costs)', 'finance', 'rmpricing', 'update', true, 'seed'),
    ('finance.rmpricing.grouphead.delete',  'Delete RM Group Header',  'Soft-delete RM group headers',                 'finance', 'rmpricing', 'delete', true, 'seed'),

    -- Group detail (items in group) CRUD
    ('finance.rmpricing.groupdetail.view',   'View RM Group Items',     'View items assigned to RM groups',             'finance', 'rmpricing', 'view',   true, 'seed'),
    ('finance.rmpricing.groupdetail.create', 'Add Items to RM Group',   'Add items to RM groups',                       'finance', 'rmpricing', 'create', true, 'seed'),
    ('finance.rmpricing.groupdetail.update', 'Update RM Group Items',   'Update RM group item fields (activate, etc.)', 'finance', 'rmpricing', 'update', true, 'seed'),
    ('finance.rmpricing.groupdetail.delete', 'Remove Items from Group', 'Remove items from RM groups',                  'finance', 'rmpricing', 'delete', true, 'seed'),

    -- Cost view + manual recalc
    ('finance.rmpricing.cost.view',        'View RM Costs',          'View calculated RM landed costs per period',   'finance', 'rmpricing', 'view',        true, 'seed'),
    ('finance.rmpricing.cost.recalculate', 'Recalculate RM Costs',   'Manually trigger RM cost recalculation',       'finance', 'rmpricing', 'recalculate', true, 'seed'),

    -- Ungrouped items report
    ('finance.rmpricing.ungrouped.view',   'View Ungrouped Items',   'View RMs not yet assigned to any group',       'finance', 'rmpricing', 'view',        true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENUS — Finance > RM Pricing > (Groups | Costs | Ungrouped)
--
-- Parent:   00000000-0000-0000-0002-000000000014 FINANCE_RM_PRICING  (LEVEL 2, next available)
-- Children: 00000000-0000-0000-0003-000000000012 FINANCE_RM_GROUPS
--           00000000-0000-0000-0003-000000000013 FINANCE_RM_COSTS
--           00000000-0000-0000-0003-000000000014 FINANCE_UNGROUPED_RM
-- =============================================================================

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    -- Parent (section header under Finance)
    ('00000000-0000-0000-0002-000000000014', '00000000-0000-0000-0001-000000000002', 'FINANCE_RM_PRICING',
     'RM Pricing', NULL, 'Layers', 'finance', 2, 40, true, true, 'seed'),

    -- Children
    ('00000000-0000-0000-0003-000000000012', '00000000-0000-0000-0002-000000000014', 'FINANCE_RM_GROUPS',
     'RM Groups', '/finance/rm-pricing/groups', 'FolderTree', 'finance', 3, 10, true, true, 'seed'),

    ('00000000-0000-0000-0003-000000000013', '00000000-0000-0000-0002-000000000014', 'FINANCE_RM_COSTS',
     'RM Costs', '/finance/rm-pricing/costs', 'Calculator', 'finance', 3, 20, true, true, 'seed'),

    ('00000000-0000-0000-0003-000000000014', '00000000-0000-0000-0002-000000000014', 'FINANCE_UNGROUPED_RM',
     'Ungrouped Items', '/finance/rm-pricing/ungrouped', 'AlertCircle', 'finance', 3, 30, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- MENU PERMISSIONS — Link menus to their view permissions
-- =============================================================================

-- RM Groups menu — visible to users with either grouphead.view OR groupdetail.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000012', permission_id, 'seed'
FROM mst_permission
WHERE permission_code IN ('finance.rmpricing.grouphead.view', 'finance.rmpricing.groupdetail.view')
  AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- RM Costs menu — visible to users with cost.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000013', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.rmpricing.cost.view'
  AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- Ungrouped Items menu — visible to users with ungrouped.view
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000014', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.rmpricing.ungrouped.view'
  AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN ALL RM PRICING PERMISSIONS TO SUPER_ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code LIKE 'finance.rmpricing.%'
  AND r.is_active = true
  AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
