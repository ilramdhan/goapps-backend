-- IAM Service Database Migrations
-- 000091: Rollback finance.mb.head.unrevoke permission + SUPER_ADMIN grant
-- FK-safe order: junction row first, then the parent, then the CHECK constraint.

-- 1. Remove the SUPER_ADMIN role_permissions row
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code = 'finance.mb.head.unrevoke'
);

-- 2. Remove the permission
DELETE FROM mst_permission
WHERE permission_code = 'finance.mb.head.unrevoke';

-- =============================================================================
-- 3. RESTORE chk_permission_action -- remove 'unrevoke' (back to the 000081 set)
-- =============================================================================
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync'
  ));
