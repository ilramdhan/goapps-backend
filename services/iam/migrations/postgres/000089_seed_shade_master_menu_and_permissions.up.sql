-- IAM Service Database Migrations
-- 000089: Seed Shade master menu (Finance > Master, alongside UOM) and its
--         finance.master.shade.* permissions.
--
-- Context: the Shade master page (/finance/master/shades) already exists in the
-- frontend and its ShadeService RPCs already exist in the finance backend
-- (Create/Get/Update/Deactivate/List/Sync). Until this migration it was reachable
-- ONLY by typing the URL, because no mst_menu row pointed at it.
--
-- User decision (2026-08-26): "taruh menu Shade di grup Master bareng UOM" —
-- parent is FINANCE_MASTER (00000000-0000-0000-0002-000000000002), the same
-- parent as FINANCE_UOM (000009:72).
--
-- Placement facts (read from existing migrations, NOT guessed):
--   FINANCE_UOM             sort_order 10 (000009:72)
--   FINANCE_UOM_CATEGORY    sort_order 12 (000016:28)
--   FINANCE_RM_CATEGORY     sort_order 15 (000011:28)
--   FINANCE_PARAMETER(S)    sort_order 20 (000009:73, 000014:28)
--   FINANCE_PRODUCT_TYPE    sort_order 25 (000039:7)
--   FINANCE_FORMULA         sort_order 30 (000015:28)
--   FINANCE_SPIN_FIXED_COST sort_order 35 (000082:39)
-- => 45 is free and lands Shade at the end of the Master group without
--    renumbering any existing sibling.
--
-- menu_id 00000000-0000-0000-0003-000000000051: the highest menu UUID currently
-- seeded is ...0050 (000083:53, FINANCE_MB_CROSS_SECTION), so ...0051 is the
-- next free one in the established 0003-* menu block.
--
-- action_type values used: view/create/update/delete/sync. All five are already
-- in the live chk_permission_action whitelist (36 values, set by 000081:31-36 —
-- 000081 is what added 'sync' for the Oracle-sync masters). This migration
-- therefore contains ZERO DDL and does NOT touch any CHECK constraint.
--
-- permission_code format obeys chk_permission_code_format (000004:55):
-- exactly 4 lowercase segments {service}.{module}.{entity}.{action}.
--
-- Every statement is idempotent (ON CONFLICT DO NOTHING).

-- =============================================================================
-- 1. PERMISSIONS — finance.master.shade.*
-- =============================================================================
-- 'delete' maps to the DeactivateShade RPC, which is a SOFT deactivate
-- (is_active = false), not a physical row delete — the backend never hard-deletes
-- a shade. The permission is named 'delete' because that is the whitelisted
-- action_type and it matches the sibling masters' naming.
-- 'sync' guards the Oracle pull button. Oracle is READ-ONLY: the sync only
-- SELECTs from MGTDAT and writes the result into Postgres.

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    (gen_random_uuid(), 'finance.master.shade.view',   'View Shades',      'View shade master list and details',                'finance', 'master', 'view',   true, 'seed'),
    (gen_random_uuid(), 'finance.master.shade.create', 'Create Shade',     'Create a new shade in the shade master',            'finance', 'master', 'create', true, 'seed'),
    (gen_random_uuid(), 'finance.master.shade.update', 'Update Shade',     'Update an existing shade in the shade master',      'finance', 'master', 'update', true, 'seed'),
    (gen_random_uuid(), 'finance.master.shade.delete', 'Deactivate Shade', 'Deactivate (soft) a shade in the shade master',     'finance', 'master', 'delete', true, 'seed'),
    (gen_random_uuid(), 'finance.master.shade.sync',   'Sync Shades',      'Sync shade master from Oracle MGTDAT (read-only)',  'finance', 'master', 'sync',   true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. MENU ENTRY — Finance > Master > Shade
-- =============================================================================
-- icon_name 'Palette' is stored PascalCase and resolved at runtime by
-- pascalToIconName() (frontend src/types/iam/menu.ts:194), which derives its map
-- from lucide's dynamicIconImports — so 'Palette' resolves to the lucide
-- "palette" icon. No hardcoded icon whitelist has to be edited.

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
    ('00000000-0000-0000-0003-000000000051', '00000000-0000-0000-0002-000000000002', 'FINANCE_SHADE', 'Shade', '/finance/master/shades', 'Palette', 'finance', 3, 45, true, true, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- =============================================================================
-- 3. MENU PERMISSIONS — visibility gate for the sidebar entry
-- =============================================================================
-- IMPORTANT: a menu with NO rows in menu_permissions is visible to EVERY
-- authenticated user. Linking the .view permission here is what makes the Shade
-- entry permission-gated like its siblings rather than world-visible.

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT '00000000-0000-0000-0003-000000000051', permission_id, 'seed'
FROM mst_permission
WHERE permission_code = 'finance.master.shade.view' AND is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- 4. BACKFILL permission.menu_id (convention established by 000066)
-- =============================================================================
-- 000066 backfilled mst_permission.menu_id for every finance.master.* permission
-- so the permission picker can group permissions by menu. New permissions must
-- follow the same convention or they show up ungrouped.

UPDATE mst_permission p
SET menu_id = m.menu_id, updated_by = 'seed', updated_at = NOW()
FROM mst_menu m
WHERE m.menu_code = 'FINANCE_SHADE'
  AND p.permission_code LIKE 'finance.master.shade.%'
  AND p.menu_id IS NULL;

-- =============================================================================
-- 5. ASSIGN ALL SHADE PERMISSIONS TO SUPER ADMIN
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code LIKE 'finance.master.shade.%'
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
