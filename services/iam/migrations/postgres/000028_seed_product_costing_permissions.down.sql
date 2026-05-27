-- Rollback: remove product costing permissions and revert action_type constraint.
-- Restores chk_permission_action to its pre-migration state (removes 'assign', 'resolve', 'reject', 'duplicate').

-- =============================================================================
-- REMOVE ROLE ASSIGNMENTS
-- =============================================================================

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.product.%'
);

-- =============================================================================
-- REMOVE PERMISSIONS
-- =============================================================================

DELETE FROM mst_permission WHERE permission_code LIKE 'finance.product.%';

-- =============================================================================
-- RESTORE chk_permission_action — remove 'assign', 'resolve', 'reject', 'duplicate'
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate'
    ));
