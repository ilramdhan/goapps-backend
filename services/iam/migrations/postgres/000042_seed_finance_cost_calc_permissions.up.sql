-- Phase C (Calc Engine) S8a.7: seed permissions for cost calculation engine.
--
-- Adds 8 permissions under finance.cost.* and assigns them to FINANCE,
-- FINANCE_MANAGER, and HEAD_FINANCE roles (if they exist). Idempotent.
--
-- Permission code format constraint: ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$
-- No underscores or hyphens in segments. "caljob" not "cal_job".

BEGIN;

-- Extend permission action allow-list to cover new verbs used by the calc engine.
-- "trigger" already exists; "cancel", "schedule", "verify" are new.
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
    CHECK (action_type IN (
        'view', 'create', 'update', 'delete', 'export', 'import',
        'submit', 'approve', 'release', 'bypass', 'recalculate', 'assign',
        'resolve', 'reject', 'duplicate', 'remove', 'lock', 'unlock',
        'unlockoverride', 'reassign', 'send', 'read', 'trigger',
        'cancel', 'schedule', 'verify'
    ));

INSERT INTO mst_permission (
    permission_id, permission_code, permission_name, description,
    service_name, module_name, action_type, is_active, created_by
) VALUES
    (gen_random_uuid(), 'finance.cost.caljob.view',     'View calc jobs',      'List and view calc job status',           'finance', 'cost', 'view',     TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.caljob.trigger',  'Trigger calc job',    'Trigger a new calc job batch',            'finance', 'cost', 'trigger',  TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.caljob.cancel',   'Cancel calc job',     'Cancel a running calc job',               'finance', 'cost', 'cancel',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.caljob.schedule', 'Schedule calc jobs',  'Edit cron schedule for auto calc',        'finance', 'cost', 'schedule', TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.result.view',     'View cost result',    'View product cost result and breakdown',  'finance', 'cost', 'view',     TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.result.verify',   'Verify cost result',  'Mark a cost result as verified',          'finance', 'cost', 'verify',   TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.result.approve',  'Approve cost result', 'Mark a cost result as approved',          'finance', 'cost', 'approve',  TRUE, 'seed'),
    (gen_random_uuid(), 'finance.cost.history.view',    'View cost history',   'View versioned cost history per product', 'finance', 'cost', 'view',     TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- FINANCE: caljob view/trigger/cancel + result view + history view (5 permissions)
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code = 'FINANCE'
  AND p.permission_code IN (
      'finance.cost.caljob.view',
      'finance.cost.caljob.trigger',
      'finance.cost.caljob.cancel',
      'finance.cost.result.view',
      'finance.cost.history.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- FINANCE_MANAGER + HEAD_FINANCE: all 8 finance.cost.* permissions
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r CROSS JOIN mst_permission p
WHERE r.role_code IN ('FINANCE_MANAGER', 'HEAD_FINANCE')
  AND p.permission_code LIKE 'finance.cost.%'
ON CONFLICT (role_id, permission_id) DO NOTHING;

COMMIT;
