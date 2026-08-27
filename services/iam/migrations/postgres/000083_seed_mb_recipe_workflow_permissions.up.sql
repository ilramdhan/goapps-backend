-- IAM Service Database Migrations
-- 000083: Seed MB recipe workflow permissions + MB Cross Section master menu
--
-- 9 permissions in ONE migration (user decision K-11, 2026-08-22): the 8
-- originally planned plus finance.mb.head.reject, which comes from K-2
-- (SUBMITTED → REJECTED requires a reason). Deliberately NOT split into a
-- separate 000084 — they ship together with the same feature.
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   chk_permission_code_format = ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
--   No underscores/hyphens inside a segment, so `mbxsection` is the concatenated
--   run (same treatment 000068 gave `mblusture` and 000082 gave `spinfixedcost`).
--   All 9 codes below were verified against this regex before writing.
--
-- chk_permission_action is NOT widened here: the live constraint (36 values,
-- last extended by 000081 with 'sync') already contains every action used below
-- — view, create, update, delete, export, duplicate, lock, unlock, reject.

-- =============================================================================
-- 1. PERMISSIONS — 9 rows
-- =============================================================================
-- uq_permission_code is a plain (non-partial) UNIQUE constraint, so a bare
-- ON CONFLICT (permission_code) target is valid here (same as 000082).

INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    -- MB recipe lock/unlock/export workflow
    (gen_random_uuid(), 'finance.mb.recipe.unlock',            'Unlock MB Recipe',        'Unlock a locked MB recipe for editing',            'finance', 'mb',         'unlock',    TRUE, 'seed'),
    (gen_random_uuid(), 'finance.mb.recipe.lock',              'Lock MB Recipe',          'Lock an MB recipe against further editing',        'finance', 'mb',         'lock',      TRUE, 'seed'),
    (gen_random_uuid(), 'finance.mb.recipe.export',            'Export MB Recipe',        'Export MB recipe data',                            'finance', 'mb',         'export',    TRUE, 'seed'),
    -- MB Spin duplicate (yarn master module)
    (gen_random_uuid(), 'finance.yarnmaster.mbspin.duplicate', 'Duplicate MB Spin',       'Duplicate an existing MB spin record',             'finance', 'yarnmaster', 'duplicate', TRUE, 'seed'),
    -- MB Cross Section master CRUD
    (gen_random_uuid(), 'finance.master.mbxsection.view',      'View MB Cross Section',   'View MB cross section master data',                'finance', 'master',     'view',      TRUE, 'seed'),
    (gen_random_uuid(), 'finance.master.mbxsection.create',    'Create MB Cross Section', 'Create MB cross section master data',              'finance', 'master',     'create',    TRUE, 'seed'),
    (gen_random_uuid(), 'finance.master.mbxsection.update',    'Update MB Cross Section', 'Update MB cross section master data',              'finance', 'master',     'update',    TRUE, 'seed'),
    (gen_random_uuid(), 'finance.master.mbxsection.delete',    'Delete MB Cross Section', 'Delete MB cross section master data',              'finance', 'master',     'delete',    TRUE, 'seed'),
    -- MB Head reject (K-2: SUBMITTED -> REJECTED, requires a reason)
    (gen_random_uuid(), 'finance.mb.head.reject',              'Reject MB Head',          'Reject a submitted MB head recipe with a reason',  'finance', 'mb',         'reject',    TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 2. MENU ENTRY — Finance > Master > MB Cross Section
-- =============================================================================
-- Level-3 menu ids are deterministic 00000000-0000-0000-0003-0000000000NN.
-- Highest occupied seq across all iam migrations is ...004f (000082, Spin Fixed
-- Cost). ...000050 is free — verified by grep across migrations/postgres.
-- sort_order 40: the level-3 children of FINANCE_MASTER occupy 10..35.
--
-- Guarding on BOTH menu_id and menu_code with WHERE NOT EXISTS (same reasoning
-- as 000082): ON CONFLICT (menu_code) alone would not catch a reused PK.

INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
SELECT '00000000-0000-0000-0003-000000000050', '00000000-0000-0000-0002-000000000002', 'FINANCE_MB_CROSS_SECTION', 'MB Cross Section', '/finance/master/mb-cross-section', 'Shapes', 'finance', 3, 40, true, true, 'seed'
WHERE NOT EXISTS (
    SELECT 1 FROM mst_menu
    WHERE menu_id = '00000000-0000-0000-0003-000000000050'
       OR menu_code = 'FINANCE_MB_CROSS_SECTION'
);

-- =============================================================================
-- 3. MENU PERMISSIONS — gate the menu on its view permission
-- =============================================================================

INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m
CROSS JOIN mst_permission p
WHERE m.menu_code = 'FINANCE_MB_CROSS_SECTION'
    AND p.permission_code = 'finance.master.mbxsection.view'
    AND p.is_active = true
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- =============================================================================
-- 4. ROLE ASSIGNMENTS — K-12 (user decision, 2026-08-22)
-- =============================================================================
-- The MB role mapping that 000083 originally left open is now decided. Matrix:
--
--   SUPER_ADMIN  : all 9 permissions
--   MB_DRAFTER   : finance.mb.recipe.export
--                  finance.yarnmaster.mbspin.duplicate
--   MB_VALIDATOR : DRAFTER's 2 + finance.mb.recipe.lock
--   MB_APPROVER  : VALIDATOR's 3 + finance.mb.recipe.unlock
--                                 + finance.mb.head.reject
--   FINANCE      : none
--
-- The tiers are cumulative by intent, but mst_role has NO parent/inherit column
-- (see 000004) — the permission interceptor resolves role_permissions rows
-- directly. So every tier is listed EXPLICITLY below; nothing is inherited.
-- Total: 9 + 2 + 3 + 5 = 19 role_permissions rows.
--
-- ⚠ The 4 MB Cross Section master permissions (finance.master.mbxsection.*)
-- stay SUPER_ADMIN-only by decision — deliberately NOT granted to any MB role.
--
-- All four role codes were verified to exist: SUPER_ADMIN (seeds/main.go),
-- MB_DRAFTER / MB_VALIDATOR / MB_APPROVER (000068_mb_roles_and_permissions).
-- FINANCE gets nothing, so it has no INSERT block at all.

-- 4a. SUPER_ADMIN — all 9
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.mb.recipe.unlock',
        'finance.mb.recipe.lock',
        'finance.mb.recipe.export',
        'finance.yarnmaster.mbspin.duplicate',
        'finance.master.mbxsection.view',
        'finance.master.mbxsection.create',
        'finance.master.mbxsection.update',
        'finance.master.mbxsection.delete',
        'finance.mb.head.reject'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 4b. MB_DRAFTER — export + duplicate (2)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_DRAFTER'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.mb.recipe.export',
        'finance.yarnmaster.mbspin.duplicate'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 4c. MB_VALIDATOR — DRAFTER's 2 + lock (3, listed explicitly)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_VALIDATOR'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.mb.recipe.export',
        'finance.yarnmaster.mbspin.duplicate',
        'finance.mb.recipe.lock'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- 4d. MB_APPROVER — VALIDATOR's 3 + unlock + reject (5, listed explicitly)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'MB_APPROVER'
    AND r.is_active = true
    AND p.is_active = true
    AND p.permission_code IN (
        'finance.mb.recipe.export',
        'finance.yarnmaster.mbspin.duplicate',
        'finance.mb.recipe.lock',
        'finance.mb.recipe.unlock',
        'finance.mb.head.reject'
    )
ON CONFLICT (role_id, permission_id) DO NOTHING;
