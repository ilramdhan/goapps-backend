-- v1.3 permissions per PRD v1.3 §0.1 D10.
-- All codes conform to chk_permission_code_format (4 lowercase-alnum segments, no underscores).

-- =============================================================================
-- EXTEND chk_permission_action whitelist — adds: lock, unlock, unlockoverride, reassign, send, read, trigger
-- =============================================================================
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass',
        'recalculate',
        'assign', 'resolve', 'reject', 'duplicate',
        'remove',
        'lock', 'unlock', 'unlockoverride', 'reassign',
        'send', 'read', 'trigger'
    ));

-- =============================================================================
-- PERMISSIONS
-- =============================================================================
INSERT INTO mst_permission (permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
    -- Workflow template admin
    ('iam.master.workflowtemplate.view',   'View Workflow Templates',  'View workflow template definitions',   'iam', 'master', 'view',   TRUE, 'seed'),
    ('iam.master.workflowtemplate.create', 'Create Workflow Template', 'Create new workflow template',         'iam', 'master', 'create', TRUE, 'seed'),
    ('iam.master.workflowtemplate.update', 'Update Workflow Template', 'Update existing workflow template',    'iam', 'master', 'update', TRUE, 'seed'),
    ('iam.master.workflowtemplate.delete', 'Delete Workflow Template', 'Soft-delete workflow template',        'iam', 'master', 'delete', TRUE, 'seed'),

    -- Workflow actor actions
    ('finance.transaction.workflow.approve',  'Approve Workflow Step',  'Advance workflow instance',          'finance', 'transaction', 'approve',  TRUE, 'seed'),
    ('finance.transaction.workflow.reject',   'Reject Workflow Step',   'Reject workflow instance',           'finance', 'transaction', 'reject',   TRUE, 'seed'),
    ('finance.transaction.workflow.reassign', 'Reassign Workflow Step', 'Reassign step to another approver',  'finance', 'transaction', 'reassign', TRUE, 'seed'),

    -- Product type admin (Finance master)
    ('finance.master.producttype.view',   'View Product Types',  'View product type list',  'finance', 'master', 'view',   TRUE, 'seed'),
    ('finance.master.producttype.create', 'Create Product Type', 'Create new product type', 'finance', 'master', 'create', TRUE, 'seed'),
    ('finance.master.producttype.update', 'Update Product Type', 'Update product type',     'finance', 'master', 'update', TRUE, 'seed'),
    ('finance.master.producttype.delete', 'Delete Product Type', 'Soft-delete product type','finance', 'master', 'delete', TRUE, 'seed'),

    -- prd_request CRUD
    ('finance.transaction.prdrequest.view',   'View Product Requests',  'View product request list',  'finance', 'transaction', 'view',   TRUE, 'seed'),
    ('finance.transaction.prdrequest.create', 'Create Product Request', 'Submit new product request', 'finance', 'transaction', 'create', TRUE, 'seed'),
    ('finance.transaction.prdrequest.update', 'Update Product Request', 'Edit DRAFT product request', 'finance', 'transaction', 'update', TRUE, 'seed'),
    ('finance.transaction.prdrequest.delete', 'Delete Product Request', 'Soft-delete product request','finance', 'transaction', 'delete', TRUE, 'seed'),
    ('finance.transaction.prdrequest.submit', 'Submit Product Request', 'Submit request to workflow', 'finance', 'transaction', 'submit', TRUE, 'seed'),

    -- cst_product CRUD
    ('finance.transaction.cstproduct.view',            'View Product Costing',    'View product list',           'finance', 'transaction', 'view',            TRUE, 'seed'),
    ('finance.transaction.cstproduct.create',          'Create Product Costing',  'Create product from request', 'finance', 'transaction', 'create',          TRUE, 'seed'),
    ('finance.transaction.cstproduct.update',          'Update Product Costing',  'Edit DRAFT product',          'finance', 'transaction', 'update',          TRUE, 'seed'),
    ('finance.transaction.cstproduct.delete',          'Delete Product Costing',  'Soft-delete product',         'finance', 'transaction', 'delete',          TRUE, 'seed'),
    ('finance.transaction.cstproduct.duplicate',       'Duplicate Product',       'Deep copy product',           'finance', 'transaction', 'duplicate',       TRUE, 'seed'),
    ('finance.transaction.cstproduct.lock',            'Lock Product',            'Lock product after approval', 'finance', 'transaction', 'lock',            TRUE, 'seed'),
    ('finance.transaction.cstproduct.unlock',          'Unlock Product',          'Unlock with password',        'finance', 'transaction', 'unlock',          TRUE, 'seed'),
    ('finance.transaction.cstproduct.unlockoverride',  'Override-Unlock Product', 'Bypass + password challenge', 'finance', 'transaction', 'unlockoverride',  TRUE, 'seed'),

    -- Chat
    ('finance.transaction.chat.send',   'Send Chat Message',   'Send a message in request thread',     'finance', 'transaction', 'send',   TRUE, 'seed'),
    ('finance.transaction.chat.read',   'Read Chat Thread',    'Read messages in request thread',      'finance', 'transaction', 'read',   TRUE, 'seed'),
    ('finance.transaction.chat.delete', 'Delete Chat Message', 'Delete own message (or any if admin)', 'finance', 'transaction', 'delete', TRUE, 'seed'),

    -- Cost calculation trigger
    ('finance.transaction.costcalc.trigger', 'Trigger Cost Calculation', 'Enqueue product cost calculation job', 'finance', 'transaction', 'trigger', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- =============================================================================
-- Grant all v1.3 permissions to SUPER_ADMIN
-- =============================================================================
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r
CROSS JOIN mst_permission p
WHERE r.role_code = 'SUPER_ADMIN'
  AND p.permission_code IN (
    'iam.master.workflowtemplate.view','iam.master.workflowtemplate.create','iam.master.workflowtemplate.update','iam.master.workflowtemplate.delete',
    'finance.transaction.workflow.approve','finance.transaction.workflow.reject','finance.transaction.workflow.reassign',
    'finance.master.producttype.view','finance.master.producttype.create','finance.master.producttype.update','finance.master.producttype.delete',
    'finance.transaction.prdrequest.view','finance.transaction.prdrequest.create','finance.transaction.prdrequest.update','finance.transaction.prdrequest.delete','finance.transaction.prdrequest.submit',
    'finance.transaction.cstproduct.view','finance.transaction.cstproduct.create','finance.transaction.cstproduct.update','finance.transaction.cstproduct.delete','finance.transaction.cstproduct.duplicate','finance.transaction.cstproduct.lock','finance.transaction.cstproduct.unlock','finance.transaction.cstproduct.unlockoverride',
    'finance.transaction.chat.send','finance.transaction.chat.read','finance.transaction.chat.delete',
    'finance.transaction.costcalc.trigger'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;
