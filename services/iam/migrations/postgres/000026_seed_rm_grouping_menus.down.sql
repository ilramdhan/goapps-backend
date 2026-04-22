-- Rollback: remove RM Pricing menus, permissions, and role assignments.
-- Also restore chk_permission_action to its pre-migration state (removes 'recalculate').

-- =============================================================================
-- REMOVE ROLE ASSIGNMENTS
-- =============================================================================

DELETE FROM role_permissions
WHERE permission_id IN (
    SELECT permission_id FROM mst_permission
    WHERE permission_code LIKE 'finance.rmpricing.%'
);

-- =============================================================================
-- REMOVE MENU PERMISSIONS
-- =============================================================================

DELETE FROM menu_permissions
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000012',
    '00000000-0000-0000-0003-000000000013',
    '00000000-0000-0000-0003-000000000014'
);

-- =============================================================================
-- REMOVE MENUS
-- =============================================================================

DELETE FROM mst_menu
WHERE menu_id IN (
    '00000000-0000-0000-0003-000000000012',
    '00000000-0000-0000-0003-000000000013',
    '00000000-0000-0000-0003-000000000014',
    '00000000-0000-0000-0002-000000000014'
);

-- =============================================================================
-- REMOVE PERMISSIONS
-- =============================================================================

DELETE FROM mst_permission
WHERE permission_code LIKE 'finance.rmpricing.%';

-- =============================================================================
-- RESTORE chk_permission_action — remove 'recalculate'
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass'
    ));
