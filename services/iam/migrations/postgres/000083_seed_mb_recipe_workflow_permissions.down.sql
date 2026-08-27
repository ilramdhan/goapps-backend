-- IAM Service Database Migrations
-- 000083: Rollback MB recipe workflow permissions + MB Cross Section menu
-- FK-safe order: junction rows first, then the parents.

-- 1. Remove role_permissions for all 9 permissions, for EVERY role the up
--    migration granted them to under K-12 (2026-08-22):
--      SUPER_ADMIN (9) + MB_DRAFTER (2) + MB_VALIDATOR (3) + MB_APPROVER (5) = 19 rows.
--    Deleting by permission_id (not by role) covers all four roles in one
--    statement and is exhaustive: these 9 permissions are created by this
--    migration, so no role_permissions row for them predates it.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
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
);

-- 2. Remove menu_permissions for the MB Cross Section menu
DELETE FROM menu_permissions
WHERE menu_id = '00000000-0000-0000-0003-000000000050';

-- 3. Remove the 9 permissions
DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.mb.recipe.unlock',
    'finance.mb.recipe.lock',
    'finance.mb.recipe.export',
    'finance.yarnmaster.mbspin.duplicate',
    'finance.master.mbxsection.view',
    'finance.master.mbxsection.create',
    'finance.master.mbxsection.update',
    'finance.master.mbxsection.delete',
    'finance.mb.head.reject'
);

-- 4. Remove the menu entry
DELETE FROM mst_menu
WHERE menu_code = 'FINANCE_MB_CROSS_SECTION';

-- chk_permission_action is deliberately NOT reverted: the up migration did not
-- change it (all 9 actions already existed in the 36-value list from 000081).
