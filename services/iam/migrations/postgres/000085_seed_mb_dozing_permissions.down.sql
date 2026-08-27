-- IAM Service Database Migrations
-- 000085: Rollback MB Dozing (LDR calculator) permissions + role assignments
-- FK-safe order: junction rows first, then the parent permission rows.

-- 1. Remove role_permissions for both permissions, for EVERY role the up
--    migration granted them to: SUPER_ADMIN (2) + MB_DRAFTER (2) = 4 rows.
--    Deleting by permission_id covers both roles in one statement and is
--    exhaustive: these 2 permissions are CREATED by this migration, so no
--    role_permissions row referencing them can predate it.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'finance.mb.dozing.calculate',
        'finance.mb.dozing.preview'
    )
);

-- 2. Remove the 2 permissions.
DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.mb.dozing.calculate',
    'finance.mb.dozing.preview'
);

-- No menu_permissions cleanup: the up migration seeded no menu and no
-- menu_permissions rows (the dozing calculator is reached from the existing MB
-- recipe screens, not from a menu entry of its own).
--
-- chk_permission_action is deliberately NOT reverted: the up migration contains
-- ZERO DDL and did not touch the constraint.
