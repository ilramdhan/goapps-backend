-- IAM Service Database Migrations
-- 000092: Rollback finance.mb.head.bulk{unvalidate,submit,validate} permissions + SUPER_ADMIN grants
-- FK-safe order: junction rows first, then the parents, then the CHECK constraint.

-- 1. Remove the SUPER_ADMIN role_permissions rows
DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code IN ('finance.mb.head.bulkunvalidate', 'finance.mb.head.bulksubmit', 'finance.mb.head.bulkvalidate')
);

-- 2. Remove the permissions
DELETE FROM mst_permission
WHERE permission_code IN ('finance.mb.head.bulkunvalidate', 'finance.mb.head.bulksubmit', 'finance.mb.head.bulkvalidate');

-- =============================================================================
-- 3. RESTORE chk_permission_action -- remove 'bulkunvalidate', 'bulksubmit', 'bulkvalidate' (back to the 000091 set)
-- =============================================================================
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync','unrevoke'
  ));
