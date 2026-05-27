-- IAM Service Database Migrations
-- 000028: Seed Product Costing permissions for finance.product.* namespace.
--
-- Adds 12 permissions covering product master CRUD + duplicate, and
-- product request CRUD + assign/resolve/reject workflow actions.
-- Extends chk_permission_action to include 'assign', 'resolve', 'reject', 'duplicate'.
-- All inserts use ON CONFLICT DO NOTHING for idempotency.

-- =============================================================================
-- EXTEND chk_permission_action — allow product workflow actions
-- =============================================================================

ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate',
        'assign', 'resolve', 'reject', 'duplicate'
    ));

-- =============================================================================
-- PERMISSIONS — finance.product.master.* and finance.product.request.*
-- Permission code format constraint: ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- (no underscores or hyphens in segments)
-- =============================================================================

INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by)
VALUES
    -- Product master CRUD + duplicate
    ('finance.product.master.view',      'View Products',       'View product master data',          'finance', 'product', 'view',      true, 'seed'),
    ('finance.product.master.create',    'Create Product',      'Create new products',               'finance', 'product', 'create',    true, 'seed'),
    ('finance.product.master.update',    'Update Product',      'Update product master data',        'finance', 'product', 'update',    true, 'seed'),
    ('finance.product.master.delete',    'Delete Product',      'Soft-delete products',              'finance', 'product', 'delete',    true, 'seed'),
    ('finance.product.master.duplicate', 'Duplicate Product',   'Duplicate existing products',       'finance', 'product', 'duplicate', true, 'seed'),

    -- Product request CRUD + workflow actions
    ('finance.product.request.view',     'View Requests',       'View product request tickets',      'finance', 'product', 'view',      true, 'seed'),
    ('finance.product.request.create',   'Create Request',      'Raise product request tickets',     'finance', 'product', 'create',    true, 'seed'),
    ('finance.product.request.update',   'Update Request',      'Update request details',            'finance', 'product', 'update',    true, 'seed'),
    ('finance.product.request.delete',   'Delete Request',      'Soft-delete requests',              'finance', 'product', 'delete',    true, 'seed'),
    ('finance.product.request.assign',   'Assign Request',      'Assign request to handler',         'finance', 'product', 'assign',    true, 'seed'),
    ('finance.product.request.resolve',  'Resolve Request',     'Resolve request with product link', 'finance', 'product', 'resolve',   true, 'seed'),
    ('finance.product.request.reject',   'Reject Request',      'Reject infeasible requests',        'finance', 'product', 'reject',    true, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- ASSIGN ALL PRODUCT COSTING PERMISSIONS TO SUPER_ADMIN ROLE
-- =============================================================================

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code LIKE 'finance.product.%'
  AND r.is_active = true
  AND p.is_active = true
ON CONFLICT (role_id, permission_id) DO NOTHING;
