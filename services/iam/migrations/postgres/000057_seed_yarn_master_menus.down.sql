-- IAM Service Database Migrations
-- 000057 DOWN: Remove Yarn Master menus and permissions
--
-- Deletes in reverse order to respect foreign key constraints:
--   1. role_permissions  (references mst_permission)
--   2. menu_permissions  (references mst_menu, mst_permission)
--   3. mst_permission    (leaf rows)
--   4. mst_menu          (level-3 leaves first, then level-2 parent — 7 rows total)

-- =============================================================================
-- REMOVE SUPER_ADMIN ROLE ASSIGNMENTS
-- =============================================================================

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.yarnmaster.%'
);

-- =============================================================================
-- REMOVE MENU PERMISSION LINKS
-- =============================================================================

DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000021',
    '00000000-0000-0000-0003-000000000022',
    '00000000-0000-0000-0003-000000000023',
    '00000000-0000-0000-0003-000000000024',
    '00000000-0000-0000-0003-000000000025',
    '00000000-0000-0000-0003-000000000026'
);

-- =============================================================================
-- REMOVE PERMISSIONS
-- =============================================================================

DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.yarnmaster.%';

-- =============================================================================
-- REMOVE MENUS — level-3 leaves first, then level-2 parent
-- =============================================================================

DELETE FROM mst_menu
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000021',
    '00000000-0000-0000-0003-000000000022',
    '00000000-0000-0000-0003-000000000023',
    '00000000-0000-0000-0003-000000000024',
    '00000000-0000-0000-0003-000000000025',
    '00000000-0000-0000-0003-000000000026',
    '00000000-0000-0000-0002-000000000016'
);
