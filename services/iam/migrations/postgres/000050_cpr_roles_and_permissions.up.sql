-- Migration 000050: CPR Roles, Permissions, and Test User Assignments
--
-- Adds 6 new permission codes for CPR product-request and product-route workflows,
-- 6 CPR-specific roles (REQUESTER / SUBMITTER / REVIEWER / ENGINEER / APPROVER / ADMIN),
-- assigns permissions to those roles, assigns new permissions to SUPER_ADMIN,
-- and optionally attaches the roles to test users (silently skipped if users absent).
--
-- Permission codes added (action segment must be in chk_permission_action):
--   finance.product.request.submit  (submit — already in constraint from 000049)
--   finance.product.request.review  (review — NEW, constraint extended below)
--   finance.product.request.reopen  (reopen — NEW, constraint extended below)
--   finance.product.route.view      (view)
--   finance.product.route.create    (create)
--   finance.product.route.update    (update)
--
-- Permission code format: ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- (segments: service=finance, module=product, entity=request|route, action=*)

BEGIN;

-- =============================================================================
-- 1. Extend chk_permission_action to add 'review' and 'reopen'
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify', 'override',
        'review', 'reopen'
    ));

-- =============================================================================
-- 2. New permission codes
-- =============================================================================

INSERT INTO mst_permission (
    permission_id, permission_code, permission_name, description,
    service_name, module_name, action_type, is_active, created_by
) VALUES
    (gen_random_uuid(), 'finance.product.request.submit',  'Submit Product Request',  'Submit, cancel, close own requests',             'finance', 'product', 'submit', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.request.review',  'Review Product Request',  'Start review, verify classification',             'finance', 'product', 'review', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.request.reopen',  'Reopen Closed Request',   'Reopen CLOSED request (admin only)',              'finance', 'product', 'reopen', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.route.view',      'View Product Route',      'View routing data',                               'finance', 'product', 'view',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.route.create',    'Create/Edit Route',       'Create product master, edit route graph',         'finance', 'product', 'create', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.product.route.update',    'Promote/Lock Route',      'Promote route, link/unlink to request, lock',    'finance', 'product', 'update', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- 3. CPR roles
-- =============================================================================

INSERT INTO mst_role (role_id, role_code, role_name, description, is_active, created_by)
VALUES
    (gen_random_uuid(), 'CPR_REQUESTER', 'CPR Requester',         'Buat dan edit draft product request',            TRUE, 'seed'),
    (gen_random_uuid(), 'CPR_SUBMITTER', 'CPR Submitter/Manager', 'Submit, close, cancel product request',          TRUE, 'seed'),
    (gen_random_uuid(), 'CPR_REVIEWER',  'CPR Reviewer/Finance',  'Review, verify, decide feasibility',             TRUE, 'seed'),
    (gen_random_uuid(), 'CPR_ENGINEER',  'CPR Costing Engineer',  'Buat routing, product master, parameter',        TRUE, 'seed'),
    (gen_random_uuid(), 'CPR_APPROVER',  'CPR Approver',          'Approve fill tasks via fill config',             TRUE, 'seed'),
    (gen_random_uuid(), 'CPR_ADMIN',     'CPR Admin',             'Reopen requests, manage fill configs, full access', TRUE, 'seed')
ON CONFLICT (role_code) DO NOTHING;

-- =============================================================================
-- 4. Assign permissions to roles
-- =============================================================================

-- CPR_REQUESTER: view + create requests, view routes
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_REQUESTER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.view',
    'finance.product.request.create',
    'finance.product.route.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- CPR_SUBMITTER: view + create + submit requests, view routes
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_SUBMITTER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.view',
    'finance.product.request.create',
    'finance.product.request.submit',
    'finance.product.route.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- CPR_REVIEWER: view + review + resolve + reject + assign requests, view routes + cal jobs
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_REVIEWER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.view',
    'finance.product.request.review',
    'finance.product.request.resolve',
    'finance.product.request.reject',
    'finance.product.request.assign',
    'finance.product.route.view',
    'finance.cost.caljob.view',
    'finance.cost.caljob.trigger'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- CPR_ENGINEER: view requests, full route access, cal jobs
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_ENGINEER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.view',
    'finance.product.route.view',
    'finance.product.route.create',
    'finance.product.route.update',
    'finance.cost.caljob.view',
    'finance.cost.caljob.trigger'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- CPR_APPROVER: view only (fill-task approval controlled by fill config)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_APPROVER'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.view',
    'finance.product.route.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- CPR_ADMIN: all permissions including reopen
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'CPR_ADMIN'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.view',
    'finance.product.request.create',
    'finance.product.request.submit',
    'finance.product.request.review',
    'finance.product.request.resolve',
    'finance.product.request.reject',
    'finance.product.request.assign',
    'finance.product.request.reopen',
    'finance.product.route.view',
    'finance.product.route.create',
    'finance.product.route.update',
    'finance.cost.caljob.view',
    'finance.cost.caljob.trigger'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- =============================================================================
-- 5. Assign new permissions to SUPER_ADMIN
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
    'finance.product.request.submit',
    'finance.product.request.review',
    'finance.product.request.reopen',
    'finance.product.route.view',
    'finance.product.route.create',
    'finance.product.route.update'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- =============================================================================
-- 6. Assign CPR roles to test users (silently skipped if users do not exist)
-- =============================================================================

-- marketing01 → CPR_REQUESTER
INSERT INTO user_roles (user_id, role_id, assigned_by)
SELECT u.user_id, r.role_id, 'seed'
FROM mst_user u CROSS JOIN mst_role r
WHERE u.username = 'marketing01' AND r.role_code = 'CPR_REQUESTER'
ON CONFLICT (user_id, role_id) DO NOTHING;

-- marketingmgr → CPR_REQUESTER + CPR_SUBMITTER
INSERT INTO user_roles (user_id, role_id, assigned_by)
SELECT u.user_id, r.role_id, 'seed'
FROM mst_user u CROSS JOIN mst_role r
WHERE u.username = 'marketingmgr' AND r.role_code IN ('CPR_REQUESTER', 'CPR_SUBMITTER')
ON CONFLICT (user_id, role_id) DO NOTHING;

-- finance01 → CPR_REVIEWER + CPR_ADMIN
INSERT INTO user_roles (user_id, role_id, assigned_by)
SELECT u.user_id, r.role_id, 'seed'
FROM mst_user u CROSS JOIN mst_role r
WHERE u.username = 'finance01' AND r.role_code IN ('CPR_REVIEWER', 'CPR_ADMIN')
ON CONFLICT (user_id, role_id) DO NOTHING;

-- financemgr → CPR_REVIEWER + CPR_ADMIN
INSERT INTO user_roles (user_id, role_id, assigned_by)
SELECT u.user_id, r.role_id, 'seed'
FROM mst_user u CROSS JOIN mst_role r
WHERE u.username = 'financemgr' AND r.role_code IN ('CPR_REVIEWER', 'CPR_ADMIN')
ON CONFLICT (user_id, role_id) DO NOTHING;

-- production01/02/03 → CPR_ENGINEER
INSERT INTO user_roles (user_id, role_id, assigned_by)
SELECT u.user_id, r.role_id, 'seed'
FROM mst_user u CROSS JOIN mst_role r
WHERE u.username IN ('production01', 'production02', 'production03')
  AND r.role_code = 'CPR_ENGINEER'
ON CONFLICT (user_id, role_id) DO NOTHING;

-- productionmgr → CPR_APPROVER + CPR_ADMIN
INSERT INTO user_roles (user_id, role_id, assigned_by)
SELECT u.user_id, r.role_id, 'seed'
FROM mst_user u CROSS JOIN mst_role r
WHERE u.username = 'productionmgr' AND r.role_code IN ('CPR_APPROVER', 'CPR_ADMIN')
ON CONFLICT (user_id, role_id) DO NOTHING;

COMMIT;
