-- IAM Service Database Migrations
-- 000087: Rollback the MB recipe UNLOCK REQUEST permission
-- FK-safe order: junction rows first, then the parent.
--
-- ⛔ Rolls back ONLY what 000087 created. It must NOT disturb
-- finance.mb.recipe.unlock (seeded and granted by 000083) — reverting this
-- migration collapses the split back to "one code guards all three unlock RPCs",
-- which is the pre-000087 behaviour, and that code must survive intact.
--
-- ⛔ 000083 is ALREADY APPLIED on the user's local database and is never edited
-- or re-run; this file is the only sanctioned way to undo 000087.
--
-- Note: if the up migration was blocked by chk_permission_code_format (see the
-- BLOCKER section in the .up.sql), nothing was inserted and both DELETEs below
-- are simply no-ops — running this down is still safe.

-- 1. Remove the 4 role grants (SUPER_ADMIN, MB_DRAFTER, MB_VALIDATOR, MB_APPROVER).
--    Deleting by permission_id covers all four roles in one statement and is
--    exhaustive: the permission is created by 000087, so no role_permissions row
--    for it can predate this migration.
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code = 'finance.mb.recipe.unlockrequest'
);

-- 2. Remove the permission itself.
DELETE FROM mst_permission
WHERE permission_code = 'finance.mb.recipe.unlockrequest';

-- chk_permission_action and chk_permission_code_format are deliberately NOT
-- reverted: the up migration contains ZERO DDL and changed neither constraint.
