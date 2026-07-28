-- =============================================================================
-- PPC (Production Planning & Control) — roles, permissions, menus
-- New service `ppc` replacing the 19-sheet Excel planning (SPG/TXT/TWT).
-- Roles + approval matrix per PRD 02-user-role (docs-markdown @ 90b47df2).
-- Permission code format: {service}.{module}.{entity}.{action}
--   regex ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$  (no underscores)
-- Menu: "Production Plan" is a level-1 ROOT (new service, mirrors FINANCE/IT/HR).
--   level-2 sections, level-3 master leaves. UUIDs deterministic + unused.
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 1. ROLES  (create missing; existing skipped)
-- -----------------------------------------------------------------------------
INSERT INTO mst_role (role_id, role_code, role_name, description, is_system, is_active, created_by) VALUES
  (gen_random_uuid(), 'PPC',        'PPC Planner',        'Production planning owner: demand, plan, work order, carry-forward', FALSE, TRUE, 'seed'),
  (gen_random_uuid(), 'PC',         'Process Control',    'Approve/reject technical WO parameters', FALSE, TRUE, 'seed'),
  (gen_random_uuid(), 'PM',         'Production Manager', 'Approve/reject WO overall, cancel, override over-production', FALSE, TRUE, 'seed'),
  (gen_random_uuid(), 'MARKETING',  'Marketing',          'Approve MTS demand; receive Balance-for-Sale notifications', FALSE, TRUE, 'seed'),
  (gen_random_uuid(), 'MANAGEMENT', 'Management',         'View dashboards, manage commodity watchlist', FALSE, TRUE, 'seed'),
  (gen_random_uuid(), 'OPERATOR',   'Operator',           'Input per-machine per-shift log: production, waste, downtime, parameter actual', FALSE, TRUE, 'seed')
