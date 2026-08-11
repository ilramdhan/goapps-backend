-- IAM Service Database Migrations
-- 000082: Seed Spin Fixed Cost menu and permissions
--
-- Adds Spin Fixed Cost as a child of Finance > Master in the sidebar navigation.
-- Also seeds the finance.master.spinfixedcost.* permissions for RBAC.
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   chk_permission_code_format = ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--   No underscores or hyphens inside a segment, so the entity segment is the
--   concatenated run `spinfixedcost` (same treatment 000016 gave `uomcategory`).

-- =============================================================================
-- PERMISSIONS — finance.master.spinfixedcost.*
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint, so a bare
-- ON CONFLICT (permission_code) target is valid here.

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('finance.master.spinfixedcost.view',   'View Spin Fixed Cost',   'View spin fixed cost list and details', 'finance', 'master', 'view',   true, 'seed'),
    ('finance.master.spinfixedcost.create', 'Create Spin Fixed Cost', 'Create new spin fixed cost entries',    'finance', 'master', 'create', true, 'seed'),
    ('finance.master.spinfixedcost.update', 'Update Spin Fixed Cost', 'Update existing spin fixed cost',       'finance', 'master', 'update', true, 'seed'),
    ('finance.master.spinfixedcost.delete', 'Delete Spin Fixed Cost', 'Delete spin fixed cost entries',        'finance', 'master', 'delete', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- MENU ENTRY — Finance > Master > Spin Fixed Cost
-- =============================================================================
-- Level-3 menu ids are deterministic 00000000-0000-0000-0003-0000000000NN.
-- Highest occupied seq across all migrations is ...004e (000081, PPC Customer),
-- so ...004f is the next free one.
--
-- DEVIATION FROM 000016: that migration used ON CONFLICT (menu_code) DO NOTHING,
-- which does NOT protect against a reused menu_id PRIMARY KEY — a fresh
-- menu_code with a colliding menu_id still raises a PK violation. Guarding on
-- both columns with WHERE NOT EXISTS instead.

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
SELECT '00000000-0000-0000-0003-00000000004f', '00000000-0000-0000-0002-000000000002', 'FINANCE_SPIN_FIXED_COST', 'Spin Fixed Cost', '/finance/master/spin-fixed-cost', 'Factory', 'finance', 3, 35, true, true, 'seed'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_menu
    WHERE menu_id = '00000000-0000-0000-0003-00000000004f'
       OR menu_code = 'FINANCE_SPIN_FIXED_COST'
);

-- =============================================================================
-- MENU PERMISSIONS — Link Spin Fixed Cost menu to its view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_SPIN_FIXED_COST'
    AND p.permission_code = 'finance.master.spinfixedcost.view'
    AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- ASSIGN SPIN FIXED COST PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================
-- Mirrors 000016: SUPER_ADMIN is the only role granted by that seed.

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.spinfixedcost.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
