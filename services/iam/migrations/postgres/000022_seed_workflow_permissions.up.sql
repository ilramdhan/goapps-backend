-- IAM Service Database Migrations
-- 000022: Seed workflow transition permissions for Employee Level
--
-- Adds submit/approve/release/bypass permissions following the
-- iam.master.employeelevel.{action} pattern (no underscores per chk_permission_code_format).
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- EXTEND action_type CHECK CONSTRAINT to allow workflow actions
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN ('view', 'create', 'update', 'delete', 'export', 'import', 'submit', 'approve', 'release', 'bypass'));

-- =============================================================================
-- PERMISSIONS — workflow transition actions
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    ('iam.master.employeelevel.submit',  'Submit Employee Level',         'Submit employee level for approval',       'iam', 'master', 'submit',  true, 'seed'),
    ('iam.master.employeelevel.approve', 'Approve Employee Level',        'Approve submitted employee level',         'iam', 'master', 'approve', true, 'seed'),
    ('iam.master.employeelevel.release', 'Release Employee Level',        'Release approved employee level',          'iam', 'master', 'release', true, 'seed'),
    ('iam.master.employeelevel.bypass',  'Bypass Release Employee Level', 'Bypass approval and release directly',     'iam', 'master', 'bypass',  true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- ASSIGN NEW PERMISSIONS TO SUPER ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
    AND p.permission_code IN (
        'iam.master.employeelevel.submit',
        'iam.master.employeelevel.approve',
        'iam.master.employeelevel.release',
        'iam.master.employeelevel.bypass'
    )
    AND r.is_active = true
    AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