ON CONFLICT (role_code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 2. PERMISSIONS
-- -----------------------------------------------------------------------------
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
  -- Masters (8 entities x view/create/update/delete)
  (gen_random_uuid(), 'ppc.master.machine.view',          'View Machine',            'View PPC machine master',            'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machine.create',        'Create Machine',          'Create PPC machine master',          'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machine.update',        'Update Machine',          'Update PPC machine master',          'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machine.delete',        'Delete Machine',          'Delete PPC machine master',          'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machinegroup.view',     'View Machine Group',      'View machine group master',          'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machinegroup.create',   'Create Machine Group',    'Create machine group master',        'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machinegroup.update',   'Update Machine Group',    'Update machine group master',        'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machinegroup.delete',   'Delete Machine Group',    'Delete machine group master',        'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.lot.view',              'View Lot',                'View lot master',                    'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.lot.create',            'Create Lot',              'Create lot master',                  'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.lot.update',            'Update Lot',              'Update lot master',                  'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.lot.delete',            'Delete Lot',              'Delete lot master',                  'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.productconfig.view',    'View Product Config',     'View product config master',         'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.productconfig.create',  'Create Product Config',   'Create product config master',       'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.productconfig.update',  'Update Product Config',   'Update product config master',       'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.productconfig.delete',  'Delete Product Config',   'Delete product config master',       'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machineparameter.view',   'View Machine Parameter',   'View machine parameter master',   'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machineparameter.create', 'Create Machine Parameter', 'Create machine parameter master', 'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machineparameter.update', 'Update Machine Parameter', 'Update machine parameter master', 'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machineparameter.delete', 'Delete Machine Parameter', 'Delete machine parameter master', 'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.threshold.view',        'View Threshold',          'View threshold master',              'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.threshold.create',      'Create Threshold',        'Create threshold master',            'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.threshold.update',      'Update Threshold',        'Update threshold master',            'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.threshold.delete',      'Delete Threshold',        'Delete threshold master',            'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.downtimereason.view',   'View Downtime Reason',    'View downtime reason master',        'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.downtimereason.create', 'Create Downtime Reason',  'Create downtime reason master',      'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.downtimereason.update', 'Update Downtime Reason',  'Update downtime reason master',      'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.downtimereason.delete', 'Delete Downtime Reason',  'Delete downtime reason master',      'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.wastecategory.view',    'View Waste Category',     'View waste category master',         'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.wastecategory.create',  'Create Waste Category',   'Create waste category master',       'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.wastecategory.update',  'Update Waste Category',   'Update waste category master',       'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.wastecategory.delete',  'Delete Waste Category',   'Delete waste category master',       'ppc', 'master', 'delete', TRUE, 'seed'),
  -- Demand
  (gen_random_uuid(), 'ppc.demand.demand.view',    'View Demand',    'View demand records',                  'ppc', 'demand', 'view',    TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.demand.demand.create',  'Create Demand',  'Create/pull demand (SO/MTS/Sample)',   'ppc', 'demand', 'create',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.demand.demand.update',  'Update Demand',  'Edit demand / carry-forward',          'ppc', 'demand', 'update',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.demand.demand.delete',  'Delete Demand',  'Cancel/remove demand',                 'ppc', 'demand', 'delete',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.demand.demand.approve', 'Approve MTS Demand', 'Approve MTS demand (Marketing)',   'ppc', 'demand', 'approve', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.demand.demand.reject',  'Reject MTS Demand',  'Reject MTS demand (Marketing)',    'ppc', 'demand', 'reject',  TRUE, 'seed'),
  -- Plan item
  (gen_random_uuid(), 'ppc.plan.planitem.view',    'View Plan Item',   'View plan items / Gantt',            'ppc', 'plan', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.plan.planitem.create',  'Create Plan Item', 'Create plan item',                   'ppc', 'plan', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.plan.planitem.update',  'Update Plan Item', 'Edit plan item',                     'ppc', 'plan', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.plan.planitem.delete',  'Delete Plan Item', 'Delete plan item',                   'ppc', 'plan', 'delete', TRUE, 'seed'),
  -- Work order
  (gen_random_uuid(), 'ppc.workorder.workorder.view',     'View Work Order',        'View work orders',                        'ppc', 'workorder', 'view',    TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.create',   'Create Work Order',      'Generate work order',                     'ppc', 'workorder', 'create',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.update',   'Update Work Order',      'Edit work order / RM allocation',         'ppc', 'workorder', 'update',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.delete',   'Cancel Work Order',      'Cancel work order (DRAFT / APPROVED+)',   'ppc', 'workorder', 'delete',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.submit',   'Submit Work Order',      'Submit work order for approval',          'ppc', 'workorder', 'submit',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.approve',  'Approve Work Order',     'Approve WO overall (PM)',                 'ppc', 'workorder', 'approve', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.reject',   'Reject Work Order',      'Reject WO overall (PM)',                  'ppc', 'workorder', 'reject',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.override', 'Override Over-Production','Override over-production block (PM)',      'ppc', 'workorder', 'override',TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.reopen',   'Reopen Final Qty',       'Reopen WO FINAL qty after 24h (PM)',      'ppc', 'workorder', 'reopen',  TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.workorder.resolve',  'Resolve Plan Change',    'Resolve plan change flag (PPC/PM)',       'ppc', 'workorder', 'resolve', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.param.approve',      'Approve WO Parameters',  'Approve technical WO parameters (PC)',     'ppc', 'workorder', 'approve', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.workorder.param.reject',       'Reject WO Parameters',   'Reject technical WO parameters (PC)',      'ppc', 'workorder', 'reject',  TRUE, 'seed'),
  -- Daily performance
  (gen_random_uuid(), 'ppc.dailyperf.shiftentry.view',    'View Shift Entry',      'View per-shift production/waste/downtime', 'ppc', 'dailyperf', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dailyperf.shiftentry.create',  'Create Shift Entry',    'Input per-shift log',                      'ppc', 'dailyperf', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dailyperf.shiftentry.update',  'Update Shift Entry',    'Adjust per-shift log',                     'ppc', 'dailyperf', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dailyperf.execution.view',     'View WO Execution',     'View WO execution parameter actuals',      'ppc', 'dailyperf', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dailyperf.execution.create',   'Create WO Execution',   'Input WO execution parameter actuals',     'ppc', 'dailyperf', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dailyperf.execution.update',   'Update WO Execution',   'Adjust WO execution parameter actuals',    'ppc', 'dailyperf', 'update', TRUE, 'seed'),
  -- Dashboard
  (gen_random_uuid(), 'ppc.dashboard.overview.view',   'View PPC Dashboards',      'View PPC production dashboards',          'ppc', 'dashboard', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dashboard.balance.view',    'View Balance for Sale',    'View Balance-for-Sale dashboard',        'ppc', 'dashboard', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dashboard.watchlist.view',  'View Commodity Watchlist', 'View commodity watchlist',               'ppc', 'dashboard', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.dashboard.watchlist.update','Manage Commodity Watchlist','Manage commodity watchlist (Management)','ppc', 'dashboard', 'update', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 3. MENUS  ("Production Plan" ROOT + sections + master leaves)
-- -----------------------------------------------------------------------------
-- LEVEL 1 — ROOT
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
  ('00000000-0000-0000-0001-000000000009', NULL, 'PPC', 'Production Plan', NULL, 'Factory', 'ppc', 1, 25, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- LEVEL 2 — sections
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
  ('00000000-0000-0000-0002-000000000022', '00000000-0000-0000-0001-000000000009', 'PPC_DEMAND',            'Demand',            '/production-plan/demand',            'ClipboardList',   'ppc', 2, 10, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0002-000000000023', '00000000-0000-0000-0001-000000000009', 'PPC_PLAN',              'Plan',              '/production-plan/plan',              'GanttChart',      'ppc', 2, 20, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0002-000000000024', '00000000-0000-0000-0001-000000000009', 'PPC_WORK_ORDERS',       'Work Orders',       '/production-plan/work-orders',       'ScrollText',      'ppc', 2, 30, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0002-000000000025', '00000000-0000-0000-0001-000000000009', 'PPC_DAILY_PERFORMANCE', 'Daily Performance', '/production-plan/daily-performance', 'Activity',        'ppc', 2, 40, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0002-000000000026', '00000000-0000-0000-0001-000000000009', 'PPC_DASHBOARDS',        'Dashboards',        '/production-plan/dashboards',        'LayoutDashboard', 'ppc', 2, 50, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0002-000000000027', '00000000-0000-0000-0001-000000000009', 'PPC_MASTERS',           'Masters',           NULL,                                 'Database',        'ppc', 2, 60, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- LEVEL 3 — master leaves (under PPC_MASTERS)
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
  ('00000000-0000-0000-0003-000000000046', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_MACHINE',           'Machine',           '/production-plan/masters/machine',           'Cog',              'ppc', 3, 10, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-000000000047', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_MACHINE_GROUP',     'Machine Group',     '/production-plan/masters/machine-group',     'Boxes',            'ppc', 3, 20, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-000000000048', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_LOT',               'Lot',               '/production-plan/masters/lot',               'Package',          'ppc', 3, 30, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-000000000049', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_PRODUCT_CONFIG',    'Product Config',    '/production-plan/masters/product-config',    'Settings2',        'ppc', 3, 40, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-00000000004a', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_MACHINE_PARAMETER', 'Machine Parameter', '/production-plan/masters/machine-parameter', 'SlidersHorizontal','ppc', 3, 50, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-00000000004b', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_THRESHOLD',         'Threshold',         '/production-plan/masters/threshold',         'Gauge',            'ppc', 3, 60, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-00000000004c', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_DOWNTIME_REASON',   'Downtime Reason',   '/production-plan/masters/downtime-reason',   'AlarmClock',       'ppc', 3, 70, TRUE, TRUE, 'seed'),
  ('00000000-0000-0000-0003-00000000004d', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_WASTE_CATEGORY',    'Waste Category',    '/production-plan/masters/waste-category',    'Trash2',           'ppc', 3, 80, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 4. MENU_PERMISSIONS  (gate each menu by its view permission)
-- -----------------------------------------------------------------------------
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m JOIN mst_permission p ON TRUE
WHERE (m.menu_code, p.permission_code) IN (
  ('PPC_DEMAND',                  'ppc.demand.demand.view'),
  ('PPC_PLAN',                    'ppc.plan.planitem.view'),
  ('PPC_WORK_ORDERS',             'ppc.workorder.workorder.view'),
  ('PPC_DAILY_PERFORMANCE',       'ppc.dailyperf.shiftentry.view'),
  ('PPC_DASHBOARDS',              'ppc.dashboard.overview.view'),
  ('PPC_MASTERS',                 'ppc.master.machine.view'),
  ('PPC_MASTER_MACHINE',          'ppc.master.machine.view'),
  ('PPC_MASTER_MACHINE_GROUP',    'ppc.master.machinegroup.view'),
  ('PPC_MASTER_LOT',              'ppc.master.lot.view'),
  ('PPC_MASTER_PRODUCT_CONFIG',   'ppc.master.productconfig.view'),
  ('PPC_MASTER_MACHINE_PARAMETER','ppc.master.machineparameter.view'),
  ('PPC_MASTER_THRESHOLD',        'ppc.master.threshold.view'),
  ('PPC_MASTER_DOWNTIME_REASON',  'ppc.master.downtimereason.view'),
  ('PPC_MASTER_WASTE_CATEGORY',   'ppc.master.wastecategory.view')
)
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 5. ROLE_PERMISSIONS
-- -----------------------------------------------------------------------------
-- SUPER_ADMIN gets everything PPC
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code LIKE 'ppc.%'
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- PPC — planning owner: masters CRUD, demand (no MTS approve), plan, WO (own lifecycle), shift/execution input, dashboards view
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'PPC' AND r.is_active = TRUE AND p.is_active = TRUE
  AND ( p.permission_code LIKE 'ppc.master.%'
     OR p.permission_code LIKE 'ppc.plan.%'
     OR p.permission_code IN (
        'ppc.demand.demand.view','ppc.demand.demand.create','ppc.demand.demand.update','ppc.demand.demand.delete',
        'ppc.workorder.workorder.view','ppc.workorder.workorder.create','ppc.workorder.workorder.update',
        'ppc.workorder.workorder.delete','ppc.workorder.workorder.submit','ppc.workorder.workorder.resolve',
        'ppc.dailyperf.shiftentry.view','ppc.dailyperf.shiftentry.create','ppc.dailyperf.shiftentry.update',
        'ppc.dailyperf.execution.view',
        'ppc.dashboard.overview.view','ppc.dashboard.balance.view'
     ) )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- PC — process control: approve/reject WO technical parameters
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'PC' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.workorder.workorder.view',
     'ppc.workorder.param.approve','ppc.workorder.param.reject',
     'ppc.dashboard.overview.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- PM — production manager: approve/reject/override/reopen/cancel WO overall, resolve plan change
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'PM' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.workorder.workorder.view','ppc.workorder.workorder.approve','ppc.workorder.workorder.reject',
     'ppc.workorder.workorder.override','ppc.workorder.workorder.reopen','ppc.workorder.workorder.delete',
     'ppc.workorder.workorder.resolve',
     'ppc.dashboard.overview.view','ppc.dashboard.balance.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Marketing — approve/reject MTS demand; view Balance-for-Sale
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'MARKETING' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.demand.demand.view','ppc.demand.demand.approve','ppc.demand.demand.reject',
     'ppc.dashboard.balance.view'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Management — dashboards view + commodity watchlist manage
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'MANAGEMENT' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.dashboard.overview.view','ppc.dashboard.balance.view',
     'ppc.dashboard.watchlist.view','ppc.dashboard.watchlist.update'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

-- Operator — per-shift log + WO execution parameter actual; view WO
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'OPERATOR' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.workorder.workorder.view',
     'ppc.dailyperf.shiftentry.view','ppc.dailyperf.shiftentry.create','ppc.dailyperf.shiftentry.update',
     'ppc.dailyperf.execution.view','ppc.dailyperf.execution.create','ppc.dailyperf.execution.update'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;
