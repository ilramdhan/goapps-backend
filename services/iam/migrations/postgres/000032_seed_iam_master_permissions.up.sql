-- IAM Service Database Migrations
-- 000032: Seed IAM master-data permissions for company / division / department /
-- section / company-mapping plus user↔company-mapping assignment.
--
-- Adds 'remove' to chk_permission_action whitelist (used by user.companymapping.remove).
-- Permission codes follow ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- (lowercase, dotted, no underscores or hyphens within segments). Multi-word
-- entities are concatenated → companymapping.

-- =============================================================================
-- EXTEND chk_permission_action — allow 'remove'
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate',
        'assign', 'resolve', 'reject', 'duplicate',
        'remove'
    ));

-- =============================================================================
-- PERMISSIONS — iam.master.{company,division,department,section,companymapping}.{view,create,update,delete}
--               iam.user.companymapping.{view,assign,remove}
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    -- Company
    ('iam.master.company.view',          'View Companies',                  'View companies',                          'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.company.create',        'Create Company',                  'Create new companies',                    'iam', 'master', 'create', true, 'seed'),
    ('iam.master.company.update',        'Update Company',                  'Update existing companies',               'iam', 'master', 'update', true, 'seed'),
    ('iam.master.company.delete',        'Delete Company',                  'Delete companies',                        'iam', 'master', 'delete', true, 'seed'),

    -- Division
    ('iam.master.division.view',         'View Divisions',                  'View divisions',                          'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.division.create',       'Create Division',                 'Create new divisions',                    'iam', 'master', 'create', true, 'seed'),
    ('iam.master.division.update',       'Update Division',                 'Update existing divisions',               'iam', 'master', 'update', true, 'seed'),
    ('iam.master.division.delete',       'Delete Division',                 'Delete divisions',                        'iam', 'master', 'delete', true, 'seed'),

    -- Department
    ('iam.master.department.view',       'View Departments',                'View departments',                        'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.department.create',     'Create Department',               'Create new departments',                  'iam', 'master', 'create', true, 'seed'),
    ('iam.master.department.update',     'Update Department',               'Update existing departments',             'iam', 'master', 'update', true, 'seed'),
    ('iam.master.department.delete',     'Delete Department',               'Delete departments',                      'iam', 'master', 'delete', true, 'seed'),

    -- Section
    ('iam.master.section.view',          'View Sections',                   'View sections',                           'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.section.create',        'Create Section',                  'Create new sections',                     'iam', 'master', 'create', true, 'seed'),
    ('iam.master.section.update',        'Update Section',                  'Update existing sections',                'iam', 'master', 'update', true, 'seed'),
    ('iam.master.section.delete',        'Delete Section',                  'Delete sections',                         'iam', 'master', 'delete', true, 'seed'),

    -- Company Mapping
    ('iam.master.companymapping.view',   'View Company Mappings',           'View company mappings',                   'iam', 'master', 'view',   true, 'seed'),
    ('iam.master.companymapping.create', 'Create Company Mapping',          'Create new company mappings',             'iam', 'master', 'create', true, 'seed'),
    ('iam.master.companymapping.update', 'Update Company Mapping',          'Update existing company mappings',        'iam', 'master', 'update', true, 'seed'),
    ('iam.master.companymapping.delete', 'Delete Company Mapping',          'Delete company mappings',                 'iam', 'master', 'delete', true, 'seed'),

    -- User ↔ Company Mapping assignment
    ('iam.user.companymapping.view',     'View User Company Mappings',      'View company mappings assigned to users', 'iam', 'user',   'view',   true, 'seed'),
    ('iam.user.companymapping.assign',   'Assign User Company Mapping',     'Assign company mappings to users',        'iam', 'user',   'assign', true, 'seed'),
    ('iam.user.companymapping.remove',   'Remove User Company Mapping',     'Remove company mappings from users',      'iam', 'user',   'remove', true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- ASSIGN ALL NEW PERMISSIONS TO SUPER_ADMIN
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND r.is_active = true
  AND p.is_active = true
  AND (
        p.permission_code LIKE 'iam.master.company.%'
     OR p.permission_code LIKE 'iam.master.division.%'
     OR p.permission_code LIKE 'iam.master.department.%'
     OR p.permission_code LIKE 'iam.master.section.%'
     OR p.permission_code LIKE 'iam.master.companymapping.%'
     OR p.permission_code LIKE 'iam.user.companymapping.%'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;
