-- =============================================================================
-- PPC gap fix: (1) missing Customer master menu, (2) missing `.sync` permissions.
--
-- (1) The frontend route goapps-frontend/src/app/(dashboard)/production-plan/
--     masters/customers/ exists, but 000079 seeded only 8 PPC master menus
--     (MACHINE, MACHINE_GROUP, LOT, PRODUCT_CONFIG, MACHINE_PARAMETER,
--     THRESHOLD, DOWNTIME_REASON, WASTE_CATEGORY) — CUSTOMER was never seeded,
--     so the dynamic sidebar hides the page entirely.
--
-- (2) No `.sync` permission existed for ANY PPC master, so the "Sync from
--     Oracle" buttons were gated only by page visibility. Sync permissions are
--     added ONLY for masters that actually expose a Sync RPC. Verified against
--     goapps-shared-proto/ppc/v1/ppc_service.proto (rpc SyncMachines,
--     SyncCustomers, SyncLots) and the real handlers in
--     goapps-backend/services/ppc/internal/delivery/grpc/
--     (machine_handler.go:SyncMachines, customer_handler.go:SyncCustomers,
--      lot_handler.go:SyncLots). No other PPC master has a Sync RPC.
--
-- Permission code format: {service}.{module}.{entity}.{action}
--   regex ^[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z][a-z0-9]*\.[a-z]+$ (no underscores)
-- =============================================================================

-- -----------------------------------------------------------------------------
-- 0. EXTEND chk_permission_action — allow 'sync'
--    Live constraint (verified via pg_get_constraintdef) held 35 values, ending
--    at 'execute' (set by 000068). 'sync' is NOT among them, so inserting a
--    `.sync` permission would violate the CHECK. 35 -> 36 values.
-- -----------------------------------------------------------------------------
ALTER TABLE mst_permission DROP CONSTRAINT IF EXISTS chk_permission_action;
ALTER TABLE mst_permission ADD CONSTRAINT chk_permission_action
  CHECK (action_type IN (
    'view','create','update','delete','export','import','submit','approve','release','bypass',
    'recalculate','assign','resolve','reject','duplicate','remove','lock','unlock','unlockoverride',
    'reassign','send','read','trigger','cancel','schedule','verify','override','review','reopen','confirm',
    'validate','unapprove','revoke','preview','execute','sync'
  ));

-- -----------------------------------------------------------------------------
-- 1. PERMISSIONS
--    (a) Customer master CRUD — 000079 seeded no customer permissions at all,
--        so the new menu would have no view permission to gate on.
--    (b) `.sync` for the three masters with a real Sync RPC.
-- -----------------------------------------------------------------------------
INSERT INTO mst_permission (permission_id, permission_code, permission_name, description, service_name, module_name, action_type, is_active, created_by) VALUES
  (gen_random_uuid(), 'ppc.master.customer.view',   'View Customer',   'View PPC customer master',   'ppc', 'master', 'view',   TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.customer.create', 'Create Customer', 'Create PPC customer master', 'ppc', 'master', 'create', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.customer.update', 'Update Customer', 'Update PPC customer master', 'ppc', 'master', 'update', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.customer.delete', 'Delete Customer', 'Delete PPC customer master', 'ppc', 'master', 'delete', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.machine.sync',    'Sync Machine',    'Sync machine master from Oracle (read-only)',  'ppc', 'master', 'sync', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.customer.sync',   'Sync Customer',   'Sync customer master from Oracle (read-only)', 'ppc', 'master', 'sync', TRUE, 'seed'),
  (gen_random_uuid(), 'ppc.master.lot.sync',        'Sync Lot',        'Sync lot master from Oracle (read-only)',      'ppc', 'master', 'sync', TRUE, 'seed')
ON CONFLICT (permission_code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 2. MENU — Customer master leaf under PPC_MASTERS
--    UUID seq: level-3 ids are deterministic 00000000-0000-0000-0003-000000000NN.
--    Live DB holds 56 level-3 rows; the PPC block occupies ...046 .. ...04d
--    (WASTE_CATEGORY is the highest at ...04d) and nothing above it exists.
--    ...04e is therefore the next genuinely free seq — verified by
--    SELECT menu_id FROM mst_menu WHERE menu_id::text LIKE '00000000-0000-0000-0003-%'
--    and by grepping every migration under migrations/postgres/.
--    NOTE: ON CONFLICT (menu_code) does NOT protect a reused menu_id PK, hence
--    the explicit free-seq check above.
--    URL is plural /production-plan/masters/customers, matching the real route
--    folder and consistent with 000080_fix_ppc_menu_urls (which pluralized the
--    other PPC master URLs).
--    sort_order 90 — appended after WASTE_CATEGORY (80), no renumbering needed.
-- -----------------------------------------------------------------------------
INSERT INTO mst_menu (menu_id, parent_id, menu_code, menu_title, menu_url, icon_name, service_name, menu_level, sort_order, is_visible, is_active, created_by)
VALUES
  ('00000000-0000-0000-0003-00000000004e', '00000000-0000-0000-0002-000000000027', 'PPC_MASTER_CUSTOMER', 'Customer', '/production-plan/masters/customers', 'Users', 'ppc', 3, 90, TRUE, TRUE, 'seed')
ON CONFLICT (menu_code) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 3. MENU_PERMISSIONS — gate the new menu by its view permission
-- -----------------------------------------------------------------------------
INSERT INTO menu_permissions (menu_id, permission_id, assigned_by)
SELECT m.menu_id, p.permission_id, 'seed'
FROM mst_menu m JOIN mst_permission p ON TRUE
WHERE (m.menu_code, p.permission_code) IN (
  ('PPC_MASTER_CUSTOMER', 'ppc.master.customer.view')
)
ON CONFLICT (menu_id, permission_id) DO NOTHING;

-- -----------------------------------------------------------------------------
-- 4. ROLE_PERMISSIONS
--    Same roles 000079 granted PPC master permissions to: SUPER_ADMIN (all
--    'ppc.%') and PPC (all 'ppc.master.%'). No other role received
--    'ppc.master.%' in 000079, so none is added here.
-- -----------------------------------------------------------------------------
INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'SUPER_ADMIN' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.master.customer.view','ppc.master.customer.create',
     'ppc.master.customer.update','ppc.master.customer.delete',
     'ppc.master.machine.sync','ppc.master.customer.sync','ppc.master.lot.sync'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;

INSERT INTO role_permissions (role_id, permission_id, assigned_by)
SELECT r.role_id, p.permission_id, 'seed'
FROM mst_role r JOIN mst_permission p ON TRUE
WHERE r.role_code = 'PPC' AND r.is_active = TRUE AND p.is_active = TRUE
  AND p.permission_code IN (
     'ppc.master.customer.view','ppc.master.customer.create',
     'ppc.master.customer.update','ppc.master.customer.delete',
     'ppc.master.machine.sync','ppc.master.customer.sync','ppc.master.lot.sync'
  )
ON CONFLICT (role_id, permission_id) DO NOTHING;
