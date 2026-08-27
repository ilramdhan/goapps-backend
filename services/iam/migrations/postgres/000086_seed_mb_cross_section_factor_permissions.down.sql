-- IAM Service Database Migrations
-- 000086: Rollback MB Cross Section FACTOR CRUD permissions + role assignments
-- FK-safe order: junction rows first, then the parent permission rows.

-- 1. Remove role_permissions for all 4 permissions, for EVERY role the up
--    migration granted them to (SUPER_ADMIN x4 = 4 rows). Deleting by
--    permission_id is exhaustive: these 4 permissions are CREATED by 000086, so
--    no role_permissions row referencing them can predate it.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'finance.master.mbxsectionfactor.view',
        'finance.master.mbxsectionfactor.create',
        'finance.master.mbxsectionfactor.update',
        'finance.master.mbxsectionfactor.delete'
    )
);

-- 2. Remove any menu_permissions links, for safety. The up migration seeded
--    NONE (see its section 3), so this normally deletes 0 rows; it is kept so a
--    later hand-added link cannot block the FK delete in step 3.
DELETE FROM menu_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN (
        'finance.master.mbxsectionfactor.view',
        'finance.master.mbxsectionfactor.create',
        'finance.master.mbxsectionfactor.update',
        'finance.master.mbxsectionfactor.delete'
    )
);

-- 3. Remove the 4 permissions.
DELETE FROM mst_permission
WHERE permission_code IN (
    'finance.master.mbxsectionfactor.view',
    'finance.master.mbxsectionfactor.create',
    'finance.master.mbxsectionfactor.update',
    'finance.master.mbxsectionfactor.delete'
);

-- No mst_menu cleanup: the up migration created no menu row.
--
-- chk_permission_action is deliberately NOT touched: the up migration contains
-- ZERO DDL (view/create/update/delete were already whitelisted since 000004).
--
-- ⚠ NOTE FOR WHOEVER ROLLS THIS BACK: after this down migration the 5
-- /finance.v1.MbCrossSectionFactorService/* entries in
-- services/finance/internal/delivery/grpc/auth_interceptor.go point at
-- permission codes that NO LONGER EXIST. That FAILS CLOSED (nobody holds a
-- non-existent permission), so the factor RPCs become inaccessible to everyone
-- except via direct user_permissions — it does NOT fail open. Revert the
-- interceptor map in the same step to restore access.
